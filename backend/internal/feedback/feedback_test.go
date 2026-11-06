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
		{name: "valid submission", input: CreateInput{CourseID: "CS455", CourseTitle: "Software Engineering", Professor: "Prof. Rao", Year: 2026, Category: "teaching", Rating: 4, Content: "Helpful examples"}},
		{
			name:    "empty course",
			input:   CreateInput{CourseTitle: "Software Engineering", Professor: "Prof. Rao", Year: 2026, Category: "teaching", Rating: 4, Content: "Helpful examples"},
			wantErr: ErrInvalidCourseID,
		},
		{
			name:    "course too long",
			input:   CreateInput{CourseID: strings.Repeat("c", 121), CourseTitle: "Software Engineering", Professor: "Prof. Rao", Year: 2026, Category: "teaching", Rating: 4, Content: "Helpful examples"},
			wantErr: ErrInvalidCourseID,
		},
		{
			name:    "empty content",
			input:   CreateInput{CourseID: "CS455", CourseTitle: "Software Engineering", Professor: "Prof. Rao", Year: 2026, Category: "teaching", Rating: 4, Content: " \n "},
			wantErr: ErrInvalidContent,
		},
		{
			name:    "content too long",
			input:   CreateInput{CourseID: "CS455", CourseTitle: "Software Engineering", Professor: "Prof. Rao", Year: 2026, Category: "teaching", Rating: 4, Content: strings.Repeat("c", MaxContentLength+1)},
			wantErr: ErrInvalidContent,
		},
		{
			name:    "empty professor",
			input:   CreateInput{CourseID: "CS455", CourseTitle: "Software Engineering", Year: 2026, Category: "teaching", Rating: 4, Content: "Helpful examples"},
			wantErr: ErrInvalidProfessor,
		},
		{
			name:    "invalid year",
			input:   CreateInput{CourseID: "CS455", CourseTitle: "Software Engineering", Professor: "Prof. Rao", Year: 0, Category: "teaching", Rating: 4, Content: "Helpful examples"},
			wantErr: ErrInvalidYear,
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

func validCreateInput() CreateInput {
	return CreateInput{
		CourseID:    "CS455",
		CourseTitle: "Software Engineering",
		Professor:   "Prof. Rao",
		Year:        2026,
		Category:    "teaching",
		Rating:      4,
		Content:     "Helpful examples",
	}
}

func TestValidateRejectsEveryInvalidField(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*CreateInput)
		wantErr error
	}{
		{"empty title", func(input *CreateInput) { input.CourseTitle = " \t" }, ErrInvalidCourseTitle},
		{"title too long", func(input *CreateInput) { input.CourseTitle = strings.Repeat("t", MaxCourseTitle+1) }, ErrInvalidCourseTitle},
		{"professor too long", func(input *CreateInput) { input.Professor = strings.Repeat("p", MaxProfessorLength+1) }, ErrInvalidProfessor},
		{"year below minimum", func(input *CreateInput) { input.Year = MinAcademicYear - 1 }, ErrInvalidYear},
		{"year above maximum", func(input *CreateInput) { input.Year = MaxAcademicYear + 1 }, ErrInvalidYear},
		{"invalid category", func(input *CreateInput) { input.Category = "attendance" }, ErrInvalidCategory},
		{"rating below minimum", func(input *CreateInput) { input.Rating = MinRating - 1 }, ErrInvalidRating},
		{"rating above maximum", func(input *CreateInput) { input.Rating = MaxRating + 1 }, ErrInvalidRating},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validCreateInput()
			test.mutate(&input)
			if err := Validate(input); !errors.Is(err, test.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateUsesRuneLimits(t *testing.T) {
	input := validCreateInput()
	input.Content = strings.Repeat("界", MaxContentLength)
	if err := Validate(input); err != nil {
		t.Fatalf("Validate() rejected content at rune limit: %v", err)
	}

	input.Content += "界"
	if !errors.Is(Validate(input), ErrInvalidContent) {
		t.Fatal("Validate() accepted content above rune limit")
	}
}

func TestNormalizeTrimsAndCanonicalizesInput(t *testing.T) {
	normalized := Normalize(CreateInput{
		CourseID:    " CS455 ",
		CourseTitle: " Software Engineering ",
		Professor:   " Prof. Rao ",
		Year:        2026,
		Category:    "  TEACHING ",
		Rating:      4,
		Content:     " Helpful examples ",
	})

	want := validCreateInput()
	if normalized != want {
		t.Fatalf("Normalize() = %+v, want %+v", normalized, want)
	}
}

func TestGenerateReferenceCodeHasExpectedFormat(t *testing.T) {
	first, err := GenerateReferenceCode()
	if err != nil {
		t.Fatalf("GenerateReferenceCode() error = %v", err)
	}
	second, err := GenerateReferenceCode()
	if err != nil {
		t.Fatalf("second GenerateReferenceCode() error = %v", err)
	}
	if len(first) != 16 || !strings.HasPrefix(first, "CE-") {
		t.Fatalf("reference code %q has unexpected format", first)
	}
	if first == second {
		t.Fatalf("two reference codes were identical: %q", first)
	}
}
