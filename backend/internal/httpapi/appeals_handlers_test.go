package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
)

type moderatorAppealTestStore struct {
	listUnclaimedFn func(context.Context, string) ([]appeals.Summary, error)
	claimFn         func(context.Context, string, string) (appeals.Summary, error)
	listMineFn      func(context.Context, string) ([]appeals.Summary, error)
	getFn           func(context.Context, string, string) (appeals.Ticket, error)
	releaseFn       func(context.Context, string, string) error
	extendFn        func(context.Context, string, string) (time.Time, error)
	resolveFn       func(context.Context, string, string, string, string) error
}

func (s *moderatorAppealTestStore) ListUnclaimed(ctx context.Context, sort string) ([]appeals.Summary, error) {
	if s.listUnclaimedFn != nil {
		return s.listUnclaimedFn(ctx, sort)
	}
	return nil, nil
}

func (s *moderatorAppealTestStore) Claim(ctx context.Context, ticketID, moderatorID string) (appeals.Summary, error) {
	if s.claimFn != nil {
		return s.claimFn(ctx, ticketID, moderatorID)
	}
	return appeals.Summary{}, nil
}

func (s *moderatorAppealTestStore) ListMine(ctx context.Context, moderatorID string) ([]appeals.Summary, error) {
	if s.listMineFn != nil {
		return s.listMineFn(ctx, moderatorID)
	}
	return nil, nil
}

func (s *moderatorAppealTestStore) Get(ctx context.Context, ticketID, moderatorID string) (appeals.Ticket, error) {
	if s.getFn != nil {
		return s.getFn(ctx, ticketID, moderatorID)
	}
	return appeals.Ticket{}, nil
}

func (s *moderatorAppealTestStore) Release(ctx context.Context, ticketID, moderatorID string) error {
	if s.releaseFn != nil {
		return s.releaseFn(ctx, ticketID, moderatorID)
	}
	return nil
}

func (s *moderatorAppealTestStore) ExtendLock(ctx context.Context, ticketID, moderatorID string) (time.Time, error) {
	if s.extendFn != nil {
		return s.extendFn(ctx, ticketID, moderatorID)
	}
	return time.Time{}, nil
}

func (s *moderatorAppealTestStore) Resolve(ctx context.Context, ticketID, moderatorID, action, comment string) error {
	if s.resolveFn != nil {
		return s.resolveFn(ctx, ticketID, moderatorID, action, comment)
	}
	return nil
}

func (s *moderatorAppealTestStore) ReleaseExpired(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func serveAppealHandler(handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestListMyAppealsHandler(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	var gotModeratorID string

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		tokenKind   string
		storeResult error
		wantStatus  int
		wantBody    string
	}{
		{"moderator id passed to store", http.MethodGet, "/api/v1/appeals/mine", "", "moderator", nil, http.StatusOK, `"appeals":[`},
		{"empty appeals are an array", http.MethodGet, "/api/v1/appeals/mine", "", "moderator", nil, http.StatusOK, `"appeals":[]`},
		{"store error", http.MethodGet, "/api/v1/appeals/mine", "", "moderator", errors.New("store failure"), http.StatusInternalServerError, "could not list appeals"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test := test
			store.listMineFn = func(_ context.Context, moderatorID string) ([]appeals.Summary, error) {
				gotModeratorID = moderatorID
				if test.storeResult != nil {
					return nil, test.storeResult
				}
				if test.name == "empty appeals are an array" {
					return []appeals.Summary{}, nil
				}
				return []appeals.Summary{{ID: "appeal-1"}}, nil
			}
			token := ""
			if test.tokenKind == "moderator" {
				token = session.AccessToken
			}
			response := serveAppealHandler(handler, test.method, test.path, test.body, token)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %s, want substring %q", response.Body.String(), test.wantBody)
			}
			if test.name == "moderator id passed to store" && gotModeratorID != session.User.ID {
				t.Fatalf("moderator id = %q, want %q", gotModeratorID, session.User.ID)
			}
		})
	}
}

func TestListAppealsStoreError(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		tokenKind   string
		storeResult error
		wantStatus  int
		wantBody    string
	}{
		{"store error", http.MethodGet, "/api/v1/appeals", "", "moderator", errors.New("store failure"), http.StatusInternalServerError, "could not list appeals"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store.listUnclaimedFn = func(context.Context, string) ([]appeals.Summary, error) {
				return nil, test.storeResult
			}
			token := ""
			if test.tokenKind == "moderator" {
				token = session.AccessToken
			}
			response := serveAppealHandler(handler, test.method, test.path, test.body, token)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %s, want substring %q", response.Body.String(), test.wantBody)
			}
		})
	}
}

func TestClaimAppealStoreError(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		tokenKind   string
		storeResult error
		wantStatus  int
		wantBody    string
	}{
		{"store error", http.MethodPost, "/api/v1/appeals/appeal-1/claim", "", "moderator", errors.New("store failure"), http.StatusInternalServerError, "could not claim appeal"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store.claimFn = func(context.Context, string, string) (appeals.Summary, error) {
				return appeals.Summary{}, test.storeResult
			}
			token := ""
			if test.tokenKind == "moderator" {
				token = session.AccessToken
			}
			response := serveAppealHandler(handler, test.method, test.path, test.body, token)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %s, want substring %q", response.Body.String(), test.wantBody)
			}
		})
	}
}

func TestGetAppealHandler(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		tokenKind   string
		storeResult error
		wantStatus  int
		wantBody    string
	}{
		{"success", http.MethodGet, "/api/v1/appeals/appeal-1", "", "moderator", nil, http.StatusOK, `"feedbackText":"text"`},
		{"not lock holder", http.MethodGet, "/api/v1/appeals/appeal-1", "", "moderator", appeals.ErrNotLockHolder, http.StatusForbidden, "no longer assigned to you"},
		{"not found", http.MethodGet, "/api/v1/appeals/appeal-1", "", "moderator", appeals.ErrNotFound, http.StatusNotFound, "appeal ticket not found"},
		{"unexpected store error", http.MethodGet, "/api/v1/appeals/appeal-1", "", "moderator", errors.New("store failure"), http.StatusInternalServerError, "could not complete appeal operation"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test := test
			store.getFn = func(context.Context, string, string) (appeals.Ticket, error) {
				if test.storeResult != nil {
					return appeals.Ticket{}, test.storeResult
				}
				return appeals.Ticket{
					Summary:      appeals.Summary{ID: "appeal-1"},
					FeedbackText: "text",
					LockedBy:     session.User.ID,
				}, nil
			}
			token := ""
			if test.tokenKind == "moderator" {
				token = session.AccessToken
			}
			response := serveAppealHandler(handler, test.method, test.path, test.body, token)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %s, want substring %q", response.Body.String(), test.wantBody)
			}
			if test.storeResult == nil && strings.Contains(response.Body.String(), "lockedBy") {
				t.Fatalf("body leaked lockedBy: %s", response.Body.String())
			}
		})
	}
}

func TestReleaseAppealHandler(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		tokenKind   string
		storeResult error
		wantStatus  int
		wantBody    string
	}{
		{"not lock holder", http.MethodPost, "/api/v1/appeals/appeal-1/release", "", "moderator", appeals.ErrNotLockHolder, http.StatusForbidden, "no longer assigned to you"},
		{"not found", http.MethodPost, "/api/v1/appeals/appeal-1/release", "", "moderator", appeals.ErrNotFound, http.StatusNotFound, "appeal ticket not found"},
		{"unexpected store error", http.MethodPost, "/api/v1/appeals/appeal-1/release", "", "moderator", errors.New("store failure"), http.StatusInternalServerError, "could not complete appeal operation"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test := test
			store.releaseFn = func(context.Context, string, string) error { return test.storeResult }
			token := ""
			if test.tokenKind == "moderator" {
				token = session.AccessToken
			}
			response := serveAppealHandler(handler, test.method, test.path, test.body, token)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %s, want substring %q", response.Body.String(), test.wantBody)
			}
		})
	}
}

func TestExtendAppealLockHandler(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		tokenKind   string
		storeResult error
		wantStatus  int
		wantBody    string
	}{
		{"success", http.MethodPost, "/api/v1/appeals/appeal-1/extend-lock", "", "moderator", nil, http.StatusOK, `"lockExpiresAt"`},
		{"not lock holder", http.MethodPost, "/api/v1/appeals/appeal-1/extend-lock", "", "moderator", appeals.ErrNotLockHolder, http.StatusForbidden, "no longer assigned to you"},
		{"unexpected store error", http.MethodPost, "/api/v1/appeals/appeal-1/extend-lock", "", "moderator", errors.New("store failure"), http.StatusInternalServerError, "could not complete appeal operation"},
	}
	expiresAt := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test := test
			store.extendFn = func(context.Context, string, string) (time.Time, error) {
				return expiresAt, test.storeResult
			}
			token := ""
			if test.tokenKind == "moderator" {
				token = session.AccessToken
			}
			response := serveAppealHandler(handler, test.method, test.path, test.body, token)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %s, want substring %q", response.Body.String(), test.wantBody)
			}
		})
	}
}

func TestResolveAppealHandler(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	largeBody := `{"action":"restore","padding":"` + strings.Repeat("x", 16*1024) + `"}`

	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		tokenKind   string
		storeResult error
		wantStatus  int
		wantBody    string
	}{
		{"invalid action", http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"hide"}`, "moderator", appeals.ErrInvalidAction, http.StatusBadRequest, "action must be restore or delete"},
		{"invalid JSON", http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{`, "moderator", nil, http.StatusBadRequest, "request must contain one valid JSON object"},
		{"unknown field", http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"restore","unknown":true}`, "moderator", nil, http.StatusBadRequest, "request must contain one valid JSON object"},
		{"two JSON values", http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"restore"} {}`, "moderator", nil, http.StatusBadRequest, "request must contain one valid JSON object"},
		{"body over limit", http.MethodPost, "/api/v1/appeals/appeal-1/resolve", largeBody, "moderator", nil, http.StatusBadRequest, "request must contain one valid JSON object"},
		{"not lock holder", http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"restore"}`, "moderator", appeals.ErrNotLockHolder, http.StatusForbidden, "no longer assigned to you"},
		{"not found", http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"restore"}`, "moderator", appeals.ErrNotFound, http.StatusNotFound, "appeal ticket not found"},
		{"unexpected store error", http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"restore"}`, "moderator", errors.New("internal store details"), http.StatusInternalServerError, "could not complete appeal operation"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test := test
			resolveCalls := 0
			gotAction := ""
			store.resolveFn = func(_ context.Context, _, _ string, action, _ string) error {
				resolveCalls++
				gotAction = action
				return test.storeResult
			}
			token := ""
			if test.tokenKind == "moderator" {
				token = session.AccessToken
			}
			response := serveAppealHandler(handler, test.method, test.path, test.body, token)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.wantBody) {
				t.Fatalf("body = %s, want substring %q", response.Body.String(), test.wantBody)
			}
			if test.name == "invalid action" && gotAction != "hide" {
				t.Fatalf("resolve action = %q, want hide", gotAction)
			}
			if test.name == "unexpected store error" && strings.Contains(response.Body.String(), "internal store details") {
				t.Fatalf("response leaked internal store error: %s", response.Body.String())
			}
			wantCalls := 1
			if strings.Contains(test.name, "JSON") || test.name == "unknown field" || test.name == "body over limit" {
				wantCalls = 0
			}
			if resolveCalls != wantCalls {
				t.Fatalf("resolve calls = %d, want %d", resolveCalls, wantCalls)
			}
		})
	}
}

func TestModeratorHandlerAuthAcrossAllRoutes(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	student := queueStudentSession(t, service)
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/appeals"},
		{http.MethodGet, "/api/v1/appeals/mine"},
		{http.MethodPost, "/api/v1/appeals/appeal-1/claim"},
		{http.MethodGet, "/api/v1/appeals/appeal-1"},
		{http.MethodPost, "/api/v1/appeals/appeal-1/release"},
		{http.MethodPost, "/api/v1/appeals/appeal-1/extend-lock"},
		{http.MethodPost, "/api/v1/appeals/appeal-1/resolve"},
	}

	for _, route := range routes {
		if got := serveAppealHandler(handler, route.method, route.path, "", "").Code; got != http.StatusUnauthorized {
			t.Errorf("%s %s without token = %d, want %d", route.method, route.path, got, http.StatusUnauthorized)
		}
		if got := serveAppealHandler(handler, route.method, route.path, "", student.AccessToken).Code; got != http.StatusForbidden {
			t.Errorf("%s %s with student token = %d, want %d", route.method, route.path, got, http.StatusForbidden)
		}
	}
}

func TestModeratorHandlersRequireAuthenticatedUserInContext(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		path    string
		body    string
	}{
		{"list mine", (&Server{}).listMyAppeals, http.MethodGet, "/api/v1/appeals/mine", ""},
		{"claim", (&Server{}).claimAppeal, http.MethodPost, "/api/v1/appeals/appeal-1/claim", ""},
		{"get detail", (&Server{}).getAppeal, http.MethodGet, "/api/v1/appeals/appeal-1", ""},
		{"release", (&Server{}).releaseAppeal, http.MethodPost, "/api/v1/appeals/appeal-1/release", ""},
		{"extend lock", (&Server{}).extendAppealLock, http.MethodPost, "/api/v1/appeals/appeal-1/extend-lock", ""},
		{"resolve", (&Server{}).resolveAppeal, http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"restore"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveAppealHandler(test.handler, test.method, test.path, test.body, "")
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusInternalServerError, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "authenticated user unavailable") {
				t.Fatalf("body = %s, want missing-user error", response.Body.String())
			}
		})
	}
}

func TestWriteNotImplemented(t *testing.T) {
	server := &Server{}
	WithCookieSecure(true)(server)
	if !server.cookieSecure {
		t.Fatal("WithCookieSecure did not enable secure cookies")
	}

	response := httptest.NewRecorder()
	writeNotImplemented(response)
	if response.Code != http.StatusNotImplemented || !strings.Contains(response.Body.String(), "not implemented") {
		t.Fatalf("response = %d %q, want 501 not-implemented response", response.Code, response.Body.String())
	}
}

var _ appeals.Store = (*moderatorAppealTestStore)(nil)
