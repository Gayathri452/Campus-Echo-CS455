package feedback

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxContentLength = 5000
	MaxCourseTitle   = 160
	MaxCategory      = 60
	MinRating        = 1
	MaxRating        = 5
	StatusSubmitted  = "submitted"
	StatusFlagged    = "flagged"
	StatusRejected   = "rejected"
	StatusPublished  = "published"
)

var (
	ErrInvalidCourseID    = errors.New("courseId must not be empty and must be at most 120 characters")
	ErrInvalidCourseTitle = errors.New("courseTitle must not be empty and must be at most 160 characters")
	ErrInvalidCategory    = errors.New("category must be one of teaching, content, assessment, workload, resources, or other")
	ErrInvalidRating      = errors.New("rating must be between 1 and 5")
	ErrInvalidContent     = errors.New("content must not be empty and must be at most 5000 characters")
	ErrReviewNotFound     = errors.New("review was not found")
	ErrAppealNotEligible  = errors.New("this review is not eligible for an appeal")
)

type CreateInput struct {
	CourseID    string `json:"courseId"`
	CourseTitle string `json:"courseTitle"`
	Category    string `json:"category"`
	Rating      int    `json:"rating"`
	Content     string `json:"content"`
}

type Feedback struct {
	ID            string     `json:"-" bson:"_id,omitempty"`
	ReferenceCode string     `json:"referenceCode" bson:"referenceCode"`
	CourseID      string     `json:"courseId" bson:"courseId"`
	CourseTitle   string     `json:"courseTitle" bson:"courseTitle"`
	Category      string     `json:"category" bson:"category"`
	Rating        int        `json:"rating" bson:"rating"`
	Content       string     `json:"content" bson:"content"`
	Status        string     `json:"status" bson:"status"`
	CreatedAt     time.Time  `json:"createdAt" bson:"createdAt"`
	PublishedAt   *time.Time `json:"publishedAt,omitempty" bson:"publishedAt,omitempty"`
}

type ExploreFilters struct {
	Search   string
	Category string
	CourseID string
	Rating   int
}

type CourseRating struct {
	CourseID      string  `json:"courseId"`
	CourseTitle   string  `json:"courseTitle"`
	ReviewCount   int64   `json:"reviewCount"`
	AverageRating float64 `json:"averageRating"`
}

type Appeal struct {
	ReferenceCode string    `json:"referenceCode" bson:"referenceCode"`
	Reason        string    `json:"reason" bson:"reason"`
	Status        string    `json:"status" bson:"status"`
	CreatedAt     time.Time `json:"createdAt" bson:"createdAt"`
}

type Store interface {
	Create(context.Context, CreateInput) (Feedback, error)
}

type Reader interface {
	Explore(context.Context, ExploreFilters) ([]Feedback, error)
	CourseRatings(context.Context, string) ([]CourseRating, error)
	FindByReference(context.Context, string) (Feedback, error)
}

type AppealStore interface {
	CreateAppeal(context.Context, string, string) (Appeal, error)
}

func Validate(input CreateInput) error {
	courseID := strings.TrimSpace(input.CourseID)
	if courseID == "" || utf8.RuneCountInString(courseID) > 120 {
		return ErrInvalidCourseID
	}
	if title := strings.TrimSpace(input.CourseTitle); title == "" || utf8.RuneCountInString(title) > MaxCourseTitle {
		return ErrInvalidCourseTitle
	}
	if !validCategories[strings.TrimSpace(input.Category)] {
		return ErrInvalidCategory
	}
	if input.Rating < MinRating || input.Rating > MaxRating {
		return ErrInvalidRating
	}

	content := strings.TrimSpace(input.Content)
	if content == "" || utf8.RuneCountInString(content) > MaxContentLength {
		return ErrInvalidContent
	}

	return nil
}

func Normalize(input CreateInput) CreateInput {
	input.CourseID = strings.TrimSpace(input.CourseID)
	input.CourseTitle = strings.TrimSpace(input.CourseTitle)
	input.Category = strings.TrimSpace(strings.ToLower(input.Category))
	input.Content = strings.TrimSpace(input.Content)
	return input
}

var validCategories = map[string]bool{
	"teaching":   true,
	"content":    true,
	"assessment": true,
	"workload":   true,
	"resources":  true,
	"other":      true,
}

func GenerateReferenceCode() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate review reference: %w", err)
	}
	return "CE-" + base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes), nil
}
