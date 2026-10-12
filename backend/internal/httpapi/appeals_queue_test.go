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

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
)

type queueFakeStore struct {
	listUnclaimedFn func(context.Context, string) ([]appeals.Summary, error)
	claimFn         func(context.Context, string, string) (appeals.Summary, error)
	listMineFn      func(context.Context, string) ([]appeals.Summary, error)
	called          int
}

func (s *queueFakeStore) ListUnclaimed(ctx context.Context, sort string) ([]appeals.Summary, error) {
	s.called++
	if s.listUnclaimedFn != nil {
		return s.listUnclaimedFn(ctx, sort)
	}
	return nil, nil
}

func (s *queueFakeStore) Claim(ctx context.Context, ticketID, moderatorID string) (appeals.Summary, error) {
	s.called++
	if s.claimFn != nil {
		return s.claimFn(ctx, ticketID, moderatorID)
	}
	return appeals.Summary{}, nil
}

func (s *queueFakeStore) ListMine(ctx context.Context, moderatorID string) ([]appeals.Summary, error) {
	s.called++
	if s.listMineFn != nil {
		return s.listMineFn(ctx, moderatorID)
	}
	return nil, nil
}

func (s *queueFakeStore) Get(context.Context, string, string) (appeals.Ticket, error) {
	return appeals.Ticket{}, appeals.ErrNotImplemented
}

func (s *queueFakeStore) Release(context.Context, string, string) error {
	return appeals.ErrNotImplemented
}

func (s *queueFakeStore) Resolve(context.Context, string, string, string, string) error {
	return appeals.ErrNotImplemented
}

func (s *queueFakeStore) ReleaseExpired(context.Context, time.Time) (int64, error) {
	return 0, appeals.ErrNotImplemented
}

func queueNewHandler(t *testing.T, store appeals.Store) (*auth.Service, http.Handler) {
	t.Helper()
	service := auth.NewService(newMemoryAuthRepository())
	return service, NewServer(nil, service, WithAppealStore(store))
}

func queueModeratorSession(t *testing.T, service *auth.Service) auth.Session {
	t.Helper()
	const (
		email    = "moderator@example.com"
		password = "long-secure-password"
	)
	if _, err := service.ProvisionModerator(context.Background(), email, password); err != nil {
		t.Fatalf("provision moderator: %v", err)
	}
	session, err := service.Login(context.Background(), email, password)
	if err != nil {
		t.Fatalf("login moderator: %v", err)
	}
	return session
}

func queueStudentSession(t *testing.T, service *auth.Service) auth.Session {
	t.Helper()
	const (
		email    = "student@example.com"
		password = "long-secure-password"
	)
	if _, err := service.RegisterStudent(context.Background(), email, password); err != nil {
		t.Fatalf("register student: %v", err)
	}
	session, err := service.Login(context.Background(), email, password)
	if err != nil {
		t.Fatalf("login student: %v", err)
	}
	return session
}

func TestQueueListUsesDefaultSortAsc(t *testing.T) {
	store := &queueFakeStore{
		listUnclaimedFn: func(_ context.Context, sort string) ([]appeals.Summary, error) {
			if sort != appeals.SortOldestFirst {
				t.Fatalf("sort = %q, want %q", sort, appeals.SortOldestFirst)
			}
			return []appeals.Summary{{ID: "123", ReferenceCode: "RC-1"}}, nil
		},
	}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/appeals", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, http.StatusOK, resp.Body.String())
	}
}

func TestQueueListPassesSortDesc(t *testing.T) {
	store := &queueFakeStore{
		listUnclaimedFn: func(_ context.Context, sort string) ([]appeals.Summary, error) {
			if sort != appeals.SortNewestFirst {
				t.Fatalf("sort = %q, want %q", sort, appeals.SortNewestFirst)
			}
			return []appeals.Summary{{ID: "123", ReferenceCode: "RC-1"}}, nil
		},
	}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/appeals?sort=desc", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, http.StatusOK, resp.Body.String())
	}
}

func TestQueueListRejectsInvalidSort(t *testing.T) {
	store := &queueFakeStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/appeals?sort=bogus", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, http.StatusBadRequest, resp.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["error"] != "invalid sort" {
		t.Fatalf("error = %q, want %q", result["error"], "invalid sort")
	}
}

func TestQueueListEncodesEmptyArrayNotNull(t *testing.T) {
	store := &queueFakeStore{
		listUnclaimedFn: func(_ context.Context, _ string) ([]appeals.Summary, error) {
			return []appeals.Summary{}, nil
		},
	}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/appeals", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "\"appeals\":[]") {
		t.Fatalf("body = %s, want empty array encoding", resp.Body.String())
	}
	var result map[string][]appeals.Summary
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["appeals"] == nil {
		t.Fatal("appeals was null, want empty array")
	}
}

func TestQueueClaimSuccess(t *testing.T) {
	store := &queueFakeStore{
		claimFn: func(_ context.Context, ticketID, moderatorID string) (appeals.Summary, error) {
			if ticketID != "abc123" {
				t.Fatalf("ticketID = %q, want %q", ticketID, "abc123")
			}
			if moderatorID == "" {
				t.Fatal("moderatorID was empty")
			}
			return appeals.Summary{ID: ticketID, ReferenceCode: "REF-1", Status: appeals.StatusLocked, CreatedAt: time.Now().UTC()}, nil
		},
	}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/appeals/abc123/claim", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, http.StatusOK, resp.Body.String())
	}
	var result appeals.Summary
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.ID != "abc123" {
		t.Fatalf("id = %q, want %q", result.ID, "abc123")
	}
}

func TestQueueClaimConflict(t *testing.T) {
	store := &queueFakeStore{
		claimFn: func(context.Context, string, string) (appeals.Summary, error) {
			return appeals.Summary{}, appeals.ErrAlreadyClaimed
		},
	}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/appeals/abc123/claim", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, http.StatusConflict, resp.Body.String())
	}
	if got := strings.TrimSpace(resp.Body.String()); got != "{\"error\":\"This ticket has already been claimed\"}" {
		t.Fatalf("body = %s, want exact conflict message", got)
	}
}

func TestQueueClaimUnknownID(t *testing.T) {
	store := &queueFakeStore{
		claimFn: func(context.Context, string, string) (appeals.Summary, error) {
			return appeals.Summary{}, appeals.ErrNotFound
		},
	}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/appeals/does-not-exist/claim", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, http.StatusNotFound, resp.Body.String())
	}
	var result map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result["error"] != appeals.ErrNotFound.Error() {
		t.Fatalf("error = %q, want %q", result["error"], appeals.ErrNotFound.Error())
	}
}

func TestQueueRejectsStudentToken(t *testing.T) {
	store := &queueFakeStore{
		listUnclaimedFn: func(context.Context, string) ([]appeals.Summary, error) {
			return nil, errors.New("store should not be called for a student")
		},
	}
	service, handler := queueNewHandler(t, store)
	session := queueStudentSession(t, service)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/appeals", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
	if store.called != 0 {
		t.Fatalf("store calls = %d, want 0 for forbidden student request", store.called)
	}
}
