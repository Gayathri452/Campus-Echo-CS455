package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kishore200524/CS455-Project/backend/internal/courses"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

// catalogFeedbackStore saves reviews and can look up catalog courses, the way
// *database.Mongo does.
type catalogFeedbackStore struct {
	catalog     map[string]courses.Course
	findErr     error
	created     feedback.CreateInput
	createCalls int
}

func (s *catalogFeedbackStore) Create(_ context.Context, input feedback.CreateInput) (feedback.Feedback, error) {
	s.created = input
	s.createCalls++
	return feedback.Feedback{
		ID:            "feedback-id",
		ReferenceCode: "CE-TEST123",
		CourseID:      input.CourseID,
		CourseTitle:   input.CourseTitle,
		Status:        feedback.StatusSubmitted,
	}, nil
}

func (s *catalogFeedbackStore) FindCourse(_ context.Context, courseID string) (courses.Course, error) {
	if s.findErr != nil {
		return courses.Course{}, s.findErr
	}
	course, ok := s.catalog[courseID]
	if !ok {
		return courses.Course{}, courses.ErrCourseNotFound
	}
	return course, nil
}

func newCatalogFeedbackStore() *catalogFeedbackStore {
	return &catalogFeedbackStore{catalog: map[string]courses.Course{
		"CS455": {CourseID: "CS455", Title: "Software Engineering"},
	}}
}

func postReview(t *testing.T, handler http.Handler, token, courseID string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"courseId":` + mustJSON(t, courseID) + `,"courseTitle":"Typed title","professor":"Prof. Rao","year":2026,"category":"teaching","rating":4,"content":"A useful course"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/feedback", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestCreateFeedbackUsesTheListedCourse(t *testing.T) {
	for _, typed := range []string{"CS455", "cs455", "  cs455 "} {
		store := newCatalogFeedbackStore()
		handler, tokens := newRoleTestServer(t, store)

		response := postReview(t, handler, tokens.student, typed)

		if response.Code != http.StatusCreated {
			t.Fatalf("%q: status = %d, want %d; body: %s", typed, response.Code, http.StatusCreated, response.Body.String())
		}
		if store.created.CourseID != "CS455" || store.created.CourseTitle != "Software Engineering" {
			t.Fatalf("%q: saved course = %q / %q, want CS455 / Software Engineering", typed, store.created.CourseID, store.created.CourseTitle)
		}
	}
}

func TestCreateFeedbackRejectsCoursesNotOnTheList(t *testing.T) {
	for _, typed := range []string{"CS 455", "C S455", "ABC999", ""} {
		store := newCatalogFeedbackStore()
		handler, tokens := newRoleTestServer(t, store)

		response := postReview(t, handler, tokens.student, typed)

		if response.Code != http.StatusBadRequest {
			t.Fatalf("%q: status = %d, want %d; body: %s", typed, response.Code, http.StatusBadRequest, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), "choose a course from the course list") {
			t.Fatalf("%q: body = %s, want the course list error", typed, response.Body.String())
		}
		if store.createCalls != 0 {
			t.Fatalf("%q: review was saved %d time(s), want 0", typed, store.createCalls)
		}
	}
}

func TestCreateFeedbackCourseLookupFailureIsServerError(t *testing.T) {
	store := newCatalogFeedbackStore()
	store.findErr = errors.New("database unavailable")
	handler, tokens := newRoleTestServer(t, store)

	response := postReview(t, handler, tokens.student, "CS455")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusInternalServerError, response.Body.String())
	}
	if store.createCalls != 0 {
		t.Fatalf("review was saved %d time(s), want 0", store.createCalls)
	}
}
