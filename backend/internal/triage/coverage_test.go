package triage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

type triageFailureRepository struct {
	ReviewRepository
	claimErr    error
	claimFalse  bool
	cooldown    bool
	cooldownErr error
	recentErr   error
	windowErr   error
	flagErr     error
	statusErr   error
}

func (repository *triageFailureRepository) ClaimForTriage(ctx context.Context, id string, lease time.Duration) (feedback.Feedback, bool, error) {
	if repository.claimErr != nil {
		return feedback.Feedback{}, false, repository.claimErr
	}
	if repository.claimFalse {
		return feedback.Feedback{}, false, nil
	}
	return repository.ReviewRepository.ClaimForTriage(ctx, id, lease)
}

func (repository *triageFailureRepository) ProviderCooldownActive(ctx context.Context) (bool, error) {
	if repository.cooldownErr != nil {
		return false, repository.cooldownErr
	}
	return repository.cooldown, nil
}

func (repository *triageFailureRepository) ListRecentByCourse(ctx context.Context, courseID string, limit int) ([]feedback.Feedback, error) {
	if repository.recentErr != nil {
		return nil, repository.recentErr
	}
	return repository.ReviewRepository.ListRecentByCourse(ctx, courseID, limit)
}

func (repository *triageFailureRepository) ListCourseReviewsInWindow(ctx context.Context, courseID string, since, until time.Time) ([]feedback.Feedback, error) {
	if repository.windowErr != nil {
		return nil, repository.windowErr
	}
	return repository.ReviewRepository.ListCourseReviewsInWindow(ctx, courseID, since, until)
}

func (repository *triageFailureRepository) FlagReviews(ctx context.Context, ids []string, reason string) error {
	if repository.flagErr != nil {
		return repository.flagErr
	}
	return repository.ReviewRepository.FlagReviews(ctx, ids, reason)
}

func (repository *triageFailureRepository) UpdateTriageStatus(ctx context.Context, id string, attempt int, status, reason string) error {
	if repository.statusErr != nil {
		return repository.statusErr
	}
	return repository.ReviewRepository.UpdateTriageStatus(ctx, id, attempt, status, reason)
}

func TestProcessJobSurfacesStoreAndCheckerFailures(t *testing.T) {
	storeFailure := errors.New("injected repository failure")
	tests := []struct {
		name        string
		repository  func(ReviewRepository) ReviewRepository
		provider    Provider
		checker     BrigadingChecker
		detector    DuplicateDetector
		wantErr     error
		wantErrText string
		wantClaimed bool
	}{
		{
			name: "claim failure",
			repository: func(base ReviewRepository) ReviewRepository {
				return &triageFailureRepository{ReviewRepository: base, claimErr: storeFailure}
			},
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				t.Fatal("provider called after claim failure")
				return AIResult{}, nil
			}),
			wantErr: storeFailure,
		},
		{
			name: "cooldown lookup failure",
			repository: func(base ReviewRepository) ReviewRepository {
				return &triageFailureRepository{ReviewRepository: base, cooldownErr: storeFailure}
			},
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				t.Fatal("provider called after cooldown lookup failure")
				return AIResult{}, nil
			}),
			wantErr: storeFailure, wantClaimed: true,
		},
		{
			name: "active cooldown avoids provider",
			repository: func(base ReviewRepository) ReviewRepository {
				return &triageFailureRepository{ReviewRepository: base, cooldown: true}
			},
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				t.Fatal("provider called during active cooldown")
				return AIResult{}, nil
			}),
			wantClaimed: true,
		},
		{
			name: "recent review query failure",
			repository: func(base ReviewRepository) ReviewRepository {
				return &triageFailureRepository{ReviewRepository: base, recentErr: storeFailure}
			},
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: DecisionPublish}, nil
			}),
			wantErr: storeFailure, wantClaimed: true,
		},
		{
			name: "review window query failure",
			repository: func(base ReviewRepository) ReviewRepository {
				return &triageFailureRepository{ReviewRepository: base, windowErr: storeFailure}
			},
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: DecisionPublish}, nil
			}),
			checker: brigadingFunc(func(context.Context, feedback.Feedback, []feedback.Feedback) (BrigadingCheck, error) {
				return BrigadingCheck{}, nil
			}),
			wantErr: storeFailure, wantClaimed: true,
		},
		{
			name:       "missing brigading checker",
			repository: func(base ReviewRepository) ReviewRepository { return base },
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: DecisionPublish}, nil
			}),
			wantErr: ErrBrigadingUnavailable, wantClaimed: true,
		},
		{
			name:       "brigading checker failure",
			repository: func(base ReviewRepository) ReviewRepository { return base },
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: DecisionPublish}, nil
			}),
			checker: brigadingFunc(func(context.Context, feedback.Feedback, []feedback.Feedback) (BrigadingCheck, error) {
				return BrigadingCheck{}, storeFailure
			}),
			wantErr: storeFailure, wantClaimed: true,
		},
		{
			name:       "detected activity needs a reason",
			repository: func(base ReviewRepository) ReviewRepository { return base },
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: DecisionPublish}, nil
			}),
			checker: brigadingFunc(func(context.Context, feedback.Feedback, []feedback.Feedback) (BrigadingCheck, error) {
				return BrigadingCheck{Detected: true}, nil
			}),
			wantErrText: "brigading checker returned a detection without a reason",
			wantClaimed: true,
		},
		{
			name: "brigading flag persistence failure",
			repository: func(base ReviewRepository) ReviewRepository {
				return &triageFailureRepository{ReviewRepository: base, flagErr: storeFailure}
			},
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: DecisionPublish}, nil
			}),
			checker: brigadingFunc(func(context.Context, feedback.Feedback, []feedback.Feedback) (BrigadingCheck, error) {
				return BrigadingCheck{Detected: true, Reason: "coordinated activity"}, nil
			}),
			wantErr: storeFailure, wantClaimed: true,
		},
		{
			name: "model decision persistence failure",
			repository: func(base ReviewRepository) ReviewRepository {
				return &triageFailureRepository{ReviewRepository: base, statusErr: storeFailure}
			},
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: DecisionFlag, ReasonCode: "abuse", Explanation: "targeted abuse"}, nil
			}),
			checker: brigadingFunc(func(context.Context, feedback.Feedback, []feedback.Feedback) (BrigadingCheck, error) {
				return BrigadingCheck{}, nil
			}),
			wantErr: storeFailure, wantClaimed: true,
		},
		{
			name: "duplicate decision persistence failure",
			repository: func(base ReviewRepository) ReviewRepository {
				return &triageFailureRepository{ReviewRepository: base, statusErr: storeFailure}
			},
			provider: providerFunc(func(context.Context, feedback.Feedback) (AIResult, error) {
				return AIResult{Decision: DecisionPublish}, nil
			}),
			checker: brigadingFunc(func(context.Context, feedback.Feedback, []feedback.Feedback) (BrigadingCheck, error) {
				return BrigadingCheck{}, nil
			}),
			detector: duplicateDetectorFunc(func(context.Context, feedback.Feedback, []feedback.Feedback) (DuplicateResult, error) {
				return DuplicateResult{IsDuplicate: true, Explanation: "repeated text"}, nil
			}),
			wantErr: storeFailure, wantClaimed: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			review := testReview("review-"+test.name, "CS455", "The course was useful and well structured.", time.Now())
			base := newMemoryReviewRepository(review)
			repository := test.repository(base)
			service := NewService(repository, test.provider, WithBrigadingChecker(test.checker))
			if test.detector != nil {
				service.duplicateDetector = test.detector
			}
			err := service.ProcessReview(context.Background(), review.ID)
			if test.wantErr != nil {
				if err == nil || !errors.Is(err, test.wantErr) {
					t.Fatalf("ProcessReview() error = %v, want wrapped %v", err, test.wantErr)
				}
			} else if test.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErrText) {
					t.Fatalf("ProcessReview() error = %v, want text %q", err, test.wantErrText)
				}
			} else if err != nil {
				t.Fatalf("ProcessReview() error = %v, want nil", err)
			}
			if _, claimed := base.leases[review.ID]; claimed {
				t.Fatal("triage lease remained held after processing returned")
			}
			if !test.wantClaimed {
				if base.reviews[review.ID].TriageAttempts != 0 {
					t.Fatalf("triage attempts = %d, want no claim", base.reviews[review.ID].TriageAttempts)
				}
			}
		})
	}
}

type duplicateDetectorFunc func(context.Context, feedback.Feedback, []feedback.Feedback) (DuplicateResult, error)

func (fn duplicateDetectorFunc) Detect(ctx context.Context, review feedback.Feedback, recent []feedback.Feedback) (DuplicateResult, error) {
	return fn(ctx, review, recent)
}

func TestTriageFailureCategoriesAndDevelopmentHelpers(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"provider authentication", &ProviderFailure{Status: 401}, "provider_auth"},
		{"provider authorization", &ProviderFailure{Status: 403}, "provider_auth"},
		{"malformed provider response", &ProviderFailure{Kind: "invalid response"}, "malformed_response"},
		{"network failure", &ProviderFailure{Kind: "network error"}, "transient_provider"},
		{"deadline failure", &ProviderFailure{Kind: "deadline exceeded"}, "transient_provider"},
		{"retryable HTTP response", &ProviderFailure{Status: 503, Retryable: true}, "transient_provider"},
		{"bad request", &ProviderFailure{Status: 400}, "provider_request"},
		{"other provider failure", &ProviderFailure{Kind: "other"}, "provider_error"},
		{"invalid classification", ErrInvalidDecision, "invalid_classification"},
		{"local processing", errors.New("storage failed"), "triage_processing"},
	}
	for _, test := range tests {
		if got := triageFailureCategory(test.err); got != test.want {
			t.Errorf("%s category = %q, want %q", test.name, got, test.want)
		}
	}
	for _, decision := range []string{"publish", " FLAG "} {
		if !isAllowedDecision(decision) {
			t.Errorf("isAllowedDecision(%q) = false", decision)
		}
	}
	if isAllowedDecision("defer") {
		t.Fatal("isAllowedDecision accepted unsupported decision")
	}
	if _, err := (developmentNoopBrigadingChecker{}).Check(context.Background(), feedback.Feedback{}, nil); err != nil {
		t.Fatalf("noop checker returned an unexpected result: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (developmentNoopBrigadingChecker{}).Check(canceled, feedback.Feedback{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("noop checker canceled context error = %v", err)
	}
	if _, err := NewMockProvider().Classify(canceled, feedback.Feedback{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("mock provider canceled context error = %v", err)
	}
}

func TestTriageOptionsIgnoreInvalidConfiguration(t *testing.T) {
	service := NewService(nil, nil,
		WithProcessingTimeout(0),
		WithLeaseDuration(-time.Second),
		WithDuplicateThreshold(0),
		WithDuplicateThreshold(1.1),
	)
	if service.processTimeout != DefaultProcessTimeout || service.leaseDuration != DefaultLeaseDuration || service.duplicateThreshold != DefaultDuplicateThreshold {
		t.Fatalf("invalid options changed defaults: timeout=%s lease=%s threshold=%f",
			service.processTimeout, service.leaseDuration, service.duplicateThreshold)
	}
	if _, err := service.provider.Classify(context.Background(), feedback.Feedback{}); err == nil {
		t.Fatal("nil provider fallback unexpectedly classified a review")
	}
}
