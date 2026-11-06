package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestMongoAuthPersistenceLifecycleIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)

	student, err := store.CreateStudent(ctx, "student@iitk.ac.in", "student-hash")
	if err != nil {
		t.Fatalf("create student: %v", err)
	}
	found, hash, err := store.FindUserByEmail(ctx, student.Email)
	if err != nil || found.ID != student.ID || hash != "student-hash" {
		t.Fatalf("find student = (%+v, %q, %v)", found, hash, err)
	}
	duplicateStudent, err := store.CreateStudent(ctx, student.Email, "ignored-hash")
	if err != nil || duplicateStudent.ID != student.ID {
		t.Fatalf("idempotent student create = (%+v, %v)", duplicateStudent, err)
	}
	if err := store.SetStudentPassword(ctx, student.Email, "new-hash"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("setting an already-set student password = %v, want invalid credentials", err)
	}

	passwordlessID := primitive.NewObjectID()
	if _, err := store.userCollection.InsertOne(ctx, bson.M{
		"_id": passwordlessID, "email": "passwordless@iitk.ac.in", "role": auth.RoleStudent,
	}); err != nil {
		t.Fatalf("insert passwordless student: %v", err)
	}
	if err := store.SetStudentPassword(ctx, "passwordless@iitk.ac.in", "installed-hash"); err != nil {
		t.Fatalf("set password for passwordless student: %v", err)
	}
	if err := store.SetStudentPassword(ctx, "missing@iitk.ac.in", "hash"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("setting password for missing student = %v, want invalid credentials", err)
	}

	moderator, err := store.CreateUser(ctx, "moderator@iitk.ac.in", "moderator-hash", auth.RoleModerator)
	if err != nil {
		t.Fatalf("create moderator: %v", err)
	}
	if _, err := store.CreateUser(ctx, moderator.Email, "duplicate-hash", auth.RoleModerator); !errors.Is(err, auth.ErrEmailAlreadyExists) {
		t.Fatalf("duplicate user create = %v, want email already exists", err)
	}
	if err := store.DeleteModerator(ctx, student.Email); !errors.Is(err, auth.ErrModeratorNotFound) {
		t.Fatalf("delete student as moderator = %v, want moderator not found", err)
	}
	if err := store.DeleteModerator(ctx, "missing@iitk.ac.in"); !errors.Is(err, auth.ErrModeratorNotFound) {
		t.Fatalf("delete missing moderator = %v, want moderator not found", err)
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	if err := store.CreateSession(ctx, student.ID, "valid-session-hash", expiresAt); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := store.CreateSession(ctx, "not-an-object-id", "invalid-user-session", expiresAt); err == nil {
		t.Fatal("create session with malformed user ID succeeded")
	}
	sessionUser, err := store.FindUserBySession(ctx, "valid-session-hash", time.Now().UTC())
	if err != nil || sessionUser.ID != student.ID {
		t.Fatalf("find session user = (%+v, %v)", sessionUser, err)
	}
	if _, err := store.FindUserBySession(ctx, "missing-session", time.Now().UTC()); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("find missing session = %v, want invalid session", err)
	}
	if err := store.CreateSession(ctx, student.ID, "expired-session-hash", time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	if _, err := store.FindUserBySession(ctx, "expired-session-hash", time.Now().UTC()); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("find expired session = %v, want invalid session", err)
	}
	if err := store.DeleteSession(ctx, "valid-session-hash"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if err := store.DeleteSession(ctx, "valid-session-hash"); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("delete missing session = %v, want invalid session", err)
	}

	now := time.Now().UTC()
	challenge := auth.OTPChallenge{
		Email: "student@iitk.ac.in", OTPHash: "stored-hash", PasswordHash: "password-hash",
		ExpiresAt: now.Add(time.Hour), RequestedAt: now,
	}
	if _, err := store.FindOTP(ctx, challenge.Email); !errors.Is(err, auth.ErrInvalidOTP) {
		t.Fatalf("find missing OTP = %v, want invalid OTP", err)
	}
	if err := store.CreateOTP(ctx, challenge); err != nil {
		t.Fatalf("create OTP: %v", err)
	}
	if got, err := store.FindOTP(ctx, challenge.Email); err != nil || got.OTPHash != challenge.OTPHash {
		t.Fatalf("find OTP = (%+v, %v)", got, err)
	}
	reserved, err := store.ReserveOTPAttempt(ctx, challenge.Email, now)
	if err != nil || reserved.Attempts != 1 {
		t.Fatalf("reserve OTP attempt = (%+v, %v)", reserved, err)
	}
	if _, err := store.otpCollection.UpdateOne(ctx, bson.M{"_id": challenge.Email}, bson.M{"$set": bson.M{"attempts": auth.OTPMaxAttempts - 1}}); err != nil {
		t.Fatalf("set OTP near attempt limit: %v", err)
	}
	if reserved, err := store.ReserveOTPAttempt(ctx, challenge.Email, now); err != nil || reserved.Attempts != auth.OTPMaxAttempts {
		t.Fatalf("reserve final OTP attempt = (%+v, %v)", reserved, err)
	}
	if _, err := store.ReserveOTPAttempt(ctx, challenge.Email, now); !errors.Is(err, auth.ErrOTPAttempts) {
		t.Fatalf("reserve beyond OTP limit = %v, want attempts exceeded", err)
	}
	if err := store.InvalidateOTP(ctx, challenge.Email); err != nil {
		t.Fatalf("invalidate OTP: %v", err)
	}
	if err := store.DeleteOTP(ctx, challenge.Email); err != nil {
		t.Fatalf("delete absent OTP: %v", err)
	}

	expiredChallenge := challenge
	expiredChallenge.Email = "expired@iitk.ac.in"
	expiredChallenge.ExpiresAt = now.Add(-time.Minute)
	expiredChallenge.Attempts = 0
	if err := store.CreateOTP(ctx, expiredChallenge); err != nil {
		t.Fatalf("create expired OTP: %v", err)
	}
	if _, err := store.ReserveOTPAttempt(ctx, expiredChallenge.Email, now); !errors.Is(err, auth.ErrInvalidOTP) {
		t.Fatalf("reserve expired OTP = %v, want invalid OTP", err)
	}
	if err := store.DeleteModerator(ctx, moderator.Email); err != nil {
		t.Fatalf("delete moderator: %v", err)
	}
}

func TestMongoFeedbackReaderAndTriageHelpersIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	now := time.Now().UTC()
	base := bson.M{
		"referenceCode": "READER-" + primitive.NewObjectID().Hex(),
		"courseId":      "CS455", "courseTitle": "Software Engineering", "professor": "Prof. Test",
		"year": 2026, "category": "teaching", "rating": 4,
		"content": "The assignment instructions were clear and useful.", "createdAt": now,
	}
	publishedID := primitive.NewObjectID()
	published := bson.M{}
	for key, value := range base {
		published[key] = value
	}
	published["_id"] = publishedID
	published["status"] = feedback.StatusPublished
	if _, err := store.feedbackCollection.InsertOne(ctx, published); err != nil {
		t.Fatalf("insert published feedback: %v", err)
	}
	flaggedID := primitive.NewObjectID()
	flagged := bson.M{}
	for key, value := range base {
		flagged[key] = value
	}
	flagged["_id"] = flaggedID
	flagged["referenceCode"] = "FLAGGED-" + flaggedID.Hex()
	flagged["status"] = feedback.StatusFlagged
	if _, err := store.feedbackCollection.InsertOne(ctx, flagged); err != nil {
		t.Fatalf("insert flagged feedback: %v", err)
	}

	reviews, err := store.Explore(ctx, feedback.ExploreFilters{
		Search: "assignment", Category: "teaching", CourseID: "CS455", Rating: 4,
	})
	if err != nil || len(reviews) != 1 || reviews[0].ID != publishedID.Hex() {
		t.Fatalf("filtered Explore = (%+v, %v), want only published review", reviews, err)
	}
	if reviews, err := store.Explore(ctx, feedback.ExploreFilters{Rating: feedback.MaxRating + 1}); err != nil || len(reviews) != 1 {
		t.Fatalf("Explore with out-of-range rating = (%+v, %v), want rating filter ignored", reviews, err)
	}
	ratings, err := store.CourseRatings(ctx, "CS455")
	if err != nil || len(ratings) != 1 || ratings[0].ReviewCount != 1 || ratings[0].AverageRating != 4 {
		t.Fatalf("CourseRatings = (%+v, %v)", ratings, err)
	}
	ratings, err = store.CourseRatings(ctx, "MISSING")
	if err != nil || len(ratings) != 0 {
		t.Fatalf("empty CourseRatings = (%+v, %v), want empty result", ratings, err)
	}
	if _, err := store.FindByID(ctx, "invalid-id"); err == nil {
		t.Fatal("FindByID accepted malformed ObjectID")
	}
	if _, err := store.FindByID(ctx, primitive.NewObjectID().Hex()); !errors.Is(err, feedback.ErrReviewNotFound) {
		t.Fatalf("FindByID missing record = %v, want not found", err)
	}
	found, err := store.FindByReference(ctx, flagged["referenceCode"].(string))
	if err != nil || found.FlagReason != feedback.FlagReasonReview {
		t.Fatalf("FindByReference legacy flagged review = (%+v, %v)", found, err)
	}
	if _, err := store.FindByReference(ctx, "missing-reference"); !errors.Is(err, feedback.ErrReviewNotFound) {
		t.Fatalf("FindByReference missing record = %v, want not found", err)
	}

	recent, err := store.ListRecentByCourse(ctx, "CS455", 1)
	if err != nil || len(recent) != 1 {
		t.Fatalf("ListRecentByCourse = (%+v, %v)", recent, err)
	}
	window, err := store.ListCourseReviewsInWindow(ctx, "CS455", now.Add(-time.Minute), now.Add(time.Minute))
	if err != nil || len(window) != 2 {
		t.Fatalf("ListCourseReviewsInWindow = (%+v, %v), want two reviews", window, err)
	}
}

func TestTriageMongoDelegatesLifecycleOperationsIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	uri := os.Getenv("MONGODB_TEST_URI")
	triageStore, err := ConnectTriage(ctx, uri, store.feedbackCollection.Database().Name())
	if err != nil {
		t.Fatalf("connect triage adapter: %v", err)
	}
	t.Cleanup(func() {
		if err := triageStore.Disconnect(context.Background()); err != nil {
			t.Errorf("disconnect triage adapter: %v", err)
		}
	})

	insertReview := func(status string) string {
		t.Helper()
		id := primitive.NewObjectID()
		_, err := store.feedbackCollection.InsertOne(ctx, bson.M{
			"_id": id, "referenceCode": "TRIAGE-ADAPTER-" + id.Hex(),
			"courseId": "CS455", "courseTitle": "Software Engineering",
			"category": "teaching", "rating": 4,
			"content": "The assignments are useful and well structured.",
			"status":  status, "createdAt": time.Now().UTC(), "triageAttempts": 0,
		})
		if err != nil {
			t.Fatalf("insert triage review: %v", err)
		}
		return id.Hex()
	}

	claimedID := insertReview(feedback.StatusSubmitted)
	if review, err := triageStore.FindByID(ctx, claimedID); err != nil || review.ID != claimedID {
		t.Fatalf("triage FindByID = (%+v, %v)", review, err)
	}
	if active, err := triageStore.ProviderCooldownActive(ctx); err != nil || active {
		t.Fatalf("initial provider cooldown = (%t, %v), want inactive", active, err)
	}
	claimed, ok, err := triageStore.ClaimForTriage(ctx, claimedID, time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim through triage adapter = (%+v, %t, %v)", claimed, ok, err)
	}
	if err := triageStore.ReleaseTriageAttempt(ctx, claimedID, claimed.TriageAttempts); err != nil {
		t.Fatalf("release triage lease: %v", err)
	}
	claimed, ok, err = triageStore.ClaimForTriage(ctx, claimedID, time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaim through triage adapter = (%+v, %t, %v)", claimed, ok, err)
	}
	if err := triageStore.UpdateTriageStatus(ctx, claimedID, claimed.TriageAttempts, feedback.StatusFlagged, "adapter flag"); err != nil {
		t.Fatalf("update fenced triage status: %v", err)
	}

	publishedID := insertReview(feedback.StatusSubmitted)
	if err := triageStore.UpdateStatus(ctx, publishedID, feedback.StatusPublished, ""); err != nil {
		t.Fatalf("update review status: %v", err)
	}
	batchFlaggedID := insertReview(feedback.StatusSubmitted)
	if err := triageStore.FlagReviews(ctx, []string{batchFlaggedID}, "batch flag"); err != nil {
		t.Fatalf("flag review: %v", err)
	}
	if err := triageStore.SaveSemanticEmbedding(ctx, publishedID, "test-model", []float32{0.1, 0.2}); err != nil {
		t.Fatalf("save semantic embedding: %v", err)
	}
	recent, err := triageStore.ListRecentByCourse(ctx, "CS455", 10)
	if err != nil || len(recent) < 3 {
		t.Fatalf("list recent reviews = (%d, %v)", len(recent), err)
	}
	window, err := triageStore.ListCourseReviewsInWindow(ctx, "CS455", time.Now().Add(-time.Minute), time.Now().Add(time.Minute))
	if err != nil || len(window) < 3 {
		t.Fatalf("list reviews in window = (%d, %v)", len(window), err)
	}
	if _, err := triageStore.ListSubmitted(ctx, 10); err != nil {
		t.Fatalf("list submitted reviews: %v", err)
	}

	deferredID := insertReview(feedback.StatusSubmitted)
	retryAt := time.Now().UTC().Add(time.Hour)
	if err := triageStore.DeferTriage(ctx, deferredID, 0, "test_failure", retryAt); err != nil {
		t.Fatalf("defer review: %v", err)
	}
	cooldownAt := time.Now().UTC().Add(time.Hour)
	if err := triageStore.SetProviderCooldown(ctx, "test_failure", cooldownAt); err != nil {
		t.Fatalf("set provider cooldown: %v", err)
	}
	if active, err := triageStore.ProviderCooldownActive(ctx); err != nil || !active {
		t.Fatalf("provider cooldown after update = (%t, %v), want active", active, err)
	}
	failedID := insertReview(feedback.StatusSubmitted)
	if err := triageStore.MarkTriageFailed(ctx, failedID, "permanent failure"); err != nil {
		t.Fatalf("mark triage failed: %v", err)
	}
}
