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

func TestTriageClaimRecoveryAndStatusPersistence(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	created, err := store.Create(ctx, feedback.CreateInput{
		CourseID: "CS455", CourseTitle: "Software Engineering", Category: "teaching",
		Rating: 4, Content: "A safe review for triage integration.",
	})
	if err != nil {
		t.Fatalf("create feedback: %v", err)
	}

	pending, err := store.ListSubmitted(ctx, 10)
	if err != nil {
		t.Fatalf("list submitted feedback: %v", err)
	}
	if len(pending) != 1 || pending[0].ID != created.ID {
		t.Fatalf("pending reviews = %+v, want only %s", pending, created.ID)
	}

	claimed, ok, err := store.ClaimForTriage(ctx, created.ID, time.Minute)
	if err != nil || !ok || claimed.ID != created.ID {
		t.Fatalf("first triage claim = (%+v, %v, %v)", claimed, ok, err)
	}
	if claimed.TriageAttempts != 1 {
		t.Fatalf("first triage attempt = %d, want 1", claimed.TriageAttempts)
	}
	if _, ok, err := store.ClaimForTriage(ctx, created.ID, time.Minute); err != nil || ok {
		t.Fatalf("duplicate triage claim = (%v, %v), want false without error", ok, err)
	}
	if pending, err := store.ListSubmitted(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("leased review listed for recovery: (%+v, %v)", pending, err)
	}

	if err := store.ReleaseTriage(ctx, created.ID); err != nil {
		t.Fatalf("release triage lease: %v", err)
	}
	if pending, err := store.ListSubmitted(ctx, 10); err != nil || len(pending) != 1 {
		t.Fatalf("released review missing from recovery: (%+v, %v)", pending, err)
	}
	claimed, ok, err = store.ClaimForTriage(ctx, created.ID, time.Minute)
	if err != nil || !ok || claimed.TriageAttempts != 2 {
		t.Fatalf("recovered triage claim = (%+v, %v, %v), want persisted attempt 2", claimed, ok, err)
	}
	if err := store.UpdateStatus(ctx, created.ID, feedback.StatusFlagged, "harassment: targeted abuse"); err != nil {
		t.Fatalf("persist triage status: %v", err)
	}
	updated, err := store.FindByReference(ctx, created.ReferenceCode)
	if err != nil {
		t.Fatalf("find updated review: %v", err)
	}
	if updated.Status != feedback.StatusFlagged || updated.FlagReason != "harassment: targeted abuse" {
		t.Fatalf("updated review = (%s, %q)", updated.Status, updated.FlagReason)
	}

	failedReview, err := store.Create(ctx, feedback.CreateInput{
		CourseID: "CS455", CourseTitle: "Software Engineering", Category: "teaching",
		Rating: 4, Content: "A review whose classifier failed.",
	})
	if err != nil {
		t.Fatalf("create review for terminal failure: %v", err)
	}
	if _, ok, err := store.ClaimForTriage(ctx, failedReview.ID, time.Minute); err != nil || !ok {
		t.Fatalf("claim review for terminal failure = (%v, %v)", ok, err)
	}
	if err := store.MarkTriageFailed(ctx, failedReview.ID, feedback.ProcessingFailureMessage); err != nil {
		t.Fatalf("persist terminal triage failure: %v", err)
	}
	failed, err := store.FindByReference(ctx, failedReview.ReferenceCode)
	if err != nil {
		t.Fatalf("find terminally failed review: %v", err)
	}
	if failed.Status != feedback.StatusTriageFailed || failed.ProcessingError != feedback.ProcessingFailureMessage {
		t.Fatalf("terminal failure state = (%s, %q)", failed.Status, failed.ProcessingError)
	}
}

func TestTriageBatchFlagPersistsReason(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	first, err := store.Create(ctx, feedback.CreateInput{
		CourseID: "CS455", CourseTitle: "Software Engineering", Category: "teaching",
		Rating: 4, Content: "First review in a batch.",
	})
	if err != nil {
		t.Fatalf("create first feedback: %v", err)
	}
	second, err := store.Create(ctx, feedback.CreateInput{
		CourseID: "CS455", CourseTitle: "Software Engineering", Category: "teaching",
		Rating: 3, Content: "Second review in a batch.",
	})
	if err != nil {
		t.Fatalf("create second feedback: %v", err)
	}
	if err := store.FlagReviews(ctx, []string{first.ID, second.ID}, "coordinated activity"); err != nil {
		t.Fatalf("flag review batch: %v", err)
	}
	for _, review := range []feedback.Feedback{first, second} {
		updated, err := store.FindByReference(ctx, review.ReferenceCode)
		if err != nil {
			t.Fatalf("find review %s: %v", review.ReferenceCode, err)
		}
		if updated.Status != feedback.StatusFlagged || updated.FlagReason != "coordinated activity" {
			t.Fatalf("review %s = (%s, %q)", review.ReferenceCode, updated.Status, updated.FlagReason)
		}
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
	if review.Status != feedback.StatusSubmitted {
		t.Fatalf("new review status = %q, want %q before asynchronous triage", review.Status, feedback.StatusSubmitted)
	}
	if err := store.UpdateStatus(ctx, review.ID, feedback.StatusFlagged, "other: requires review"); err != nil {
		t.Fatalf("mark review flagged for appeal: %v", err)
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
	if ticket.FeedbackText != input.Content || ticket.FlagReason != "other: requires review" || ticket.AppealComment != "Please reconsider my feedback." {
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
