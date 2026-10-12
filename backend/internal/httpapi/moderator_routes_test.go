package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
)

// moderatorRoutes lists every moderator endpoint and the mux pattern that must
// serve it. Person A and Person B add their own handler tests in
// appeals_queue_test.go and appeals_resolve_test.go.
var moderatorRoutes = []struct {
	method  string
	path    string
	pattern string
}{
	{http.MethodGet, "/api/v1/appeals", "GET /api/v1/appeals"},
	{http.MethodGet, "/api/v1/appeals/mine", "GET /api/v1/appeals/mine"},
	{http.MethodPost, "/api/v1/appeals/abc/claim", "POST /api/v1/appeals/{id}/claim"},
	{http.MethodGet, "/api/v1/appeals/abc", "GET /api/v1/appeals/{id}"},
	{http.MethodPost, "/api/v1/appeals/abc/release", "POST /api/v1/appeals/{id}/release"},
	{http.MethodPost, "/api/v1/appeals/abc/resolve", "POST /api/v1/appeals/{id}/resolve"},
}

func newModeratorTestServer(t *testing.T) (handler http.Handler, studentToken string) {
	t.Helper()
	service := auth.NewService(newMemoryAuthRepository())
	ctx := context.Background()

	student, err := service.RegisterStudent(ctx, "student@college.edu", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(nil, service), student.AccessToken
}

func serve(handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(""))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// The role check runs before any handler, so this stays valid as handlers are
// implemented.
func TestModeratorRoutesRequireModeratorRole(t *testing.T) {
	handler, studentToken := newModeratorTestServer(t)

	for _, route := range moderatorRoutes {
		if got := serve(handler, route.method, route.path, "").Code; got != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want %d", route.method, route.path, got, http.StatusUnauthorized)
		}
		if got := serve(handler, route.method, route.path, studentToken).Code; got != http.StatusForbidden {
			t.Errorf("%s %s as a student = %d, want %d", route.method, route.path, got, http.StatusForbidden)
		}
	}
}

// Checks routing only (no handler runs), so it also stays valid as handlers
// are implemented. It guards the "mine" vs "{id}" precedence.
func TestModeratorRoutesMatchExpectedPatterns(t *testing.T) {
	handler, _ := newModeratorTestServer(t)
	mux, ok := handler.(*http.ServeMux)
	if !ok {
		t.Fatalf("NewServer returned %T, want *http.ServeMux", handler)
	}

	for _, route := range moderatorRoutes {
		request := httptest.NewRequest(route.method, route.path, nil)
		if _, pattern := mux.Handler(request); pattern != route.pattern {
			t.Errorf("%s %s matched %q, want %q", route.method, route.path, pattern, route.pattern)
		}
	}
}
