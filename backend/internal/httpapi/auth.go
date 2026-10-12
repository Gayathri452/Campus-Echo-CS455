package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
)

type contextKey string

const userContextKey contextKey = "authenticated-user"

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type otpRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirmPassword"`
}

type otpVerification struct {
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

const sessionCookieName = "campusecho_session"

func (s *Server) requestOTP(w http.ResponseWriter, r *http.Request) {
	var input otpRequest
	if err := decodeRequestJSON(w, r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one valid JSON object"})
		return
	}
	if err := s.authService.RequestOTP(r.Context(), input.Email, input.Password, input.ConfirmPassword); err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "OTP sent if the email address is eligible"})
}

func (s *Server) verifyOTP(w http.ResponseWriter, r *http.Request) {
	var input otpVerification
	if err := decodeRequestJSON(w, r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one valid JSON object"})
		return
	}
	session, err := s.authService.VerifyOTP(r.Context(), input.Email, input.Code, input.Password)
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    session.AccessToken,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, session.User)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	s.loginAsRole(w, r, "")
}

func (s *Server) loginAsRole(w http.ResponseWriter, r *http.Request, role string) {
	var input credentials
	if err := decodeRequestJSON(w, r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one valid JSON object"})
		return
	}

	var session auth.Session
	var err error
	if role == "" {
		session, err = s.authService.Login(r.Context(), input.Email, input.Password)
	} else {
		session, err = s.authService.LoginAs(r.Context(), input.Email, input.Password, role)
	}
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) loginStudent(w http.ResponseWriter, r *http.Request) {
	s.loginAsRole(w, r, auth.RoleStudent)
}

func (s *Server) loginAdministrator(w http.ResponseWriter, r *http.Request) {
	s.loginAsRole(w, r, auth.RoleAdministrator)
}

func (s *Server) loginModerator(w http.ResponseWriter, r *http.Request) {
	s.loginAsRole(w, r, auth.RoleModerator)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	token, ok := sessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session cookie or bearer token required"})
		return
	}
	if err := s.authService.Logout(r.Context(), token); err != nil {
		s.writeAuthError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.cookieSecure, SameSite: http.SameSiteLaxMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) currentUser(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authenticated user unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) provisionModerator(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if err := decodeRequestJSON(w, r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one valid JSON object"})
		return
	}

	user, err := s.authService.ProvisionModerator(r.Context(), input.Email, input.Password)
	if err != nil {
		s.writeAuthError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

func (s *Server) removeModerator(w http.ResponseWriter, r *http.Request) {
	if err := s.authService.RemoveModerator(r.Context(), r.PathValue("email")); err != nil {
		s.writeAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := s.authenticateRequest(w, r)
		if !ok {
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey, user)))
	})
}

func (s *Server) requireRole(role string, next http.Handler) http.Handler {
	return s.requireUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, _ := userFromContext(r.Context())
		if user.Role != role {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "insufficient permissions"})
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (s *Server) authenticateRequest(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	token, ok := sessionToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session cookie or bearer token required"})
		return auth.User{}, false
	}
	user, err := s.authService.Authenticate(r.Context(), token)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidSession) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
			return auth.User{}, false
		}
		log.Printf("authenticate request: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not authenticate request"})
		return auth.User{}, false
	}
	return user, true
}

func (s *Server) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidInput):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email or password does not meet requirements"})
	case errors.Is(err, auth.ErrEmailNotAllowed):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only @iitk.ac.in email addresses are allowed"})
	case errors.Is(err, auth.ErrInvalidOTP):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired OTP"})
	case errors.Is(err, auth.ErrOTPAttempts), errors.Is(err, auth.ErrOTPResends), errors.Is(err, auth.ErrOTPResendTooSoon):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "OTP request limit reached; try again later"})
	case errors.Is(err, auth.ErrEmailService):
		log.Printf("email delivery: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "email delivery is not configured"})
	case errors.Is(err, auth.ErrPasswordSetup):
		log.Printf("student password setup: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "student password setup is unavailable"})
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
	case errors.Is(err, auth.ErrRoleMismatch):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "this account cannot use the selected login"})
	case errors.Is(err, auth.ErrInvalidSession):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
	case errors.Is(err, auth.ErrEmailAlreadyExists):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email is already registered"})
	case errors.Is(err, auth.ErrModeratorNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "moderator account not found"})
	default:
		log.Printf("authentication operation: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authentication operation failed"})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func sessionToken(r *http.Request) (string, bool) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, true
	}
	return bearerToken(r)
}

func userFromContext(ctx context.Context) (auth.User, bool) {
	user, ok := ctx.Value(userContextKey).(auth.User)
	return user, ok
}
