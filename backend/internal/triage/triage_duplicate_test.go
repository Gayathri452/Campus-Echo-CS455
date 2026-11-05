package triage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

func TestDuplicateDetectionIsCourseAndChronologyScoped(t *testing.T) {
	now := time.Now().UTC()
	old := testReview("older-review", "CS455", "The weekly lectures and examples were very helpful for understanding the material.", now.Add(-time.Minute))
	current := testReview("newer-review", "CS455", "The weekly lectures and examples were really helpful for understanding the material.", now)
	otherCourse := current
	otherCourse.CourseID = "CS101"
	detector := NewSimilarityDuplicateDetector(0.82)

	result, err := detector.Detect(context.Background(), current, []feedback.Feedback{old})
	if err != nil || !result.IsDuplicate || result.Similarity < 0.82 {
		t.Fatalf("same-course near-duplicate = (%+v, %v), want similarity >= 0.82", result, err)
	}
	result, err = detector.Detect(context.Background(), otherCourse, []feedback.Feedback{old})
	if err != nil || result.IsDuplicate {
		t.Fatalf("cross-course duplicate = (%+v, %v), want no duplicate", result, err)
	}
	result, err = detector.Detect(context.Background(), old, []feedback.Feedback{current})
	if err != nil || result.IsDuplicate {
		t.Fatalf("older review classified as duplicate = (%+v, %v), want no duplicate", result, err)
	}
}

func TestDuplicateThresholdBoundary(t *testing.T) {
	now := time.Now().UTC()
	older := testReview("a", "CS455", "Repeated useful course feedback helps students understand the class", now.Add(-time.Minute))
	newer := testReview("b", "CS455", "Repeated useful course feedback helps students understand the class", now)
	detector := NewSimilarityDuplicateDetector(1)
	result, err := detector.Detect(context.Background(), newer, []feedback.Feedback{older})
	if err != nil || !result.IsDuplicate || result.Similarity < 1 {
		t.Fatalf("similarity exactly at threshold = (%+v, %v), want duplicate", result, err)
	}
}

func TestDuplicateSimilarityThresholdAroundPoint82(t *testing.T) {
	now := time.Now().UTC()
	detector := NewSimilarityDuplicateDetector(0.82)
	shared16 := makeSharedTokens(16)
	belowOlder := testReview("below-a", "CS455", shared16+"olderone oldertwo olderthree olderfour", now.Add(-time.Minute))
	belowNewer := testReview("below-b", "CS455", shared16+"newerone newertwo newerthree newerfour", now)
	below, err := detector.Detect(context.Background(), belowNewer, []feedback.Feedback{belowOlder})
	if err != nil {
		t.Fatalf("below-threshold Detect() error = %v", err)
	}
	if below.IsDuplicate || below.Similarity >= 0.82 {
		t.Fatalf("below-threshold result = %+v, want similarity < 0.82 and no duplicate", below)
	}

	shared18 := makeSharedTokens(18)
	aboveOlder := testReview("above-a", "CS455", shared18+"olderone oldertwo olderthree", now.Add(-time.Minute))
	aboveNewer := testReview("above-b", "CS455", shared18+"newerone newertwo newerthree", now)
	above, err := detector.Detect(context.Background(), aboveNewer, []feedback.Feedback{aboveOlder})
	if err != nil {
		t.Fatalf("above-threshold Detect() error = %v", err)
	}
	if !above.IsDuplicate || above.Similarity < 0.82 {
		t.Fatalf("above-threshold result = %+v, want similarity >= 0.82 and duplicate", above)
	}
}

func makeSharedTokens(count int) string {
	tokens := make([]string, count)
	for i := range tokens {
		tokens[i] = fmt.Sprintf("shared%02d", i)
	}
	return strings.Join(tokens, " ") + " "
}

func TestDuplicateDetectionSupportsUnicodeAndSkipsShortReviews(t *testing.T) {
	now := time.Now().UTC()
	detector := NewSimilarityDuplicateDetector(DefaultDuplicateThreshold)

	t.Run("Hindi review is compared", func(t *testing.T) {
		older := testReview(
			"a",
			"CS455",
			"प्रोफेसर विषय को स्पष्ट तरीके से समझाते हैं और उदाहरण उपयोगी थे",
			now.Add(-time.Minute),
		)
		newer := testReview(
			"b",
			"CS455",
			"प्रोफेसर विषय को स्पष्ट तरीके से समझाते हैं, और उदाहरण उपयोगी थे!",
			now,
		)
		if got := normalizeText(newer.Content); !strings.Contains(got, "प्रोफेसर") {
			t.Fatalf("normalized Hindi text = %q, want Unicode letters preserved", got)
		}

		result, err := detector.Detect(context.Background(), newer, []feedback.Feedback{older})
		if err != nil || !result.IsDuplicate {
			t.Fatalf("Hindi duplicate = (%+v, %v), want duplicate", result, err)
		}
	})

	t.Run("identical two-word reviews are too short to compare", func(t *testing.T) {
		older := testReview("a", "CS455", "Good course", now.Add(-time.Minute))
		newer := testReview("b", "CS455", "Good course", now)

		result, err := detector.Detect(context.Background(), newer, []feedback.Feedback{older})
		if err != nil || result.IsDuplicate {
			t.Fatalf("short review duplicate = (%+v, %v), want no duplicate", result, err)
		}
	})

	t.Run("short candidate does not match a longer review", func(t *testing.T) {
		older := testReview("a", "CS455", "Good course", now.Add(-time.Minute))
		newer := testReview(
			"b",
			"CS455",
			"This is a good course with clear explanations and useful examples",
			now,
		)

		result, err := detector.Detect(context.Background(), newer, []feedback.Feedback{older})
		if err != nil || result.IsDuplicate {
			t.Fatalf("long review against short candidate = (%+v, %v), want no duplicate", result, err)
		}
	})
}

func TestDuplicateCheckLoadsOnlyLatestHistoryLimit(t *testing.T) {
	review := testReview("review-1", "CS455", "This review contains enough words for comparison", time.Now())
	repository := newMemoryReviewRepository(review)
	service := serviceForTest(
		repository,
		providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
			return AIResult{Decision: DecisionPublish}, nil
		}),
	)
	if err := service.ProcessReview(context.Background(), review.ID); err != nil {
		t.Fatalf("process review: %v", err)
	}

	repository.mu.Lock()
	limit := repository.recentQueryLimit
	repository.mu.Unlock()
	if limit != DuplicateHistoryLimit {
		t.Fatalf("duplicate history query limit = %d, want %d", limit, DuplicateHistoryLimit)
	}
}

func TestConcurrentSimilarReviewsKeepOlderReviewAndFlagNewer(t *testing.T) {
	now := time.Now().UTC()
	older := testReview("review-a", "CS455", "The examples were useful and clear.", now)
	newer := testReview("review-b", "CS455", "The examples were useful and clear!", now.Add(time.Second))
	repository := newMemoryReviewRepository(older, newer)
	service := serviceForTest(repository, providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
		return AIResult{Decision: DecisionPublish}, nil
	}), WithDuplicateThreshold(0.8))

	var done sync.WaitGroup
	for _, review := range []feedback.Feedback{older, newer} {
		done.Add(1)
		go func(id string) {
			defer done.Done()
			if err := service.ProcessReview(context.Background(), id); err != nil {
				t.Errorf("process %s: %v", id, err)
			}
		}(review.ID)
	}
	done.Wait()
	assertReviewState(t, repository, older.ID, feedback.StatusPublished, "")
	assertReviewState(t, repository, newer.ID, feedback.StatusFlagged, "duplicate_spam: Review closely repeats an earlier submission for this course.")
}
