package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
)

type moderatorRouteCase struct {
	method string
	path   string
	body   string
}

var allModeratorRouteCases = []moderatorRouteCase{
	{http.MethodGet, "/api/v1/appeals", ""},
	{http.MethodGet, "/api/v1/appeals/mine", ""},
	{http.MethodPost, "/api/v1/appeals/appeal-1/claim", ""},
	{http.MethodGet, "/api/v1/appeals/appeal-1", ""},
	{http.MethodPost, "/api/v1/appeals/appeal-1/release", ""},
	{http.MethodPost, "/api/v1/appeals/appeal-1/extend-lock", ""},
	{http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"restore"}`},
}

func moderatorStoreWithError(err error) *moderatorAppealTestStore {
	return &moderatorAppealTestStore{
		listUnclaimedFn: func(context.Context, string) ([]appeals.Summary, error) {
			return nil, err
		},
		listMineFn: func(context.Context, string) ([]appeals.Summary, error) {
			return nil, err
		},
		claimFn: func(context.Context, string, string) (appeals.Summary, error) {
			return appeals.Summary{}, err
		},
		getFn: func(context.Context, string, string) (appeals.Ticket, error) {
			return appeals.Ticket{}, err
		},
		releaseFn: func(context.Context, string, string) error { return err },
		extendFn: func(context.Context, string, string) (time.Time, error) {
			return time.Time{}, err
		},
		resolveFn: func(context.Context, string, string, string, string) error { return err },
	}
}

func TestAdversarialResolveBodies(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	const scriptComment = "<script>\x00alert(1)</script>"
	restoreBody, err := json.Marshal(map[string]string{"action": "restore", "comment": "restore note"})
	if err != nil {
		t.Fatal(err)
	}
	scriptBody, err := json.Marshal(map[string]string{"action": "delete", "comment": scriptComment})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		body        string
		wantStatus  int
		wantAction  string
		wantComment string
		storeErr    error
	}{
		{"empty body", "", http.StatusBadRequest, "", "", nil},
		{"JSON array", `[]`, http.StatusBadRequest, "", "", nil},
		{"JSON string", `"restore"`, http.StatusBadRequest, "", "", nil},
		{"uppercase action", `{"action":"RESTORE"}`, http.StatusBadRequest, "RESTORE", "", appeals.ErrInvalidAction},
		{"empty action", `{"action":""}`, http.StatusBadRequest, "", "", appeals.ErrInvalidAction},
		{"script and null bytes pass unchanged", string(scriptBody), http.StatusNoContent, "delete", scriptComment, nil},
		{"comment on restore passes unchanged", string(restoreBody), http.StatusNoContent, "restore", "restore note", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotAction, gotComment := "", ""
			store.resolveFn = func(_ context.Context, _, _ string, action, comment string) error {
				gotAction, gotComment = action, comment
				return test.storeErr
			}
			response := serveAppealHandler(handler, http.MethodPost, "/api/v1/appeals/appeal-1/resolve", test.body, session.AccessToken)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if gotAction != test.wantAction || gotComment != test.wantComment {
				t.Fatalf("store received action %q, comment %q; want %q, %q", gotAction, gotComment, test.wantAction, test.wantComment)
			}
			if strings.Contains(response.Body.String(), "panic") {
				t.Fatalf("unexpected panic-shaped response: %s", response.Body.String())
			}
		})
	}
}

func TestKnownBug_WhitespaceDeleteComment(t *testing.T) {
	// Bug: whitespace-only delete comments are accepted instead of rejected.
	store := &moderatorAppealTestStore{resolveFn: func(context.Context, string, string, string, string) error { return nil }}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	response := serveAppealHandler(handler, http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"delete","comment":"   "}`, session.AccessToken)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}

func TestKnownBug_LongResolveComment(t *testing.T) {
	// Bug: a valid but absurdly long comment is accepted by the store.
	store := &moderatorAppealTestStore{resolveFn: func(context.Context, string, string, string, string) error { return nil }}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	body := `{"action":"delete","comment":"` + strings.Repeat("x", 15000) + `"}`
	response := serveAppealHandler(handler, http.MethodPost, "/api/v1/appeals/appeal-1/resolve", body, session.AccessToken)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body length: %d", response.Code, http.StatusBadRequest, response.Body.Len())
	}
}

func TestAdversarialWrongMethods(t *testing.T) {
	service, handler := queueNewHandler(t, &moderatorAppealTestStore{})
	session := queueModeratorSession(t, service)
	for _, route := range allModeratorRouteCases {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			wrongMethod := http.MethodGet
			if route.method == http.MethodGet {
				wrongMethod = http.MethodPost
			}
			response := serveAppealHandler(handler, wrongMethod, route.path, route.body, session.AccessToken)
			if response.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405; body: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestAdversarialTicketIDs(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"non-id", "not-an-id", "not-an-id"},
		{"long-id", strings.Repeat("a", 200), strings.Repeat("a", 200)},
		{"encoded operator", `{"$ne":null}`, `{"$ne":null}`},
		{"encoded traversal", "../x", "../x"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &moderatorAppealTestStore{}
			service, handler := queueNewHandler(t, store)
			session := queueModeratorSession(t, service)
			calledID := ""
			store.claimFn = func(_ context.Context, id, _ string) (appeals.Summary, error) {
				calledID = id
				return appeals.Summary{}, appeals.ErrNotFound
			}
			store.getFn = func(_ context.Context, id, _ string) (appeals.Ticket, error) {
				calledID = id
				return appeals.Ticket{}, appeals.ErrNotFound
			}
			store.releaseFn = func(_ context.Context, id, _ string) error {
				calledID = id
				return appeals.ErrNotFound
			}
			store.extendFn = func(_ context.Context, id, _ string) (time.Time, error) {
				calledID = id
				return time.Time{}, appeals.ErrNotFound
			}
			store.resolveFn = func(_ context.Context, id, _, _, _ string) error {
				calledID = id
				return appeals.ErrNotFound
			}

			escaped := url.PathEscape(test.id)
			for _, request := range []moderatorRouteCase{
				{http.MethodPost, "/api/v1/appeals/" + escaped + "/claim", ""},
				{http.MethodGet, "/api/v1/appeals/" + escaped, ""},
				{http.MethodPost, "/api/v1/appeals/" + escaped + "/release", ""},
				{http.MethodPost, "/api/v1/appeals/" + escaped + "/extend-lock", ""},
				{http.MethodPost, "/api/v1/appeals/" + escaped + "/resolve", `{"action":"restore"}`},
			} {
				response := serveAppealHandler(handler, request.method, request.path, request.body, session.AccessToken)
				if response.Code != http.StatusNotFound {
					t.Errorf("%s %s status = %d, want 404; body: %s", request.method, request.path, response.Code, response.Body.String())
				}
				if calledID != test.want {
					t.Errorf("store id = %q, want %q", calledID, test.want)
				}
			}
		})
	}
}

func TestAdversarialRepeatOperations(t *testing.T) {
	tests := []struct {
		name  string
		route moderatorRouteCase
		set   func(*moderatorAppealTestStore, *bool)
	}{
		{"claim", moderatorRouteCase{http.MethodPost, "/api/v1/appeals/appeal-1/claim", ""}, func(s *moderatorAppealTestStore, done *bool) {
			s.claimFn = func(context.Context, string, string) (appeals.Summary, error) {
				if *done {
					return appeals.Summary{}, appeals.ErrNotLockHolder
				}
				*done = true
				return appeals.Summary{}, nil
			}
		}},
		{"release", moderatorRouteCase{http.MethodPost, "/api/v1/appeals/appeal-1/release", ""}, func(s *moderatorAppealTestStore, done *bool) {
			s.releaseFn = func(context.Context, string, string) error {
				if *done {
					return appeals.ErrNotLockHolder
				}
				*done = true
				return nil
			}
		}},
		{"resolve", moderatorRouteCase{http.MethodPost, "/api/v1/appeals/appeal-1/resolve", `{"action":"restore"}`}, func(s *moderatorAppealTestStore, done *bool) {
			s.resolveFn = func(context.Context, string, string, string, string) error {
				if *done {
					return appeals.ErrNotLockHolder
				}
				*done = true
				return nil
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &moderatorAppealTestStore{}
			done := false
			test.set(store, &done)
			service, handler := queueNewHandler(t, store)
			session := queueModeratorSession(t, service)
			first := serveAppealHandler(handler, test.route.method, test.route.path, test.route.body, session.AccessToken)
			second := serveAppealHandler(handler, test.route.method, test.route.path, test.route.body, session.AccessToken)
			if first.Code != http.StatusOK && first.Code != http.StatusNoContent {
				t.Fatalf("first call status = %d; body: %s", first.Code, first.Body.String())
			}
			if second.Code != http.StatusForbidden {
				t.Fatalf("second call status = %d, want 403; body: %s", second.Code, second.Body.String())
			}
		})
	}
}

func TestAdversarialAppealsSortQuery(t *testing.T) {
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	tests := []struct {
		name       string
		query      string
		wantSort   string
		wantStatus int
	}{
		{"duplicate values use first value", "?sort=asc&sort=desc", appeals.SortOldestFirst, http.StatusOK},
		{"uppercase rejected", "?sort=ASC", "", http.StatusBadRequest},
		{"empty uses default", "?sort=", appeals.SortOldestFirst, http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotSort := ""
			store.listUnclaimedFn = func(_ context.Context, sort string) ([]appeals.Summary, error) {
				gotSort = sort
				return []appeals.Summary{}, nil
			}
			response := serveAppealHandler(handler, http.MethodGet, "/api/v1/appeals"+test.query, "", session.AccessToken)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if gotSort != test.wantSort {
				t.Fatalf("store sort = %q, want %q", gotSort, test.wantSort)
			}
		})
	}
}

func TestSecurityModeratorRouteTokensAndBodies(t *testing.T) {
	store := &moderatorAppealTestStore{
		getFn: func(context.Context, string, string) (appeals.Ticket, error) {
			return appeals.Ticket{Summary: appeals.Summary{ID: "appeal-1"}, LockedBy: "moderator@example.com"}, nil
		},
	}
	repository := newMemoryAuthRepository()
	service := auth.NewService(repository)
	ctx := context.Background()
	if _, err := service.ProvisionModerator(ctx, "moderator@example.com", "long-secure-password"); err != nil {
		t.Fatal(err)
	}
	moderator, err := service.Login(ctx, "moderator@example.com", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	student, err := service.RegisterStudent(ctx, "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAdministrator(ctx, "admin@iitk.ac.in", "long-secure-password"); err != nil {
		t.Fatal(err)
	}
	administrator, err := service.Login(ctx, "admin@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(&memoryStore{}, service, WithAppealStore(store))
	truncated := moderator.AccessToken[:len(moderator.AccessToken)-1]
	tokens := []struct {
		name   string
		token  string
		status int
	}{
		{"no token", "", http.StatusUnauthorized},
		{"garbage", "garbage", http.StatusUnauthorized},
		{"truncated moderator token", truncated, http.StatusUnauthorized},
		{"student", student.AccessToken, http.StatusForbidden},
		{"administrator", administrator.AccessToken, http.StatusForbidden},
		{"moderator", moderator.AccessToken, 0},
	}
	for _, route := range allModeratorRouteCases {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			for _, token := range tokens {
				t.Run(token.name, func(t *testing.T) {
					response := serveAppealHandler(handler, route.method, route.path, route.body, token.token)
					if token.status != 0 && response.Code != token.status {
						t.Fatalf("status = %d, want %d; body: %s", response.Code, token.status, response.Body.String())
					}
					if token.status == 0 {
						if response.Code < 200 || response.Code >= 300 {
							t.Fatalf("moderator status = %d; body: %s", response.Code, response.Body.String())
						}
						for _, secret := range []string{moderator.User.Email, moderator.User.ID, "lockedBy", "passwordHash", "token"} {
							if strings.Contains(response.Body.String(), secret) {
								t.Errorf("response leaked %q: %s", secret, response.Body.String())
							}
						}
					}
				})
			}
		})
	}

	// The in-memory repository exposes its user map, so removing the user invalidates existing sessions.
	delete(repository.users, moderator.User.Email)
	response := serveAppealHandler(handler, http.MethodGet, "/api/v1/appeals", "", moderator.AccessToken)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("removed moderator token status = %d, want 401", response.Code)
	}
}

func TestKnownBug_CookieOverridesBearer(t *testing.T) {
	// Bug: sessionToken prefers a stale cookie over a valid Bearer token.
	store := &moderatorAppealTestStore{}
	service, handler := queueNewHandler(t, store)
	moderator := queueModeratorSession(t, service)
	student := queueStudentSession(t, service)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/appeals", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: student.AccessToken})
	request.Header.Set("Authorization", "Bearer "+moderator.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", response.Code, response.Body.String())
	}
}

func TestFailureGenericStoreErrors(t *testing.T) {
	const internalError = "mongo: secret connection string"
	store := moderatorStoreWithError(errors.New(internalError))
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	wantMessages := []string{
		"could not list appeals", "could not list appeals", "could not claim appeal",
		"could not complete appeal operation", "could not complete appeal operation",
		"could not complete appeal operation", "could not complete appeal operation",
	}
	for i, route := range allModeratorRouteCases {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := serveAppealHandler(handler, route.method, route.path, route.body, session.AccessToken)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500; body: %s", response.Code, response.Body.String())
			}
			if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if body["error"] != wantMessages[i] {
				t.Fatalf("error = %q, want %q", body["error"], wantMessages[i])
			}
			if strings.Contains(response.Body.String(), internalError) {
				t.Fatalf("response leaked internal store error: %s", response.Body.String())
			}
		})
	}
}

func TestFailureCancelledRequestContext(t *testing.T) {
	store := moderatorStoreWithError(context.Canceled)
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	for _, route := range allModeratorRouteCases {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			request.Header.Set("Authorization", "Bearer "+session.AccessToken)
			ctx, cancel := context.WithCancel(request.Context())
			cancel()
			request = request.WithContext(ctx)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code < 400 || response.Code >= 600 {
				t.Fatalf("status = %d, want normal error response; body: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestFailureSentinelMappingAcrossRoutes(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"not found", appeals.ErrNotFound},
		{"not lock holder", appeals.ErrNotLockHolder},
		{"already claimed", appeals.ErrAlreadyClaimed},
		{"invalid action", appeals.ErrInvalidAction},
		{"comment required", appeals.ErrCommentRequired},
	}
	want := map[string]map[string]int{
		"/api/v1/appeals":                      {"not found": 500, "not lock holder": 500, "already claimed": 500, "invalid action": 500, "comment required": 500},
		"/api/v1/appeals/mine":                 {"not found": 500, "not lock holder": 500, "already claimed": 500, "invalid action": 500, "comment required": 500},
		"/api/v1/appeals/appeal-1/claim":       {"not found": 404, "not lock holder": 500, "already claimed": 409, "invalid action": 500, "comment required": 500},
		"/api/v1/appeals/appeal-1":             {"not found": 404, "not lock holder": 403, "already claimed": 500, "invalid action": 400, "comment required": 400},
		"/api/v1/appeals/appeal-1/release":     {"not found": 404, "not lock holder": 403, "already claimed": 500, "invalid action": 400, "comment required": 400},
		"/api/v1/appeals/appeal-1/extend-lock": {"not found": 404, "not lock holder": 403, "already claimed": 500, "invalid action": 400, "comment required": 400},
		"/api/v1/appeals/appeal-1/resolve":     {"not found": 404, "not lock holder": 403, "already claimed": 500, "invalid action": 400, "comment required": 400},
	}
	var currentErr error
	store := &moderatorAppealTestStore{
		listUnclaimedFn: func(context.Context, string) ([]appeals.Summary, error) { return nil, currentErr },
		listMineFn:      func(context.Context, string) ([]appeals.Summary, error) { return nil, currentErr },
		claimFn:         func(context.Context, string, string) (appeals.Summary, error) { return appeals.Summary{}, currentErr },
		getFn:           func(context.Context, string, string) (appeals.Ticket, error) { return appeals.Ticket{}, currentErr },
		releaseFn:       func(context.Context, string, string) error { return currentErr },
		extendFn:        func(context.Context, string, string) (time.Time, error) { return time.Time{}, currentErr },
		resolveFn:       func(context.Context, string, string, string, string) error { return currentErr },
	}
	service, handler := queueNewHandler(t, store)
	session := queueModeratorSession(t, service)
	for _, route := range allModeratorRouteCases {
		for _, sentinel := range sentinels {
			t.Run(fmt.Sprintf("%s/%s", route.path, sentinel.name), func(t *testing.T) {
				currentErr = sentinel.err
				if currentErr == appeals.ErrNotFound || currentErr == appeals.ErrNotLockHolder || currentErr == appeals.ErrAlreadyClaimed {
					currentErr = fmt.Errorf("%w: mongo: secret connection string", currentErr)
				}
				response := serveAppealHandler(handler, route.method, route.path, route.body, session.AccessToken)
				if response.Code != want[route.path][sentinel.name] {
					t.Fatalf("status = %d, want %d; body: %s", response.Code, want[route.path][sentinel.name], response.Body.String())
				}
				if strings.Contains(response.Body.String(), "mongo: secret connection string") {
					t.Fatalf("response leaked internal error: %s", response.Body.String())
				}
			})
		}
	}
}

func FuzzResolveBody(f *testing.F) {
	for _, body := range []string{
		`{"action":"restore"}`,
		`{"action":"delete","comment":"ok"}`,
		"",
		"{",
		strings.Repeat("x", 20*1024),
	} {
		f.Add(body)
	}
	store := &moderatorAppealTestStore{}
	service := auth.NewService(newMemoryAuthRepository())
	if _, err := service.ProvisionModerator(context.Background(), "moderator@example.com", "long-secure-password"); err != nil {
		f.Fatal(err)
	}
	session, err := service.Login(context.Background(), "moderator@example.com", "long-secure-password")
	if err != nil {
		f.Fatal(err)
	}
	handler := NewServer(nil, service, WithAppealStore(store))
	f.Fuzz(func(t *testing.T, body string) {
		response := serveAppealHandler(handler, http.MethodPost, "/api/v1/appeals/appeal-1/resolve", body, session.AccessToken)
		switch response.Code {
		case http.StatusNoContent, http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound:
		default:
			t.Fatalf("unexpected status %d for body %q", response.Code, body)
		}
	})
}
