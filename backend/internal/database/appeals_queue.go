package database

// Person A owns this file: queue, claim and active-appeals queries
// (SCRUM-10, SCRUM-11). Replace each stub body; keep the signatures.
//
// Rules from docs/moderator-api.md that these queries must follow:
//   - A ticket is claimable when status is under_appeal, OR status is locked
//     and lockedAt <= appeals.LockExpiryCutoff(now). Use the same filter for
//     ListUnclaimed and for Claim, so an expired lock can be claimed even if
//     the sweep has not run yet.
//   - Claim must be ONE atomic update (FindOneAndUpdate with the claimable
//     filter in the query), never a read followed by a write. No match means
//     ErrAlreadyClaimed if the ticket exists, ErrNotFound if it does not.
//   - Never read or write any student identity here.

import (
	"context"
	"errors"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var _ appeals.QueueStore = (*Mongo)(nil)

type appealSummaryDoc struct {
	ID            primitive.ObjectID `bson:"_id"`
	ReferenceCode string             `bson:"referenceCode"`
	CourseID      string             `bson:"courseId"`
	Year          int                `bson:"year"`
	Professor     string             `bson:"professor"`
	Status        string             `bson:"status"`
	LockedAt      *time.Time         `bson:"lockedAt,omitempty"`
	CreatedAt     time.Time          `bson:"createdAt"`
}

func summaryFromDoc(doc appealSummaryDoc) appeals.Summary {
	return appeals.Summary{
		ID:            doc.ID.Hex(),
		ReferenceCode: doc.ReferenceCode,
		CourseID:      doc.CourseID,
		Year:          doc.Year,
		Professor:     doc.Professor,
		Status:        doc.Status,
		LockedAt:      doc.LockedAt,
		CreatedAt:     doc.CreatedAt,
	}
}

func claimableFilter(now time.Time) bson.M {
	return bson.M{
		"$or": []bson.M{
			{"status": appeals.StatusUnderAppeal},
			{"status": appeals.StatusLocked, "lockedAt": bson.M{"$lte": appeals.LockExpiryCutoff(now)}},
		},
	}
}

func (m *Mongo) ListUnclaimed(ctx context.Context, sort string) ([]appeals.Summary, error) {
	if sort == "" {
		sort = appeals.SortOldestFirst
	}
	direction := 1
	if sort == appeals.SortNewestFirst {
		direction = -1
	}

	cursor, err := m.appealCollection.Find(ctx, claimableFilter(time.Now().UTC()), options.Find().SetSort(bson.D{{Key: "createdAt", Value: direction}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []appealSummaryDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	result := make([]appeals.Summary, 0, len(docs))
	for _, doc := range docs {
		result = append(result, summaryFromDoc(doc))
	}
	return result, nil
}

func (m *Mongo) Claim(ctx context.Context, ticketID, moderatorID string) (appeals.Summary, error) {
	objectID, err := primitive.ObjectIDFromHex(ticketID)
	if err != nil {
		return appeals.Summary{}, appeals.ErrNotFound
	}

	now := time.Now().UTC()
	var updated appealSummaryDoc
	result := m.appealCollection.FindOneAndUpdate(ctx,
		bson.M{"_id": objectID, "$or": claimableFilter(now)["$or"]},
		bson.M{"$set": bson.M{"status": appeals.StatusLocked, "lockedBy": moderatorID, "lockedAt": now}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	if err := result.Decode(&updated); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			var existing appealSummaryDoc
			if err := m.appealCollection.FindOne(ctx, bson.M{"_id": objectID}).Decode(&existing); err == nil {
				return appeals.Summary{}, appeals.ErrAlreadyClaimed
			}
			return appeals.Summary{}, appeals.ErrNotFound
		}
		return appeals.Summary{}, err
	}

	return summaryFromDoc(updated), nil
}

func (m *Mongo) ListMine(ctx context.Context, moderatorID string) ([]appeals.Summary, error) {
	now := time.Now().UTC()
	filter := bson.M{
		"status":    appeals.StatusLocked,
		"lockedBy":  moderatorID,
		"lockedAt":  bson.M{"$gt": appeals.LockExpiryCutoff(now)},
	}
	cursor, err := m.appealCollection.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var docs []appealSummaryDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, err
	}

	result := make([]appeals.Summary, 0, len(docs))
	for _, doc := range docs {
		result = append(result, summaryFromDoc(doc))
	}
	return result, nil
}
