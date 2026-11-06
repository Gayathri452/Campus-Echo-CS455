package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

// nilExploreStore returns a nil slice from Explore, which is what the MongoDB
// store used to return when no published review matched.
type nilExploreStore struct {
	memoryStore
}

func (s *nilExploreStore) Explore(context.Context, feedback.ExploreFilters) ([]feedback.Feedback, error) {
	return nil, nil
}

func TestExploreFeedbackReturnsEmptyListWhenNothingMatches(t *testing.T) {
	store := &nilExploreStore{}
	authService := auth.NewService(newMemoryAuthRepository())
	session, err := authService.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(store, authService)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/feedback/explore?search=nothing-matches", nil)
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if body := strings.TrimSpace(response.Body.String()); body != "[]" {
		t.Fatalf("body = %q, want [] (an empty list, not null)", body)
	}
}
