package database

import (
	"context"
	"fmt"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TriageStore interface {
	FindByID(context.Context, string) (feedback.Feedback, error)
	ClaimForTriage(context.Context, string, time.Duration) (feedback.Feedback, bool, error)
	ReleaseTriage(context.Context, string) error
	UpdateStatus(context.Context, string, string, string) error
	FlagReviews(context.Context, []string, string) error
	ListRecentByCourse(context.Context, string, int) ([]feedback.Feedback, error)
	ListCourseReviewsInWindow(context.Context, string, time.Time, time.Time) ([]feedback.Feedback, error)
	SaveSemanticEmbedding(context.Context, string, string, []float32) error
	ListSubmitted(context.Context, int) ([]feedback.Feedback, error)
	MarkTriageFailed(context.Context, string, string) error
	Disconnect(context.Context) error
}

type TriageMongo struct {
	store *Mongo
}

var _ TriageStore = (*TriageMongo)(nil)

func ConnectTriage(ctx context.Context, uri, databaseName string) (*TriageMongo, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("create triage MongoDB client: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if disconnectErr := client.Disconnect(disconnectCtx); disconnectErr != nil {
			return nil, fmt.Errorf("ping triage MongoDB: %w (disconnect client: %v)", err, disconnectErr)
		}
		return nil, fmt.Errorf("ping triage MongoDB: %w", err)
	}

	return &TriageMongo{
		store: &Mongo{
			client:             client,
			feedbackCollection: client.Database(databaseName).Collection("feedback"),
		},
	}, nil
}

func (m *TriageMongo) FindByID(ctx context.Context, id string) (feedback.Feedback, error) {
	return m.store.FindByID(ctx, id)
}

func (m *TriageMongo) ClaimForTriage(ctx context.Context, id string, lease time.Duration) (feedback.Feedback, bool, error) {
	return m.store.ClaimForTriage(ctx, id, lease)
}

func (m *TriageMongo) ReleaseTriage(ctx context.Context, id string) error {
	return m.store.ReleaseTriage(ctx, id)
}

func (m *TriageMongo) UpdateStatus(ctx context.Context, id, status, reason string) error {
	return m.store.UpdateStatus(ctx, id, status, reason)
}

func (m *TriageMongo) FlagReviews(ctx context.Context, ids []string, reason string) error {
	return m.store.FlagReviews(ctx, ids, reason)
}

func (m *TriageMongo) ListRecentByCourse(ctx context.Context, courseID string, limit int) ([]feedback.Feedback, error) {
	return m.store.ListRecentByCourse(ctx, courseID, limit)
}

func (m *TriageMongo) ListCourseReviewsInWindow(ctx context.Context, courseID string, since, until time.Time) ([]feedback.Feedback, error) {
	return m.store.ListCourseReviewsInWindow(ctx, courseID, since, until)
}

func (m *TriageMongo) SaveSemanticEmbedding(ctx context.Context, id, model string, embedding []float32) error {
	return m.store.SaveSemanticEmbedding(ctx, id, model, embedding)
}

func (m *TriageMongo) ListSubmitted(ctx context.Context, limit int) ([]feedback.Feedback, error) {
	return m.store.ListSubmitted(ctx, limit)
}

func (m *TriageMongo) MarkTriageFailed(ctx context.Context, id, reason string) error {
	return m.store.MarkTriageFailed(ctx, id, reason)
}

func (m *TriageMongo) Disconnect(ctx context.Context) error {
	if err := m.store.Disconnect(ctx); err != nil {
		return fmt.Errorf("disconnect triage MongoDB client: %w", err)
	}
	return nil
}
