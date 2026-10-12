// Package announcements holds administrator notices for students, such as
// "Mid-semester feedback for CS455A is open until Friday". Announcements are
// read-only for students and never touch reviews or student identity.
package announcements

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Kishore200524/CS455-Project/backend/internal/courses"
)

const (
	MaxTitleLength   = 120
	MaxMessageLength = 1000
	// MaxStudentList caps how many active announcements students receive.
	MaxStudentList = 50
	// MaxAdminList caps how many announcements the administrator view lists.
	MaxAdminList = 200
	// MaxLifetime is the furthest in the future an expiry may be set.
	MaxLifetime = 366 * 24 * time.Hour
)

var (
	ErrInvalidTitle    = errors.New("title must not be empty and must be at most 120 characters")
	ErrInvalidMessage  = errors.New("message must not be empty and must be at most 1000 characters")
	ErrInvalidCourseID = errors.New("courseId must be 1-20 letters, digits, spaces or hyphens")
	ErrInvalidExpiry   = errors.New("expiresAt must be in the future and at most one year away")
	ErrNotFound        = errors.New("announcement was not found")
)

// Announcement is one notice. CourseID is optional: empty means it applies to
// every course. ExpiresAt is optional: nil means it stays until removed.
type Announcement struct {
	ID        string     `json:"id" bson:"_id,omitempty"`
	Title     string     `json:"title" bson:"title"`
	Message   string     `json:"message" bson:"message"`
	CourseID  string     `json:"courseId,omitempty" bson:"courseId,omitempty"`
	Pinned    bool       `json:"pinned" bson:"pinned"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty" bson:"expiresAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt" bson:"createdAt"`
}

// CreateInput is the administrator's request body.
type CreateInput struct {
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	CourseID  string     `json:"courseId"`
	Pinned    bool       `json:"pinned"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// Store is implemented by *database.Mongo.
type Store interface {
	// ListAnnouncements returns pinned first, then newest first. With
	// activeOnly, announcements that expired before now are left out.
	ListAnnouncements(ctx context.Context, now time.Time, activeOnly bool, limit int) ([]Announcement, error)
	CreateAnnouncement(ctx context.Context, announcement Announcement) (Announcement, error)
	DeleteAnnouncement(ctx context.Context, id string) error
}

// Normalize trims the text fields and upper-cases the course ID. Line breaks
// inside the message are kept.
func Normalize(input CreateInput) CreateInput {
	input.Title = strings.Join(strings.Fields(input.Title), " ")
	input.Message = strings.TrimSpace(strings.ReplaceAll(input.Message, "\r\n", "\n"))
	input.CourseID = courses.NormalizeCourseID(input.CourseID)
	if input.ExpiresAt != nil {
		utc := input.ExpiresAt.UTC()
		input.ExpiresAt = &utc
	}
	return input
}

// Validate checks a normalized input at time now.
func Validate(input CreateInput, now time.Time) error {
	if input.Title == "" || utf8.RuneCountInString(input.Title) > MaxTitleLength {
		return ErrInvalidTitle
	}
	if input.Message == "" || utf8.RuneCountInString(input.Message) > MaxMessageLength {
		return ErrInvalidMessage
	}
	if input.CourseID != "" {
		if err := courses.Validate(courses.CreateInput{CourseID: input.CourseID, Title: "x"}); err != nil {
			return ErrInvalidCourseID
		}
	}
	if input.ExpiresAt != nil && (!input.ExpiresAt.After(now) || input.ExpiresAt.After(now.Add(MaxLifetime))) {
		return ErrInvalidExpiry
	}
	return nil
}
