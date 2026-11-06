package appeals

import (
	"errors"
	"testing"
)

func TestValidateResolutionEnforcesActionAndDeleteComment(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		comment string
		wantErr error
	}{
		{name: "restore without comment", action: ActionRestore},
		{name: "delete with comment", action: ActionDelete, comment: "The review was assessed and removed."},
		{name: "unsupported action", action: "publish", wantErr: ErrInvalidAction},
		{name: "delete with whitespace comment", action: ActionDelete, comment: " \t\n", wantErr: ErrCommentRequired},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateResolution(test.action, test.comment)
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("ValidateResolution() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && err != nil {
				t.Fatalf("ValidateResolution() error = %v, want nil", err)
			}
		})
	}
}
