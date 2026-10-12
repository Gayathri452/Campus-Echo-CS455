package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
)

type resolveTestStore struct {
	resolveAction  string
	resolveComment string
	resolveCalls   int
	releaseCalls   int
}

func (s *resolveTestStore) ListUnclaimed(context.Context, string) ([]appeals.Summary, error) {
	return []appeals.Summary{}, nil
}

func (s *resolveTestStore) Claim(context.Context, string, string) (appeals.Summary, error) {
	return appeals.Summary{}, appeals.ErrNotFound
}

func (s *resolveTestStore) ListMine(context.Context, string) ([]appeals.Summary, error) {
	return []appeals.Summary{}, nil
}

func (s *resolveTestStore) Get(context.Context, string, string) (appeals.Ticket, error) {
	return appeals.Ticket{}, appeals.ErrNotFound
}

func (s *resolveTestStore) Release(context.Context, string, string) error {
	s.releaseCalls++
	return nil
}

func (s *resolveTestStore) Resolve(_ context.Context, _, _, action, comment string) error {
	s.resolveCalls++
	s.resolveAction = action
	s.resolveComment = comment
	if action == appeals.ActionDelete && comment == "" {
		return appeals.ErrCommentRequired
	}
	return nil
}

func (s *resolveTestStore) ReleaseExpired(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func newResolveTestHandler(t *testing.T, store *resolveTestStore) (http.Handler, string) {
	t.Helper()
	repository := newMemoryAuthRepository()
	authService := auth.NewService(repository)
	session, err := authService.RegisterStudent(context.Background(), "moderator@college.edu", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	user := repository.users[session.User.Email]
	user.Role = auth.RoleModerator
	repository.users[session.User.Email] = user
	return NewServer(&memoryStore{}, authService, WithAppealStore(store)), session.AccessToken
}

func TestResolveAppealRequiresDeleteComment(t *testing.T) {
	store := &resolveTestStore{}
	handler, accessToken := newResolveTestHandler(t, store)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appeals/ticket-id/resolve", strings.NewReader(
		`{"action":"delete"}`,
	))
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	if store.resolveCalls != 1 || store.resolveAction != appeals.ActionDelete || store.resolveComment != "" {
		t.Fatalf("resolve called with action %q and comment %q (%d calls)", store.resolveAction, store.resolveComment, store.resolveCalls)
	}
}

func TestReleaseAppealReturnsNoContent(t *testing.T) {
	store := &resolveTestStore{}
	handler, accessToken := newResolveTestHandler(t, store)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appeals/ticket-id/release", nil)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if store.releaseCalls != 1 {
		t.Fatalf("release called %d times, want 1", store.releaseCalls)
	}
}

func TestResolveAppealRestoreReturnsNoContent(t *testing.T) {
	store := &resolveTestStore{}
	handler, accessToken := newResolveTestHandler(t, store)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appeals/ticket-id/resolve", strings.NewReader(
		`{"action":"restore"}`,
	))
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if store.resolveCalls != 1 || store.resolveAction != appeals.ActionRestore {
		t.Fatalf("resolve called with action %q (%d calls), want restore once", store.resolveAction, store.resolveCalls)
	}
}

func TestResolveAppealDeletePassesComment(t *testing.T) {
	store := &resolveTestStore{}
	handler, accessToken := newResolveTestHandler(t, store)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/appeals/ticket-id/resolve", strings.NewReader(
		`{"action":"delete","comment":"Breaks the review guidelines"}`,
	))
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if store.resolveCalls != 1 || store.resolveAction != appeals.ActionDelete || store.resolveComment != "Breaks the review guidelines" {
		t.Fatalf("resolve called with action %q and comment %q (%d calls)", store.resolveAction, store.resolveComment, store.resolveCalls)
	}
}