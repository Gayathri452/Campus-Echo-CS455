package triage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
	mu                            sync.Mutex
	reviews                       map[string]feedback.Feedback
	leases                        map[string]time.Time
	providerCooldownUntil         time.Time
	providerCooldownCalls         int
	providerCooldownBlockedClaims int
	clock                         func() time.Time
	recentQueryLimit              int
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
	now := repository.nowLocked()
	review, exists := repository.reviews[id]
	if !exists {
		return feedback.Feedback{}, false, feedback.ErrReviewNotFound
	}
	if now.Before(repository.providerCooldownUntil) {
		repository.providerCooldownBlockedClaims++
		return feedback.Feedback{}, false, nil
	}
	if review.Status != feedback.StatusSubmitted || now.Before(repository.leases[id]) ||
		(review.TriageRetryAt != nil && now.Before(*review.TriageRetryAt)) {
		return feedback.Feedback{}, false, nil
	}
	review.TriageAttempts++
	review.TriageCycleAttempts++
	repository.leases[id] = now.Add(lease)
	repository.reviews[id] = review
	return review, true, nil
}

func (repository *memoryReviewRepository) ProviderCooldownActive(_ context.Context) (bool, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	return repository.nowLocked().Before(repository.providerCooldownUntil), nil
}

func (repository *memoryReviewRepository) MarkTriageFailed(_ context.Context, id, reason string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	review, exists := repository.reviews[id]
	if !exists {
		return feedback.ErrReviewNotFound
	}
	if review.Status != feedback.StatusSubmitted {
		return feedback.ErrReviewNotEligible
	}
	review.Status = feedback.StatusTriageFailed
	review.ProcessingError = reason
	repository.reviews[id] = review
	delete(repository.leases, id)
	return nil
}

func (repository *memoryReviewRepository) DeferTriage(_ context.Context, id string, attempt int, category string, retryAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	review, exists := repository.reviews[id]
	if !exists {
		return feedback.ErrReviewNotFound
	}
	if review.Status != feedback.StatusSubmitted || review.TriageAttempts != attempt {
		return feedback.ErrReviewNotEligible
	}
	if repository.nowLocked().Before(repository.leases[id]) {
		return feedback.ErrReviewNotEligible
	}
	review.TriageFailureCategory = category
	review.TriageRetryAt = &retryAt
	review.TriageCycleAttempts = 0
	repository.reviews[id] = review
	delete(repository.leases, id)
	return nil
}

func (repository *memoryReviewRepository) SetProviderCooldown(_ context.Context, _ string, retryAt time.Time) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.providerCooldownCalls++
	if retryAt.After(repository.providerCooldownUntil) {
		repository.providerCooldownUntil = retryAt
	}
	return nil
}

func (repository *memoryReviewRepository) nowLocked() time.Time {
	if repository.clock != nil {
		return repository.clock()
	}
	return time.Now()
}

func (repository *memoryReviewRepository) ReleaseTriage(_ context.Context, id string) error {
	repository.mu.Lock()
	delete(repository.leases, id)
	repository.mu.Unlock()
	return nil
}

func (repository *memoryReviewRepository) ReleaseTriageAttempt(_ context.Context, id string, attempt int) error {
	repository.mu.Lock()
	if review, exists := repository.reviews[id]; exists && review.TriageAttempts == attempt {
		delete(repository.leases, id)
	}
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
	review.TriageFailureCategory = ""
	review.TriageRetryAt = nil
	repository.reviews[id] = review
	delete(repository.leases, id)
	return nil
}

func (repository *memoryReviewRepository) UpdateTriageStatus(_ context.Context, id string, attempt int, status, reason string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	review, exists := repository.reviews[id]
	if !exists {
		return feedback.ErrReviewNotFound
	}
	if review.Status != feedback.StatusSubmitted || review.TriageAttempts != attempt ||
		!repository.nowLocked().Before(repository.leases[id]) {
		return feedback.ErrReviewNotEligible
	}
	review.Status = status
	review.FlagReason = reason
	review.TriageFailureCategory = ""
	review.TriageRetryAt = nil
	repository.reviews[id] = review
	delete(repository.leases, id)
	return nil
}

func (repository *memoryReviewRepository) FlagReviews(_ context.Context, ids []string, reason string) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	for _, id := range ids {
		review, exists := repository.reviews[id]
		if exists && (review.Status == feedback.StatusSubmitted || review.Status == feedback.StatusPublished) {
			review.Status = feedback.StatusFlagged
			review.FlagReason = reason
			review.PublishedAt = nil
			repository.reviews[id] = review
			delete(repository.leases, id)
		}
	}
	return nil
}

func (repository *memoryReviewRepository) ListRecentByCourse(_ context.Context, courseID string, limit int) ([]feedback.Feedback, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	repository.recentQueryLimit = limit
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

func (repository *memoryReviewRepository) ListCourseReviewsInWindow(_ context.Context, courseID string, since, until time.Time) ([]feedback.Feedback, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	results := make([]feedback.Feedback, 0, len(repository.reviews))
	for _, review := range repository.reviews {
		if review.CourseID == courseID && !review.CreatedAt.Before(since) && !review.CreatedAt.After(until) {
			results = append(results, review)
		}
	}
	return prioritizeReviews(results), nil
}

func (repository *memoryReviewRepository) SaveSemanticEmbedding(_ context.Context, id, model string, embedding []float32) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	review, exists := repository.reviews[id]
	if !exists {
		return feedback.ErrReviewNotFound
	}
	review.SemanticEmbedding = append([]float32(nil), embedding...)
	review.SemanticEmbeddingModel = model
	repository.reviews[id] = review
	return nil
}

func (repository *memoryReviewRepository) ListSubmitted(_ context.Context, limit int) ([]feedback.Feedback, error) {
	repository.mu.Lock()
	defer repository.mu.Unlock()
	results := make([]feedback.Feedback, 0)
	now := repository.nowLocked()
	if now.Before(repository.providerCooldownUntil) {
		return results, nil
	}
	for id, review := range repository.reviews {
		if review.Status == feedback.StatusSubmitted && !now.Before(repository.leases[id]) &&
			(review.TriageRetryAt == nil || !now.Before(*review.TriageRetryAt)) {
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

func TestJobFailureUnwrapsProviderFailure(t *testing.T) {
	providerFailure := &ProviderFailure{Status: 401, Kind: "authentication", Message: "invalid credentials"}
	jobFailure := &JobFailure{
		Attempt:   1,
		Retryable: false,
		Err:       fmt.Errorf("%w: %w", ErrClassifierFailed, providerFailure),
	}

	var recovered *ProviderFailure
	if !errors.As(jobFailure, &recovered) {
		t.Fatal("errors.As did not recover ProviderFailure through JobFailure")
	}
	if recovered != providerFailure {
		t.Fatalf("recovered provider failure = %p, want original %p", recovered, providerFailure)
	}
	if !errors.Is(jobFailure, ErrClassifierFailed) {
		t.Fatal("errors.Is did not recover ErrClassifierFailed through JobFailure")
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

func TestRetriesPersistMonotonicAttemptCountAndEventuallyPublish(t *testing.T) {
	review := testReview("review-1", "CS455", "The exam was unfair and too difficult.", time.Now())
	repository := newMemoryReviewRepository(review)
	var attempts []int
	var attemptsMu sync.Mutex
	provider := providerFunc(func(_ context.Context, review feedback.Feedback) (AIResult, error) {
		attemptsMu.Lock()
		attempts = append(attempts, review.TriageAttempts)
		call := len(attempts)
		attemptsMu.Unlock()
		if call < 3 {
			return AIResult{}, errors.New("temporary provider failure")
		}
		return AIResult{Decision: DecisionPublish}, nil
	})
	service := serviceForTest(repository, provider, WithRetryBackoff(time.Millisecond), WithMaxRetries(2))
	service.Start(context.Background())
	defer service.Stop()
	if err := service.Dispatch(context.Background(), review.ID); err != nil {
		t.Fatalf("dispatch review: %v", err)
	}
	waitForStatus(t, repository, review.ID, feedback.StatusPublished)

	attemptsMu.Lock()
	defer attemptsMu.Unlock()
	if fmt.Sprint(attempts) != "[1 2 3]" {
		t.Fatalf("provider attempts = %v, want [1 2 3]", attempts)
	}
	stored, err := repository.FindByID(context.Background(), review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TriageAttempts != 3 {
		t.Fatalf("persisted attempts = %d, want 3", stored.TriageAttempts)
	}
}

func TestRetryExhaustionPersistsFailureWithoutPublishing(t *testing.T) {
	review := testReview("review-1", "CS455", "Review that cannot be classified.", time.Now())
	repository := newMemoryReviewRepository(review)
	var calls atomic.Int32
	service := serviceForTest(repository, providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
		calls.Add(1)
		return AIResult{}, errors.New("temporary provider failure")
	}), WithRetryBackoff(time.Millisecond), WithMaxRetries(2))
	service.Start(context.Background())
	defer service.Stop()
	if err := service.Dispatch(context.Background(), review.ID); err != nil {
		t.Fatalf("dispatch review: %v", err)
	}
	waitForDeferredCategory(t, repository, review.ID, "triage_processing")
	stored, err := repository.FindByID(context.Background(), review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != feedback.StatusSubmitted || stored.TriageAttempts != 3 ||
		stored.TriageCycleAttempts != 0 || stored.TriageRetryAt == nil || calls.Load() != 3 {
		t.Fatalf("exhausted review state = %+v, provider calls = %d", stored, calls.Load())
	}
}

func TestBackoffIsExponentialCappedAndJittered(t *testing.T) {
	tests := []struct {
		attempt int
		lower   time.Duration
		upper   time.Duration
	}{
		{attempt: 1, lower: 50 * time.Millisecond, upper: 100 * time.Millisecond},
		{attempt: 2, lower: 100 * time.Millisecond, upper: 200 * time.Millisecond},
		{attempt: 3, lower: 200 * time.Millisecond, upper: 400 * time.Millisecond},
		{attempt: 20, lower: 15 * time.Second, upper: 30 * time.Second},
	}
	for _, test := range tests {
		delay := backoffDelay(100*time.Millisecond, test.attempt)
		if delay < test.lower || delay > test.upper {
			t.Errorf("backoff attempt %d = %s, want [%s, %s]", test.attempt, delay, test.lower, test.upper)
		}
	}
}

func TestConcurrentServicesCannotClaimReviewTwice(t *testing.T) {
	review := testReview("review-1", "CS455", "One review.", time.Now())
	repository := newMemoryReviewRepository(review)
	var calls atomic.Int32
	provider := providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
		calls.Add(1)
		time.Sleep(10 * time.Millisecond)
		return AIResult{Decision: DecisionPublish}, nil
	})
	first := serviceForTest(repository, provider)
	second := serviceForTest(repository, provider)
	var done sync.WaitGroup
	done.Add(2)
	go func() {
		defer done.Done()
		_ = first.ProcessReview(context.Background(), review.ID)
	}()
	go func() {
		defer done.Done()
		_ = second.ProcessReview(context.Background(), review.ID)
	}()
	done.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("classifier calls = %d, want one atomic lease claim", got)
	}
	assertReviewState(t, repository, review.ID, feedback.StatusPublished, "")
}

func TestExpiredWorkerCannotReleaseOrOverwriteNewerClaim(t *testing.T) {
	review := testReview("review-stale-worker", "CS455", "One review.", time.Now())
	repository := newMemoryReviewRepository(review)
	stale, claimed, err := repository.ClaimForTriage(context.Background(), review.ID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("first claim = (%t, %v)", claimed, err)
	}

	repository.mu.Lock()
	repository.leases[review.ID] = time.Now().Add(-time.Second)
	repository.mu.Unlock()
	current, claimed, err := repository.ClaimForTriage(context.Background(), review.ID, time.Minute)
	if err != nil || !claimed || current.TriageAttempts <= stale.TriageAttempts {
		t.Fatalf("replacement claim = (%+v, %t, %v)", current, claimed, err)
	}
	if err := repository.ReleaseTriageAttempt(context.Background(), review.ID, stale.TriageAttempts); err != nil {
		t.Fatalf("stale worker release: %v", err)
	}
	if _, claimed, err := repository.ClaimForTriage(context.Background(), review.ID, time.Minute); err != nil || claimed {
		t.Fatalf("claim after stale release = (%t, %v), want blocked by current lease", claimed, err)
	}
	if err := repository.UpdateTriageStatus(context.Background(), review.ID, current.TriageAttempts, feedback.StatusFlagged, "current result"); err != nil {
		t.Fatalf("update from current worker: %v", err)
	}
	if err := repository.UpdateTriageStatus(context.Background(), review.ID, stale.TriageAttempts, feedback.StatusPublished, ""); !errors.Is(err, feedback.ErrReviewNotEligible) {
		t.Fatalf("stale worker update error = %v, want not eligible", err)
	}
	stored, err := repository.FindByID(context.Background(), review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != feedback.StatusFlagged || stored.FlagReason != "current result" {
		t.Fatalf("stale worker overwrote current result: %+v", stored)
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
