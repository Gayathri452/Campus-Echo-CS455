package database

import (
	"context"
	"fmt"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/announcements"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// announcementCollection holds administrator notices. It is reached through
// the feedback collection's database so Connect and the Mongo struct do not
// need to change.
func (m *Mongo) announcementCollection() *mongo.Collection {
	return m.feedbackCollection.Database().Collection("announcements")
}

type announcementDoc struct {
	ID        primitive.ObjectID `bson:"_id"`
	Title     string             `bson:"title"`
	Message   string             `bson:"message"`
	CourseID  string             `bson:"courseId,omitempty"`
	Pinned    bool               `bson:"pinned"`
	ExpiresAt *time.Time         `bson:"expiresAt,omitempty"`
	CreatedAt time.Time          `bson:"createdAt"`
}

func (doc announcementDoc) toAnnouncement() announcements.Announcement {
	return announcements.Announcement{
		ID:        doc.ID.Hex(),
		Title:     doc.Title,
		Message:   doc.Message,
		CourseID:  doc.CourseID,
		Pinned:    doc.Pinned,
		ExpiresAt: doc.ExpiresAt,
		CreatedAt: doc.CreatedAt,
	}
}

// ListAnnouncements returns pinned announcements first, then newest first.
func (m *Mongo) ListAnnouncements(ctx context.Context, now time.Time, activeOnly bool, limit int) ([]announcements.Announcement, error) {
	filter := bson.M{}
	if activeOnly {
		filter["$or"] = []bson.M{
			{"expiresAt": bson.M{"$exists": false}},
			{"expiresAt": nil},
			{"expiresAt": bson.M{"$gt": now}},
		}
	}
	findOptions := options.Find().SetSort(bson.D{{Key: "pinned", Value: -1}, {Key: "createdAt", Value: -1}})
	if limit > 0 {
		findOptions.SetLimit(int64(limit))
	}
	cursor, err := m.announcementCollection().Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("find announcements: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []announcementDoc
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode announcements: %w", err)
	}
	results := make([]announcements.Announcement, 0, len(docs))
	for _, doc := range docs {
		results = append(results, doc.toAnnouncement())
	}
	return results, nil
}

// CreateAnnouncement stores a validated announcement and returns it with its
// new ID.
func (m *Mongo) CreateAnnouncement(ctx context.Context, announcement announcements.Announcement) (announcements.Announcement, error) {
	if announcement.CreatedAt.IsZero() {
		announcement.CreatedAt = time.Now().UTC()
	}
	doc := announcementDoc{
		ID:        primitive.NewObjectID(),
		Title:     announcement.Title,
		Message:   announcement.Message,
		CourseID:  announcement.CourseID,
		Pinned:    announcement.Pinned,
		ExpiresAt: announcement.ExpiresAt,
		CreatedAt: announcement.CreatedAt,
	}
	if _, err := m.announcementCollection().InsertOne(ctx, doc); err != nil {
		return announcements.Announcement{}, fmt.Errorf("insert announcement: %w", err)
	}
	return doc.toAnnouncement(), nil
}

// DeleteAnnouncement removes an announcement by ID.
func (m *Mongo) DeleteAnnouncement(ctx context.Context, id string) error {
	objectID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return announcements.ErrNotFound
	}
	result, err := m.announcementCollection().DeleteOne(ctx, bson.M{"_id": objectID})
	if err != nil {
		return fmt.Errorf("delete announcement: %w", err)
	}
	if result.DeletedCount == 0 {
		return announcements.ErrNotFound
	}
	return nil
}

var _ announcements.Store = (*Mongo)(nil)
