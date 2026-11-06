package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type otpFailureRepository struct {
	*memoryRepository
	findUserErr      error
	findOTPErr       error
	invalidateErr    error
	createOTPErr     error
	reserveOTPErr    error
	deleteOTPErr     error
	createStudentErr error
	setPasswordErr   error
	createSessionErr error
}

func (repository *otpFailureRepository) FindUserByEmail(ctx context.Context, email string) (User, string, error) {
	if repository.findUserErr != nil {
		return User{}, "", repository.findUserErr
	}
	return repository.memoryRepository.FindUserByEmail(ctx, email)
}

func (repository *otpFailureRepository) FindOTP(ctx context.Context, email string) (OTPChallenge, error) {
	if repository.findOTPErr != nil {
		return OTPChallenge{}, repository.findOTPErr
	}
	return repository.memoryRepository.FindOTP(ctx, email)
}

func (repository *otpFailureRepository) InvalidateOTP(ctx context.Context, email string) error {
	if repository.invalidateErr != nil {
		return repository.invalidateErr
	}
	return repository.memoryRepository.InvalidateOTP(ctx, email)
}

func (repository *otpFailureRepository) CreateOTP(ctx context.Context, challenge OTPChallenge) error {
	if repository.createOTPErr != nil {
		return repository.createOTPErr
	}
	return repository.memoryRepository.CreateOTP(ctx, challenge)
}

func (repository *otpFailureRepository) ReserveOTPAttempt(ctx context.Context, email string, now time.Time) (OTPChallenge, error) {
	if repository.reserveOTPErr != nil {
		return OTPChallenge{}, repository.reserveOTPErr
	}
	return repository.memoryRepository.ReserveOTPAttempt(ctx, email, now)
}

func (repository *otpFailureRepository) DeleteOTP(ctx context.Context, email string) error {
	if repository.deleteOTPErr != nil {
		return repository.deleteOTPErr
	}
	return repository.memoryRepository.DeleteOTP(ctx, email)
}

func (repository *otpFailureRepository) CreateStudent(ctx context.Context, email, passwordHash string) (User, error) {
	if repository.createStudentErr != nil {
		return User{}, repository.createStudentErr
	}
	return repository.memoryRepository.CreateStudent(ctx, email, passwordHash)
}

func (repository *otpFailureRepository) SetStudentPassword(ctx context.Context, email, passwordHash string) error {
	if repository.setPasswordErr != nil {
		return repository.setPasswordErr
	}
	return repository.memoryRepository.SetStudentPassword(ctx, email, passwordHash)
}

func (repository *otpFailureRepository) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	if repository.createSessionErr != nil {
		return repository.createSessionErr
	}
	return repository.memoryRepository.CreateSession(ctx, userID, tokenHash, expiresAt)
}

type failingOTPEmail struct{ err error }

func (email failingOTPEmail) SendOTP(context.Context, string, string) error { return email.err }

func TestRequestOTPReturnsRepositoryAndEmailFailures(t *testing.T) {
	storeFailure := errors.New("injected OTP repository failure")
	tests := []struct {
		name        string
		repository  *otpFailureRepository
		email       EmailService
		previous    OTPChallenge
		wantErr     error
		wantErrText string
	}{
		{name: "existing user lookup failure", repository: &otpFailureRepository{memoryRepository: newMemoryRepository(), findUserErr: storeFailure}, email: &memoryEmailService{}, wantErrText: "check existing user"},
		{name: "previous challenge lookup failure", repository: &otpFailureRepository{memoryRepository: newMemoryRepository(), findOTPErr: storeFailure}, email: &memoryEmailService{}, wantErrText: "find previous OTP"},
		{name: "invalidate previous challenge failure", repository: &otpFailureRepository{memoryRepository: newMemoryRepository(), invalidateErr: storeFailure}, email: &memoryEmailService{}, wantErrText: "invalidate previous OTP"},
		{name: "save challenge failure", repository: &otpFailureRepository{memoryRepository: newMemoryRepository(), createOTPErr: storeFailure}, email: &memoryEmailService{}, wantErrText: "save OTP"},
		{name: "email delivery failure", repository: &otpFailureRepository{memoryRepository: newMemoryRepository()}, email: failingOTPEmail{err: storeFailure}, wantErr: ErrEmailService},
		{name: "resend cooldown", repository: &otpFailureRepository{memoryRepository: newMemoryRepository()}, email: &memoryEmailService{}, previous: OTPChallenge{Email: "student@iitk.ac.in", RequestedAt: time.Unix(100, 0)}, wantErr: ErrOTPResendTooSoon},
		{name: "resend limit", repository: &otpFailureRepository{memoryRepository: newMemoryRepository()}, email: &memoryEmailService{}, previous: OTPChallenge{Email: "student@iitk.ac.in", RequestedAt: time.Unix(1, 0), ResendCount: OTPMaxResends}, wantErr: ErrOTPResends},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := time.Unix(100, 0).UTC()
			service := NewService(test.repository, test.email)
			service.now = func() time.Time { return now }
			if test.previous.Email != "" {
				test.repository.otps[test.previous.Email] = test.previous
			}
			err := service.RequestOTP(context.Background(), "student@iitk.ac.in", "strong-student-password", "strong-student-password")
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("RequestOTP() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErrText != "" && (err == nil || !containsText(err.Error(), test.wantErrText)) {
				t.Fatalf("RequestOTP() error = %v, want text %q", err, test.wantErrText)
			}
			if test.wantErr == nil && test.wantErrText == "" && err != nil {
				t.Fatalf("RequestOTP() error = %v, want nil", err)
			}
			if test.name == "email delivery failure" {
				if _, exists := test.repository.otps["student@iitk.ac.in"]; exists {
					t.Fatal("OTP challenge remained stored after email delivery failed")
				}
			}
		})
	}
}

func TestVerifyOTPReturnsRepositoryFailures(t *testing.T) {
	storeFailure := errors.New("injected OTP repository failure")
	now := time.Now().UTC()
	tests := []struct {
		name     string
		prepare  func(*otpFailureRepository)
		wantErr  error
		wantText string
	}{
		{
			name: "reserve attempt failure",
			prepare: func(repository *otpFailureRepository) {
				repository.reserveOTPErr = storeFailure
			},
			wantText: "reserve OTP attempt",
		},
		{
			name: "invalid code",
			prepare: func(repository *otpFailureRepository) {
				repository.otps["student@iitk.ac.in"] = OTPChallenge{OTPHash: hashOTP("654321"), ExpiresAt: now.Add(time.Minute)}
			},
			wantErr: ErrInvalidOTP,
		},
		{
			name: "student creation failure",
			prepare: func(repository *otpFailureRepository) {
				repository.createStudentErr = storeFailure
				repository.otps["student@iitk.ac.in"] = OTPChallenge{OTPHash: hashOTP("123456"), PasswordHash: "stored-hash", ExpiresAt: now.Add(time.Minute)}
			},
			wantErr: storeFailure,
		},
		{
			name: "legacy student password update failure",
			prepare: func(repository *otpFailureRepository) {
				repository.users["student@iitk.ac.in"] = User{Email: "student@iitk.ac.in", Role: RoleStudent}
				repository.otps["student@iitk.ac.in"] = OTPChallenge{OTPHash: hashOTP("123456"), PasswordHash: "stored-hash", ExpiresAt: now.Add(time.Minute)}
				repository.setPasswordErr = storeFailure
			},
			wantText: "set student password",
		},
		{
			name: "consume OTP failure",
			prepare: func(repository *otpFailureRepository) {
				repository.otps["student@iitk.ac.in"] = OTPChallenge{OTPHash: hashOTP("123456"), PasswordHash: "stored-hash", ExpiresAt: now.Add(time.Minute)}
				repository.deleteOTPErr = storeFailure
			},
			wantText: "consume OTP",
		},
		{
			name: "session persistence failure",
			prepare: func(repository *otpFailureRepository) {
				repository.users["student@iitk.ac.in"] = User{ID: "student-id", Email: "student@iitk.ac.in", Role: RoleStudent}
				repository.password["student@iitk.ac.in"] = "existing-hash"
				repository.otps["student@iitk.ac.in"] = OTPChallenge{OTPHash: hashOTP("123456"), ExpiresAt: now.Add(time.Minute)}
				repository.createSessionErr = storeFailure
			},
			wantText: "save session",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &otpFailureRepository{memoryRepository: newMemoryRepository()}
			test.prepare(repository)
			service := NewService(repository)
			service.now = func() time.Time { return now }
			_, err := service.VerifyOTP(context.Background(), "student@iitk.ac.in", "123456")
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("VerifyOTP() error = %v, want %v", err, test.wantErr)
			}
			if test.wantText != "" && (err == nil || !containsText(err.Error(), test.wantText)) {
				t.Fatalf("VerifyOTP() error = %v, want text %q", err, test.wantText)
			}
		})
	}
}

func containsText(value, part string) bool {
	return strings.Contains(value, part)
}
