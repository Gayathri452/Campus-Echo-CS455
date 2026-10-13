package courses

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	got := Normalize(CreateInput{CourseID: "  cs   455 ", Title: "  Software   Engineering ", Instructor: " Prof.  Rao "})
	want := CreateInput{CourseID: "CS 455", Title: "Software Engineering", Instructor: "Prof. Rao"}
	if got != want {
		t.Fatalf("Normalize = %+v, want %+v", got, want)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name  string
		input CreateInput
		want  error
	}{
		{"valid", CreateInput{CourseID: "CS455", Title: "Software Engineering"}, nil},
		{"valid with hyphen and instructor", CreateInput{CourseID: "ESC-101A", Title: "Programming", Instructor: "Prof. Rao"}, nil},
		{"empty id", CreateInput{Title: "Software Engineering"}, ErrInvalidCourseID},
		{"id too long", CreateInput{CourseID: strings.Repeat("A", 21), Title: "x"}, ErrInvalidCourseID},
		{"id with symbols", CreateInput{CourseID: "CS455;DROP", Title: "x"}, ErrInvalidCourseID},
		{"id starting with hyphen", CreateInput{CourseID: "-CS455", Title: "x"}, ErrInvalidCourseID},
		{"empty title", CreateInput{CourseID: "CS455"}, ErrInvalidTitle},
		{"title too long", CreateInput{CourseID: "CS455", Title: strings.Repeat("t", 161)}, ErrInvalidTitle},
		{"instructor too long", CreateInput{CourseID: "CS455", Title: "x", Instructor: strings.Repeat("i", 121)}, ErrInvalidInstructor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(Normalize(tc.input)); !errors.Is(err, tc.want) {
				t.Fatalf("Validate = %v, want %v", err, tc.want)
			}
		})
	}
}
