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
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
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
		"$or":      unexpiredLockConditions(now),
	}
	var record struct {
		ID                primitive.ObjectID `bson:"_id"`
		ReferenceCode     string             `bson:"referenceCode"`
		CourseID          string             `bson:"courseId"`
		Year              int                `bson:"year"`
		Professor         string             `bson:"professor"`
		Status            string             `bson:"status"`
		LockedAt          *time.Time         `bson:"lockedAt"`
		LockExpiresAt     *time.Time         `bson:"lockExpiresAt"`
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
			Status: record.Status, LockedAt: record.LockedAt,
			LockExpiresAt: record.LockExpiresAt, CreatedAt: record.CreatedAt,
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
		"$unset": bson.M{"lockedBy": "", "lockedAt": "", "lockExpiresAt": ""},
	})
	if err != nil {
		return fmt.Errorf("release appeal ticket: %w", err)
	}
	if result.MatchedCount == 0 {
		return appeals.ErrNotLockHolder
	}
	return nil
}

func (m *Mongo) ExtendLock(ctx context.Context, ticketID, moderatorID string) (time.Time, error) {
	id, err := primitive.ObjectIDFromHex(ticketID)
	if err != nil {
		return time.Time{}, appeals.ErrNotLockHolder
	}

	now := time.Now().UTC()
	expiresAt := now.Add(appeals.ExtendedLockDuration)
	result, err := m.appealCollection.UpdateOne(ctx, activeLockFilter(id, moderatorID, now), bson.M{
		"$set": bson.M{"lockExpiresAt": expiresAt},
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("extend appeal lock: %w", err)
	}
	if result.MatchedCount == 0 {
		return time.Time{}, appeals.ErrNotLockHolder
	}
	return expiresAt, nil
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
	now := time.Now().UTC()
	filter := activeLockFilter(id, moderatorID, now)
	status := appeals.StatusRestored
	reviewStatus := feedback.StatusPublished
	if action == appeals.ActionDelete {
		status = appeals.StatusDeleted
		reviewStatus = feedback.StatusDeleted
	}
	session, err := m.client.StartSession()
	if err != nil {
		return fmt.Errorf("start appeal resolution session: %w", err)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (interface{}, error) {
		var ticket struct {
			ReviewID string `bson:"reviewId"`
		}
		if err := m.appealCollection.FindOne(txCtx, filter).Decode(&ticket); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				return nil, appeals.ErrNotLockHolder
			}
			return nil, fmt.Errorf("read appeal before resolving: %w", err)
		}
		reviewID, err := primitive.ObjectIDFromHex(ticket.ReviewID)
		if err != nil {
			return nil, fmt.Errorf("invalid review ID on appeal ticket: %w", err)
		}

		result, err := m.appealCollection.UpdateOne(txCtx, filter, bson.M{
			"$set": bson.M{
				"status": status, "resolutionComment": comment, "resolvedAt": now,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("resolve appeal ticket: %w", err)
		}
		if result.MatchedCount == 0 {
			return nil, appeals.ErrNotLockHolder
		}

		reviewSet := bson.M{"status": reviewStatus}
		reviewUpdate := bson.M{"$set": reviewSet}
		if action == appeals.ActionRestore {
			reviewSet["publishedAt"] = now
			reviewUpdate["$unset"] = bson.M{"flagReason": ""}
		}
		reviewResult, err := m.feedbackCollection.UpdateOne(txCtx, bson.M{"_id": reviewID}, reviewUpdate)
		if err != nil {
			return nil, fmt.Errorf("update review status: %w", err)
		}
		if reviewResult.MatchedCount == 0 {
			return nil, appeals.ErrNotFound
		}
		return nil, nil
	})
	if err != nil {
		if errors.Is(err, appeals.ErrNotLockHolder) || errors.Is(err, appeals.ErrNotFound) {
			return err
		}
		return fmt.Errorf("resolve appeal transaction: %w", err)
	}
	return nil
}

func (m *Mongo) ReleaseExpired(ctx context.Context, now time.Time) (int64, error) {
	result, err := m.appealCollection.UpdateMany(ctx, bson.M{
		"status": appeals.StatusLocked,
		"$or":    expiredLockConditions(now),
	}, bson.M{
		"$set":   bson.M{"status": appeals.StatusUnderAppeal},
		"$unset": bson.M{"lockedBy": "", "lockedAt": "", "lockExpiresAt": ""},
	})
	if err != nil {
		return 0, fmt.Errorf("release expired appeal locks: %w", err)
	}
	return result.ModifiedCount, nil
}

func (m *Mongo) PurgeDeletedReviews(ctx context.Context, before time.Time) (int64, error) {
	cursor, err := m.appealCollection.Find(ctx, bson.M{
		"status":     appeals.StatusDeleted,
		"resolvedAt": bson.M{"$lte": before},
	}, options.Find().SetProjection(bson.M{"_id": 1, "reviewId": 1}))
	if err != nil {
		return 0, fmt.Errorf("find deleted appeals eligible for purge: %w", err)
	}
	defer cursor.Close(ctx)

	var tickets []struct {
		ID       primitive.ObjectID `bson:"_id"`
		ReviewID string             `bson:"reviewId"`
	}
	if err := cursor.All(ctx, &tickets); err != nil {
		return 0, fmt.Errorf("decode deleted appeals eligible for purge: %w", err)
	}
	if len(tickets) == 0 {
		return 0, nil
	}

	ticketIDs := make([]primitive.ObjectID, 0, len(tickets))
	reviewIDs := make([]primitive.ObjectID, 0, len(tickets))
	for _, ticket := range tickets {
		reviewID, err := primitive.ObjectIDFromHex(ticket.ReviewID)
		if err != nil {
			return 0, fmt.Errorf("parse review ID for deleted appeal %s: %w", ticket.ID.Hex(), err)
		}
		ticketIDs = append(ticketIDs, ticket.ID)
		reviewIDs = append(reviewIDs, reviewID)
	}

	if _, err := m.feedbackCollection.DeleteMany(ctx, bson.M{"_id": bson.M{"$in": reviewIDs}}); err != nil {
		return 0, fmt.Errorf("permanently delete expired feedback: %w", err)
	}
	result, err := m.appealCollection.DeleteMany(ctx, bson.M{
		"_id":        bson.M{"$in": ticketIDs},
		"status":     appeals.StatusDeleted,
		"resolvedAt": bson.M{"$lte": before},
	})
	if err != nil {
		return 0, fmt.Errorf("permanently delete expired appeal tickets: %w", err)
	}
	return result.DeletedCount, nil
}

func activeLockFilter(id primitive.ObjectID, moderatorID string, now time.Time) bson.M {
	return bson.M{
		"_id":      id,
		"status":   appeals.StatusLocked,
		"lockedBy": moderatorID,
		"$or":      unexpiredLockConditions(now),
	}
}

var _ = options.ReturnDocument(0)
