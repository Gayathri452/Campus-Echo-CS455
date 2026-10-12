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
)

type memoryStore struct {
	created feedback.Feedback
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

func TestCreateFeedback(t *testing.T) {
	store := &memoryStore{}
	authService := auth.NewService(newMemoryAuthRepository())
	session, err := authService.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(store, authService)
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
	if result.ReferenceCode != "CE-TEST123" || result.Status != feedback.StatusSubmitted {
		t.Fatalf("unexpected response: %+v", result)
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
