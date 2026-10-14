package database

import (
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestBrigadingPersistenceAndWindowIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	now := time.Now().UTC()
	publishedAt := now.Add(-time.Minute)
	insertReview := func(courseID, status string, createdAt time.Time) string {
		t.Helper()
		id := primitive.NewObjectID()
		document := bson.M{
			"_id":           id,
			"referenceCode": "BRIGADE-" + id.Hex(),
			"courseId":      courseID,
			"courseTitle":   "Test course",
			"category":      "teaching",
			"rating":        3,
			"content":       "The course has a clear and useful structure",
			"status":        status,
			"createdAt":     createdAt,
		}
		if status == feedback.StatusPublished {
			document["publishedAt"] = publishedAt
		}
		if _, err := store.feedbackCollection.InsertOne(ctx, document); err != nil {
			t.Fatalf("insert test feedback: %v", err)
		}
		return id.Hex()
	}

	submittedID := insertReview("CS455", feedback.StatusSubmitted, now.Add(-time.Minute))
	publishedID := insertReview("CS455", feedback.StatusPublished, now)
	deletedID := insertReview("CS455", "deleted", now)
	outsideWindowID := insertReview("CS455", feedback.StatusSubmitted, now.Add(-11*time.Minute))
	otherCourseID := insertReview("CS456", feedback.StatusSubmitted, now)

	wantEmbedding := []float32{0.25, 0.5, 0.75}
	if err := store.SaveSemanticEmbedding(ctx, submittedID, "test-embedding-model", wantEmbedding); err != nil {
		t.Fatalf("save semantic embedding: %v", err)
	}

	recent, err := store.ListCourseReviewsInWindow(ctx, "CS455", now.Add(-10*time.Minute), now)
	if err != nil {
		t.Fatalf("list reviews in velocity window: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("window returned %d reviews, want 3", len(recent))
	}
	var savedEmbeddingFound bool
	for _, review := range recent {
		if review.ID == submittedID {
			savedEmbeddingFound = review.SemanticEmbeddingModel == "test-embedding-model" &&
				len(review.SemanticEmbedding) == len(wantEmbedding)
			for i := range wantEmbedding {
				savedEmbeddingFound = savedEmbeddingFound && review.SemanticEmbedding[i] == wantEmbedding[i]
			}
		}
		if review.ID == outsideWindowID || review.ID == otherCourseID {
			t.Fatalf("window query returned out-of-scope review %s", review.ID)
		}
	}
	if !savedEmbeddingFound {
		t.Fatal("window query did not return the persisted semantic embedding and model")
	}

	if err := store.FlagReviews(ctx, []string{submittedID, publishedID, deletedID}, "brigading test"); err != nil {
		t.Fatalf("flag review batch: %v", err)
	}
	for _, expected := range []struct {
		id             string
		status         string
		publishedAtNil bool
	}{
		{id: submittedID, status: feedback.StatusFlagged, publishedAtNil: true},
		{id: publishedID, status: feedback.StatusFlagged, publishedAtNil: true},
		{id: deletedID, status: "deleted"},
	} {
		review, err := store.FindByID(ctx, expected.id)
		if err != nil {
			t.Fatalf("find review %s: %v", expected.id, err)
		}
		if review.Status != expected.status {
			t.Errorf("review %s status = %q, want %q", expected.id, review.Status, expected.status)
		}
		if expected.publishedAtNil && review.PublishedAt != nil {
			t.Errorf("review %s retains publication timestamp after flagging", expected.id)
		}
		if expected.status == feedback.StatusFlagged && review.FlagReason != "brigading test" {
			t.Errorf("review %s flag reason = %q, want brigading test", expected.id, review.FlagReason)
		}
	}
}
