package announcements

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)

func TestNormalize(t *testing.T) {
	expires := time.Date(2026, 10, 20, 18, 0, 0, 0, time.FixedZone("IST", 5*3600+1800))
	got := Normalize(CreateInput{Title: "  Feedback   window ", Message: "  Line one\r\nLine two  ", CourseID: " cs455a ", ExpiresAt: &expires})
	if got.Title != "Feedback window" || got.Message != "Line one\nLine two" || got.CourseID != "CS455A" {
		t.Fatalf("Normalize = %+v", got)
	}
	if got.ExpiresAt.Location() != time.UTC || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("expiry = %v, want the same instant in UTC", got.ExpiresAt)
	}
}

func TestValidate(t *testing.T) {
	future := testNow.Add(48 * time.Hour)
	past := testNow.Add(-time.Minute)
	tooFar := testNow.Add(MaxLifetime + time.Hour)
	cases := []struct {
		name  string
		input CreateInput
		want  error
	}{
		{"valid without course or expiry", CreateInput{Title: "Welcome", Message: "Reviews are anonymous."}, nil},
		{"valid with course and expiry", CreateInput{Title: "Window open", Message: "Until Friday", CourseID: "CS455A", ExpiresAt: &future}, nil},
		{"empty title", CreateInput{Message: "x"}, ErrInvalidTitle},
		{"long title", CreateInput{Title: strings.Repeat("t", 121), Message: "x"}, ErrInvalidTitle},
		{"empty message", CreateInput{Title: "x"}, ErrInvalidMessage},
		{"long message", CreateInput{Title: "x", Message: strings.Repeat("m", 1001)}, ErrInvalidMessage},
		{"bad course", CreateInput{Title: "x", Message: "x", CourseID: "CS<455>"}, ErrInvalidCourseID},
		{"expired", CreateInput{Title: "x", Message: "x", ExpiresAt: &past}, ErrInvalidExpiry},
		{"too far", CreateInput{Title: "x", Message: "x", ExpiresAt: &tooFar}, ErrInvalidExpiry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(Normalize(tc.input), testNow); !errors.Is(err, tc.want) {
				t.Fatalf("Validate = %v, want %v", err, tc.want)
			}
		})
	}
}
