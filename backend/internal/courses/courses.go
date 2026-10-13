// Package courses holds the administrator-managed course catalog: the list of
// courses students pick from when they search for or review a course.
//
// The catalog only offers suggestions. Review submission does not require a
// catalog course, so existing behaviour is unchanged.
package courses

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxCourseIDLength   = 20
	MaxTitleLength      = 160
	MaxInstructorLength = 120
	// MaxListLimit caps how many courses one list request returns.
	MaxListLimit = 500
)

var (
	ErrInvalidCourseID   = errors.New("courseId must be 1-20 letters, digits, spaces or hyphens")
	ErrInvalidTitle      = errors.New("title must not be empty and must be at most 160 characters")
	ErrInvalidInstructor = errors.New("instructor must be at most 120 characters")
	ErrCourseExists      = errors.New("a course with this courseId already exists")
	ErrCourseNotFound    = errors.New("course was not found")
)

var courseIDPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9 -]*$`)

// Course is one catalog entry. CourseID is stored upper-case (for example
// "CS455") so lookups do not depend on how an administrator typed it.
type Course struct {
	CourseID   string    `json:"courseId" bson:"courseId"`
	Title      string    `json:"title" bson:"title"`
	Instructor string    `json:"instructor,omitempty" bson:"instructor,omitempty"`
	CreatedAt  time.Time `json:"createdAt" bson:"createdAt"`
}

// CreateInput is the administrator's request body.
type CreateInput struct {
	CourseID   string `json:"courseId"`
	Title      string `json:"title"`
	Instructor string `json:"instructor"`
}

// Store is implemented by *database.Mongo.
type Store interface {
	ListCourses(ctx context.Context, search string, limit int) ([]Course, error)
	CreateCourse(ctx context.Context, course Course) (Course, error)
	DeleteCourse(ctx context.Context, courseID string) error
}

// NormalizeCourseID trims, collapses inner spaces and upper-cases an ID.
func NormalizeCourseID(courseID string) string {
	return strings.ToUpper(strings.Join(strings.Fields(courseID), " "))
}

// Normalize trims every field and upper-cases the course ID.
func Normalize(input CreateInput) CreateInput {
	input.CourseID = NormalizeCourseID(input.CourseID)
	input.Title = strings.Join(strings.Fields(input.Title), " ")
	input.Instructor = strings.Join(strings.Fields(input.Instructor), " ")
	return input
}

// Validate checks a normalized input.
func Validate(input CreateInput) error {
	if input.CourseID == "" || utf8.RuneCountInString(input.CourseID) > MaxCourseIDLength || !courseIDPattern.MatchString(input.CourseID) {
		return ErrInvalidCourseID
	}
	if input.Title == "" || utf8.RuneCountInString(input.Title) > MaxTitleLength {
		return ErrInvalidTitle
	}
	if utf8.RuneCountInString(input.Instructor) > MaxInstructorLength {
		return ErrInvalidInstructor
	}
	return nil
}
