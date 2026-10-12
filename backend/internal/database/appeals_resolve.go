package database

// Person B owns this file: detail, release, resolve and the expiry sweep
// (SCRUM-12, SCRUM-13). Replace each stub body; keep the signatures.
//
// Rules from docs/moderator-api.md that these operations must follow:
//   - Get, Release and Resolve only work for the moderator in lockedBy, and
//     only while the lock is unexpired (see appeals.IsLockExpired). Do the
//     lockedBy/status check inside the update filter, not in Go after reading.
//   - Release sets status back to under_appeal and clears lockedBy/lockedAt.
//   - Resolve sets status to restored or deleted, stores resolutionComment and
//     resolvedAt, and updates the review in the feedback collection
//     (restored -> published, deleted -> deleted).
//   - ReleaseExpired is one UpdateMany: status locked and
//     lockedAt <= appeals.LockExpiryCutoff(now) -> under_appeal.
//   - Never read or write any student identity here.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ appeals.ResolveStore = (*Mongo)(nil)

func (m *Mongo) Get(ctx context.Context, ticketID, moderatorID string) (appeals.Ticket, error) {
	id, err := primitive.ObjectIDFromHex(ticketID)
	if err != nil {
		return appeals.Ticket{}, appeals.ErrNotFound
	}
	now := time.Now().UTC()
	filter := bson.M{
		"_id":      id,
		"status":   appeals.StatusLocked,
		"lockedBy": moderatorID,
		"lockedAt": bson.M{"$gt": appeals.LockExpiryCutoff(now)},
	}
	var record struct {
		ID                primitive.ObjectID `bson:"_id"`
		ReferenceCode     string             `bson:"referenceCode"`
		CourseID          string             `bson:"courseId"`
		Year              int                `bson:"year"`
		Professor         string             `bson:"professor"`
		Status            string             `bson:"status"`
		LockedAt          *time.Time         `bson:"lockedAt"`
		CreatedAt         time.Time          `bson:"createdAt"`
		ReviewID          string             `bson:"reviewId"`
		FeedbackText      string             `bson:"feedbackText"`
		FlagReason        string             `bson:"flagReason"`
		AppealComment     string             `bson:"appealComment"`
		ResolutionComment string             `bson:"resolutionComment"`
		ResolvedAt        *time.Time         `bson:"resolvedAt"`
	}
	err = m.appealCollection.FindOne(ctx, filter).Decode(&record)
	if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
		return appeals.Ticket{}, fmt.Errorf("find appeal ticket: %w", err)
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		count, countErr := m.appealCollection.CountDocuments(ctx, bson.M{"_id": id})
		if countErr != nil {
			return appeals.Ticket{}, fmt.Errorf("check appeal ticket: %w", countErr)
		}
		if count == 0 {
			return appeals.Ticket{}, appeals.ErrNotFound
		}
		return appeals.Ticket{}, appeals.ErrNotLockHolder
	}
	return appeals.Ticket{
		Summary: appeals.Summary{
			ID: record.ID.Hex(), ReferenceCode: record.ReferenceCode,
			CourseID: record.CourseID, Year: record.Year, Professor: record.Professor,
			Status: record.Status, LockedAt: record.LockedAt, CreatedAt: record.CreatedAt,
		},
		ReviewID: record.ReviewID, FeedbackText: record.FeedbackText,
		FlagReason: record.FlagReason, AppealComment: record.AppealComment,
		LockedBy: moderatorID, ResolutionComment: record.ResolutionComment,
		ResolvedAt: record.ResolvedAt,
	}, nil
}

func (m *Mongo) Release(ctx context.Context, ticketID, moderatorID string) error {
	id, err := primitive.ObjectIDFromHex(ticketID)
	if err != nil {
		return appeals.ErrNotLockHolder
	}
	result, err := m.appealCollection.UpdateOne(ctx, activeLockFilter(id, moderatorID, time.Now().UTC()), bson.M{
		"$set":   bson.M{"status": appeals.StatusUnderAppeal},
		"$unset": bson.M{"lockedBy": "", "lockedAt": ""},
	})
	if err != nil {
		return fmt.Errorf("release appeal ticket: %w", err)
	}
	if result.MatchedCount == 0 {
		return appeals.ErrNotLockHolder
	}
	return nil
}

func (m *Mongo) Resolve(ctx context.Context, ticketID, moderatorID, action, comment string) error {
	if action != appeals.ActionRestore && action != appeals.ActionDelete {
		return appeals.ErrInvalidAction
	}
	if action == appeals.ActionDelete && comment == "" {
		return appeals.ErrCommentRequired
	}
	id, err := primitive.ObjectIDFromHex(ticketID)
	if err != nil {
		return appeals.ErrNotLockHolder
	}
	var ticket struct {
		ReviewID string `bson:"reviewId"`
	}
	now := time.Now().UTC()
	filter := activeLockFilter(id, moderatorID, now)
	if err := m.appealCollection.FindOne(ctx, filter).Decode(&ticket); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return appeals.ErrNotLockHolder
		}
		return fmt.Errorf("read appeal before resolving: %w", err)
	}
	reviewID, err := primitive.ObjectIDFromHex(ticket.ReviewID)
	if err != nil {
		return fmt.Errorf("invalid review ID on appeal ticket: %w", err)
	}
	status := appeals.StatusRestored
	reviewStatus := "published"
	if action == appeals.ActionDelete {
		status = appeals.StatusDeleted
		reviewStatus = "deleted"
	}
	result, err := m.appealCollection.UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{
			"status": status, "resolutionComment": comment, "resolvedAt": now,
		},
	})
	if err != nil {
		return fmt.Errorf("resolve appeal ticket: %w", err)
	}
	if result.MatchedCount == 0 {
		return appeals.ErrNotLockHolder
	}
	reviewResult, err := m.feedbackCollection.UpdateOne(ctx, bson.M{"_id": reviewID}, bson.M{
		"$set": bson.M{"status": reviewStatus},
	})
	if err == nil && reviewResult.MatchedCount == 0 {
		err = mongo.ErrNoDocuments
	}
	if err != nil {
		_, rollbackErr := m.appealCollection.UpdateOne(ctx, bson.M{
			"_id": id, "status": status, "resolvedAt": now,
		}, bson.M{
			"$set":   bson.M{"status": appeals.StatusLocked},
			"$unset": bson.M{"resolutionComment": "", "resolvedAt": ""},
		})
		if rollbackErr != nil {
			return fmt.Errorf("update review: %v; restore appeal lock: %w", err, rollbackErr)
		}
		if errors.Is(err, mongo.ErrNoDocuments) {
			return appeals.ErrNotFound
		}
		return fmt.Errorf("update review status: %w", err)
	}
	return nil
}

func (m *Mongo) ReleaseExpired(ctx context.Context, now time.Time) (int64, error) {
	result, err := m.appealCollection.UpdateMany(ctx, bson.M{
		"status":   appeals.StatusLocked,
		"lockedAt": bson.M{"$lte": appeals.LockExpiryCutoff(now)},
	}, bson.M{
		"$set":   bson.M{"status": appeals.StatusUnderAppeal},
		"$unset": bson.M{"lockedBy": "", "lockedAt": ""},
	})
	if err != nil {
		return 0, fmt.Errorf("release expired appeal locks: %w", err)
	}
	return result.ModifiedCount, nil
}

func activeLockFilter(id primitive.ObjectID, moderatorID string, now time.Time) bson.M {
	return bson.M{
		"_id":      id,
		"status":   appeals.StatusLocked,
		"lockedBy": moderatorID,
		"lockedAt": bson.M{"$gt": appeals.LockExpiryCutoff(now)},
	}
}

var _ = options.ReturnDocument(0)
