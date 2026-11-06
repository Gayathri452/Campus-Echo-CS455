package auth

import (
	"context"
	"errors"
	"testing"
)

type moderatorTestRepository struct {
	*memoryRepository
	deleteErr error
}

func (repository *moderatorTestRepository) DeleteModerator(_ context.Context, email string) error {
	if repository.deleteErr != nil {
		return repository.deleteErr
	}
	user, exists := repository.users[email]
	if !exists || user.Role != RoleModerator {
		return ErrModeratorNotFound
	}
	delete(repository.users, email)
	delete(repository.password, email)
	return nil
}

type repositoryWithoutModerator struct {
	Repository
}

type failingUserLookup struct {
	Repository
	err error
}

func (repository failingUserLookup) FindUserByEmail(context.Context, string) (User, string, error) {
	return User{}, "", repository.err
}

func TestRemoveModeratorChecksRoleAndRepositoryFailures(t *testing.T) {
	storeFailure := errors.New("injected user lookup failure")
	tests := []struct {
		name       string
		repository Repository
		email      string
		wantErr    error
		wantText   string
	}{
		{name: "malformed email", repository: &moderatorTestRepository{memoryRepository: newMemoryRepository()}, email: "not-an-email", wantErr: ErrModeratorNotFound},
		{name: "missing account", repository: &moderatorTestRepository{memoryRepository: newMemoryRepository()}, email: "missing@example.com", wantErr: ErrModeratorNotFound},
		{name: "wrong role", repository: &moderatorTestRepository{memoryRepository: &memoryRepository{users: map[string]User{"student@example.com": {Email: "student@example.com", Role: RoleStudent}}}}, email: "student@example.com", wantErr: ErrModeratorNotFound},
		{name: "lookup failure", repository: failingUserLookup{Repository: newMemoryRepository(), err: storeFailure}, email: "moderator@example.com", wantErr: ErrModeratorNotFound},
		{name: "moderator deletion unsupported", repository: repositoryWithoutModerator{Repository: moderatorWithRole()}, email: "moderator@example.com", wantText: "moderator repository is unavailable"},
		{name: "delete failure", repository: &moderatorTestRepository{memoryRepository: moderatorWithRole(), deleteErr: storeFailure}, email: "moderator@example.com", wantErr: storeFailure},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := NewService(test.repository).RemoveModerator(context.Background(), test.email)
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("RemoveModerator() error = %v, want %v", err, test.wantErr)
			}
			if test.wantText != "" && (err == nil || err.Error() != test.wantText) {
				t.Fatalf("RemoveModerator() error = %v, want %q", err, test.wantText)
			}
			if test.wantErr == nil && test.wantText == "" && err != nil {
				t.Fatalf("RemoveModerator() error = %v, want nil", err)
			}
		})
	}

	repository := &moderatorTestRepository{memoryRepository: moderatorWithRole()}
	if err := NewService(repository).RemoveModerator(context.Background(), "moderator@example.com"); err != nil {
		t.Fatalf("RemoveModerator() error = %v, want nil", err)
	}
	if _, exists := repository.users["moderator@example.com"]; exists {
		t.Fatal("RemoveModerator() left the moderator account in the repository")
	}
}

func moderatorWithRole() *memoryRepository {
	repository := newMemoryRepository()
	repository.users["moderator@example.com"] = User{Email: "moderator@example.com", Role: RoleModerator}
	return repository
}
