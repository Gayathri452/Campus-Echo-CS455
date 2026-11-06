package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func seedModeratorTicket(t *testing.T, store *Mongo, ctx context.Context, status, lockedBy string, lockedAt time.Time) (string, string) {
	t.Helper()

	reviewID := primitive.NewObjectID()
	referenceCode := "MODERATOR-" + reviewID.Hex()
	_, err := store.feedbackCollection.InsertOne(ctx, bson.M{
		"_id": reviewID, "referenceCode": referenceCode, "status": feedback.StatusFlagged,
	})
	if err != nil {
		t.Fatalf("insert review: %v", err)
	}
	fields := queueTicketFields(status, time.Now().UTC())
	fields["referenceCode"] = referenceCode
	fields["reviewId"] = reviewID.Hex()
	fields["feedbackText"] = "review text"
	fields["flagReason"] = "flag reason"
	fields["appealComment"] = "appeal comment"
	fields["lockedBy"] = lockedBy
	fields["lockedAt"] = lockedAt
	fields["lockExpiresAt"] = lockedAt.Add(appeals.LockDuration)
	return queueInsertTicket(t, ctx, store.appealCollection, fields), reviewID.Hex()
}

func TestAppealGetIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	tests := []struct {
		name    string
		holder  string
		lockAge time.Duration
		idKind  string
		wantErr error
	}{
		{"holder gets ticket", "moderator-1", time.Minute, "seeded", nil},
		{"other moderator is rejected", "moderator-1", time.Minute, "seeded", appeals.ErrNotLockHolder},
		{"expired lock is rejected", "moderator-1", 2 * time.Hour, "seeded", appeals.ErrNotLockHolder},
		{"unknown id is not found", "moderator-1", time.Minute, "unknown", appeals.ErrNotFound},
		{"bad object id is not found", "moderator-1", time.Minute, "bad", appeals.ErrNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			id, reviewID := seedModeratorTicket(t, store, ctx, appeals.StatusLocked, test.holder, now.Add(-test.lockAge))
			moderatorID := test.holder
			if test.name == "other moderator is rejected" {
				moderatorID = "moderator-2"
			}
			switch test.idKind {
			case "unknown":
				id = primitive.NewObjectID().Hex()
			case "bad":
				id = "not-an-object-id"
			}

			ticket, err := store.Get(ctx, id, moderatorID)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("Get error = %v, want %v", err, test.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Get: %v", err)
				}
				if ticket.ID != id || ticket.ReviewID != reviewID || ticket.LockedBy != moderatorID ||
					ticket.FeedbackText != "review text" || ticket.FlagReason != "flag reason" ||
					ticket.AppealComment != "appeal comment" {
					t.Fatalf("Get returned unexpected ticket: %+v", ticket)
				}
			}

			if test.idKind == "seeded" {
				objectID, _ := primitive.ObjectIDFromHex(id)
				var stored bson.M
				if err := store.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&stored); err != nil {
					t.Fatalf("read stored ticket: %v", err)
				}
				if stored["status"] != appeals.StatusLocked {
					t.Fatalf("Get changed ticket status: %#v", stored)
				}
			}
		})
	}
}

func TestAppealReleaseIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	tests := []struct {
		name    string
		lockAge time.Duration
		idKind  string
		holder  string
		wantErr error
	}{
		{"holder releases", time.Minute, "seeded", "moderator-1", nil},
		{"other moderator rejected", time.Minute, "seeded", "moderator-2", appeals.ErrNotLockHolder},
		{"expired lock rejected", 2 * time.Hour, "seeded", "moderator-1", appeals.ErrNotLockHolder},
		{"bad id rejected", time.Minute, "bad", "moderator-1", appeals.ErrNotLockHolder},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			id, _ := seedModeratorTicket(t, store, ctx, appeals.StatusLocked, "moderator-1", now.Add(-test.lockAge))
			if test.idKind == "bad" {
				id = "bad-id"
			}
			err := store.Release(ctx, id, test.holder)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Release error = %v, want %v", err, test.wantErr)
			}
			if test.idKind == "bad" {
				return
			}
			objectID, _ := primitive.ObjectIDFromHex(id)
			var stored bson.M
			if err := store.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&stored); err != nil {
				t.Fatalf("read ticket: %v", err)
			}
			if test.wantErr == nil {
				if stored["status"] != appeals.StatusUnderAppeal {
					t.Fatalf("status = %v, want %q", stored["status"], appeals.StatusUnderAppeal)
				}
				for _, field := range []string{"lockedBy", "lockedAt", "lockExpiresAt"} {
					if _, exists := stored[field]; exists {
						t.Errorf("%s remains after release: %v", field, stored[field])
					}
				}
			} else if stored["status"] != appeals.StatusLocked || stored["lockedBy"] != "moderator-1" {
				t.Fatalf("failed release changed ticket: %#v", stored)
			}
		})
	}
}

func TestAppealExtendLockIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	tests := []struct {
		name    string
		lockAge time.Duration
		idKind  string
		holder  string
		wantErr error
	}{
		{"holder extends lock", time.Minute, "seeded", "moderator-1", nil},
		{"other moderator rejected", time.Minute, "seeded", "moderator-2", appeals.ErrNotLockHolder},
		{"expired lock rejected", 2 * time.Hour, "seeded", "moderator-1", appeals.ErrNotLockHolder},
		{"bad id rejected", time.Minute, "bad", "moderator-1", appeals.ErrNotLockHolder},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			id, _ := seedModeratorTicket(t, store, ctx, appeals.StatusLocked, "moderator-1", now.Add(-test.lockAge))
			if test.idKind == "bad" {
				id = "bad-id"
			}
			before := time.Now().UTC()
			expiresAt, err := store.ExtendLock(ctx, id, test.holder)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ExtendLock error = %v, want %v", err, test.wantErr)
			}
			if test.idKind == "bad" {
				return
			}
			objectID, _ := primitive.ObjectIDFromHex(id)
			var stored bson.M
			if err := store.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&stored); err != nil {
				t.Fatalf("read ticket: %v", err)
			}
			if test.wantErr != nil {
				if stored["lockExpiresAt"] == nil {
					t.Fatalf("failed extension removed expiry: %#v", stored)
				}
				return
			}
			wantMin := before.Add(appeals.ExtendedLockDuration).Add(-5 * time.Second)
			wantMax := time.Now().UTC().Add(appeals.ExtendedLockDuration).Add(5 * time.Second)
			if expiresAt.Before(wantMin) || expiresAt.After(wantMax) {
				t.Fatalf("expiry = %v, want about now + %v", expiresAt, appeals.ExtendedLockDuration)
			}
			storedExpiry, ok := stored["lockExpiresAt"].(primitive.DateTime)
			if !ok || storedExpiry.Time().Sub(expiresAt).Abs() > time.Second {
				t.Fatalf("stored expiry = %v, returned expiry = %v", stored["lockExpiresAt"], expiresAt)
			}
		})
	}
}

func TestAppealResolveIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	tests := []struct {
		name     string
		action   string
		comment  string
		holder   string
		lockAge  time.Duration
		idKind   string
		review   string
		wantErr  error
		wantText string
	}{
		{"restore publishes review", appeals.ActionRestore, "", "moderator-1", time.Minute, "seeded", "present", nil, ""},
		{"delete saves comment and deletes review", appeals.ActionDelete, "guideline violation", "moderator-1", time.Minute, "seeded", "present", nil, ""},
		{"empty delete comment rejected", appeals.ActionDelete, "", "moderator-1", time.Minute, "seeded", "present", appeals.ErrCommentRequired, ""},
		{"unknown action rejected", "hide", "", "moderator-1", time.Minute, "seeded", "present", appeals.ErrInvalidAction, ""},
		{"other moderator rejected", appeals.ActionRestore, "", "moderator-2", time.Minute, "seeded", "present", appeals.ErrNotLockHolder, ""},
		{"expired lock rejected", appeals.ActionRestore, "", "moderator-1", 2 * time.Hour, "seeded", "present", appeals.ErrNotLockHolder, ""},
		{"bad id rejected", appeals.ActionRestore, "", "moderator-1", time.Minute, "bad", "present", appeals.ErrNotLockHolder, ""},
		{"missing review rolls back ticket", appeals.ActionRestore, "", "moderator-1", time.Minute, "seeded", "missing", appeals.ErrNotFound, ""},
		{"invalid review id errors without mutation", appeals.ActionRestore, "", "moderator-1", time.Minute, "seeded", "invalid", nil, "invalid review ID"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			id, reviewID := seedModeratorTicket(t, store, ctx, appeals.StatusLocked, "moderator-1", now.Add(-test.lockAge))
			objectID, _ := primitive.ObjectIDFromHex(id)
			reviewObjectID, _ := primitive.ObjectIDFromHex(reviewID)
			if test.review == "missing" {
				if _, err := store.feedbackCollection.DeleteOne(ctx, bson.M{"_id": reviewObjectID}); err != nil {
					t.Fatalf("delete review for rollback test: %v", err)
				}
			} else if test.review == "invalid" {
				if _, err := store.appealCollection.UpdateOne(ctx, bson.M{"_id": objectID}, bson.M{"$set": bson.M{"reviewId": "not-an-object-id"}}); err != nil {
					t.Fatalf("set invalid review id: %v", err)
				}
			}
			idForResolve := id
			if test.idKind == "bad" {
				idForResolve = "bad-id"
			}
			err := store.Resolve(ctx, idForResolve, test.holder, test.action, test.comment)
			switch {
			case test.wantErr != nil && !errors.Is(err, test.wantErr):
				t.Fatalf("Resolve error = %v, want %v", err, test.wantErr)
			case test.wantText != "" && (err == nil || !strings.Contains(err.Error(), test.wantText)):
				t.Fatalf("Resolve error = %v, want substring %q", err, test.wantText)
			case test.wantErr == nil && test.wantText == "" && err != nil:
				t.Fatalf("Resolve: %v", err)
			}

			var stored bson.M
			if err := store.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&stored); err != nil {
				t.Fatalf("read ticket: %v", err)
			}
			switch test.name {
			case "restore publishes review":
				if stored["status"] != appeals.StatusRestored || stored["resolvedAt"] == nil {
					t.Fatalf("restored ticket = %#v", stored)
				}
				var review bson.M
				if err := store.feedbackCollection.FindOne(ctx, bson.M{"_id": reviewObjectID}).Decode(&review); err != nil {
					t.Fatalf("read restored review: %v", err)
				}
				if review["status"] != feedback.StatusPublished {
					t.Fatalf("review status = %v, want %q", review["status"], feedback.StatusPublished)
				}
			case "delete saves comment and deletes review":
				if stored["status"] != appeals.StatusDeleted || stored["resolutionComment"] != test.comment || stored["resolvedAt"] == nil {
					t.Fatalf("deleted ticket = %#v", stored)
				}
				var review bson.M
				if err := store.feedbackCollection.FindOne(ctx, bson.M{"_id": reviewObjectID}).Decode(&review); err != nil {
					t.Fatalf("read deleted review: %v", err)
				}
				if review["status"] != feedback.StatusDeleted {
					t.Fatalf("review status = %v, want %q", review["status"], feedback.StatusDeleted)
				}
			default:
				if stored["status"] != appeals.StatusLocked {
					t.Fatalf("failed resolve changed ticket status: %#v", stored)
				}
				if _, exists := stored["resolvedAt"]; exists {
					t.Fatalf("failed resolve left resolvedAt: %#v", stored)
				}
				if _, exists := stored["resolutionComment"]; exists {
					t.Fatalf("failed resolve left resolutionComment: %#v", stored)
				}
				if test.review != "missing" {
					var review bson.M
					if err := store.feedbackCollection.FindOne(ctx, bson.M{"_id": reviewObjectID}).Decode(&review); err != nil {
						t.Fatalf("read unchanged review: %v", err)
					}
					if review["status"] != feedback.StatusFlagged {
						t.Fatalf("failed resolve changed review: %#v", review)
					}
				}
			}
		})
	}
}

func TestAppealReleaseExpiredIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	tests := []struct{ name string }{{"releases expired explicit and legacy locks only"}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			freshID, _ := seedModeratorTicket(t, store, ctx, appeals.StatusLocked, "fresh-mod", now.Add(-5*time.Minute))
			expiredID, _ := seedModeratorTicket(t, store, ctx, appeals.StatusLocked, "expired-mod", now.Add(-2*time.Hour))
			legacyID, _ := seedModeratorTicket(t, store, ctx, appeals.StatusLocked, "legacy-mod", now.Add(-2*time.Hour))
			underAppealID, _ := seedModeratorTicket(t, store, ctx, appeals.StatusUnderAppeal, "", now.Add(-2*time.Hour))
			restoredID, _ := seedModeratorTicket(t, store, ctx, appeals.StatusRestored, "restored-mod", now.Add(-2*time.Hour))
			legacyObjectID, _ := primitive.ObjectIDFromHex(legacyID)
			if _, err := store.appealCollection.UpdateOne(ctx, bson.M{"_id": legacyObjectID}, bson.M{"$unset": bson.M{"lockExpiresAt": ""}}); err != nil {
				t.Fatalf("make legacy lock: %v", err)
			}

			count, err := store.ReleaseExpired(ctx, now)
			if err != nil || count != 2 {
				t.Fatalf("ReleaseExpired = (%d, %v), want (2, nil)", count, err)
			}
			for _, id := range []string{expiredID, legacyID} {
				objectID, _ := primitive.ObjectIDFromHex(id)
				var doc bson.M
				if err := store.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&doc); err != nil {
					t.Fatalf("read released ticket %s: %v", id, err)
				}
				if doc["status"] != appeals.StatusUnderAppeal {
					t.Errorf("ticket %s status = %v, want under_appeal", id, doc["status"])
				}
				for _, field := range []string{"lockedBy", "lockedAt", "lockExpiresAt"} {
					if _, exists := doc[field]; exists {
						t.Errorf("ticket %s retains %s: %v", id, field, doc[field])
					}
				}
			}
			for _, id := range []string{freshID, underAppealID, restoredID} {
				objectID, _ := primitive.ObjectIDFromHex(id)
				var doc bson.M
				if err := store.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&doc); err != nil {
					t.Fatalf("read unchanged ticket %s: %v", id, err)
				}
				wantStatus := appeals.StatusLocked
				if id == underAppealID {
					wantStatus = appeals.StatusUnderAppeal
				} else if id == restoredID {
					wantStatus = appeals.StatusRestored
				}
				if doc["status"] != wantStatus {
					t.Errorf("ticket %s status = %v, want %s", id, doc["status"], wantStatus)
				}
			}
			count, err = store.ReleaseExpired(ctx, now)
			if err != nil || count != 0 {
				t.Fatalf("second ReleaseExpired = (%d, %v), want (0, nil)", count, err)
			}
		})
	}
}

func TestAppealClaimAdditionalCasesIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	tests := []struct {
		name    string
		status  string
		lockAge time.Duration
	}{
		{"expired lock can be claimed", appeals.StatusLocked, 2 * time.Hour},
		{"restored ticket claim error", appeals.StatusRestored, time.Minute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			id, _ := seedModeratorTicket(t, store, ctx, test.status, "previous-mod", now.Add(-test.lockAge))
			summary, err := store.Claim(ctx, id, "claiming-mod")
			objectID, _ := primitive.ObjectIDFromHex(id)
			var stored bson.M
			if findErr := store.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&stored); findErr != nil {
				t.Fatalf("read claimed ticket: %v", findErr)
			}
			if test.status == appeals.StatusLocked {
				if err != nil {
					t.Fatalf("claim expired lock: %v", err)
				}
				if summary.ID != id || stored["status"] != appeals.StatusLocked || stored["lockedBy"] != "claiming-mod" {
					t.Fatalf("claim result = %+v, stored ticket = %#v", summary, stored)
				}
				return
			}
			if err == nil {
				t.Fatal("claim restored ticket succeeded; want an error")
			}
			t.Logf("claiming restored ticket returned %T: %v", err, err)
			if stored["status"] != appeals.StatusRestored || stored["lockedBy"] != "previous-mod" {
				t.Fatalf("failed claim changed restored ticket: %#v", stored)
			}
		})
	}
}

func TestAppealPurgeDeletedReviewsAdditionalCasesIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	tests := []struct {
		name string
	}{
		{"newer tickets are retained"},
		{"no matches returns zero"},
		{"valid records are purged despite malformed review id"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Now().UTC()
			cutoff := now.Add(-72 * time.Hour)
			switch test.name {
			case "newer tickets are retained":
				id, reviewID := seedModeratorTicket(t, store, ctx, appeals.StatusDeleted, "", now)
				objectID, _ := primitive.ObjectIDFromHex(id)
				if _, err := store.appealCollection.UpdateOne(ctx, bson.M{"_id": objectID}, bson.M{"$set": bson.M{"resolvedAt": cutoff.Add(time.Second)}}); err != nil {
					t.Fatalf("set resolvedAt: %v", err)
				}
				count, err := store.PurgeDeletedReviews(ctx, cutoff)
				if err != nil || count != 0 {
					t.Fatalf("PurgeDeletedReviews = (%d, %v), want (0, nil)", count, err)
				}
				var ticket bson.M
				if err := store.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&ticket); err != nil {
					t.Fatalf("new ticket was purged: %v", err)
				}
				reviewObjectID, _ := primitive.ObjectIDFromHex(reviewID)
				if err := store.feedbackCollection.FindOne(ctx, bson.M{"_id": reviewObjectID}).Err(); err != nil {
					t.Fatalf("new ticket review was purged: %v", err)
				}
			case "no matches returns zero":
				count, err := store.PurgeDeletedReviews(ctx, cutoff)
				if err != nil || count != 0 {
					t.Fatalf("PurgeDeletedReviews = (%d, %v), want (0, nil)", count, err)
				}
			default:
				validIDs := make([]string, 0, 2)
				validReviewIDs := make([]string, 0, 2)
				for i := 0; i < 2; i++ {
					id, reviewID := seedModeratorTicket(t, store, ctx, appeals.StatusDeleted, "", cutoff.Add(-time.Hour))
					validIDs = append(validIDs, id)
					validReviewIDs = append(validReviewIDs, reviewID)
					objectID, _ := primitive.ObjectIDFromHex(id)
					if _, err := store.appealCollection.UpdateOne(ctx, bson.M{"_id": objectID}, bson.M{"$set": bson.M{"resolvedAt": cutoff.Add(-time.Hour)}}); err != nil {
						t.Fatalf("set resolvedAt: %v", err)
					}
				}
				badID, badReviewID := seedModeratorTicket(t, store, ctx, appeals.StatusDeleted, "", cutoff.Add(-time.Hour))
				badObjectID, _ := primitive.ObjectIDFromHex(badID)
				if _, err := store.appealCollection.UpdateOne(ctx, bson.M{"_id": badObjectID}, bson.M{"$set": bson.M{"reviewId": "invalid-review-id", "resolvedAt": cutoff.Add(-time.Hour)}}); err != nil {
					t.Fatalf("set invalid review id: %v", err)
				}
				count, err := store.PurgeDeletedReviews(ctx, cutoff)
				if err != nil || count != int64(len(validIDs)) {
					t.Fatalf("PurgeDeletedReviews = (%d, %v), want (%d, nil)", count, err, len(validIDs))
				}
				for i, id := range validIDs {
					objectID, _ := primitive.ObjectIDFromHex(id)
					if err := store.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Err(); !errors.Is(err, mongo.ErrNoDocuments) {
						t.Errorf("valid ticket %s lookup error = %v, want purged", id, err)
					}
					reviewObjectID, _ := primitive.ObjectIDFromHex(validReviewIDs[i])
					if err := store.feedbackCollection.FindOne(ctx, bson.M{"_id": reviewObjectID}).Err(); !errors.Is(err, mongo.ErrNoDocuments) {
						t.Errorf("valid review %s lookup error = %v, want purged", validReviewIDs[i], err)
					}
				}
				var malformedTicket bson.M
				if err := store.appealCollection.FindOne(ctx, bson.M{"_id": badObjectID}).Decode(&malformedTicket); err != nil {
					t.Errorf("malformed ticket lookup error = %v, want ticket retained", err)
				}
				badReviewObjectID, _ := primitive.ObjectIDFromHex(badReviewID)
				if err := store.feedbackCollection.FindOne(ctx, bson.M{"_id": badReviewObjectID}).Err(); err != nil {
					t.Errorf("malformed ticket's review lookup error = %v, want review retained", err)
				}
			}
		})
	}
}
