package triage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

type providerFunc func(context.Context, feedback.Feedback) (AIResult, error)

func (fn providerFunc) Classify(ctx context.Context, review feedback.Feedback) (AIResult, error) {
	return fn(ctx, review)
}

type brigadingFunc func(context.Context, feedback.Feedback, []feedback.Feedback) (BrigadingCheck, error)

func (fn brigadingFunc) Check(ctx context.Context, review feedback.Feedback, recent []feedback.Feedback) (BrigadingCheck, error) {
	return fn(ctx, review, recent)
}

type memoryReviewRepository struct {
	mu      sync.Mutex
	reviews map[string]feedback.Feedback
	leases  map[string]time.Time
}

func newMemoryReviewRepository(reviews ...feedback.Feedback) *memoryReviewRepository {
	repository := &memoryReviewRepository{
		reviews: make(map[string]feedback.Feedback, len(reviews)),
		leases:  make(map[string]time.Time),
	}
	for _, review := range reviews {
		repository.reviews[review.ID] = review
	}
	return repository
}

func (repository *memoryReviewRepository) FindByID(_ context.Context, id string) (feedback.Feedback, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	review, exists := repository.reviews[id]
	if !exists {
		return feedback.Feedback{}, feedback.ErrReviewNotFound
	}
	return review, nil
}

func (repository *memoryReviewRepository) ClaimForTriage(_ context.Context, id string, lease time.Duration) (feedback.Feedback, bool, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	review, exists := repository.reviews[id]
	if !exists {
		return feedback.Feedback{}, false, feedback.ErrReviewNotFound
	}
	if review.Status != feedback.StatusSubmitted || time.Now().Before(repository.leases[id]) {
		return feedback.Feedback{}, false, nil
	}
	repository.leases[id] = time.Now().Add(lease)
	return review, true, nil
}

func (repository *memoryReviewRepository) ReleaseTriage(_ context.Context, id string) error {
	repository.mu.Lock()
	delete(repository.leases, id)
	repository.mu.Unlock()
	return nil
}

func (repository *memoryReviewRepository) UpdateStatus(_ context.Context, id, status, reason string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	review, exists := repository.reviews[id]
	if !exists {
		return feedback.ErrReviewNotFound
	}
	if review.Status != feedback.StatusSubmitted {
		return feedback.ErrReviewNotEligible
	}
	review.Status = status
	review.FlagReason = reason
	repository.reviews[id] = review
	delete(repository.leases, id)
	return nil
}

func (repository *memoryReviewRepository) FlagReviews(_ context.Context, ids []string, reason string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, id := range ids {
		review, exists := repository.reviews[id]
		if exists && review.Status == feedback.StatusSubmitted {
			review.Status = feedback.StatusFlagged
			review.FlagReason = reason
			repository.reviews[id] = review
			delete(repository.leases, id)
		}
	}
	return nil
}

func (repository *memoryReviewRepository) ListRecentByCourse(_ context.Context, courseID string, limit int) ([]feedback.Feedback, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	results := make([]feedback.Feedback, 0, len(repository.reviews))
	for _, review := range repository.reviews {
		if review.CourseID == courseID {
			results = append(results, review)
		}
	}
	results = prioritizeReviews(results)
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (repository *memoryReviewRepository) ListSubmitted(_ context.Context, limit int) ([]feedback.Feedback, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	results := make([]feedback.Feedback, 0)
	now := time.Now()
	for id, review := range repository.reviews {
		if review.Status == feedback.StatusSubmitted && !now.Before(repository.leases[id]) {
			results = append(results, review)
		}
	}
	results = prioritizeReviews(results)
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func testReview(id, courseID, content string, createdAt time.Time) feedback.Feedback {
	return feedback.Feedback{
		ID:        id,
		CourseID:  courseID,
		Content:   content,
		Status:    feedback.StatusSubmitted,
		CreatedAt: createdAt,
	}
}

func serviceForTest(repository ReviewRepository, provider Provider, options ...Option) *Service {
	options = append(options, WithBrigadingChecker(NewDevelopmentNoopBrigadingChecker()))
	return NewService(repository, provider, options...)
}

func TestSuccessfulClassificationPublishesAcceptableReview(t *testing.T) {
	review := testReview("review-1", "CS455", "The course was challenging, but the lectures were useful.", time.Now())
	repository := newMemoryReviewRepository(review)
	service := serviceForTest(repository, providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
		return AIResult{Decision: DecisionPublish}, nil
	}))

	if err := service.ProcessReview(context.Background(), review.ID); err != nil {
		t.Fatalf("process review: %v", err)
	}
	assertReviewState(t, repository, review.ID, feedback.StatusPublished, "")
}

func TestAbusiveReviewIsFlaggedWithReason(t *testing.T) {
	review := testReview("review-1", "CS455", "You are an idiot.", time.Now())
	repository := newMemoryReviewRepository(review)
	service := serviceForTest(repository, NewDevelopmentMockProvider())

	if err := service.ProcessReview(context.Background(), review.ID); err != nil {
		t.Fatalf("process review: %v", err)
	}
	assertReviewState(t, repository, review.ID, feedback.StatusFlagged, "personal_attack: Review contains a direct personal attack or harassment.")
}

func TestHarshLegitimateCriticismIsNotFlaggedByDevelopmentProvider(t *testing.T) {
	review := testReview("review-1", "CS455", "This course is badly organized and the assignments are unfair.", time.Now())
	repository := newMemoryReviewRepository(review)
	service := serviceForTest(repository, NewDevelopmentMockProvider())

	if err := service.ProcessReview(context.Background(), review.ID); err != nil {
		t.Fatalf("process review: %v", err)
	}
	assertReviewState(t, repository, review.ID, feedback.StatusPublished, "")
}

func TestProviderFailuresAndMalformedDecisionsStaySubmitted(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
	}{
		{
			name: "provider error",
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{}, errors.New("provider unavailable")
			}),
		},
		{
			name: "unsupported decision",
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: "maybe"}, nil
			}),
		},
		{
			name: "flag missing reason",
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: DecisionFlag}, nil
			}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			review := testReview("review-1", "CS455", "Review body", time.Now())
			repository := newMemoryReviewRepository(review)
			service := serviceForTest(repository, test.provider)

			if err := service.ProcessReview(context.Background(), review.ID); err == nil {
				t.Fatal("ProcessReview() succeeded, want error")
			}
			assertReviewState(t, repository, review.ID, feedback.StatusSubmitted, "")
		})
	}
}

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
	older := testReview("a", "CS455", "Repeated useful course feedback", now.Add(-time.Minute))
	newer := testReview("b", "CS455", "Repeated useful course feedback", now)
	detector := NewSimilarityDuplicateDetector(1)
	result, err := detector.Detect(context.Background(), newer, []feedback.Feedback{older})
	if err != nil || !result.IsDuplicate || result.Similarity < 1 {
		t.Fatalf("similarity exactly at threshold = (%+v, %v), want duplicate", result, err)
	}
}

func TestAsyncDispatchDoesNotWaitForSlowProviderAndRetries(t *testing.T) {
	review := testReview("review-1", "CS455", "Useful course review", time.Now())
	repository := newMemoryReviewRepository(review)
	var calls int
	var callsMu sync.Mutex
	provider := providerFunc(func(ctx context.Context, _ feedback.Feedback) (AIResult, error) {
		callsMu.Lock()
		calls++
		call := calls
		callsMu.Unlock()
		if call == 1 {
			return AIResult{}, errors.New("temporary failure")
		}
		select {
		case <-ctx.Done():
			return AIResult{}, ctx.Err()
		case <-time.After(150 * time.Millisecond):
			return AIResult{Decision: DecisionPublish}, nil
		}
	})
	service := serviceForTest(repository, provider, WithRetryBackoff(5*time.Millisecond), WithMaxRetries(2))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.Start(ctx)
	defer service.Stop()

	start := time.Now()
	if err := service.Dispatch(context.Background(), review.ID); err != nil {
		t.Fatalf("dispatch review: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("Dispatch waited %s for background processing", elapsed)
	}
	waitForStatus(t, repository, review.ID, feedback.StatusPublished)
	callsMu.Lock()
	defer callsMu.Unlock()
	if calls != 2 {
		t.Fatalf("provider calls = %d, want retry call count 2", calls)
	}
}

func TestStartupRecoveryProcessesPersistedSubmittedReview(t *testing.T) {
	review := testReview("review-1", "CS455", "Pending after restart", time.Now())
	repository := newMemoryReviewRepository(review)
	service := serviceForTest(repository, providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
		return AIResult{Decision: DecisionPublish}, nil
	}), WithRecoveryInterval(10*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service.Start(ctx)
	defer service.Stop()

	waitForStatus(t, repository, review.ID, feedback.StatusPublished)
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

func TestWorkerQueueSaturationReturnsError(t *testing.T) {
	started := make(chan struct{})
	var startedOnce sync.Once
	pool := NewWorkerPool(1, 1, time.Millisecond, 0, func(ctx context.Context, _ Job) error {
		startedOnce.Do(func() { close(started) })
		<-ctx.Done()
		return ctx.Err()
	})
	defer pool.Stop()
	if err := pool.Enqueue(context.Background(), Job{ReviewID: "running"}); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := pool.Enqueue(context.Background(), Job{ReviewID: "queued"}); err != nil {
		t.Fatal(err)
	}
	if err := pool.Enqueue(context.Background(), Job{ReviewID: "full"}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("queue saturation error = %v, want ErrQueueFull", err)
	}
}

func TestQueueSizeOptionIsAppliedAndStopIsIdempotent(t *testing.T) {
	service := NewService(
		newMemoryReviewRepository(),
		providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
			return AIResult{Decision: DecisionPublish}, nil
		}),
		WithQueueSize(3),
	)
	if service.queueSize != 3 {
		t.Fatalf("queue size = %d, want 3", service.queueSize)
	}
	service.Start(context.Background())
	service.Stop()
	service.Stop()
}

func assertReviewState(t *testing.T, repository *memoryReviewRepository, id, status, reason string) {
	t.Helper()
	review, err := repository.FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("find review %s: %v", id, err)
	}
	if review.Status != status || review.FlagReason != reason {
		t.Fatalf("review %s state = (%s, %q), want (%s, %q)", id, review.Status, review.FlagReason, status, reason)
	}
}

func waitForStatus(t *testing.T, repository *memoryReviewRepository, id, status string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		review, err := repository.FindByID(context.Background(), id)
		if err == nil && review.Status == status {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	review, err := repository.FindByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("review status = %s, want %s: %s", review.Status, status, fmt.Sprint(review.FlagReason))
}
