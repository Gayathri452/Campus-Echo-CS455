package triage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

const (
	DefaultWorkers            = 2
	DefaultQueueSize          = 64
	DefaultRetryBackoff       = 2 * time.Second
	DefaultDuplicateThreshold = 0.82
	DefaultProcessTimeout     = 25 * time.Second
	DefaultRecoveryInterval   = 15 * time.Second
	DefaultLeaseDuration      = time.Minute
)

const (
	DecisionPublish = "publish"
	DecisionFlag    = "flag"
)

var (
	ErrQueueFull            = errors.New("triage queue is full")
	ErrServiceNotStarted    = errors.New("triage service is not started")
	ErrInvalidDecision      = errors.New("invalid triage decision")
	ErrReviewNotFound       = errors.New("review not found")
	ErrReviewNotEligible    = errors.New("review is not eligible for triage")
	ErrClassifierFailed     = errors.New("AI classifier failed")
	ErrBrigadingUnavailable = errors.New("brigading checker is unavailable")
)

type AIResult struct {
	Decision    string         `json:"decision"`
	ReasonCode  string         `json:"reasonCode,omitempty"`
	Explanation string         `json:"explanation,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type Provider interface {
	Classify(ctx context.Context, review feedback.Feedback) (AIResult, error)
}

type DuplicateResult struct {
	IsDuplicate bool    `json:"isDuplicate"`
	Similarity  float64 `json:"similarity"`
	ReasonCode  string  `json:"reasonCode,omitempty"`
	Explanation string  `json:"explanation,omitempty"`
}

type DuplicateDetector interface {
	Detect(ctx context.Context, review feedback.Feedback, previous []feedback.Feedback) (DuplicateResult, error)
}

type BrigadingCheck struct {
	Detected  bool
	Reason    string
	ReviewIDs []string
}

type BrigadingChecker interface {
	Check(ctx context.Context, review feedback.Feedback, recent []feedback.Feedback) (BrigadingCheck, error)
}

type ReviewRepository interface {
	FindByID(ctx context.Context, id string) (feedback.Feedback, error)
	ClaimForTriage(ctx context.Context, id string, lease time.Duration) (feedback.Feedback, bool, error)
	ReleaseTriage(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, id, status, reason string) error
	FlagReviews(ctx context.Context, ids []string, reason string) error
	ListRecentByCourse(ctx context.Context, courseID string, limit int) ([]feedback.Feedback, error)
	ListSubmitted(ctx context.Context, limit int) ([]feedback.Feedback, error)
}

type Dispatcher interface {
	Dispatch(context.Context, string) error
}

type Job struct {
	ReviewID string
	Attempt  int
}

type WorkerPool struct {
	jobs     chan Job
	cancel   context.CancelFunc
	fn       func(context.Context, Job) error
	backoff  time.Duration
	maxRetry int
	wg       sync.WaitGroup
	stopOnce sync.Once
	mu       sync.RWMutex
	stopped  bool
}

func NewWorkerPool(size, queueSize int, backoff time.Duration, maxRetry int, fn func(context.Context, Job) error) *WorkerPool {
	return newWorkerPool(context.Background(), size, queueSize, backoff, maxRetry, fn)
}

func newWorkerPool(parent context.Context, size, queueSize int, backoff time.Duration, maxRetry int, fn func(context.Context, Job) error) *WorkerPool {
	if size <= 0 {
		size = DefaultWorkers
	}
	if queueSize <= 0 {
		queueSize = DefaultQueueSize
	}
	if backoff <= 0 {
		backoff = DefaultRetryBackoff
	}
	if maxRetry < 0 {
		maxRetry = 0
	}
	ctx, cancel := context.WithCancel(parent)
	pool := &WorkerPool{
		jobs:     make(chan Job, queueSize),
		cancel:   cancel,
		fn:       fn,
		backoff:  backoff,
		maxRetry: maxRetry,
	}
	for i := 0; i < size; i++ {
		pool.wg.Add(1)
		go pool.worker(ctx)
	}
	return pool
}

func (p *WorkerPool) Enqueue(ctx context.Context, job Job) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.stopped {
		return ErrServiceNotStarted
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	select {
	case p.jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return ErrQueueFull
	}
}

func (p *WorkerPool) worker(ctx context.Context) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-p.jobs:
			if err := p.fn(ctx, job); err != nil {
				log.Printf("triage processing failed: review_id=%s attempt=%d error=%v", job.ReviewID, job.Attempt+1, err)
				if job.Attempt < p.maxRetry {
					retry := job
					retry.Attempt++
					if err := waitBackoff(ctx, p.backoff, retry.Attempt); err != nil {
						return
					}
					if err := p.Enqueue(ctx, retry); err != nil && !errors.Is(err, ErrQueueFull) {
						log.Printf("triage retry could not be queued: review_id=%s error=%v", job.ReviewID, err)
					} else if errors.Is(err, ErrQueueFull) {
						log.Printf("triage retry deferred to pending-review recovery: review_id=%s", job.ReviewID)
					}
				}
			}
		}
	}
}

func waitBackoff(ctx context.Context, base time.Duration, attempt int) error {
	delay := base
	for i := 1; i < attempt && delay < 30*time.Second; i++ {
		delay *= 2
	}
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *WorkerPool) Stop() {
	p.stopOnce.Do(func() {
		p.mu.Lock()
		p.stopped = true
		p.cancel()
		p.mu.Unlock()
		p.wg.Wait()
	})
}

type Service struct {
	store              ReviewRepository
	provider           Provider
	duplicateDetector  DuplicateDetector
	brigadingChecker   BrigadingChecker
	workerPool         *WorkerPool
	workers            int
	queueSize          int
	maxRetries         int
	retryBackoff       time.Duration
	processTimeout     time.Duration
	recoveryInterval   time.Duration
	leaseDuration      time.Duration
	duplicateThreshold float64
	mu                 sync.Mutex
	started            bool
	stopping           bool
	queued             map[string]struct{}
	inFlight           map[string]struct{}
	cancel             context.CancelFunc
	recoveryWG         sync.WaitGroup
	stopDone           chan struct{}
}

type Option func(*Service)

func WithWorkers(count int) Option {
	return func(service *Service) {
		if count > 0 {
			service.workers = count
		}
	}
}

func WithQueueSize(size int) Option {
	return func(service *Service) {
		if size > 0 {
			service.queueSize = size
		}
	}
}

func WithRetryBackoff(backoff time.Duration) Option {
	return func(service *Service) {
		if backoff > 0 {
			service.retryBackoff = backoff
		}
	}
}

func WithMaxRetries(count int) Option {
	return func(service *Service) {
		if count >= 0 {
			service.maxRetries = count
		}
	}
}

func WithProcessingTimeout(timeout time.Duration) Option {
	return func(service *Service) {
		if timeout > 0 {
			service.processTimeout = timeout
		}
	}
}

func WithRecoveryInterval(interval time.Duration) Option {
	return func(service *Service) {
		if interval > 0 {
			service.recoveryInterval = interval
		}
	}
}

func WithLeaseDuration(duration time.Duration) Option {
	return func(service *Service) {
		if duration > 0 {
			service.leaseDuration = duration
		}
	}
}

func WithDuplicateThreshold(threshold float64) Option {
	return func(service *Service) {
		if threshold > 0 && threshold <= 1 {
			service.duplicateThreshold = threshold
			service.duplicateDetector = NewSimilarityDuplicateDetector(threshold)
		}
	}
}

func WithBrigadingChecker(checker BrigadingChecker) Option {
	return func(service *Service) { service.brigadingChecker = checker }
}

func NewService(repo ReviewRepository, provider Provider, opts ...Option) *Service {
	if provider == nil {
		provider = NewAnthropicProvider("", "", "", DefaultProcessTimeout)
	}
	service := &Service{
		store:              repo,
		provider:           provider,
		duplicateDetector:  NewSimilarityDuplicateDetector(DefaultDuplicateThreshold),
		duplicateThreshold: DefaultDuplicateThreshold,
		workers:            DefaultWorkers,
		queueSize:          DefaultQueueSize,
		maxRetries:         3,
		retryBackoff:       DefaultRetryBackoff,
		processTimeout:     DefaultProcessTimeout,
		recoveryInterval:   DefaultRecoveryInterval,
		leaseDuration:      DefaultLeaseDuration,
		queued:             make(map[string]struct{}),
		inFlight:           make(map[string]struct{}),
		stopDone:           make(chan struct{}),
	}
	for _, option := range opts {
		option(service)
	}
	return service
}

func (s *Service) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started || s.stopping {
		s.mu.Unlock()
		return
	}
	serviceCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.workerPool = newWorkerPool(serviceCtx, s.workers, s.queueSize, s.retryBackoff, s.maxRetries, s.processJob)
	s.started = true
	s.recoveryWG.Add(1)
	s.mu.Unlock()
	go s.recoverPending(serviceCtx)
	log.Printf("triage workers started: workers=%d queue_size=%d provider=%T", s.workers, s.queueSize, s.provider)
}

func (s *Service) Stop() {
	s.mu.Lock()
	if s.stopping {
		done := s.stopDone
		s.mu.Unlock()
		<-done
		return
	}
	s.stopping = true
	cancel := s.cancel
	pool := s.workerPool
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if pool != nil {
		pool.Stop()
	}
	s.recoveryWG.Wait()
	close(s.stopDone)
}

func (s *Service) Dispatch(ctx context.Context, reviewID string) error {
	if reviewID == "" {
		return ErrReviewNotFound
	}
	s.mu.Lock()
	if !s.started || s.stopping || s.workerPool == nil {
		s.mu.Unlock()
		return ErrServiceNotStarted
	}
	if _, exists := s.queued[reviewID]; exists {
		s.mu.Unlock()
		return nil
	}
	if _, exists := s.inFlight[reviewID]; exists {
		s.mu.Unlock()
		return nil
	}
	s.queued[reviewID] = struct{}{}
	pool := s.workerPool
	s.mu.Unlock()

	if err := pool.Enqueue(ctx, Job{ReviewID: reviewID}); err != nil {
		s.mu.Lock()
		delete(s.queued, reviewID)
		s.mu.Unlock()
		return err
	}
	return nil
}

func (s *Service) ProcessReview(ctx context.Context, reviewID string) error {
	return s.processJob(ctx, Job{ReviewID: reviewID})
}

func (s *Service) recoverPending(ctx context.Context) {
	defer s.recoveryWG.Done()
	s.scanPending(ctx)
	ticker := time.NewTicker(s.recoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scanPending(ctx)
		}
	}
}

func (s *Service) scanPending(ctx context.Context) {
	reviews, err := s.store.ListSubmitted(ctx, 100)
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("triage pending-review recovery failed: %v", err)
		}
		return
	}
	for _, review := range reviews {
		if err := s.Dispatch(ctx, review.ID); err != nil {
			if ctx.Err() == nil && !errors.Is(err, ErrQueueFull) {
				log.Printf("triage pending review could not be queued: review_id=%s error=%v", review.ID, err)
			}
			return
		}
	}
}

func (s *Service) processJob(parent context.Context, job Job) error {
	if job.ReviewID == "" {
		return ErrReviewNotFound
	}
	s.mu.Lock()
	delete(s.queued, job.ReviewID)
	if _, exists := s.inFlight[job.ReviewID]; exists {
		s.mu.Unlock()
		return nil
	}
	s.inFlight[job.ReviewID] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.inFlight, job.ReviewID)
		s.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(parent, s.processTimeout)
	defer cancel()
	review, claimed, err := s.store.ClaimForTriage(ctx, job.ReviewID, s.leaseDuration)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.store.ReleaseTriage(releaseCtx, job.ReviewID); err != nil {
			log.Printf("triage lease release failed: review_id=%s error=%v", job.ReviewID, err)
		}
	}()

	result, err := s.provider.Classify(ctx, review)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrClassifierFailed, err)
	}
	result.Decision = strings.ToLower(strings.TrimSpace(result.Decision))
	if result.Decision != DecisionPublish && result.Decision != DecisionFlag {
		return fmt.Errorf("%w: provider returned an unsupported decision", ErrInvalidDecision)
	}

	if result.Decision == DecisionFlag {
		reason, err := formatFlagReason(result)
		if err != nil {
			return err
		}
		return s.store.UpdateStatus(ctx, review.ID, feedback.StatusFlagged, reason)
	}

	recent, err := s.store.ListRecentByCourse(ctx, review.CourseID, 100)
	if err != nil {
		return err
	}
	duplicate, err := s.duplicateDetector.Detect(ctx, review, recent)
	if err != nil {
		return err
	}
	if duplicate.IsDuplicate {
		return s.store.UpdateStatus(ctx, review.ID, feedback.StatusFlagged, duplicate.Explanation)
	}
	if s.brigadingChecker == nil {
		return ErrBrigadingUnavailable
	}
	check, err := s.brigadingChecker.Check(ctx, review, recent)
	if err != nil {
		return fmt.Errorf("brigading check: %w", err)
	}
	if check.Detected {
		if strings.TrimSpace(check.Reason) == "" {
			return errors.New("brigading checker returned a detection without a reason")
		}
		ids := uniqueReviewIDs(check.ReviewIDs, review.ID)
		return s.store.FlagReviews(ctx, ids, check.Reason)
	}
	return s.store.UpdateStatus(ctx, review.ID, feedback.StatusPublished, "")
}

func formatFlagReason(result AIResult) (string, error) {
	reasonCode := strings.ToLower(strings.TrimSpace(result.ReasonCode))
	switch reasonCode {
	case "abuse", "harassment", "hate_speech", "personal_attack", "other":
	default:
		return "", fmt.Errorf("%w: flagged result has no valid reason category", ErrInvalidDecision)
	}
	explanation := strings.TrimSpace(result.Explanation)
	if explanation == "" || len(explanation) > 500 {
		return "", fmt.Errorf("%w: flagged result has no concise explanation", ErrInvalidDecision)
	}
	return reasonCode + ": " + explanation, nil
}

func uniqueReviewIDs(ids []string, currentID string) []string {
	seen := make(map[string]struct{}, len(ids)+1)
	result := make([]string, 0, len(ids)+1)
	for _, id := range append(ids, currentID) {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

type developmentNoopBrigadingChecker struct{}

func NewDevelopmentNoopBrigadingChecker() BrigadingChecker {
	return developmentNoopBrigadingChecker{}
}

func (developmentNoopBrigadingChecker) Check(ctx context.Context, _ feedback.Feedback, _ []feedback.Feedback) (BrigadingCheck, error) {
	if err := ctx.Err(); err != nil {
		return BrigadingCheck{}, err
	}
	return BrigadingCheck{}, nil
}

func isAllowedDecision(decision string) bool {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case DecisionPublish, DecisionFlag:
		return true
	default:
		return false
	}
}

type developmentMockProvider struct{}

// NewDevelopmentMockProvider returns a deterministic local-only classifier. It
// is not a substitute for the configured production provider.
func NewDevelopmentMockProvider() Provider {
	return developmentMockProvider{}
}

func NewMockProvider() Provider {
	return NewDevelopmentMockProvider()
}

func (developmentMockProvider) Classify(ctx context.Context, review feedback.Feedback) (AIResult, error) {
	if err := ctx.Err(); err != nil {
		return AIResult{}, err
	}
	text := strings.ToLower(review.Content)
	abusive := []struct {
		phrase string
		code   string
	}{
		{"kill yourself", "harassment"},
		{"you are stupid", "personal_attack"},
		{"you are an idiot", "personal_attack"},
		{"i hate you", "harassment"},
	}
	for _, item := range abusive {
		if strings.Contains(text, item.phrase) {
			return AIResult{
				Decision:    DecisionFlag,
				ReasonCode:  item.code,
				Explanation: "Review contains a direct personal attack or harassment.",
			}, nil
		}
	}
	return AIResult{Decision: DecisionPublish}, nil
}

type similarityDetector struct {
	threshold float64
}

func NewSimilarityDuplicateDetector(threshold float64) DuplicateDetector {
	if threshold <= 0 || threshold > 1 {
		threshold = DefaultDuplicateThreshold
	}
	return &similarityDetector{threshold: threshold}
}

func (d *similarityDetector) Detect(ctx context.Context, review feedback.Feedback, previous []feedback.Feedback) (DuplicateResult, error) {
	for _, candidate := range previous {
		if err := ctx.Err(); err != nil {
			return DuplicateResult{}, err
		}
		if candidate.ID == review.ID || candidate.CourseID != review.CourseID {
			continue
		}
		if candidate.Status != feedback.StatusSubmitted && candidate.Status != feedback.StatusPublished {
			continue
		}
		if !isEarlierReview(candidate, review) {
			continue
		}
		similarity := textSimilarity(review.Content, candidate.Content)
		if similarity >= d.threshold {
			return DuplicateResult{
				IsDuplicate: true,
				Similarity:  similarity,
				ReasonCode:  "duplicate_spam",
				Explanation: "duplicate_spam: Review closely repeats an earlier submission for this course.",
			}, nil
		}
	}
	return DuplicateResult{ReasonCode: "not_duplicate"}, nil
}

func isEarlierReview(candidate, review feedback.Feedback) bool {
	if candidate.CreatedAt.Before(review.CreatedAt) {
		return true
	}
	return candidate.CreatedAt.Equal(review.CreatedAt) && candidate.ID < review.ID
}

func textSimilarity(left, right string) float64 {
	leftNormalized := normalizeText(left)
	rightNormalized := normalizeText(right)
	if leftNormalized == "" || rightNormalized == "" {
		return 0
	}
	if leftNormalized == rightNormalized {
		return 1
	}
	return cosineSimilarity(leftNormalized, rightNormalized)
}

func cosineSimilarity(left, right string) float64 {
	leftVec := tokenCounts(left)
	rightVec := tokenCounts(right)
	if len(leftVec) == 0 || len(rightVec) == 0 {
		return 0
	}
	keys := make(map[string]struct{}, len(leftVec)+len(rightVec))
	for key := range leftVec {
		keys[key] = struct{}{}
	}
	for key := range rightVec {
		keys[key] = struct{}{}
	}
	numerator, leftNorm, rightNorm := 0.0, 0.0, 0.0
	for key := range keys {
		leftValue, inLeft := leftVec[key]
		rightValue, inRight := rightVec[key]
		if inLeft {
			leftNorm += leftValue * leftValue
		}
		if inRight {
			rightNorm += rightValue * rightValue
		}
		if inLeft && inRight {
			numerator += leftValue * rightValue
		}
	}
	if numerator == 0 || leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return numerator / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}

func tokenCounts(text string) map[string]float64 {
	counts := make(map[string]float64)
	for _, token := range strings.Fields(text) {
		if token != "" {
			counts[token]++
		}
	}
	return counts
}

func normalizeText(text string) string {
	lower := strings.ToLower(text)
	lower = strings.Map(func(char rune) rune {
		switch {
		case char >= 'a' && char <= 'z':
			return char
		case char >= '0' && char <= '9':
			return char
		case char == ' ' || char == '\n' || char == '\t':
			return ' '
		default:
			return ' '
		}
	}, lower)
	return strings.Join(strings.Fields(lower), " ")
}

func prioritizeReviews(reviews []feedback.Feedback) []feedback.Feedback {
	sorted := append([]feedback.Feedback(nil), reviews...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CreatedAt.Equal(sorted[j].CreatedAt) {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})
	return sorted
}
