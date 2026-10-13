package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"github.com/Kishore200524/CS455-Project/backend/internal/triage"
)

type memoryStore struct {
	created feedback.Feedback
	appeal  *feedback.Appeal
}

type captureTriageDispatcher struct {
	reviewID string
	err      error
}

func (dispatcher *captureTriageDispatcher) Dispatch(_ context.Context, reviewID string) error {
	dispatcher.reviewID = reviewID
	return dispatcher.err
}

type memoryAuthRepository struct {
	users         map[string]auth.User
	passwords     map[string]string
	sessions      map[string]string
	sessionExpiry map[string]time.Time
	otps          map[string]auth.OTPChallenge
}

func newMemoryAuthRepository() *memoryAuthRepository {
	return &memoryAuthRepository{
		users:         make(map[string]auth.User),
		passwords:     make(map[string]string),
		sessions:      make(map[string]string),
		sessionExpiry: make(map[string]time.Time),
		otps:          make(map[string]auth.OTPChallenge),
	}
}

func (m *memoryAuthRepository) CreateUser(_ context.Context, email, passwordHash, role string) (auth.User, error) {
	if _, exists := m.users[email]; exists {
		return auth.User{}, auth.ErrEmailAlreadyExists
	}
	user := auth.User{ID: email, Email: email, Role: role, CreatedAt: time.Now().UTC()}
	m.users[email] = user
	m.passwords[email] = passwordHash
	return user, nil
}

func (m *memoryAuthRepository) FindUserByEmail(_ context.Context, email string) (auth.User, string, error) {
	user, exists := m.users[email]
	if !exists {
		return auth.User{}, "", auth.ErrInvalidCredentials
	}
	return user, m.passwords[email], nil
}

func (m *memoryAuthRepository) CreateSession(_ context.Context, userID, tokenHash string, expiresAt time.Time) error {
	m.sessions[tokenHash] = userID
	m.sessionExpiry[tokenHash] = expiresAt
	return nil
}

func (m *memoryAuthRepository) FindUserBySession(_ context.Context, tokenHash string, now time.Time) (auth.User, error) {
	userID, exists := m.sessions[tokenHash]
	if !exists || !now.Before(m.sessionExpiry[tokenHash]) {
		return auth.User{}, auth.ErrInvalidSession
	}
	user, exists := m.users[userID]
	if !exists {
		return auth.User{}, auth.ErrInvalidSession
	}
	return user, nil
}

func (m *memoryAuthRepository) DeleteSession(_ context.Context, tokenHash string) error {
	if _, exists := m.sessions[tokenHash]; !exists {
		return errors.New("session not found")
	}
	delete(m.sessions, tokenHash)
	delete(m.sessionExpiry, tokenHash)
	return nil
}

func (m *memoryStore) Create(_ context.Context, input feedback.CreateInput) (feedback.Feedback, error) {
	m.created = feedback.Feedback{
		ID:            "feedback-id",
		ReferenceCode: "CE-TEST123",
		CourseID:      input.CourseID,
		CourseTitle:   input.CourseTitle,
		Category:      input.Category,
		Rating:        input.Rating,
		Content:       input.Content,
		Status:        feedback.StatusSubmitted,
	}
	return m.created, nil
}

func (m *memoryStore) Explore(
	context.Context,
	feedback.ExploreFilters,
) ([]feedback.Feedback, error) {
	return []feedback.Feedback{}, nil
}

func (m *memoryStore) CourseRatings(
	context.Context,
	string,
) ([]feedback.CourseRating, error) {
	return []feedback.CourseRating{}, nil
}

func (m *memoryStore) FindByReference(
	_ context.Context,
	reference string,
) (feedback.Feedback, error) {
	if m.created.ReferenceCode != reference {
		return feedback.Feedback{}, feedback.ErrReviewNotFound
	}

	result := m.created
	if m.appeal != nil {
		result.AppealStatus = m.appeal.Status
	}

	return result, nil
}

func (m *memoryStore) CreateAppeal(
	_ context.Context,
	reference, reason string,
) (feedback.Appeal, error) {
	if m.created.ReferenceCode != reference {
		return feedback.Appeal{}, feedback.ErrReviewNotFound
	}

	if m.created.Status != feedback.StatusFlagged &&
		m.created.Status != feedback.StatusRejected {
		return feedback.Appeal{}, feedback.ErrAppealNotEligible
	}

	if m.appeal != nil {
		return feedback.Appeal{}, feedback.ErrAppealAlreadyExists
	}

	appeal := feedback.Appeal{
		ReferenceCode: reference,
		Reason:        reason,
		Status:        feedback.StatusUnderAppeal,
		CreatedAt:     time.Now().UTC(),
	}

	m.appeal = &appeal
	return appeal, nil
}

func TestCreateFeedback(t *testing.T) {
	store := &memoryStore{}
	authService := auth.NewService(newMemoryAuthRepository())
	session, err := authService.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := &captureTriageDispatcher{}
	handler := NewServer(store, authService, WithTriageDispatcher(dispatcher))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(
		`{"courseId":" CS455 ","courseTitle":" Software Engineering ","category":"teaching","rating":4,"content":" A useful example "}`,
	))
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	if store.created.CourseID != "CS455" || store.created.CourseTitle != "Software Engineering" || store.created.Content != "A useful example" {
		t.Fatalf("stored feedback was not normalized: %+v", store.created)
	}

	var result feedback.Feedback
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ReferenceCode != "CE-TEST123" || result.Status != feedback.StatusSubmitted || result.FlagReason != "" {
		t.Fatalf("unexpected response: %+v", result)
	}
	if dispatcher.reviewID != store.created.ID {
		t.Fatalf("dispatched review ID = %q, want %q", dispatcher.reviewID, store.created.ID)
	}
}

func TestCreateFeedbackAcknowledgesWhenTriageQueueIsUnavailable(t *testing.T) {
	store := &memoryStore{}
	authService := auth.NewService(newMemoryAuthRepository())
	session, err := authService.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(store, authService, WithTriageDispatcher(&captureTriageDispatcher{err: triage.ErrQueueFull}))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(
		`{"courseId":"CS455","courseTitle":"Software Engineering","category":"teaching","rating":4,"content":"Useful examples."}`,
	))
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusCreated, response.Body.String())
	}
	var result feedback.Feedback
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ReferenceCode != store.created.ReferenceCode || result.Status != feedback.StatusSubmitted {
		t.Fatalf("saved review was not acknowledged: %+v", result)
	}
}

func TestReviewStatusReturnsSavedFlagReason(t *testing.T) {
	store := &memoryStore{created: feedback.Feedback{
		ReferenceCode: "CE-TEST123",
		CourseID:      "CS455",
		CourseTitle:   "Software Engineering",
		Content:       "Review body",
		Status:        feedback.StatusFlagged,
		FlagReason:    "harassment: targeted abuse",
	}}
	authService := auth.NewService(newMemoryAuthRepository())
	session, err := authService.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(store, authService)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/feedback/status/CE-TEST123", nil)
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result feedback.Feedback
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Status != feedback.StatusFlagged || result.FlagReason != store.created.FlagReason {
		t.Fatalf("status response = (%s, %q)", result.Status, result.FlagReason)
	}
}

func TestReviewStatusIsReadOnlyAndReturnsTriageFailure(t *testing.T) {
	store := &memoryStore{created: feedback.Feedback{
		ID:              "feedback-id",
		ReferenceCode:   "CE-FAILED1",
		CourseID:        "CS455",
		CourseTitle:     "Software Engineering",
		Content:         "Review body",
		Status:          feedback.StatusTriageFailed,
		ProcessingError: feedback.ProcessingFailureMessage,
	}}
	authService := auth.NewService(newMemoryAuthRepository())
	session, err := authService.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	dispatcher := &captureTriageDispatcher{}
	handler := NewServer(store, authService, WithTriageDispatcher(dispatcher))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/feedback/status/CE-FAILED1", nil)
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result feedback.Feedback
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Status != feedback.StatusTriageFailed || result.ProcessingError != feedback.ProcessingFailureMessage {
		t.Fatalf("failure status response = (%s, %q)", result.Status, result.ProcessingError)
	}
	if dispatcher.reviewID != "" {
		t.Fatalf("status polling dispatched review %q; GET must be read-only", dispatcher.reviewID)
	}
}

func TestFeedbackAppealAppearsInStatusAndCanOnlyBeSubmittedOnce(t *testing.T) {
	store := &memoryStore{}
	authService := auth.NewService(newMemoryAuthRepository())
	session, err := authService.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(store, authService)

	submit := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(
		`{"courseId":"CS455","courseTitle":"Software Engineering","category":"teaching","rating":4,"content":"Helpful examples"}`,
	))
	submit.Header.Set("Authorization", "Bearer "+session.AccessToken)
	submitResponse := httptest.NewRecorder()
	handler.ServeHTTP(submitResponse, submit)
	if submitResponse.Code != http.StatusCreated {
		t.Fatalf("submission status = %d, want %d; body: %s", submitResponse.Code, http.StatusCreated, submitResponse.Body.String())
	}
	if store.created.Status != feedback.StatusSubmitted {
		t.Fatalf("new review status = %q, want submitted before asynchronous triage", store.created.Status)
	}
	store.created.Status = feedback.StatusFlagged
	store.created.FlagReason = feedback.FlagReasonReview

	appealRequest := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/feedback/status/CE-TEST123/appeal", strings.NewReader(`{"reason":"Please reconsider this review."}`))
		request.Header.Set("Authorization", "Bearer "+session.AccessToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	firstAppeal := appealRequest()
	if firstAppeal.Code != http.StatusCreated {
		t.Fatalf("first appeal status = %d, want %d; body: %s", firstAppeal.Code, http.StatusCreated, firstAppeal.Body.String())
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "/api/v1/feedback/status/CE-TEST123", nil)
	statusRequest.Header.Set("Authorization", "Bearer "+session.AccessToken)
	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, statusRequest)
	var status feedback.Feedback
	if err := json.NewDecoder(statusResponse.Body).Decode(&status); err != nil {
		t.Fatalf("decode review status: %v", err)
	}
	if statusResponse.Code != http.StatusOK || status.Status != feedback.StatusFlagged || status.FlagReason != feedback.FlagReasonReview || status.AppealStatus != feedback.StatusUnderAppeal {
		t.Fatalf("unexpected review status response (%d): %+v", statusResponse.Code, status)
	}

	secondAppeal := appealRequest()
	if secondAppeal.Code != http.StatusConflict {
		t.Fatalf("second appeal status = %d, want %d; body: %s", secondAppeal.Code, http.StatusConflict, secondAppeal.Body.String())
	}
}

func TestCreateFeedbackRejectsInvalidInput(t *testing.T) {
	store := &memoryStore{}
	authService := auth.NewService(newMemoryAuthRepository())
	session, err := authService.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(store, authService)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(
		`{"courseId":"CS455","courseTitle":"Software Engineering","category":"teaching","rating":4,"content":" "}`,
	))
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	if store.created.ID != "" {
		t.Fatal("invalid feedback must not be stored")
	}
}
