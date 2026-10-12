package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memoryRepository struct {
	users    map[string]User
	password map[string]string
	sessions map[string]string
	otps     map[string]OTPChallenge
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		users:    make(map[string]User),
		password: make(map[string]string),
		sessions: make(map[string]string),
		otps:     make(map[string]OTPChallenge),
	}
}

func (m *memoryRepository) CreateStudent(_ context.Context, email, passwordHash string) (User, error) {
	if user, exists := m.users[email]; exists {
		return user, nil
	}
	user := User{ID: email, Email: email, Role: RoleStudent, CreatedAt: time.Now().UTC()}
	m.users[email] = user
	m.password[email] = passwordHash
	return user, nil
}

func (m *memoryRepository) SetStudentPassword(_ context.Context, email, passwordHash string) error {
	user, exists := m.users[email]
	if !exists || user.Role != RoleStudent || m.password[email] != "" {
		return ErrInvalidCredentials
	}
	m.password[email] = passwordHash
	return nil
}

func (m *memoryRepository) FindOTP(_ context.Context, email string) (OTPChallenge, error) {
	challenge, exists := m.otps[email]
	if !exists {
		return OTPChallenge{}, ErrInvalidOTP
	}
	return challenge, nil
}

func (m *memoryRepository) InvalidateOTP(_ context.Context, email string) error {
	delete(m.otps, email)
	return nil
}

func (m *memoryRepository) CreateOTP(_ context.Context, challenge OTPChallenge) error {
	m.otps[challenge.Email] = challenge
	return nil
}

func (m *memoryRepository) IncrementOTPAttempts(_ context.Context, email string) (int, error) {
	challenge, exists := m.otps[email]
	if !exists {
		return 0, ErrInvalidOTP
	}
	challenge.Attempts++
	m.otps[email] = challenge
	return challenge.Attempts, nil
}

func (m *memoryRepository) DeleteOTP(_ context.Context, email string) error {
	delete(m.otps, email)
	return nil
}

type memoryEmailService struct {
	email string
	code  string
}

func (m *memoryEmailService) SendOTP(_ context.Context, email, code string) error {
	m.email = email
	m.code = code
	return nil
}

func (m *memoryRepository) CreateUser(_ context.Context, email, passwordHash, role string) (User, error) {
	if _, exists := m.users[email]; exists {
		return User{}, ErrEmailAlreadyExists
	}
	user := User{ID: email, Email: email, Role: role, CreatedAt: time.Now().UTC()}
	m.users[email] = user
	m.password[email] = passwordHash
	return user, nil
}

func (m *memoryRepository) FindUserByEmail(_ context.Context, email string) (User, string, error) {
	user, exists := m.users[email]
	if !exists {
		return User{}, "", ErrInvalidCredentials
	}
	return user, m.password[email], nil
}

func (m *memoryRepository) CreateSession(_ context.Context, userID, tokenHash string, _ time.Time) error {
	m.sessions[tokenHash] = userID
	return nil
}

func (m *memoryRepository) FindUserBySession(_ context.Context, tokenHash string, _ time.Time) (User, error) {
	userID, exists := m.sessions[tokenHash]
	if !exists {
		return User{}, ErrInvalidSession
	}
	return m.users[userID], nil
}

func (m *memoryRepository) DeleteSession(_ context.Context, tokenHash string) error {
	delete(m.sessions, tokenHash)
	return nil
}

func TestStudentRegistrationCreatesHashedPasswordAndSession(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(repository)

	session, err := service.RegisterStudent(context.Background(), "  Student@iitk.ac.in ", "long-secure-password")
	if err != nil {
		t.Fatalf("RegisterStudent() error = %v", err)
	}
	if session.User.Role != RoleStudent || session.User.Email != "student@iitk.ac.in" {
		t.Fatalf("unexpected user in session: %+v", session.User)
	}
	if repository.password[session.User.Email] == "long-secure-password" {
		t.Fatal("password was stored in plaintext")
	}
	if session.AccessToken == "" || session.TokenType != "Bearer" {
		t.Fatalf("invalid session response: %+v", session)
	}
	if _, err := service.Authenticate(context.Background(), session.AccessToken); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
}

func TestLoginRejectsInvalidPassword(t *testing.T) {
	service := NewService(newMemoryRepository())
	if _, err := service.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password"); err != nil {
		t.Fatal(err)
	}

	_, err := service.Login(context.Background(), "student@iitk.ac.in", "wrong-password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestModeratorProvisioningDoesNotIssuePublicStudentSession(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(repository)

	user, err := service.ProvisionModerator(context.Background(), "moderator@college.edu", "long-secure-password")
	if err != nil {
		t.Fatalf("ProvisionModerator() error = %v", err)
	}
	if user.Role != RoleModerator {
		t.Fatalf("role = %q, want %q", user.Role, RoleModerator)
	}
	if _, err := service.Authenticate(context.Background(), "not-a-valid-session-token"); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidSession", err)
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	service := NewService(newMemoryRepository())
	session, err := service.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(context.Background(), session.AccessToken); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	if _, err := service.Authenticate(context.Background(), session.AccessToken); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Authenticate() error = %v, want ErrInvalidSession", err)
	}
}

func TestRegistrationRejectsWeakPasswordAndInvalidEmail(t *testing.T) {
	service := NewService(newMemoryRepository())
	tests := []struct {
		email    string
		password string
	}{
		{email: "not-an-email", password: "long-secure-password"},
		{email: "student@college.edu", password: "short"},
	}
	for _, test := range tests {
		if _, err := service.RegisterStudent(context.Background(), test.email, test.password); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("RegisterStudent(%q) error = %v, want ErrInvalidInput", test.email, err)
		}
	}
}

func TestOTPVerificationCreatesStudentAndConsumesOTP(t *testing.T) {
	repository := newMemoryRepository()
	mailer := &memoryEmailService{}
	service := NewService(repository, mailer)

	if err := service.RequestOTP(context.Background(), "Student@iitk.ac.in", "long-secure-password", "long-secure-password"); err != nil {
		t.Fatalf("RequestOTP() error = %v", err)
	}
	if mailer.code == "" || repository.otps[mailer.email].OTPHash == mailer.code {
		t.Fatal("OTP must be sent separately from its stored hash")
	}

	session, err := service.VerifyOTP(context.Background(), mailer.email, mailer.code)
	if err != nil {
		t.Fatalf("VerifyOTP() error = %v", err)
	}
	if session.User.Role != RoleStudent || session.User.Email != mailer.email {
		t.Fatalf("unexpected verified user: %+v", session.User)
	}
	if _, err := service.Login(context.Background(), mailer.email, "long-secure-password"); err != nil {
		t.Fatalf("verified student password login: %v", err)
	}
	if _, exists := repository.otps[mailer.email]; exists {
		t.Fatal("OTP was not consumed after successful verification")
	}
	if _, err := service.Authenticate(context.Background(), session.AccessToken); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if _, err := service.VerifyOTP(context.Background(), mailer.email, mailer.code); !errors.Is(err, ErrInvalidOTP) {
		t.Fatalf("replayed VerifyOTP() error = %v, want ErrInvalidOTP", err)
	}
}

func TestOTPRejectsNonIITKEmail(t *testing.T) {
	service := NewService(newMemoryRepository(), &memoryEmailService{})
	if err := service.RequestOTP(context.Background(), "student@example.com", "long-secure-password", "long-secure-password"); !errors.Is(err, ErrEmailNotAllowed) {
		t.Fatalf("RequestOTP() error = %v, want ErrEmailNotAllowed", err)
	}
}

func TestOTPRegistrationSetsPasswordForLegacyStudent(t *testing.T) {
	repository := newMemoryRepository()
	repository.users["legacy@iitk.ac.in"] = User{
		ID:        "legacy-id",
		Email:     "legacy@iitk.ac.in",
		Role:      RoleStudent,
		CreatedAt: time.Now().UTC(),
	}
	mailer := &memoryEmailService{}
	service := NewService(repository, mailer)

	if err := service.RequestOTP(context.Background(), "legacy@iitk.ac.in", "sixsix", "sixsix"); err != nil {
		t.Fatalf("RequestOTP() error = %v", err)
	}
	if _, err := service.VerifyOTP(context.Background(), mailer.email, mailer.code); err != nil {
		t.Fatalf("VerifyOTP() error = %v", err)
	}
	if err := service.RequestOTP(context.Background(), "legacy@iitk.ac.in", "another", "another"); !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("second registration error = %v, want ErrEmailAlreadyExists", err)
	}
	if _, err := service.Login(context.Background(), "legacy@iitk.ac.in", "sixsix"); err != nil {
		t.Fatalf("legacy student login after password setup: %v", err)
	}
}
