package triage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

func BenchmarkDuplicateDetectionLatest200(b *testing.B) {
	now := time.Now().UTC()
	current := testReview(
		"current",
		"CS455",
		"Current submission discusses quantum mechanics and laboratory measurements",
		now,
	)
	previous := make([]feedback.Feedback, DuplicateHistoryLimit)
	for i := range previous {
		previous[i] = testReview(
			fmt.Sprintf("previous-%03d", i),
			"CS455",
			fmt.Sprintf("Earlier submission describes unrelated topic number%d with several extra details", i),
			now.Add(-time.Duration(i+1)*time.Second),
		)
	}
	detector := NewSimilarityDuplicateDetector(DefaultDuplicateThreshold)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := detector.Detect(context.Background(), current, previous); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBrigadingPairwiseSimilarity(b *testing.B) {
	for _, cohortSize := range []int{16, 50, 100, 250} {
		b.Run(fmt.Sprintf("reviews_%d", cohortSize), func(b *testing.B) {
			reviews := make([]feedback.Feedback, cohortSize)
			for i := range reviews {
				reviews[i].SemanticEmbedding = []float32{0.6, 0.8}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := averagePairwiseSimilarity(reviews); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
