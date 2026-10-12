package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	RoleStudent       = "student"
	RoleModerator     = "moderator"
	RoleAdministrator = "administrator"
	SessionDuration   = 12 * time.Hour
	OTPDuration       = 10 * time.Minute
	OTPMaxAttempts    = 5
	OTPMaxResends     = 5
	OTPResendCooldown = 60 * time.Second
	OTPLength         = 6
	minPasswordLength = 6
	maxPasswordBytes  = 72
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailAlreadyExists = errors.New("email is already registered")
	ErrInvalidSession     = errors.New("invalid or expired session")
	ErrInvalidInput       = errors.New("invalid authentication input")
	ErrInvalidOTP         = errors.New("invalid or expired OTP")
	ErrOTPAttempts        = errors.New("OTP attempts exceeded")
	ErrOTPResendTooSoon   = errors.New("OTP resend requested too soon")
	ErrOTPResends         = errors.New("OTP resend limit exceeded")
	ErrEmailNotAllowed    = errors.New("only IITK email addresses are allowed")
	ErrEmailService       = errors.New("email service is unavailable")
	ErrPasswordSetup      = errors.New("student password setup is unavailable")
	ErrModeratorNotFound  = errors.New("moderator account not found")
	ErrRoleMismatch       = errors.New("account role does not match this login")
)

type User struct {
	ID        string    `json:"id" bson:"_id,omitempty"`
	Email     string    `json:"email" bson:"email"`
	Role      string    `json:"role" bson:"role"`
	CreatedAt time.Time `json:"createdAt" bson:"createdAt"`
}

type Session struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresAt   time.Time `json:"expiresAt"`
	User        User      `json:"user"`
}

type OTPChallenge struct {
	Email        string    `bson:"email"`
	OTPHash      string    `bson:"otpHash"`
	PasswordHash string    `bson:"passwordHash"`
	ExpiresAt    time.Time `bson:"expiresAt"`
	Attempts     int       `bson:"attempts"`
	RequestedAt  time.Time `bson:"requestedAt"`
	ResendCount  int       `bson:"resendCount"`
}

type EmailService interface {
	SendOTP(context.Context, string, string) error
}

type Repository interface {
	CreateUser(context.Context, string, string, string) (User, error)
	FindUserByEmail(context.Context, string) (User, string, error)
	CreateSession(context.Context, string, string, time.Time) error
	FindUserBySession(context.Context, string, time.Time) (User, error)
	DeleteSession(context.Context, string) error
}

type otpRepository interface {
	FindOTP(context.Context, string) (OTPChallenge, error)
	InvalidateOTP(context.Context, string) error
	CreateOTP(context.Context, OTPChallenge) error
	IncrementOTPAttempts(context.Context, string) (int, error)
	DeleteOTP(context.Context, string) error
}

type studentRepository interface {
	CreateStudent(context.Context, string, string) (User, error)
}

type studentPasswordRepository interface {
	SetStudentPassword(context.Context, string, string) error
}

type moderatorRepository interface {
	DeleteModerator(context.Context, string) error
}

type Service struct {
	repository Repository
	email      EmailService
	now        func() time.Time
}

func NewService(repository Repository, emailServices ...EmailService) *Service {
	var email EmailService
	if len(emailServices) > 0 {
		email = emailServices[0]
	}
	return &Service{repository: repository, email: email, now: time.Now}
}

func (s *Service) RequestOTP(ctx context.Context, email, password, confirmPassword string) error {
	normalized, err := normalizeEmail(email)
	if err != nil || !isIITKEmail(normalized) {
		return ErrEmailNotAllowed
	}
	if password != confirmPassword || validatePassword(password) != nil {
		return ErrInvalidInput
	}
	store, ok := s.repository.(otpRepository)
	if !ok || s.email == nil {
		return ErrEmailService
	}
	existingUser, existingHash, findErr := s.repository.FindUserByEmail(ctx, normalized)
	if findErr == nil {
		if existingUser.Role != RoleStudent || existingHash != "" {
			return ErrEmailAlreadyExists
		}
	} else if !errors.Is(findErr, ErrInvalidCredentials) {
		return fmt.Errorf("check existing user: %w", findErr)
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash registration password: %w", err)
	}
	now := s.now().UTC()
	previous, findErr := store.FindOTP(ctx, normalized)
	if findErr != nil && !errors.Is(findErr, ErrInvalidOTP) {
		return fmt.Errorf("find previous OTP: %w", findErr)
	}
	if findErr == nil {
		if now.Before(previous.RequestedAt.Add(OTPResendCooldown)) {
			return ErrOTPResendTooSoon
		}
		if previous.ResendCount >= OTPMaxResends {
			return ErrOTPResends
		}
	}
	code, err := generateOTP()
	if err != nil {
		return fmt.Errorf("generate OTP: %w", err)
	}
	challenge := OTPChallenge{
		Email:        normalized,
		OTPHash:      hashOTP(code),
		PasswordHash: string(passwordHash),
		ExpiresAt:    now.Add(OTPDuration),
		RequestedAt:  now,
	}
	if findErr == nil {
		challenge.ResendCount = previous.ResendCount + 1
	}
	if err := store.InvalidateOTP(ctx, normalized); err != nil {
		return fmt.Errorf("invalidate previous OTP: %w", err)
	}
	if err := store.CreateOTP(ctx, challenge); err != nil {
		return fmt.Errorf("save OTP: %w", err)
	}
	if err := s.email.SendOTP(ctx, normalized, code); err != nil {
		_ = store.DeleteOTP(ctx, normalized)
		return fmt.Errorf("%w: %v", ErrEmailService, err)
	}
	return nil
}

func (s *Service) VerifyOTP(ctx context.Context, email, code string, registrationPasswords ...string) (Session, error) {
	normalized, err := normalizeEmail(email)
	if err != nil || !isIITKEmail(normalized) || len(code) != OTPLength || !allDigits(code) {
		return Session{}, ErrInvalidOTP
	}
	store, storeOK := s.repository.(otpRepository)
	students, studentsOK := s.repository.(studentRepository)
	if !storeOK || !studentsOK {
		return Session{}, ErrInvalidOTP
	}
	challenge, err := store.FindOTP(ctx, normalized)
	if err != nil || s.now().UTC().After(challenge.ExpiresAt) {
		return Session{}, ErrInvalidOTP
	}
	if challenge.Attempts >= OTPMaxAttempts {
		return Session{}, ErrOTPAttempts
	}
	if !secureOTPMatch(challenge.OTPHash, code) {
		attempts, incrementErr := store.IncrementOTPAttempts(ctx, normalized)
		if incrementErr != nil {
			return Session{}, fmt.Errorf("record OTP attempt: %w", incrementErr)
		}
		if attempts >= OTPMaxAttempts {
			return Session{}, ErrOTPAttempts
		}
		return Session{}, ErrInvalidOTP
	}
	registrationPassword := ""
	if len(registrationPasswords) > 0 {
		registrationPassword = registrationPasswords[0]
	}
	user, existingHash, err := s.repository.FindUserByEmail(ctx, normalized)
	if errors.Is(err, ErrInvalidCredentials) {
		passwordHash := challenge.PasswordHash
		if passwordHash == "" {
			if validatePassword(registrationPassword) != nil {
				return Session{}, ErrPasswordSetup
			}
			hash, hashErr := bcrypt.GenerateFromPassword([]byte(registrationPassword), bcrypt.DefaultCost)
			if hashErr != nil {
				return Session{}, fmt.Errorf("hash registration password: %w", hashErr)
			}
			passwordHash = string(hash)
		}
		if passwordHash == "" {
			return Session{}, ErrInvalidOTP
		}
		user, err = students.CreateStudent(ctx, normalized, passwordHash)
	} else if err == nil && user.Role == RoleStudent && existingHash == "" {
		passwords, ok := s.repository.(studentPasswordRepository)
		passwordHash := challenge.PasswordHash
		if passwordHash == "" {
			if validatePassword(registrationPassword) != nil {
				return Session{}, ErrPasswordSetup
			}
			hash, hashErr := bcrypt.GenerateFromPassword([]byte(registrationPassword), bcrypt.DefaultCost)
			if hashErr != nil {
				return Session{}, fmt.Errorf("hash registration password: %w", hashErr)
			}
			passwordHash = string(hash)
		}
		if !ok || passwordHash == "" {
			return Session{}, ErrPasswordSetup
		}
		if err := passwords.SetStudentPassword(ctx, normalized, passwordHash); err != nil {
			return Session{}, fmt.Errorf("set student password: %w", err)
		}
	} else if err == nil && existingHash == "" {
		return Session{}, ErrInvalidOTP
	}
	if err != nil {
		return Session{}, err
	}
	if err := store.DeleteOTP(ctx, normalized); err != nil {
		return Session{}, fmt.Errorf("consume OTP: %w", err)
	}
	return s.createSession(ctx, user)
}

func (s *Service) RegisterStudent(ctx context.Context, email, password string) (Session, error) {
	email, err := normalizeEmail(email)
	if err != nil || !isIITKEmail(email) || validatePassword(password) != nil {
		return Session{}, ErrInvalidInput
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Session{}, fmt.Errorf("hash student password: %w", err)
	}
	user, err := s.repository.CreateUser(ctx, email, string(hash), RoleStudent)
	if err != nil {
		return Session{}, err
	}

	return s.createSession(ctx, user)
}

func (s *Service) ProvisionModerator(ctx context.Context, email, password string) (User, error) {
	email, err := normalizeEmail(email)
	if err != nil || validatePassword(password) != nil {
		return User{}, ErrInvalidInput
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash moderator password: %w", err)
	}
	user, err := s.repository.CreateUser(ctx, email, string(hash), RoleModerator)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) RemoveModerator(ctx context.Context, email string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return ErrModeratorNotFound
	}
	user, _, err := s.repository.FindUserByEmail(ctx, email)
	if errors.Is(err, ErrInvalidCredentials) || err != nil || user.Role != RoleModerator {
		return ErrModeratorNotFound
	}
	moderators, ok := s.repository.(moderatorRepository)
	if !ok {
		return fmt.Errorf("moderator repository is unavailable")
	}
	if err := moderators.DeleteModerator(ctx, email); err != nil {
		return err
	}
	return nil
}

func (s *Service) CreateAdministrator(ctx context.Context, email, password string) (User, error) {
	email, err := normalizeEmail(email)
	if err != nil || validatePassword(password) != nil {
		return User{}, ErrInvalidInput
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, fmt.Errorf("hash administrator password: %w", err)
	}
	user, err := s.repository.CreateUser(ctx, email, string(hash), RoleAdministrator)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	email, err := normalizeEmail(email)
	if err != nil || password == "" || len(password) > maxPasswordBytes {
		return Session{}, ErrInvalidCredentials
	}

	user, hash, err := s.repository.FindUserByEmail(ctx, email)
	if err != nil {
		return Session{}, err
	}
	if hash == "" {
		return Session{}, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return Session{}, ErrInvalidCredentials
	}
	return s.createSession(ctx, user)
}

func (s *Service) LoginAs(ctx context.Context, email, password, role string) (Session, error) {
	session, err := s.Login(ctx, email, password)
	if err != nil {
		return Session{}, err
	}
	if session.User.Role != role {
		_ = s.Logout(ctx, session.AccessToken)
		return Session{}, ErrRoleMismatch
	}
	return session, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	if len(token) < 32 || len(token) > 128 {
		return User{}, ErrInvalidSession
	}
	user, err := s.repository.FindUserBySession(ctx, hashToken(token), s.now().UTC())
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if len(token) < 32 || len(token) > 128 {
		return ErrInvalidSession
	}
	if err := s.repository.DeleteSession(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Service) createSession(ctx context.Context, user User) (Session, error) {
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return Session{}, fmt.Errorf("generate session token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(rawToken)
	expiresAt := s.now().UTC().Add(SessionDuration)
	if err := s.repository.CreateSession(ctx, user.ID, hashToken(token), expiresAt); err != nil {
		return Session{}, fmt.Errorf("save session: %w", err)
	}
	return Session{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		User:        user,
	}, nil
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) > 254 || strings.ContainsAny(email, "\r\n") {
		return "", ErrInvalidInput
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || !strings.Contains(email, "@") {
		return "", ErrInvalidInput
	}
	return email, nil
}

func isIITKEmail(email string) bool {
	normalized, err := normalizeEmail(email)
	return err == nil && strings.HasSuffix(normalized, "@iitk.ac.in")
}

func generateOTP() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func hashOTP(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func secureOTPMatch(storedHash, code string) bool {
	provided := sha256.Sum256([]byte(code))
	decoded, err := hex.DecodeString(storedHash)
	return err == nil && len(decoded) == len(provided) && subtle.ConstantTimeCompare(decoded, provided[:]) == 1
}

func allDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < minPasswordLength || len(password) > maxPasswordBytes {
		return ErrInvalidInput
	}
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
