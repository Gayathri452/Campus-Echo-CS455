package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
)

func TestWriteAuthErrorMapsServiceErrorsWithoutLeakingDetails(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"invalid input", auth.ErrInvalidInput, http.StatusBadRequest, "email or password does not meet requirements"},
		{"email not allowed", auth.ErrEmailNotAllowed, http.StatusBadRequest, "only @iitk.ac.in email addresses are allowed"},
		{"invalid OTP", auth.ErrInvalidOTP, http.StatusUnauthorized, "invalid or expired OTP"},
		{"OTP attempts", auth.ErrOTPAttempts, http.StatusTooManyRequests, "OTP request limit reached; try again later"},
		{"OTP resend", auth.ErrOTPResends, http.StatusTooManyRequests, "OTP request limit reached; try again later"},
		{"OTP cooldown", auth.ErrOTPResendTooSoon, http.StatusTooManyRequests, "OTP request limit reached; try again later"},
		{"login rate", auth.ErrLoginRateLimited, http.StatusTooManyRequests, "too many authentication requests; try again later"},
		{"OTP request rate", auth.ErrOTPRequestRateLimited, http.StatusTooManyRequests, "too many authentication requests; try again later"},
		{"email service", auth.ErrEmailService, http.StatusServiceUnavailable, "email delivery is not configured"},
		{"password setup", auth.ErrPasswordSetup, http.StatusInternalServerError, "student password setup is unavailable"},
		{"invalid credentials", auth.ErrInvalidCredentials, http.StatusUnauthorized, "invalid email or password"},
		{"role mismatch", auth.ErrRoleMismatch, http.StatusForbidden, "this account cannot use the selected login"},
		{"invalid session", auth.ErrInvalidSession, http.StatusUnauthorized, "invalid or expired session"},
		{"duplicate email", auth.ErrEmailAlreadyExists, http.StatusConflict, "email is already registered"},
		{"missing moderator", auth.ErrModeratorNotFound, http.StatusNotFound, "moderator account not found"},
		{"unexpected failure", errors.New("private database connection detail"), http.StatusInternalServerError, "authentication operation failed"},
	}
	server := &Server{}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			server.writeAuthError(response, test.err)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if !strings.Contains(response.Body.String(), test.message) {
				t.Fatalf("body = %q, want message %q", response.Body.String(), test.message)
			}
			if strings.Contains(response.Body.String(), "private database connection detail") {
				t.Fatalf("response leaked internal error: %s", response.Body.String())
			}
		})
	}
}

func TestAuthRoutesCoverLogoutRateLimitAndUnexpectedRepositoryErrors(t *testing.T) {
	repository := newMemoryAuthRepository()
	service := auth.NewService(repository)
	session, err := service.RegisterStudent(context.Background(), "routes@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(nil, service)

	unauthenticatedLogout := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, unauthenticatedLogout)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("logout without credentials = %d, want 401", response.Code)
	}

	logout := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logout.AddCookie(&http.Cookie{Name: sessionCookieName, Value: session.AccessToken})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, logout)
	if response.Code != http.StatusNoContent || !strings.Contains(response.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("logout response = (%d, %q), want 204 and expired cookie", response.Code, response.Header().Get("Set-Cookie"))
	}

	if got := clientIP(httptest.NewRequest(http.MethodGet, "/", nil)); got != "192.0.2.1" {
		t.Fatalf("parsed client IP = %q, want 192.0.2.1", got)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.RemoteAddr = "not-a-host-port"
	if got := clientIP(request); got != request.RemoteAddr {
		t.Fatalf("unparsed client IP = %q, want %q", got, request.RemoteAddr)
	}

	limiterHandler := NewServer(nil, auth.NewService(newMemoryAuthRepository()))
	var rateLimited *httptest.ResponseRecorder
	for i := 0; i < 21; i++ {
		request := httptest.NewRequest(http.MethodPost, "/api/auth/admin/login", strings.NewReader(`{"email":"nobody@example.test","password":"nope"}`))
		response := httptest.NewRecorder()
		limiterHandler.ServeHTTP(response, request)
		if i == 20 {
			rateLimited = response
		}
	}
	if rateLimited == nil || rateLimited.Code != http.StatusTooManyRequests {
		t.Fatalf("21st login request status = %v, want 429", rateLimited)
	}

	internalFailure := errors.New("database password must not be returned")
	faulty := &authFailureRepository{memoryAuthRepository: newMemoryAuthRepository(), findSessionErr: internalFailure}
	faultyService := auth.NewService(faulty)
	valid, err := faultyService.RegisterStudent(context.Background(), "fault@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	faultyHandler := NewServer(nil, faultyService)
	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.Header.Set("Authorization", "Bearer "+valid.AccessToken)
	response = httptest.NewRecorder()
	faultyHandler.ServeHTTP(response, me)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), internalFailure.Error()) {
		t.Fatalf("auth backend failure response = (%d, %s)", response.Code, response.Body.String())
	}

	faultyLogout := &authFailureRepository{memoryAuthRepository: newMemoryAuthRepository(), deleteSessionErr: internalFailure}
	logoutService := auth.NewService(faultyLogout)
	logoutSession, err := logoutService.RegisterStudent(context.Background(), "logout@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	logoutHandler := NewServer(nil, logoutService)
	logoutRequest := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutRequest.Header.Set("Authorization", "Bearer "+logoutSession.AccessToken)
	response = httptest.NewRecorder()
	logoutHandler.ServeHTTP(response, logoutRequest)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), internalFailure.Error()) {
		t.Fatalf("logout backend failure response = (%d, %s)", response.Code, response.Body.String())
	}
}

type authFailureRepository struct {
	*memoryAuthRepository
	findSessionErr   error
	deleteSessionErr error
}

func (repository *authFailureRepository) FindUserBySession(ctx context.Context, tokenHash string, now time.Time) (auth.User, error) {
	if repository.findSessionErr != nil {
		return auth.User{}, repository.findSessionErr
	}
	return repository.memoryAuthRepository.FindUserBySession(ctx, tokenHash, now)
}

func (repository *authFailureRepository) DeleteSession(ctx context.Context, tokenHash string) error {
	if repository.deleteSessionErr != nil {
		return repository.deleteSessionErr
	}
	return repository.memoryAuthRepository.DeleteSession(ctx, tokenHash)
}
