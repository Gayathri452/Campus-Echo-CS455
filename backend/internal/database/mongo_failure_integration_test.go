package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestMongoOperationsReturnCanceledDatabaseErrorsIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel()

	reviewID := primitive.NewObjectID().Hex()
	now := time.Now().UTC()
	tests := []struct {
		name string
		call func() error
	}{
		{"create student", func() error { _, err := store.CreateStudent(cancelledCtx, "cancelled@iitk.ac.in", "hash"); return err }},
		{"set student password", func() error { return store.SetStudentPassword(cancelledCtx, "cancelled@iitk.ac.in", "hash") }},
		{"create user", func() error {
			_, err := store.CreateUser(cancelledCtx, "cancelled@example.com", "hash", auth.RoleModerator)
			return err
		}},
		{"delete moderator", func() error { return store.DeleteModerator(cancelledCtx, "cancelled@example.com") }},
		{"find user by email", func() error { _, _, err := store.FindUserByEmail(cancelledCtx, "cancelled@example.com"); return err }},
		{"create session", func() error {
			return store.CreateSession(cancelledCtx, primitive.NewObjectID().Hex(), "cancelled-session", now.Add(time.Hour))
		}},
		{"find user by session", func() error { _, err := store.FindUserBySession(cancelledCtx, "cancelled-session", now); return err }},
		{"delete session", func() error { return store.DeleteSession(cancelledCtx, "cancelled-session") }},
		{"find OTP", func() error { _, err := store.FindOTP(cancelledCtx, "cancelled@iitk.ac.in"); return err }},
		{"invalidate OTP", func() error { return store.InvalidateOTP(cancelledCtx, "cancelled@iitk.ac.in") }},
		{"create OTP", func() error {
			return store.CreateOTP(cancelledCtx, auth.OTPChallenge{Email: "cancelled@iitk.ac.in", ExpiresAt: now.Add(time.Hour)})
		}},
		{"reserve OTP attempt", func() error { _, err := store.ReserveOTPAttempt(cancelledCtx, "cancelled@iitk.ac.in", now); return err }},
		{"delete OTP", func() error { return store.DeleteOTP(cancelledCtx, "cancelled@iitk.ac.in") }},
		{"create feedback", func() error { _, err := store.Create(cancelledCtx, feedback.CreateInput{}); return err }},
		{"explore feedback", func() error { _, err := store.Explore(cancelledCtx, feedback.ExploreFilters{}); return err }},
		{"course ratings", func() error { _, err := store.CourseRatings(cancelledCtx, "CS455"); return err }},
		{"find feedback by reference", func() error { _, err := store.FindByReference(cancelledCtx, "missing-reference"); return err }},
		{"find feedback by ID", func() error { _, err := store.FindByID(cancelledCtx, reviewID); return err }},
		{"claim triage", func() error { _, _, err := store.ClaimForTriage(cancelledCtx, reviewID, time.Minute); return err }},
		{"release triage", func() error { return store.ReleaseTriageAttempt(cancelledCtx, reviewID, 1) }},
		{"update status", func() error { return store.UpdateStatus(cancelledCtx, reviewID, feedback.StatusPublished, "") }},
		{"save semantic embedding", func() error {
			return store.SaveSemanticEmbedding(cancelledCtx, reviewID, "test-model", []float32{0.25})
		}},
		{"mark triage failed", func() error { return store.MarkTriageFailed(cancelledCtx, reviewID, "test failure") }},
		{"defer triage", func() error { return store.DeferTriage(cancelledCtx, reviewID, 1, "test", now.Add(time.Minute)) }},
		{"set provider cooldown", func() error { return store.SetProviderCooldown(cancelledCtx, "test", now.Add(time.Minute)) }},
		{"provider cooldown lookup", func() error { _, err := store.ProviderCooldownActive(cancelledCtx); return err }},
		{"flag reviews", func() error { return store.FlagReviews(cancelledCtx, []string{reviewID}, "test reason") }},
		{"list submitted", func() error { _, err := store.ListSubmitted(cancelledCtx, 10); return err }},
		{"list recent reviews", func() error { _, err := store.ListRecentByCourse(cancelledCtx, "CS455", 10); return err }},
		{"list reviews in window", func() error {
			_, err := store.ListCourseReviewsInWindow(cancelledCtx, "CS455", now.Add(-time.Hour), now)
			return err
		}},
		{"create appeal", func() error {
			_, err := store.CreateAppeal(cancelledCtx, "missing-reference", "test reason")
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("operation error = %v, want context cancellation", err)
			}
		})
	}
}
