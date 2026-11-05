package triage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"google.golang.org/genai"
)

func TestPermanentGeminiFailureDefersReviewsAndAppliesProviderCooldown(t *testing.T) {
	const secret = "AIza0123456789012345678901234567890123"
	reviews := []feedback.Feedback{
		testReview("review-1", "CS455", "First review text.", time.Now()),
		testReview("review-2", "CS101", "Second review text.", time.Now()),
		reviewWithContent("review-3", "CS201", "Third review text."),
	}
	repository := newMemoryReviewRepository(reviews...)
	var calls int
	var mu sync.Mutex
	client := &fakeGeminiClient{
		generate: func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return nil, genai.APIError{Code: 401, Message: "invalid credentials " + secret}
		},
	}
	provider := newGeminiProviderWithClient(client, "test-model", time.Second)
	provider.apiKey = secret
	service := serviceForTest(
		repository,
		provider,
		WithWorkers(1),
		WithRecoveryInterval(5*time.Millisecond),
		WithProviderCooldown(100*time.Millisecond),
		WithRecoveryCooldown(30*time.Millisecond),
	)
	service.Start(context.Background())
	defer service.Stop()

	waitForDeferredCategory(t, repository, "review-1", "provider_auth")
	time.Sleep(40 * time.Millisecond)
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 1 {
		t.Fatalf("Gemini calls during provider cooldown = %d, want 1", gotCalls)
	}
	for i, review := range reviews {
		stored, err := repository.FindByID(context.Background(), review.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status != feedback.StatusSubmitted {
			t.Fatalf("deferred review state = %+v", stored)
		}
		if i == 0 && (stored.TriageFailureCategory != "provider_auth" ||
			stored.TriageRetryAt == nil || stored.TriageCycleAttempts != 0) {
			t.Fatalf("failed review deferral metadata = %+v", stored)
		}
		if i > 0 && (stored.TriageFailureCategory != "" || stored.TriageRetryAt != nil) {
			t.Fatalf("untouched review unexpectedly received per-review deferral metadata = %+v", stored)
		}
		if stored.TriageFailureCategory == secret || stored.ProcessingError == secret {
			t.Fatal("provider credentials were persisted")
		}
	}
	if active, err := repository.ProviderCooldownActive(context.Background()); err != nil || !active {
		t.Fatalf("provider cooldown active = (%t, %v), want true", active, err)
	}
	if _, claimed, err := repository.ClaimForTriage(context.Background(), "review-2", time.Minute); err != nil || claimed {
		t.Fatalf("claim another submitted review during provider cooldown = (%t, %v), want blocked", claimed, err)
	}
}

func TestProviderCooldownBlocksNewSubmissionsUntilSchedulerResumes(t *testing.T) {
	initial := testReview("review-initial", "CS455", "Initial review.", time.Now())
	existing := testReview("review-existing", "CS101", "Already submitted review.", time.Now().Add(time.Second))
	repository := newMemoryReviewRepository(initial)
	var clockNanos atomic.Int64
	clockNanos.Store(time.Now().UnixNano())
	repository.clock = func() time.Time {
		return time.Unix(0, clockNanos.Load())
	}
	var calls atomic.Int32
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	client := &fakeGeminiClient{
		generate: func(ctx context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			if calls.Add(1) == 1 {
				close(providerStarted)
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-releaseProvider:
				}
				return nil, genai.APIError{Code: 401, Message: "invalid credentials"}
			}
			return geminiTextResponse(`{"decision":"publish","reasonCode":"acceptable","explanation":"Permissible review."}`), nil
		},
	}
	service := serviceForTest(
		repository,
		newGeminiProviderWithClient(client, "test-model", time.Second),
		WithWorkers(1),
		WithRecoveryInterval(time.Hour),
		WithProviderCooldown(time.Hour),
	)
	service.Start(context.Background())
	defer service.Stop()
	select {
	case <-providerStarted:
	case <-time.After(time.Second):
		t.Fatal("startup recovery did not send the first review to Gemini")
	}

	repository.mu.Lock()
	repository.reviews[existing.ID] = existing
	repository.mu.Unlock()
	close(releaseProvider)
	waitForDeferredCategory(t, repository, initial.ID, "provider_auth")
	waitForProviderCooldownActivation(t, repository)
	firstAttempt, err := repository.FindByID(context.Background(), initial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstAttempt.TriageAttempts != 1 || firstAttempt.TriageFailureCategory != "provider_auth" {
		t.Fatalf("first review after HTTP 401 = %+v, want one attempt and provider_auth deferral", firstAttempt)
	}
	newReview := testReview("review-during-cooldown", "CS201", "New review.", time.Now())
	repository.mu.Lock()
	repository.reviews[newReview.ID] = newReview
	repository.mu.Unlock()
	for _, review := range []feedback.Feedback{existing, newReview} {
		if err := service.ProcessReview(context.Background(), review.ID); err != nil {
			t.Fatalf("process %s during provider cooldown: %v", review.ID, err)
		}
	}
	repository.mu.Lock()
	blockedClaims := repository.providerCooldownBlockedClaims
	cooldownActivations := repository.providerCooldownCalls
	repository.mu.Unlock()
	if blockedClaims != 2 {
		t.Fatalf("claims blocked by provider cooldown = %d, want 2", blockedClaims)
	}
	if cooldownActivations != 1 {
		t.Fatalf("provider cooldown activations = %d, want exactly 1", cooldownActivations)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("Gemini calls while provider cooldown is active = %d, want 1", got)
	}
	for _, review := range []feedback.Feedback{initial, existing, newReview} {
		stored, err := repository.FindByID(context.Background(), review.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status != feedback.StatusSubmitted {
			t.Fatalf("review %s status during cooldown = %q, want submitted", review.ID, stored.Status)
		}
	}

	repository.mu.Lock()
	cooldownUntil := repository.providerCooldownUntil
	repository.mu.Unlock()
	clockNanos.Store(cooldownUntil.Add(time.Nanosecond).UnixNano())
	service.scanPending(context.Background())
	for _, review := range []feedback.Feedback{initial, existing, newReview} {
		waitForStatus(t, repository, review.ID, feedback.StatusPublished)
	}
	if got := calls.Load(); got != 4 {
		t.Fatalf("Gemini calls after cooldown recovery = %d, want 4 (one 401 and three recoveries)", got)
	}
}

func waitForProviderCooldownActivation(t *testing.T, repository *memoryReviewRepository) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		repository.mu.Lock()
		calls := repository.providerCooldownCalls
		repository.mu.Unlock()
		if calls > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("triage service did not activate provider cooldown")
}

func TestTransientGeminiFailureExhaustionDefersWithNewBoundedCycle(t *testing.T) {
	review := testReview("review-1", "CS455", "Review text.", time.Now())
	repository := newMemoryReviewRepository(review)
	var calls int
	var mu sync.Mutex
	client := &fakeGeminiClient{
		generate: func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return nil, genai.APIError{Code: 504, Message: "gateway timeout"}
		},
	}
	service := serviceForTest(
		repository,
		newGeminiProviderWithClient(client, "test-model", time.Second),
		WithWorkers(1),
		WithMaxRetries(1),
		WithRetryBackoff(time.Millisecond),
		WithRecoveryInterval(5*time.Millisecond),
		WithRecoveryCooldown(100*time.Millisecond),
	)
	service.Start(context.Background())
	defer service.Stop()

	waitForDeferredCategory(t, repository, review.ID, "transient_provider")
	time.Sleep(35 * time.Millisecond)
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 2 {
		t.Fatalf("Gemini calls in exhausted cycle = %d, want 2", gotCalls)
	}
	stored, err := repository.FindByID(context.Background(), review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != feedback.StatusSubmitted || stored.TriageAttempts != 2 || stored.TriageCycleAttempts != 0 ||
		stored.TriageRetryAt == nil {
		t.Fatalf("exhausted cycle state = %+v", stored)
	}
}

func TestExhaustedRecoveryCyclesRemainBoundedByCooldown(t *testing.T) {
	review := testReview("review-1", "CS455", "Review text.", time.Now())
	repository := newMemoryReviewRepository(review)
	var calls atomic.Int32
	client := &fakeGeminiClient{
		generate: func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			calls.Add(1)
			return nil, genai.APIError{Code: 504, Message: "gateway timeout"}
		},
	}
	service := serviceForTest(
		repository,
		newGeminiProviderWithClient(client, "test-model", time.Second),
		WithWorkers(1),
		WithMaxRetries(1),
		WithRetryBackoff(time.Millisecond),
		WithRecoveryInterval(5*time.Millisecond),
		WithRecoveryCooldown(60*time.Millisecond),
	)
	service.Start(context.Background())
	defer service.Stop()

	waitForProviderCalls(t, &calls, 4)
	time.Sleep(20 * time.Millisecond)
	stored, err := repository.FindByID(context.Background(), review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 4 {
		t.Fatalf("Gemini calls across two bounded cycles = %d, want 4", got)
	}
	if stored.Status != feedback.StatusSubmitted || stored.TriageAttempts != 4 ||
		stored.TriageCycleAttempts != 0 || stored.TriageRetryAt == nil {
		t.Fatalf("second exhausted cycle state = %+v", stored)
	}
}

func TestRecoveryDoesNotExceedCycleBudgetAfterWorkerCrash(t *testing.T) {
	review := testReview("review-1", "CS455", "Review text.", time.Now())
	review.TriageAttempts = 2
	review.TriageCycleAttempts = 2
	repository := newMemoryReviewRepository(review)
	var calls atomic.Int32
	service := serviceForTest(
		repository,
		providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
			calls.Add(1)
			return AIResult{Decision: DecisionPublish}, nil
		}),
		WithWorkers(1),
		WithMaxRetries(1),
		WithRecoveryInterval(5*time.Millisecond),
		WithRecoveryCooldown(100*time.Millisecond),
	)
	service.Start(context.Background())
	defer service.Stop()

	waitForDeferredCategory(t, repository, review.ID, "triage_processing")
	if got := calls.Load(); got != 0 {
		t.Fatalf("provider calls after exhausted cycle recovery = %d, want 0", got)
	}
	stored, err := repository.FindByID(context.Background(), review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TriageAttempts != 2 || stored.TriageCycleAttempts != 0 || stored.TriageRetryAt == nil {
		t.Fatalf("recovered exhausted cycle state = %+v", stored)
	}
}

func TestMalformedGeminiResponseExhaustionDefersWithoutPublishing(t *testing.T) {
	review := testReview("review-1", "CS455", "Review text.", time.Now())
	repository := newMemoryReviewRepository(review)
	var calls int
	var mu sync.Mutex
	client := &fakeGeminiClient{
		generate: func(context.Context, string, []*genai.Content, *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			return geminiTextResponse(`{"decision":"publish","reasonCode":"acceptable","explanation":""}`), nil
		},
	}
	service := serviceForTest(
		repository,
		newGeminiProviderWithClient(client, "test-model", time.Second),
		WithWorkers(1),
		WithMaxRetries(1),
		WithRetryBackoff(time.Millisecond),
		WithRecoveryInterval(5*time.Millisecond),
		WithRecoveryCooldown(100*time.Millisecond),
	)
	service.Start(context.Background())
	defer service.Stop()

	waitForDeferredCategory(t, repository, review.ID, "malformed_response")
	time.Sleep(25 * time.Millisecond)
	stored, err := repository.FindByID(context.Background(), review.ID)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 2 || stored.Status != feedback.StatusSubmitted ||
		stored.TriageRetryAt == nil || stored.Status == feedback.StatusPublished {
		t.Fatalf("malformed-response state = %+v, calls = %d", stored, gotCalls)
	}
}

func TestDeferredReviewRecoversAfterRestartAndClearsRetryMetadata(t *testing.T) {
	review := testReview("review-1", "CS455", "Review text.", time.Now())
	repository := newMemoryReviewRepository(review)
	authClient := &fakeGeminiClient{
		err: genai.APIError{Code: 401, Message: "invalid credentials"},
	}
	firstService := serviceForTest(
		repository,
		newGeminiProviderWithClient(authClient, "test-model", time.Second),
		WithWorkers(1),
		WithRecoveryInterval(5*time.Millisecond),
		WithProviderCooldown(30*time.Millisecond),
	)
	firstService.Start(context.Background())
	if err := firstService.Dispatch(context.Background(), review.ID); err != nil {
		t.Fatalf("dispatch review: %v", err)
	}
	waitForDeferredCategory(t, repository, review.ID, "provider_auth")
	firstService.Stop()

	time.Sleep(35 * time.Millisecond)
	successClient := &fakeGeminiClient{
		response: geminiTextResponse(`{"decision":"publish","reasonCode":"acceptable","explanation":"Permissible review."}`),
	}
	secondService := serviceForTest(
		repository,
		newGeminiProviderWithClient(successClient, "test-model", time.Second),
		WithRecoveryInterval(5*time.Millisecond),
	)
	secondService.Start(context.Background())
	defer secondService.Stop()

	waitForStatus(t, repository, review.ID, feedback.StatusPublished)
	stored, err := repository.FindByID(context.Background(), review.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TriageAttempts != 2 || stored.TriageCycleAttempts != 1 ||
		stored.TriageRetryAt != nil || stored.TriageFailureCategory != "" {
		t.Fatalf("recovered review metadata = %+v", stored)
	}
}

func TestMultipleRecoveryScansDoNotDuplicateInflightProcessing(t *testing.T) {
	review := testReview("review-1", "CS455", "Review text.", time.Now())
	repository := newMemoryReviewRepository(review)
	started := make(chan struct{})
	release := make(chan struct{})
	var calls int
	var mu sync.Mutex
	client := &fakeGeminiClient{
		generate: func(ctx context.Context, _ string, _ []*genai.Content, _ *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
			mu.Lock()
			calls++
			mu.Unlock()
			close(started)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return geminiTextResponse(`{"decision":"publish","reasonCode":"acceptable","explanation":"Permissible review."}`), nil
			}
		},
	}
	service := serviceForTest(
		repository,
		newGeminiProviderWithClient(client, "test-model", time.Second),
		WithWorkers(1),
		WithRecoveryInterval(5*time.Millisecond),
	)
	service.Start(context.Background())
	defer service.Stop()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial recovery did not start classification")
	}
	time.Sleep(40 * time.Millisecond)
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 1 {
		t.Fatalf("Gemini calls during active processing = %d, want 1", gotCalls)
	}
	close(release)
	waitForStatus(t, repository, review.ID, feedback.StatusPublished)
}

func TestActiveLeaseExcludesReviewFromRecovery(t *testing.T) {
	review := testReview("review-1", "CS455", "Review text.", time.Now())
	repository := newMemoryReviewRepository(review)
	if _, claimed, err := repository.ClaimForTriage(context.Background(), review.ID, time.Minute); err != nil || !claimed {
		t.Fatalf("pre-claim review = claimed %t, error %v", claimed, err)
	}
	var calls atomic.Int32
	service := serviceForTest(
		repository,
		providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
			calls.Add(1)
			return AIResult{Decision: DecisionPublish}, nil
		}),
		WithRecoveryInterval(5*time.Millisecond),
	)
	service.Start(context.Background())
	defer service.Stop()
	time.Sleep(35 * time.Millisecond)
	if got := calls.Load(); got != 0 {
		t.Fatalf("provider calls while review has an active lease = %d, want 0", got)
	}
}

func TestQueueSaturationLeavesReviewForRecovery(t *testing.T) {
	repository := newMemoryReviewRepository()
	started := make(chan struct{})
	release := make(chan struct{})
	var firstCall sync.Once
	service := serviceForTest(
		repository,
		providerFunc(func(ctx context.Context, review feedback.Feedback) (AIResult, error) {
			firstCall.Do(func() {
				close(started)
				select {
				case <-ctx.Done():
				case <-release:
				}
			})
			return AIResult{Decision: DecisionPublish}, nil
		}),
		WithWorkers(1),
		WithQueueSize(1),
		WithRecoveryInterval(5*time.Millisecond),
	)
	service.Start(context.Background())
	defer service.Stop()
	reviews := []feedback.Feedback{
		testReview("review-1", "CS455", "First distinct text.", time.Now()),
		testReview("review-2", "CS101", "Second distinct text.", time.Now()),
		reviewWithContent("review-3", "CS201", "Third distinct text."),
	}
	repository.mu.Lock()
	for _, review := range reviews {
		repository.reviews[review.ID] = review
	}
	repository.mu.Unlock()
	if err := service.Dispatch(context.Background(), reviews[0].ID); err != nil {
		t.Fatalf("dispatch first review: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first review did not start")
	}
	if err := service.Dispatch(context.Background(), reviews[1].ID); err != nil {
		t.Fatalf("dispatch second review: %v", err)
	}
	if err := service.Dispatch(context.Background(), reviews[2].ID); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("dispatch third review error = %v, want queue-full", err)
	}
	close(release)
	for _, review := range reviews {
		waitForStatus(t, repository, review.ID, feedback.StatusPublished)
	}
}

func reviewWithContent(id, courseID, content string) feedback.Feedback {
	return testReview(id, courseID, content, time.Now())
}

func waitForDeferredCategory(t *testing.T, repository *memoryReviewRepository, id, category string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		review, err := repository.FindByID(context.Background(), id)
		if err == nil && review.Status == feedback.StatusSubmitted &&
			review.TriageFailureCategory == category && review.TriageRetryAt != nil {
			return
		}

		time.Sleep(time.Millisecond)
	}
	review, err := repository.FindByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("review was not deferred as %q: %+v", category, review)
}

func waitForProviderCalls(t *testing.T, calls *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if calls.Load() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("Gemini calls = %d, want at least %d", calls.Load(), want)
}
