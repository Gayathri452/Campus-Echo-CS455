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
)

type memoryEmailService struct {
	email string
	code  string
}

func (m *memoryEmailService) SendOTP(_ context.Context, email, code string) error {
	m.email = email
	m.code = code
	return nil
}

func (m *memoryAuthRepository) CreateStudent(_ context.Context, email, passwordHash string) (auth.User, error) {
	if user, exists := m.users[email]; exists {
		return user, nil
	}
	user := auth.User{ID: email, Email: email, Role: auth.RoleStudent, CreatedAt: time.Now().UTC()}
	m.users[email] = user
	m.passwords[email] = passwordHash
	return user, nil
}

func (m *memoryAuthRepository) FindOTP(_ context.Context, email string) (auth.OTPChallenge, error) {
	challenge, exists := m.otps[email]
	if !exists {
		return auth.OTPChallenge{}, auth.ErrInvalidOTP
	}
	return challenge, nil
}

func (m *memoryAuthRepository) InvalidateOTP(_ context.Context, email string) error {
	delete(m.otps, email)
	return nil
}

func (m *memoryAuthRepository) CreateOTP(_ context.Context, challenge auth.OTPChallenge) error {
	m.otps[challenge.Email] = challenge
	return nil
}

func (m *memoryAuthRepository) IncrementOTPAttempts(_ context.Context, email string) (int, error) {
	challenge, exists := m.otps[email]
	if !exists {
		return 0, auth.ErrInvalidOTP
	}
	challenge.Attempts++
	m.otps[email] = challenge
	return challenge.Attempts, nil
}

func (m *memoryAuthRepository) DeleteOTP(_ context.Context, email string) error {
	delete(m.otps, email)
	return nil
}

func (m *memoryAuthRepository) DeleteModerator(_ context.Context, email string) error {
	user, exists := m.users[email]
	if !exists || user.Role != auth.RoleModerator {
		return auth.ErrModeratorNotFound
	}
	delete(m.users, email)
	delete(m.passwords, email)
	return nil
}

func TestRegistrationLoginAndAuthenticatedIdentity(t *testing.T) {
	repository := newMemoryAuthRepository()
	mailer := &memoryEmailService{}
	service := auth.NewService(repository, mailer)
	handler := NewServer(nil, service)

	requestOTP := httptest.NewRequest(http.MethodPost, "/api/auth/request-otp", strings.NewReader(
		`{"email":"Student@iitk.ac.in","password":"long-secure-password","confirmPassword":"long-secure-password"}`,
	))
	requestOTPResponse := httptest.NewRecorder()
	handler.ServeHTTP(requestOTPResponse, requestOTP)
	if requestOTPResponse.Code != http.StatusAccepted {
		t.Fatalf("OTP request status = %d, want %d: %s", requestOTPResponse.Code, http.StatusAccepted, requestOTPResponse.Body.String())
	}

	verify := httptest.NewRequest(http.MethodPost, "/api/auth/verify-otp", strings.NewReader(
		`{"email":"student@iitk.ac.in","code":"`+mailer.code+`"}`,
	))
	verifyResponse := httptest.NewRecorder()
	handler.ServeHTTP(verifyResponse, verify)
	if verifyResponse.Code != http.StatusOK {
		t.Fatalf("verification status = %d, want %d: %s", verifyResponse.Code, http.StatusOK, verifyResponse.Body.String())
	}
	var user auth.User
	if err := json.NewDecoder(verifyResponse.Body).Decode(&user); err != nil {
		t.Fatalf("decode verification response: %v", err)
	}
	if user.Role != auth.RoleStudent || user.Email != "student@iitk.ac.in" {
		t.Fatalf("unexpected verified user: %+v", user)
	}
	if strings.Contains(verifyResponse.Body.String(), "password") {
		t.Fatal("authentication response must not include password data")
	}

	me := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	me.Header.Set("Cookie", verifyResponse.Header().Get("Set-Cookie"))
	meResponse := httptest.NewRecorder()
	handler.ServeHTTP(meResponse, me)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("identity status = %d, want %d", meResponse.Code, http.StatusOK)
	}

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(
		`{"email":"student@iitk.ac.in","password":"long-secure-password"}`,
	))
	loginResponse := httptest.NewRecorder()
	handler.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d: %s", loginResponse.Code, http.StatusOK, loginResponse.Body.String())
	}
}

func TestProtectedRoutesRequireRole(t *testing.T) {
	service := auth.NewService(newMemoryAuthRepository())
	studentSession, err := service.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(nil, service)

	noToken := httptest.NewRequest(http.MethodPost, "/api/v1/admin/moderators", strings.NewReader(
		`{"email":"moderator@college.edu","password":"long-secure-password"}`,
	))
	noTokenResponse := httptest.NewRecorder()
	handler.ServeHTTP(noTokenResponse, noToken)
	if noTokenResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing-token status = %d, want %d", noTokenResponse.Code, http.StatusUnauthorized)
	}

	roleEscalation := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(
		`{"email":"attacker@college.edu","password":"long-secure-password","role":"administrator"}`,
	))
	roleEscalationResponse := httptest.NewRecorder()
	handler.ServeHTTP(roleEscalationResponse, roleEscalation)
	if roleEscalationResponse.Code != http.StatusNotFound {
		t.Fatalf("direct registration status = %d, want %d", roleEscalationResponse.Code, http.StatusNotFound)
	}

	studentRequest := httptest.NewRequest(http.MethodPost, "/api/v1/admin/moderators", strings.NewReader(
		`{"email":"moderator@college.edu","password":"long-secure-password"}`,
	))
	studentRequest.Header.Set("Authorization", "Bearer "+studentSession.AccessToken)
	studentResponse := httptest.NewRecorder()
	handler.ServeHTTP(studentResponse, studentRequest)
	if studentResponse.Code != http.StatusForbidden {
		t.Fatalf("student moderator-provision status = %d, want %d", studentResponse.Code, http.StatusForbidden)
	}
}

func TestRoleSpecificLoginRejectsWrongRole(t *testing.T) {
	repository := newMemoryAuthRepository()
	service := auth.NewService(repository)
	if _, err := service.RegisterStudent(context.Background(), "student@iitk.ac.in", "studentpass"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ProvisionModerator(context.Background(), "moderator@example.com", "modpass1"); err != nil {
		t.Fatal(err)
	}
	handler := NewServer(nil, service)

	wrongRole := httptest.NewRequest(http.MethodPost, "/api/auth/student/login", strings.NewReader(
		`{"email":"moderator@example.com","password":"modpass1"}`,
	))
	wrongRoleResponse := httptest.NewRecorder()
	handler.ServeHTTP(wrongRoleResponse, wrongRole)
	if wrongRoleResponse.Code != http.StatusForbidden {
		t.Fatalf("wrong-role status = %d, want %d", wrongRoleResponse.Code, http.StatusForbidden)
	}

	correctRole := httptest.NewRequest(http.MethodPost, "/api/auth/moderator/login", strings.NewReader(
		`{"email":"moderator@example.com","password":"modpass1"}`,
	))
	correctRoleResponse := httptest.NewRecorder()
	handler.ServeHTTP(correctRoleResponse, correctRole)
	if correctRoleResponse.Code != http.StatusOK {
		t.Fatalf("correct-role status = %d, want %d: %s", correctRoleResponse.Code, http.StatusOK, correctRoleResponse.Body.String())
	}
}

func TestAdministratorCanProvisionModerator(t *testing.T) {
	service := auth.NewService(newMemoryAuthRepository())
	if _, err := service.CreateAdministrator(context.Background(), "admin@college.edu", "long-secure-password"); err != nil {
		t.Fatal(err)
	}
	adminSession, err := service.Login(context.Background(), "admin@college.edu", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(nil, service)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/moderators", strings.NewReader(
		`{"email":"moderator@college.edu","password":"moderator-secure-password"}`,
	))
	request.Header.Set("Authorization", "Bearer "+adminSession.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("provision status = %d, want %d: %s", response.Code, http.StatusCreated, response.Body.String())
	}

	var moderator auth.User
	if err := json.NewDecoder(response.Body).Decode(&moderator); err != nil {
		t.Fatalf("decode moderator response: %v", err)
	}
	if moderator.Role != auth.RoleModerator {
		t.Fatalf("provisioned role = %q, want %q", moderator.Role, auth.RoleModerator)
	}

	moderatorSession, err := service.Login(context.Background(), moderator.Email, "moderator-secure-password")
	if err != nil {
		t.Fatalf("moderator login: %v", err)
	}
	feedbackRequest := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(
		`{"courseId":"CS455","content":"Feedback"}`,
	))
	feedbackRequest.Header.Set("Authorization", "Bearer "+moderatorSession.AccessToken)
	feedbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(feedbackResponse, feedbackRequest)
	if feedbackResponse.Code != http.StatusForbidden {
		t.Fatalf("moderator feedback status = %d, want %d", feedbackResponse.Code, http.StatusForbidden)
	}
}

func TestOnlyAdministratorCanRemoveModerator(t *testing.T) {
	repository := newMemoryAuthRepository()
	service := auth.NewService(repository)
	if _, err := service.CreateAdministrator(context.Background(), "admin@example.com", "adminpass"); err != nil {
		t.Fatal(err)
	}
	moderator, err := service.ProvisionModerator(context.Background(), "moderator@example.com", "modpass1")
	if err != nil {
		t.Fatal(err)
	}
	adminSession, err := service.Login(context.Background(), "admin@example.com", "adminpass")
	if err != nil {
		t.Fatal(err)
	}
	handler := NewServer(nil, service)

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/moderators/moderator@example.com", nil)
	request.Header.Set("Authorization", "Bearer "+adminSession.AccessToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("remove status = %d, want %d: %s", response.Code, http.StatusNoContent, response.Body.String())
	}
	if _, err := service.Login(context.Background(), moderator.Email, "modpass1"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("removed moderator login error = %v, want ErrInvalidCredentials", err)
	}
}
