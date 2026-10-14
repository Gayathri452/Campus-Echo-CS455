package triage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

const (
	DefaultGeminiEmbeddingModel = "gemini-embedding-001"
	maxEmbeddingBatchSize       = 100
)

type SemanticEmbedder interface {
	Embed(context.Context, []string) ([][]float32, error)
	Model() string
}

type semanticEmbeddingStore interface {
	SaveSemanticEmbedding(context.Context, string, string, []float32) error
}

type GeminiBrigadingChecker struct {
	embedder SemanticEmbedder
	store    semanticEmbeddingStore
	now      func() time.Time
}

func NewGeminiBrigadingChecker(embedder SemanticEmbedder, store semanticEmbeddingStore) *GeminiBrigadingChecker {
	return &GeminiBrigadingChecker{
		embedder: embedder,
		store:    store,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (c *GeminiBrigadingChecker) Check(ctx context.Context, current feedback.Feedback, recent []feedback.Feedback) (BrigadingCheck, error) {
	if err := ctx.Err(); err != nil {
		return BrigadingCheck{}, err
	}
	if c.embedder == nil || c.store == nil {
		return BrigadingCheck{}, errors.New("Gemini brigading checker requires an embedder and embedding store")
	}

	now := c.now()
	cutoff := now.Add(-BrigadingWindow)
	cohort := make([]feedback.Feedback, 0, len(recent)+1)
	seen := make(map[string]struct{}, len(recent)+1)
	candidates := make([]feedback.Feedback, 0, len(recent)+1)
	candidates = append(candidates, recent...)
	candidates = append(candidates, current)
	for _, review := range candidates {
		if review.ID == "" || review.CourseID != current.CourseID ||
			review.CreatedAt.Before(cutoff) || review.CreatedAt.After(now) ||
			countReviewWords(review.Content) < BrigadingMinWordCount {
			continue
		}
		if _, exists := seen[review.ID]; exists {
			continue
		}
		seen[review.ID] = struct{}{}
		cohort = append(cohort, review)
	}
	if len(cohort) <= BrigadingReviewThreshold {
		return BrigadingCheck{}, nil
	}

	sort.Slice(cohort, func(i, j int) bool { return cohort[i].ID < cohort[j].ID })
	missing := make([]int, 0, len(cohort))
	texts := make([]string, 0, len(cohort))
	for i := range cohort {
		if validEmbedding(cohort[i].SemanticEmbedding) && cohort[i].SemanticEmbeddingModel == c.embedder.Model() {
			continue
		}
		missing = append(missing, i)
		texts = append(texts, cohort[i].Content)
	}

	for start := 0; start < len(texts); start += maxEmbeddingBatchSize {
		end := min(start+maxEmbeddingBatchSize, len(texts))
		embeddings, err := c.embedder.Embed(ctx, texts[start:end])
		if err != nil {
			return BrigadingCheck{}, fmt.Errorf("embed review cohort: %w", err)
		}
		if len(embeddings) != end-start {
			return BrigadingCheck{}, fmt.Errorf("embedding provider returned %d vectors for %d reviews", len(embeddings), end-start)
		}
		for offset, embedding := range embeddings {
			if !validEmbedding(embedding) {
				return BrigadingCheck{}, errors.New("embedding provider returned an invalid vector")
			}
			index := missing[start+offset]
			if err := c.store.SaveSemanticEmbedding(ctx, cohort[index].ID, c.embedder.Model(), embedding); err != nil {
				return BrigadingCheck{}, fmt.Errorf("persist semantic embedding: %w", err)
			}
			cohort[index].SemanticEmbedding = embedding
			cohort[index].SemanticEmbeddingModel = c.embedder.Model()
		}
	}

	average, err := averagePairwiseSimilarity(cohort)
	if err != nil {
		return BrigadingCheck{}, err
	}
	if average <= BrigadingSimilarityLimit {
		return BrigadingCheck{}, nil
	}

	ids := make([]string, 0, len(cohort))
	for _, review := range cohort {
		ids = append(ids, review.ID)
	}
	return BrigadingCheck{
		Detected:          true,
		Reason:            fmt.Sprintf("brigading: %d reviews in 10 minutes with %.1f%% average semantic similarity", len(cohort), average*100),
		ReviewIDs:         ids,
		Velocity:          len(cohort),
		AverageSimilarity: average,
	}, nil
}

func countReviewWords(text string) int {
	return len(strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}))
}

func validEmbedding(embedding []float32) bool {
	if len(embedding) == 0 {
		return false
	}
	norm := 0.0
	for _, value := range embedding {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return false
		}
		norm += float64(value) * float64(value)
	}
	return norm > 0
}

func averagePairwiseSimilarity(reviews []feedback.Feedback) (float64, error) {
	if len(reviews) < 2 {
		return 0, errors.New("at least two reviews are required to calculate semantic similarity")
	}
	total := 0.0
	pairs := 0
	for i := 0; i < len(reviews); i++ {
		for j := i + 1; j < len(reviews); j++ {
			similarity, err := vectorCosineSimilarity(reviews[i].SemanticEmbedding, reviews[j].SemanticEmbedding)
			if err != nil {
				return 0, err
			}
			total += similarity
			pairs++
		}
	}
	return total / float64(pairs), nil
}

func vectorCosineSimilarity(left, right []float32) (float64, error) {
	if !validEmbedding(left) || !validEmbedding(right) || len(left) != len(right) {
		return 0, errors.New("semantic embeddings must be valid vectors with matching dimensions")
	}
	dot, leftNorm, rightNorm := 0.0, 0.0, 0.0
	for i := range left {
		l, r := float64(left[i]), float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), nil
}
