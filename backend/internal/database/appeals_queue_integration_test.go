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
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
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

func TestStudentAppealCreatesModeratorQueueTicket(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	input := feedback.CreateInput{
		CourseID:    "CS455",
		CourseTitle: "Software Engineering",
		Category:    "teaching",
		Rating:      4,
		Content:     "The project workload was too high.",
	}
	review, err := store.Create(ctx, input)
	if err != nil {
		t.Fatalf("create feedback: %v", err)
	}
	if review.Status != feedback.StatusFlagged || review.FlagReason == "" {
		t.Fatalf("new review = %+v, want a flagged review with a reason", review)
	}

	count, err := store.appealCollection.CountDocuments(ctx, bson.M{"referenceCode": review.ReferenceCode})
	if err != nil {
		t.Fatalf("count appeals before student appeal: %v", err)
	}
	if count != 0 {
		t.Fatalf("appeal count before student appeal = %d, want 0", count)
	}

	appeal, err := store.CreateAppeal(ctx, review.ReferenceCode, "Please reconsider my feedback.")
	if err != nil {
		t.Fatalf("create student appeal: %v", err)
	}
	if appeal.Status != feedback.StatusUnderAppeal {
		t.Fatalf("appeal status = %q, want %q", appeal.Status, feedback.StatusUnderAppeal)
	}

	queued, err := store.ListUnclaimed(ctx, appeals.SortOldestFirst)
	if err != nil {
		t.Fatalf("list moderator queue: %v", err)
	}
	if len(queued) != 1 || queued[0].ReferenceCode != review.ReferenceCode || queued[0].CourseID != input.CourseID {
		t.Fatalf("moderator queue after appeal = %+v, want the appealed review", queued)
	}
	if queued[0].Year != 0 || queued[0].Professor != "" {
		t.Fatalf("missing course details = year %d, professor %q; want no fabricated values", queued[0].Year, queued[0].Professor)
	}

	status, err := store.FindByReference(ctx, review.ReferenceCode)
	if err != nil {
		t.Fatalf("find student review status: %v", err)
	}
	if status.AppealStatus != appeals.StatusUnderAppeal {
		t.Fatalf("student appeal status = %q, want %q", status.AppealStatus, appeals.StatusUnderAppeal)
	}

	if _, err := store.CreateAppeal(ctx, review.ReferenceCode, "A second appeal."); !errors.Is(err, feedback.ErrAppealAlreadyExists) {
		t.Fatalf("second appeal error = %v, want %v", err, feedback.ErrAppealAlreadyExists)
	}

	const moderatorID = "appeal-flow-moderator"
	claimed, err := store.Claim(ctx, queued[0].ID, moderatorID)
	if err != nil {
		t.Fatalf("claim student appeal: %v", err)
	}
	ticket, err := store.Get(ctx, claimed.ID, moderatorID)
	if err != nil {
		t.Fatalf("get claimed student appeal: %v", err)
	}
	if ticket.FeedbackText != input.Content || ticket.FlagReason != feedback.FlagReasonReview || ticket.AppealComment != "Please reconsider my feedback." {
		t.Fatalf("moderator ticket details = %+v, want submitted review, flag reason, and appeal comment", ticket)
	}
	if err := store.Resolve(ctx, claimed.ID, moderatorID, appeals.ActionRestore, ""); err != nil {
		t.Fatalf("resolve student appeal: %v", err)
	}

	resolved, err := store.FindByReference(ctx, review.ReferenceCode)
	if err != nil {
		t.Fatalf("find resolved student review: %v", err)
	}
	if resolved.Status != feedback.StatusPublished || resolved.AppealStatus != appeals.StatusRestored {
		t.Fatalf("resolved review status = %q, appeal status = %q; want published/restored", resolved.Status, resolved.AppealStatus)
	}
}
