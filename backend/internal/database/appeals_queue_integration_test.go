package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func queueIntegrationMongo(t *testing.T) (*Mongo, context.Context) {
	t.Helper()

	uri := strings.TrimSpace(os.Getenv("MONGODB_TEST_URI"))
	if uri == "" {
		t.Skip("set MONGODB_TEST_URI to run MongoDB integration tests")
	}

	databaseName := "cs455_appeals_test_" + primitive.NewObjectID().Hex()
	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 15*time.Second)
	store, err := Connect(connectCtx, uri, databaseName)
	cancelConnect()
	if err != nil {
		t.Fatalf("connect to MongoDB test database: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(func() {
		cancel()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := store.client.Database(databaseName).Drop(cleanupCtx); err != nil {
			t.Errorf("drop MongoDB test database %q: %v", databaseName, err)
		}
		if err := store.Disconnect(cleanupCtx); err != nil {
			t.Errorf("disconnect from MongoDB test database: %v", err)
		}
	})

	return store, ctx
}

func queueInsertTicket(t *testing.T, ctx context.Context, collection *mongo.Collection, fields bson.M) string {
	t.Helper()

	result, err := collection.InsertOne(ctx, fields)
	if err != nil {
		t.Fatalf("insert appeal ticket: %v", err)
	}
	id, ok := result.InsertedID.(primitive.ObjectID)
	if !ok {
		t.Fatalf("inserted appeal ID has type %T, want primitive.ObjectID", result.InsertedID)
	}
	return id.Hex()
}

func queueTicketFields(status string, createdAt time.Time) bson.M {
	return bson.M{
		"referenceCode": "QUEUE-TEST",
		"courseId":      "CS455",
		"year":          2026,
		"professor":     "Prof. Test",
		"status":        status,
		"createdAt":     createdAt,
	}
}

func queueSummaryIDs(summaries []appeals.Summary) []string {
	ids := make([]string, len(summaries))
	for i, summary := range summaries {
		ids[i] = summary.ID
	}
	return ids
}

func TestQueueClaimConcurrentRequests(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	const rounds = 20
	const claimants = 50

	for round := 0; round < rounds; round++ {
		ticket := queueTicketFields(appeals.StatusUnderAppeal, time.Now().UTC())
		ticketID := queueInsertTicket(t, ctx, store.appealCollection, ticket)

		start := make(chan struct{})
		results := make(chan error, claimants)
		var ready sync.WaitGroup
		ready.Add(claimants)
		var done sync.WaitGroup
		done.Add(claimants)

		for claimant := 0; claimant < claimants; claimant++ {
			moderatorID := fmt.Sprintf("moderator-%d", claimant)
			go func() {
				defer done.Done()
				ready.Done()
				<-start
				_, err := store.Claim(ctx, ticketID, moderatorID)
				results <- err
			}()
		}

		ready.Wait()
		close(start)
		done.Wait()
		close(results)

		successes := 0
		alreadyClaimed := 0
		for err := range results {
			switch {
			case err == nil:
				successes++
			case errors.Is(err, appeals.ErrAlreadyClaimed):
				alreadyClaimed++
			default:
				t.Fatalf("round %d: unexpected claim error: %v", round+1, err)
			}
		}

		if successes != 1 || alreadyClaimed != claimants-1 {
			t.Fatalf("round %d: successes = %d, already claimed = %d; want 1 and %d",
				round+1, successes, alreadyClaimed, claimants-1)
		}
	}
}

func TestQueueListUnclaimedIncludesExpiredButNotFreshLocks(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	now := time.Now().UTC()

	expired := queueTicketFields(appeals.StatusLocked, now.Add(-2*time.Hour))
	expired["lockedAt"] = now.Add(-45 * time.Minute)
	expiredID := queueInsertTicket(t, ctx, store.appealCollection, expired)

	fresh := queueTicketFields(appeals.StatusLocked, now.Add(-time.Hour))
	fresh["lockedAt"] = now.Add(-5 * time.Minute)
	queueInsertTicket(t, ctx, store.appealCollection, fresh)

	got, err := store.ListUnclaimed(ctx, appeals.SortOldestFirst)
	if err != nil {
		t.Fatalf("list unclaimed appeals: %v", err)
	}
	if len(got) != 1 || got[0].ID != expiredID {
		t.Fatalf("claimable ticket IDs = %v, want [%s]", queueSummaryIDs(got), expiredID)
	}
}

func TestQueueListUnclaimedSortOrder(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	now := time.Now().UTC()

	oldestID := queueInsertTicket(t, ctx, store.appealCollection,
		queueTicketFields(appeals.StatusUnderAppeal, now.Add(-3*time.Hour)))
	middleID := queueInsertTicket(t, ctx, store.appealCollection,
		queueTicketFields(appeals.StatusUnderAppeal, now.Add(-2*time.Hour)))
	newestID := queueInsertTicket(t, ctx, store.appealCollection,
		queueTicketFields(appeals.StatusUnderAppeal, now.Add(-time.Hour)))

	ascending, err := store.ListUnclaimed(ctx, appeals.SortOldestFirst)
	if err != nil {
		t.Fatalf("list appeals oldest first: %v", err)
	}
	if got, want := queueSummaryIDs(ascending), []string{oldestID, middleID, newestID}; !equalQueueIDs(got, want) {
		t.Fatalf("ascending IDs = %v, want %v", got, want)
	}

	descending, err := store.ListUnclaimed(ctx, appeals.SortNewestFirst)
	if err != nil {
		t.Fatalf("list appeals newest first: %v", err)
	}
	if got, want := queueSummaryIDs(descending), []string{newestID, middleID, oldestID}; !equalQueueIDs(got, want) {
		t.Fatalf("descending IDs = %v, want %v", got, want)
	}
}

func TestQueueListMineReturnsOnlyCurrentUnexpiredLocks(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	now := time.Now().UTC()
	const moderatorID = "moderator-current"

	mineFirst := queueTicketFields(appeals.StatusLocked, now.Add(-2*time.Hour))
	mineFirst["lockedBy"] = moderatorID
	mineFirst["lockedAt"] = now.Add(-10 * time.Minute)
	mineFirstID := queueInsertTicket(t, ctx, store.appealCollection, mineFirst)

	mineSecond := queueTicketFields(appeals.StatusLocked, now.Add(-time.Hour))
	mineSecond["lockedBy"] = moderatorID
	mineSecond["lockedAt"] = now.Add(-20 * time.Minute)
	mineSecondID := queueInsertTicket(t, ctx, store.appealCollection, mineSecond)

	expiredMine := queueTicketFields(appeals.StatusLocked, now.Add(-3*time.Hour))
	expiredMine["lockedBy"] = moderatorID
	expiredMine["lockedAt"] = now.Add(-45 * time.Minute)
	queueInsertTicket(t, ctx, store.appealCollection, expiredMine)

	otherModerator := queueTicketFields(appeals.StatusLocked, now.Add(-4*time.Hour))
	otherModerator["lockedBy"] = "moderator-other"
	otherModerator["lockedAt"] = now.Add(-5 * time.Minute)
	queueInsertTicket(t, ctx, store.appealCollection, otherModerator)

	queueInsertTicket(t, ctx, store.appealCollection,
		queueTicketFields(appeals.StatusUnderAppeal, now))

	got, err := store.ListMine(ctx, moderatorID)
	if err != nil {
		t.Fatalf("list current moderator appeals: %v", err)
	}
	if want := []string{mineFirstID, mineSecondID}; !equalQueueIDs(queueSummaryIDs(got), want) {
		t.Fatalf("active ticket IDs = %v, want %v", queueSummaryIDs(got), want)
	}
}

func equalQueueIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
