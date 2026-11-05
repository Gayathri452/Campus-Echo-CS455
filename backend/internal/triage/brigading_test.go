package triage

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

type semanticEmbedderFunc func(context.Context, []string) ([][]float32, error)

func (fn semanticEmbedderFunc) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return fn(ctx, texts)
}

func (semanticEmbedderFunc) Model() string { return DefaultGeminiEmbeddingModel }

func TestBrigadingDetectionFlagsAllReturnedReviewIDs(t *testing.T) {
	now := time.Now().UTC()
	current := testReview("current", "CS455", "Current review", now)
	related := testReview("related", "CS455", "Related review", now.Add(-time.Minute))
	repository := newMemoryReviewRepository(current, related)
	service := NewService(repository, providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
		return AIResult{Decision: DecisionPublish}, nil
	}), WithBrigadingChecker(brigadingFunc(func(context.Context, feedback.Feedback, []feedback.Feedback) (BrigadingCheck, error) {
		return BrigadingCheck{Detected: true, Reason: "coordinated activity", ReviewIDs: []string{related.ID}}, nil
	})))

	if err := service.ProcessReview(context.Background(), current.ID); err != nil {
		t.Fatalf("process review: %v", err)
	}
	assertReviewState(t, repository, current.ID, feedback.StatusFlagged, "coordinated activity")
	assertReviewState(t, repository, related.ID, feedback.StatusFlagged, "coordinated activity")
}

func TestGeminiBrigadingCheckerRequirementsAndBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		count      int
		oldCount   int
		shortCount int
		vectorFor  func(int) []float32
		want       bool
	}{
		{
			name:      "V=30 and S=30 percent stays organic",
			count:     30,
			vectorFor: func(i int) []float32 { return equicorrelatedEmbedding(30, 0.30, i) },
		},
		{
			name:      "V below 15 and S=95 percent flags no brigading batch",
			count:     14,
			vectorFor: func(i int) []float32 { return equicorrelatedEmbedding(14, 0.95, i) },
		},
		{
			name:      "V=15 does not exceed the strict velocity threshold",
			count:     15,
			vectorFor: func(i int) []float32 { return equicorrelatedEmbedding(15, 0.95, i) },
		},
		{
			name:      "V=20 and S=85 percent triggers brigading",
			count:     20,
			vectorFor: func(i int) []float32 { return equicorrelatedEmbedding(20, 0.85, i) },
			want:      true,
		},
		{
			name:      "16 highly similar reviews exceed both thresholds",
			count:     16,
			vectorFor: func(int) []float32 { return []float32{1, 0} },
			want:      true,
		},
		{
			name:      "reviews outside the ten minute window do not count",
			count:     14,
			oldCount:  2,
			vectorFor: func(int) []float32 { return []float32{1, 0} },
		},
		{
			name:       "five word reviews do not count toward velocity",
			count:      16,
			shortCount: 1,
			vectorFor:  func(int) []float32 { return []float32{1, 0} },
		},
		{
			name:  "similarity exactly at 75 percent does not trigger",
			count: 16,
			vectorFor: func(i int) []float32 {
				vector := make([]float32, 17)
				vector[0] = float32(math.Sqrt(0.75))
				vector[i+1] = 0.5
				return vector
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reviews := make([]feedback.Feedback, 0, test.count+test.oldCount)
			for i := 0; i < test.count+test.oldCount; i++ {
				content := "The workload and lectures were consistently very challenging."
				if i < test.shortCount {
					content = "Too much weekly work"
				}
				created := now.Add(-time.Duration(i) * time.Second)
				if i >= test.count {
					created = now.Add(-BrigadingWindow - time.Second)
				}
				reviews = append(reviews, feedback.Feedback{
					ID: fmt.Sprintf("review-%02d", i), CourseID: "CS455", Content: content,
					Status: feedback.StatusPublished, CreatedAt: created,
					SemanticEmbedding: test.vectorFor(i % test.count), SemanticEmbeddingModel: DefaultGeminiEmbeddingModel,
				})
			}
			repository := newMemoryReviewRepository(reviews...)
			current := reviews[0]
			checker := NewGeminiBrigadingChecker(
				semanticEmbedderFunc(func(context.Context, []string) ([][]float32, error) {
					t.Fatal("cached embeddings should not call the external provider")
					return nil, nil
				}),
				repository,
			)
			checker.now = func() time.Time { return now }

			result, err := checker.Check(context.Background(), current, reviews[1:])
			if err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if result.Detected != test.want {
				t.Fatalf("Detected = %v, want %v (velocity=%d similarity=%.4f)", result.Detected, test.want, result.Velocity, result.AverageSimilarity)
			}
			if test.want && (result.Velocity != test.count || len(result.ReviewIDs) != test.count || result.AverageSimilarity <= BrigadingSimilarityLimit) {
				t.Fatalf("brigading result = %+v, want %d reviews and similarity > 0.75", result, test.count)
			}
		})
	}
}

func TestBrigadingCheckerDeduplicatesAndFiltersIneligibleReviews(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	reviews := make([]feedback.Feedback, 16)
	for i := range reviews {
		reviews[i] = feedback.Feedback{
			ID: fmt.Sprintf("eligible-%02d", i), CourseID: "CS455",
			Content: "The workload and lectures were consistently very challenging.",
			Status:  feedback.StatusPublished, CreatedAt: now.Add(-time.Duration(i) * time.Second),
			SemanticEmbedding: []float32{1, 0}, SemanticEmbeddingModel: DefaultGeminiEmbeddingModel,
		}
	}
	duplicate := reviews[0]
	otherCourse := reviews[1]
	otherCourse.ID = "other-course"
	otherCourse.CourseID = "CS101"
	outsideWindow := reviews[2]
	outsideWindow.ID = "outside-window"
	outsideWindow.CreatedAt = now.Add(-BrigadingWindow - time.Second)
	futureReview := reviews[3]
	futureReview.ID = "future-review"
	futureReview.CreatedAt = now.Add(time.Second)
	recent := append(append([]feedback.Feedback(nil), reviews[1:]...), duplicate, otherCourse, outsideWindow, futureReview)

	repository := newMemoryReviewRepository(reviews...)
	checker := NewGeminiBrigadingChecker(
		semanticEmbedderFunc(func(context.Context, []string) ([][]float32, error) {
			t.Fatal("cached embeddings should not call the external provider")
			return nil, nil
		}),
		repository,
	)
	checker.now = func() time.Time { return now }

	result, err := checker.Check(context.Background(), reviews[0], recent)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !result.Detected || result.Velocity != 16 || len(result.ReviewIDs) != 16 {
		t.Fatalf("brigading result = %+v, want exactly 16 unique in-scope reviews", result)
	}
}

func TestBrigadingCheckerRejectsInvalidSimilarityVectors(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	reviews := make([]feedback.Feedback, 16)
	for i := range reviews {
		reviews[i] = feedback.Feedback{
			ID: fmt.Sprintf("review-%02d", i), CourseID: "CS455",
			Content: "The workload and lectures were consistently very challenging.",
			Status:  feedback.StatusPublished, CreatedAt: now.Add(-time.Duration(i) * time.Second),
			SemanticEmbedding: []float32{1, 0}, SemanticEmbeddingModel: DefaultGeminiEmbeddingModel,
		}
	}
	reviews[15].SemanticEmbedding = []float32{1, 0, 0}
	repository := newMemoryReviewRepository(reviews...)
	checker := NewGeminiBrigadingChecker(
		semanticEmbedderFunc(func(context.Context, []string) ([][]float32, error) {
			t.Fatal("all supplied vectors are cached and valid individually")
			return nil, nil
		}),
		repository,
	)
	checker.now = func() time.Time { return now }

	if _, err := checker.Check(context.Background(), reviews[0], reviews[1:]); err == nil {
		t.Fatal("Check() succeeded with mismatched vector dimensions, want an error")
	}
}

func equicorrelatedEmbedding(size int, similarity float64, index int) []float32 {
	vector := make([]float32, size+1)
	vector[0] = float32(math.Sqrt(similarity))
	vector[index+1] = float32(math.Sqrt(1 - similarity))
	return vector
}

func TestBrigadingIsEvaluatedBeforeDuplicateMicroFlagging(t *testing.T) {
	now := time.Now().UTC()
	reviews := make([]feedback.Feedback, 16)
	for i := range reviews {
		reviews[i] = testReview(fmt.Sprintf("review-%02d", i), "CS455", "The lectures and assignments were helpful every single week.", now.Add(-time.Duration(i)*time.Second))
		reviews[i].Status = feedback.StatusPublished
		reviews[i].SemanticEmbedding = []float32{1, 0}
		reviews[i].SemanticEmbeddingModel = DefaultGeminiEmbeddingModel
	}
	reviews[0].Status = feedback.StatusSubmitted
	repository := newMemoryReviewRepository(reviews...)
	checker := NewGeminiBrigadingChecker(
		semanticEmbedderFunc(func(context.Context, []string) ([][]float32, error) {
			t.Fatal("cached embeddings should not call the external provider")
			return nil, nil
		}),
		repository,
	)
	service := NewService(repository, providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
		return AIResult{Decision: DecisionPublish}, nil
	}), WithBrigadingChecker(checker))

	if err := service.ProcessReview(context.Background(), reviews[0].ID); err != nil {
		t.Fatalf("process review: %v", err)
	}
	updated, err := repository.FindByID(context.Background(), reviews[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != feedback.StatusFlagged || !strings.Contains(updated.FlagReason, "brigading:") {
		t.Fatalf("review state = (%s, %q), want brigading flag", updated.Status, updated.FlagReason)
	}
	for _, review := range reviews[1:] {
		updated, err := repository.FindByID(context.Background(), review.ID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.Status != feedback.StatusFlagged {
			t.Errorf("review %s status = %s, want flagged", review.ID, updated.Status)
		}
	}
}
