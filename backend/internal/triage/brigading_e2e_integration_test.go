package triage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/database"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestGeminiBrigadingWorkflowWithMongoIntegration(t *testing.T) {
	// Reviews are deleted after the test. To keep them in MongoDB, set
	// GEMINI_BRIGADING_E2E_DATABASE to the database name and remove the
	// `if persistReviews { ... }` DeleteMany cleanup block below.
	if testing.Short() || strings.TrimSpace(os.Getenv("GEMINI_BRIGADING_E2E")) != "1" {
		t.Skip("set GEMINI_BRIGADING_E2E=1 to run the real Gemini and MongoDB workflow test")
	}

	mongoURI := strings.TrimSpace(os.Getenv("MONGODB_TEST_URI"))
	if mongoURI == "" {
		t.Fatal("MONGODB_TEST_URI must be set to a disposable MongoDB server")
	}
	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	if apiKey == "" {
		t.Fatal("GEMINI_API_KEY must be set for the Gemini brigading workflow test")
	}

	databaseName := strings.TrimSpace(os.Getenv("GEMINI_BRIGADING_E2E_DATABASE"))
	persistReviews := databaseName != ""
	if !persistReviews {
		databaseName = "cs455_gemini_brigading_test_" + primitive.NewObjectID().Hex()
	}
	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelConnect()

	testClient, err := mongo.Connect(connectCtx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		t.Fatalf("create MongoDB cleanup client: %v", err)
	}
	if err := testClient.Ping(connectCtx, nil); err != nil {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if disconnectErr := testClient.Disconnect(disconnectCtx); disconnectErr != nil {
			t.Fatalf("ping MongoDB test server: %v (disconnect cleanup client: %v)", err, disconnectErr)
		}
		t.Fatalf("ping MongoDB test server: %v", err)
	}

	store, err := database.Connect(connectCtx, mongoURI, databaseName)
	if err != nil {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if disconnectErr := testClient.Disconnect(disconnectCtx); disconnectErr != nil {
			t.Fatalf("connect MongoDB test store: %v (disconnect cleanup client: %v)", err, disconnectErr)
		}
		t.Fatalf("connect MongoDB test store: %v", err)
	}
	courseID := "E2E-" + primitive.NewObjectID().Hex()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if persistReviews {
			// To keep these reviews, set GEMINI_BRIGADING_E2E_DATABASE and remove this DeleteMany cleanup block.
			_, err := testClient.Database(databaseName).Collection("feedback").
				DeleteMany(cleanupCtx, bson.M{"courseId": courseID})
			if err != nil {
				t.Errorf("delete Gemini brigading test reviews for course %q: %v", courseID, err)
			}
		} else {
			if err := testClient.Database(databaseName).Drop(cleanupCtx); err != nil {
				t.Errorf("drop temporary Gemini brigading database %q: %v", databaseName, err)
			}
		}
		if err := store.Disconnect(cleanupCtx); err != nil {
			t.Errorf("disconnect MongoDB test store: %v", err)
		}
		if err := testClient.Disconnect(cleanupCtx); err != nil {
			t.Errorf("disconnect MongoDB cleanup client: %v", err)
		}
	})

	embeddingModel := strings.TrimSpace(os.Getenv("GEMINI_EMBEDDING_MODEL"))
	if embeddingModel == "" {
		embeddingModel = DefaultGeminiEmbeddingModel
	}
	embedder, err := NewGeminiSemanticEmbedder(apiKey, embeddingModel, 90*time.Second)
	if err != nil {
		t.Fatalf("initialize Gemini semantic embedder: %v", err)
	}
	checker := NewGeminiBrigadingChecker(embedder, store)
	service := NewService(
		store,
		NewDevelopmentMockProvider(),
		WithBrigadingChecker(checker),
		WithMaxRetries(0),
		WithProcessingTimeout(2*time.Minute),
	)
	reviews := []string{
		"The course content is poorly organized and difficult to follow throughout the semester.",
		"Lectures often lack proper structure, making it challenging to understand the topics being taught.",
		"The teaching approach needs improvement because concepts are not explained clearly enough.",
		"Most classes move too quickly without giving students enough time to understand important concepts.",
		"The course material is presented in a confusing manner, and the overall learning experience is disappointing.",
		"It is difficult to keep up with this course because explanations during lectures are frequently unclear.",
		"The instructor should improve the way topics are explained and organize the lectures more effectively.",
		"The syllabus is interesting, but the classroom teaching does not make the concepts easy to understand.",
		"Students would benefit from better explanations and more structured lectures throughout the course.",
		"The pace of teaching is inconsistent, and several important topics are covered without sufficient explanation.",
		"I expected a better learning experience, but the lectures have been difficult to understand.",
		"The course needs a more organized teaching strategy because the current approach makes learning unnecessarily difficult.",
		"Important concepts are rushed during lectures, leaving students confused about the subject.",
		"The overall teaching quality could be improved with clearer explanations and better lecture planning.",
		"The lectures do not provide enough clarity, and students are often left struggling with the material.",
		"The course would be much better if topics were explained systematically and at a more reasonable pace.",
	}

	reviewIDs := make([]string, 0, len(reviews))
	referenceCodes := make([]string, 0, len(reviews))
	for index, content := range reviews {
		created, err := store.Create(connectCtx, feedback.CreateInput{
			CourseID:    courseID,
			CourseTitle: "Agentic AI brigading test",
			Category:    "teaching",
			Rating:      2,
			Content:     content,
		})
		if err != nil {
			t.Fatalf("create synthetic review %d: %v", index+1, err)
		}
		reviewIDs = append(reviewIDs, created.ID)
		referenceCodes = append(referenceCodes, created.ReferenceCode)
	}

	runCtx, cancelRun := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancelRun()
	for index, reviewID := range reviewIDs {
		if err := service.ProcessReview(runCtx, reviewID); err != nil {
			t.Fatalf("triage synthetic review %d: %v", index+1, err)
		}
	}

	var failures []string
	for index, reviewID := range reviewIDs {
		review, err := store.FindByID(runCtx, reviewID)
		if err != nil {
			t.Fatalf("read synthetic review %d result: %v", index+1, err)
		}
		if review.Status != feedback.StatusFlagged || !strings.Contains(review.FlagReason, "brigading:") {
			failures = append(failures, fmt.Sprintf("review %d status=%q reason=%q", index+1, review.Status, review.FlagReason))
		}
	}
	if len(failures) > 0 {
		t.Fatalf("expected all 16 synthetic reviews to be flagged as a brigading cohort; results: %s", strings.Join(failures, "; "))
	}
	t.Logf("Gemini brigading workflow flagged all %d synthetic reviews; embedding model=%s; course=%s",
		len(reviewIDs), embeddingModel, courseID)
	if persistReviews {
		t.Logf("Test reviews will be deleted from MongoDB database %q during cleanup. Reference codes: %s",
			databaseName, strings.Join(referenceCodes, ", "))
	}
}
