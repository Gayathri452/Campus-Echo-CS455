package feedback

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateInput
		wantErr error
	}{
		{name: "valid submission", input: CreateInput{CourseID: "CS455", CourseTitle: "Software Engineering", Category: "teaching", Rating: 4, Content: "Helpful examples"}},
		{
			name:    "empty course",
			input:   CreateInput{CourseTitle: "Software Engineering", Category: "teaching", Rating: 4, Content: "Helpful examples"},
			wantErr: ErrInvalidCourseID,
		},
		{
			name:    "course too long",
			input:   CreateInput{CourseID: strings.Repeat("c", 121), CourseTitle: "Software Engineering", Category: "teaching", Rating: 4, Content: "Helpful examples"},
			wantErr: ErrInvalidCourseID,
		},
		{
			name:    "empty content",
			input:   CreateInput{CourseID: "CS455", CourseTitle: "Software Engineering", Category: "teaching", Rating: 4, Content: " \n "},
			wantErr: ErrInvalidContent,
		},
		{
			name:    "content too long",
			input:   CreateInput{CourseID: "CS455", CourseTitle: "Software Engineering", Category: "teaching", Rating: 4, Content: strings.Repeat("c", MaxContentLength+1)},
			wantErr: ErrInvalidContent,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := Validate(test.input)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestFeedbackJSONOmitsSemanticEmbedding(t *testing.T) {
	body, err := json.Marshal(Feedback{
		SemanticEmbedding:      []float32{0.1, 0.2},
		SemanticEmbeddingModel: "test-model",
	})
	if err != nil {
		t.Fatalf("marshal feedback: %v", err)
	}
	if strings.Contains(string(body), "semanticEmbedding") || strings.Contains(string(body), "test-model") {
		t.Fatalf("feedback JSON exposed internal embedding fields: %s", body)
	}
}
