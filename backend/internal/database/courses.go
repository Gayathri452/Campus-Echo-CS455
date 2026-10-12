package database

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/courses"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// courseCollection is the administrator-managed catalog. It lives in the same
// database as feedback, reached through the feedback collection so the shared
// Mongo struct and Connect do not need to change.
func (m *Mongo) courseCollection() *mongo.Collection {
	return m.feedbackCollection.Database().Collection("courses")
}

// ensureCourseIndex makes courseId unique. CreateOne is idempotent, so calling
// it before each insert is safe; inserts are rare administrator actions.
func (m *Mongo) ensureCourseIndex(ctx context.Context) error {
	_, err := m.courseCollection().Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "courseId", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		return fmt.Errorf("create unique course index: %w", err)
	}
	return nil
}

// ListCourses returns catalog courses sorted by course ID. A non-empty search
// matches the course ID, title or instructor, case-insensitively.
func (m *Mongo) ListCourses(ctx context.Context, search string, limit int) ([]courses.Course, error) {
	if limit <= 0 || limit > courses.MaxListLimit {
		limit = courses.MaxListLimit
	}
	filter := bson.M{}
	if search != "" {
		pattern := primitive.Regex{Pattern: regexp.QuoteMeta(search), Options: "i"}
		filter["$or"] = []bson.M{
			{"courseId": pattern},
			{"title": pattern},
			{"instructor": pattern},
		}
	}
	cursor, err := m.courseCollection().Find(ctx, filter,
		options.Find().SetSort(bson.D{{Key: "courseId", Value: 1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("find courses: %w", err)
	}
	defer cursor.Close(ctx)

	results := []courses.Course{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, fmt.Errorf("decode courses: %w", err)
	}
	return results, nil
}

// CreateCourse inserts a validated course. A duplicate course ID returns
// courses.ErrCourseExists.
func (m *Mongo) CreateCourse(ctx context.Context, course courses.Course) (courses.Course, error) {
	if err := m.ensureCourseIndex(ctx); err != nil {
		return courses.Course{}, err
	}
	if course.CreatedAt.IsZero() {
		course.CreatedAt = time.Now().UTC()
	}
	_, err := m.courseCollection().InsertOne(ctx, course)
	if mongo.IsDuplicateKeyError(err) {
		return courses.Course{}, courses.ErrCourseExists
	}
	if err != nil {
		return courses.Course{}, fmt.Errorf("insert course: %w", err)
	}
	return course, nil
}

// DeleteCourse removes a course from the catalog. Existing reviews that name
// the course are not touched.
func (m *Mongo) DeleteCourse(ctx context.Context, courseID string) error {
	result, err := m.courseCollection().DeleteOne(ctx, bson.M{"courseId": courseID})
	if err != nil {
		return fmt.Errorf("delete course: %w", err)
	}
	if result.DeletedCount == 0 {
		return courses.ErrCourseNotFound
	}
	return nil
}

var _ courses.Store = (*Mongo)(nil)
