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
		"referenceCode": "QUEUE-TEST-" + primitive.NewObjectID().Hex(),
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

	if err := store.ReleaseTriageAttempt(ctx, created.ID, claimed.TriageAttempts); err != nil {
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

func TestTriageDeferredRecoveryMetadata(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	created, err := store.Create(ctx, feedback.CreateInput{
		CourseID: "CS455", CourseTitle: "Software Engineering", Category: "teaching",
		Rating: 4, Content: "A review deferred after a provider failure.",
	})
	if err != nil {
		t.Fatalf("create feedback: %v", err)
	}
	firstClaim, ok, err := store.ClaimForTriage(ctx, created.ID, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim review: (%v, %v)", ok, err)
	}
	if firstClaim.TriageAttempts != 1 || firstClaim.TriageCycleAttempts != 1 {
		t.Fatalf("first attempt metadata = (%d, %d), want (1, 1)",
			firstClaim.TriageAttempts, firstClaim.TriageCycleAttempts)
	}

	retryAt := time.Now().UTC().Truncate(time.Millisecond).Add(time.Minute)
	if err := store.DeferTriage(ctx, created.ID, firstClaim.TriageAttempts, "provider_auth", retryAt); !errors.Is(err, feedback.ErrReviewNotEligible) {
		t.Fatalf("defer actively leased review error = %v, want not eligible", err)
	}
	if err := store.ReleaseTriageAttempt(ctx, created.ID, firstClaim.TriageAttempts); err != nil {
		t.Fatalf("release triage lease: %v", err)
	}
	if err := store.DeferTriage(ctx, created.ID, firstClaim.TriageAttempts, "provider_auth", retryAt); err != nil {
		t.Fatalf("defer review: %v", err)
	}
	deferred, err := store.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("find deferred review: %v", err)
	}
	if deferred.Status != feedback.StatusSubmitted || deferred.TriageFailureCategory != "provider_auth" ||
		deferred.TriageRetryAt == nil || !deferred.TriageRetryAt.Equal(retryAt) ||
		deferred.TriageCycleAttempts != 0 || deferred.TriageAttempts != 1 {
		t.Fatalf("deferred state = %+v", deferred)
	}
	if pending, err := store.ListSubmitted(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("future retry listed as eligible: (%+v, %v)", pending, err)
	}
	if _, ok, err := store.ClaimForTriage(ctx, created.ID, time.Minute); err != nil || ok {
		t.Fatalf("claim during cooldown = (%v, %v), want not claimed", ok, err)
	}

	if err := store.DeferTriage(ctx, created.ID, firstClaim.TriageAttempts, "provider_auth", time.Now().UTC().Add(-time.Second)); err != nil {
		t.Fatalf("make retry eligible: %v", err)
	}
	pending, err := store.ListSubmitted(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].ID != created.ID {
		t.Fatalf("eligible deferred review = (%+v, %v)", pending, err)
	}
	recovered, ok, err := store.ClaimForTriage(ctx, created.ID, time.Minute)
	if err != nil || !ok || recovered.TriageAttempts != 2 || recovered.TriageCycleAttempts != 1 {
		t.Fatalf("recovered claim = (%+v, %v, %v), want lifetime 2 and cycle 1", recovered, ok, err)
	}
	if err := store.UpdateStatus(ctx, created.ID, feedback.StatusPublished, ""); err != nil {
		t.Fatalf("publish recovered review: %v", err)
	}
	published, err := store.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("find published review: %v", err)
	}
	if published.Status != feedback.StatusPublished || published.TriageRetryAt != nil ||
		published.TriageFailureCategory != "" || published.TriageAttempts != 2 {
		t.Fatalf("published recovery metadata = %+v", published)
	}
	if err := store.DeferTriage(ctx, created.ID, recovered.TriageAttempts, "provider_auth", time.Now().Add(time.Minute)); !errors.Is(err, feedback.ErrReviewNotEligible) {
		t.Fatalf("defer published review error = %v, want not eligible", err)
	}
}

func TestTriageProviderCooldownBlocksNewAndExistingReviewsAndExpires(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	existing, err := store.Create(ctx, feedback.CreateInput{
		CourseID: "CS455", CourseTitle: "Course", Category: "teaching",
		Rating: 4, Content: "An existing review during provider cooldown.",
	})
	if err != nil {
		t.Fatalf("create existing feedback: %v", err)
	}
	retryAt := time.Now().UTC().Add(5 * time.Minute)
	if err := store.SetProviderCooldown(ctx, "provider_auth", retryAt); err != nil {
		t.Fatalf("persist provider cooldown: %v", err)
	}
	// A second store handle models a backend restart reading the durable singleton.
	restarted := &Mongo{
		feedbackCollection:    store.feedbackCollection,
		triageStateCollection: store.triageStateCollection,
	}
	newlySubmitted, err := store.Create(ctx, feedback.CreateInput{
		CourseID: "CS101", CourseTitle: "Course", Category: "teaching",
		Rating: 4, Content: "A newly submitted review during provider cooldown.",
	})
	if err != nil {
		t.Fatalf("create feedback during cooldown: %v", err)
	}
	for name, handle := range map[string]*Mongo{"original": store, "restarted": restarted} {
		active, err := handle.ProviderCooldownActive(ctx)
		if err != nil || !active {
			t.Fatalf("%s store cooldown active = (%t, %v), want active", name, active, err)
		}
	}
	pending, err := store.ListSubmitted(ctx, 10)
	if err != nil {
		t.Fatalf("list submitted during provider cooldown: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("submitted reviews listed during provider cooldown: %+v", pending)
	}
	for _, review := range []feedback.Feedback{existing, newlySubmitted} {
		if _, claimed, err := restarted.ClaimForTriage(ctx, review.ID, time.Minute); err != nil || claimed {
			t.Fatalf("claim %s during persisted cooldown = (%t, %v), want not claimed", review.ID, claimed, err)
		}
	}
	if !time.Now().UTC().Before(retryAt) {
		t.Fatal("provider cooldown expired before active-cooldown assertions completed")
	}
	// Advance the persisted deadline to model expiration without waiting for wall-clock time.
	if _, err := store.triageStateCollection.UpdateOne(
		ctx,
		bson.M{"_id": "provider_cooldown"},
		bson.M{"$set": bson.M{"retryAt": time.Now().UTC().Add(-time.Second)}},
	); err != nil {
		t.Fatalf("expire persisted provider cooldown: %v", err)
	}
	for name, handle := range map[string]*Mongo{"original": store, "restarted": restarted} {
		active, err := handle.ProviderCooldownActive(ctx)
		if err != nil || active {
			t.Fatalf("%s store cooldown active after expiration = (%t, %v), want inactive", name, active, err)
		}
	}
	pending, err = restarted.ListSubmitted(ctx, 10)
	if err != nil || len(pending) != 2 {
		t.Fatalf("submitted reviews after cooldown = (%+v, %v), want both reviews", pending, err)
	}
	for _, review := range []feedback.Feedback{existing, newlySubmitted} {
		if _, claimed, err := restarted.ClaimForTriage(ctx, review.ID, time.Minute); err != nil || !claimed {
			t.Fatalf("claim %s after cooldown expiration = (%t, %v), want claimed", review.ID, claimed, err)
		}
	}
}

func TestTriageExpiredWorkerCannotOverwriteNewerClaimResult(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	review, err := store.Create(ctx, feedback.CreateInput{
		CourseID: "CS455", CourseTitle: "Course", Category: "teaching",
		Rating: 4, Content: "A review used to verify triage fencing.",
	})
	if err != nil {
		t.Fatalf("create feedback: %v", err)
	}
	stale, ok, err := store.ClaimForTriage(ctx, review.ID, time.Millisecond)
	if err != nil || !ok {
		t.Fatalf("claim stale worker = (%t, %v)", ok, err)
	}
	time.Sleep(3 * time.Millisecond)
	current, ok, err := store.ClaimForTriage(ctx, review.ID, time.Minute)
	if err != nil || !ok || current.TriageAttempts <= stale.TriageAttempts {
		t.Fatalf("claim current worker = (%+v, %t, %v)", current, ok, err)
	}
	if err := store.UpdateTriageStatus(ctx, review.ID, stale.TriageAttempts, feedback.StatusPublished, ""); !errors.Is(err, feedback.ErrReviewNotEligible) {
		t.Fatalf("stale worker update error = %v, want not eligible", err)
	}
	stored, err := store.FindByID(ctx, review.ID)
	if err != nil {
		t.Fatalf("find review after stale update: %v", err)
	}
	if stored.Status != feedback.StatusSubmitted {
		t.Fatalf("stale worker changed review status to %q, want %q", stored.Status, feedback.StatusSubmitted)
	}
	if err := store.ReleaseTriageAttempt(ctx, review.ID, stale.TriageAttempts); err != nil {
		t.Fatalf("release stale worker lease: %v", err)
	}
	if _, ok, err := store.ClaimForTriage(ctx, review.ID, time.Minute); err != nil || ok {
		t.Fatalf("claim after stale release = (%t, %v), want blocked by newer lease", ok, err)
	}
	if err := store.UpdateTriageStatus(ctx, review.ID, current.TriageAttempts, feedback.StatusFlagged, "newer worker decision"); err != nil {
		t.Fatalf("persist current worker result: %v", err)
	}
	stored, err = store.FindByID(ctx, review.ID)
	if err != nil {
		t.Fatalf("find result after stale update: %v", err)
	}
	if stored.Status != feedback.StatusFlagged || stored.FlagReason != "newer worker decision" {
		t.Fatalf("stale worker changed newer result: %+v", stored)
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
		Professor:   "Prof. Rao",
		Year:        2026,
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
	if queued[0].Year != input.Year || queued[0].Professor != input.Professor {
		t.Fatalf("course details = year %d, professor %q; want %d and %q", queued[0].Year, queued[0].Professor, input.Year, input.Professor)
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
	if resolved.PublishedAt == nil || resolved.FlagReason != "" {
		t.Fatalf("restored review publishedAt = %v, flagReason = %q; want a publish date and no flag reason", resolved.PublishedAt, resolved.FlagReason)
	}
}

func TestResolveAppealDoesNotResolveTicketWhenReviewIsMissing(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	ticket := queueTicketFields(appeals.StatusUnderAppeal, time.Now().UTC())
	ticket["reviewId"] = primitive.NewObjectID().Hex()
	ticketID := queueInsertTicket(t, ctx, store.appealCollection, ticket)

	const moderatorID = "missing-review-moderator"
	claimed, err := store.Claim(ctx, ticketID, moderatorID)
	if err != nil {
		t.Fatalf("claim appeal: %v", err)
	}
	if err := store.Resolve(ctx, claimed.ID, moderatorID, appeals.ActionRestore, ""); !errors.Is(err, appeals.ErrNotFound) {
		t.Fatalf("resolve appeal with missing review error = %v, want %v", err, appeals.ErrNotFound)
	}

	var resolved struct {
		Status string `bson:"status"`
	}
	id, err := primitive.ObjectIDFromHex(claimed.ID)
	if err != nil {
		t.Fatalf("parse appeal ID: %v", err)
	}
	if err := store.appealCollection.FindOne(ctx, bson.M{"_id": id}).Decode(&resolved); err != nil {
		t.Fatalf("read appeal after failed resolution: %v", err)
	}
	if resolved.Status != appeals.StatusLocked {
		t.Fatalf("appeal status after failed resolution = %q, want %q", resolved.Status, appeals.StatusLocked)
	}
}

func TestDeletedAppealIsRetainedThenPurged(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	input := feedback.CreateInput{
		CourseID: "CS455", CourseTitle: "Software Engineering", Professor: "Prof. Rao", Year: 2026,
		Category: "teaching", Rating: 4, Content: "A review to verify deletion retention.",
	}
	review, err := store.Create(ctx, input)
	if err != nil {
		t.Fatalf("create feedback: %v", err)
	}
	if err := store.UpdateStatus(ctx, review.ID, feedback.StatusFlagged, "other: requires review"); err != nil {
		t.Fatalf("flag feedback: %v", err)
	}
	if _, err := store.CreateAppeal(ctx, review.ReferenceCode, "Please reconsider this review."); err != nil {
		t.Fatalf("create appeal: %v", err)
	}
	queued, err := store.ListUnclaimed(ctx, appeals.SortOldestFirst)
	if err != nil || len(queued) != 1 {
		t.Fatalf("list appeal queue = (%+v, %v), want one ticket", queued, err)
	}
	const moderatorID = "retention-test-moderator"
	claimed, err := store.Claim(ctx, queued[0].ID, moderatorID)
	if err != nil {
		t.Fatalf("claim appeal: %v", err)
	}
	if err := store.Resolve(ctx, claimed.ID, moderatorID, appeals.ActionDelete, "Removed for violating guidelines."); err != nil {
		t.Fatalf("delete appealed review: %v", err)
	}

	var ticket struct {
		ID                primitive.ObjectID `bson:"_id"`
		ResolvedAt        time.Time          `bson:"resolvedAt"`
		ResolutionComment string             `bson:"resolutionComment"`
	}
	if err := store.appealCollection.FindOne(ctx, bson.M{"referenceCode": review.ReferenceCode}).Decode(&ticket); err != nil {
		t.Fatalf("read deleted appeal ticket: %v", err)
	}
	status, err := store.FindByReference(ctx, review.ReferenceCode)
	if err != nil {
		t.Fatalf("student status during retention: %v", err)
	}
	if status.Status != feedback.StatusDeleted || status.AppealResolutionComment != ticket.ResolutionComment {
		t.Fatalf("retained status = (%q, %q), want deleted with resolution comment", status.Status, status.AppealResolutionComment)
	}

	if count, err := store.PurgeDeletedReviews(ctx, ticket.ResolvedAt.Add(-time.Nanosecond)); err != nil || count != 0 {
		t.Fatalf("purge before retention expires = (%d, %v), want (0, nil)", count, err)
	}
	if count, err := store.PurgeDeletedReviews(ctx, ticket.ResolvedAt.Add(time.Nanosecond)); err != nil || count != 1 {
		t.Fatalf("purge after retention expires = (%d, %v), want (1, nil)", count, err)
	}
	if _, err := store.FindByReference(ctx, review.ReferenceCode); !errors.Is(err, feedback.ErrReviewNotFound) {
		t.Fatalf("student status after purge error = %v, want review not found", err)
	}
	if count, err := store.appealCollection.CountDocuments(ctx, bson.M{"_id": ticket.ID}); err != nil || count != 0 {
		t.Fatalf("appeal ticket count after purge = (%d, %v), want (0, nil)", count, err)
	}
}
