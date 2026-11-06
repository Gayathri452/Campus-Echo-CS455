package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memoryRepository struct {
	users    map[string]User
	password map[string]string
	sessions map[string]string
	otps     map[string]OTPChallenge
	otpMutex sync.Mutex
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

func (m *memoryRepository) ReserveOTPAttempt(_ context.Context, email string, now time.Time) (OTPChallenge, error) {
	m.otpMutex.Lock()
	defer m.otpMutex.Unlock()
	challenge, exists := m.otps[email]
	if !exists || now.After(challenge.ExpiresAt) {
		return OTPChallenge{}, ErrInvalidOTP
	}
	if challenge.Attempts >= OTPMaxAttempts {
		return OTPChallenge{}, ErrOTPAttempts
	}
	challenge.Attempts++
	m.otps[email] = challenge
	return challenge, nil
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

func TestLoginRateLimitsRepeatedFailuresAndResetsAfterSuccess(t *testing.T) {
	repository := newMemoryRepository()
	service := NewService(repository)
	if _, err := service.RegisterStudent(context.Background(), "student@iitk.ac.in", "long-secure-password"); err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < 5; attempt++ {
		if _, err := service.Login(context.Background(), "student@iitk.ac.in", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("failed login %d error = %v, want ErrInvalidCredentials", attempt+1, err)
		}
	}
	if _, err := service.Login(context.Background(), "student@iitk.ac.in", "wrong-password"); !errors.Is(err, ErrLoginRateLimited) {
		t.Fatalf("sixth failed login error = %v, want ErrLoginRateLimited", err)
	}
	if _, err := service.Login(context.Background(), "student@iitk.ac.in", "long-secure-password"); !errors.Is(err, ErrLoginRateLimited) {
		t.Fatalf("valid login while limited error = %v, want ErrLoginRateLimited", err)
	}
}

func TestPrivilegedAccountsRejectShortPasswords(t *testing.T) {
	service := NewService(newMemoryRepository())
	if _, err := service.CreateAdministrator(context.Background(), "admin@example.com", "sixsix"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("short administrator password error = %v, want ErrInvalidInput", err)
	}
	if _, err := service.ProvisionModerator(context.Background(), "moderator@example.com", "sixsix"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("short moderator password error = %v, want ErrInvalidInput", err)
	}
}

func TestOTPRequestsRateLimitRepeatedEmailRequests(t *testing.T) {
	service := NewService(newMemoryRepository(), &memoryEmailService{})
	for request := 0; request < 5; request++ {
		err := service.RequestOTP(context.Background(), "student@iitk.ac.in", "long-secure-password", "long-secure-password")
		if request == 0 {
			if err != nil {
				t.Fatalf("first OTP request error = %v", err)
			}
			continue
		}
		if !errors.Is(err, ErrOTPResendTooSoon) {
			t.Fatalf("OTP request %d error = %v, want ErrOTPResendTooSoon", request+1, err)
		}
	}
	if err := service.RequestOTP(context.Background(), "student@iitk.ac.in", "long-secure-password", "long-secure-password"); !errors.Is(err, ErrOTPRequestRateLimited) {
		t.Fatalf("sixth OTP request error = %v, want ErrOTPRequestRateLimited", err)
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

func TestOTPVerificationReservesAtMostFiveConcurrentAttempts(t *testing.T) {
	repository := newMemoryRepository()
	email := "student@iitk.ac.in"
	repository.otps[email] = OTPChallenge{
		Email:     email,
		OTPHash:   hashOTP("123456"),
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	service := NewService(repository)

	const requests = 100
	results := make(chan error, requests)
	start := make(chan struct{})
	var done sync.WaitGroup
	done.Add(requests)
	for i := 0; i < requests; i++ {
		go func() {
			defer done.Done()
			<-start
			_, err := service.VerifyOTP(context.Background(), email, "000000")
			results <- err
		}()
	}
	close(start)
	done.Wait()
	close(results)

	invalidOTP := 0
	attemptsExceeded := 0
	for err := range results {
		switch {
		case errors.Is(err, ErrInvalidOTP):
			invalidOTP++
		case errors.Is(err, ErrOTPAttempts):
			attemptsExceeded++
		default:
			t.Fatalf("unexpected concurrent verification error: %v", err)
		}
	}
	if invalidOTP != OTPMaxAttempts {
		t.Fatalf("invalid OTP responses = %d, want %d", invalidOTP, OTPMaxAttempts)
	}
	if attemptsExceeded != requests-OTPMaxAttempts {
		t.Fatalf("attempt-limit responses = %d, want %d", attemptsExceeded, requests-OTPMaxAttempts)
	}
	if repository.otps[email].Attempts != OTPMaxAttempts {
		t.Fatalf("reserved attempts = %d, want %d", repository.otps[email].Attempts, OTPMaxAttempts)
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
