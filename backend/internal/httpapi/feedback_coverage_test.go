package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

type feedbackCoverageStore struct {
	createErr  error
	explore    []feedback.Feedback
	exploreErr error
	ratings    []feedback.CourseRating
	ratingsErr error
	review     feedback.Feedback
	reviewErr  error
	appeal     feedback.Appeal
	appealErr  error
}

func (store *feedbackCoverageStore) Create(_ context.Context, input feedback.CreateInput) (feedback.Feedback, error) {
	if store.createErr != nil {
		return feedback.Feedback{}, store.createErr
	}
	return feedback.Feedback{
		ID: "review-id", ReferenceCode: "CE-TEST123", CourseID: input.CourseID,
		CourseTitle: input.CourseTitle, Professor: input.Professor, Year: input.Year,
		Category: input.Category, Rating: input.Rating, Content: input.Content,
		Status: feedback.StatusSubmitted,
	}, nil
}

func (store *feedbackCoverageStore) Explore(context.Context, feedback.ExploreFilters) ([]feedback.Feedback, error) {
	return store.explore, store.exploreErr
}

func (store *feedbackCoverageStore) CourseRatings(context.Context, string) ([]feedback.CourseRating, error) {
	return store.ratings, store.ratingsErr
}

func (store *feedbackCoverageStore) FindByReference(context.Context, string) (feedback.Feedback, error) {
	return store.review, store.reviewErr
}

func (store *feedbackCoverageStore) CreateAppeal(context.Context, string, string) (feedback.Appeal, error) {
	return store.appeal, store.appealErr
}

type feedbackOnlyStore struct{}

func (feedbackOnlyStore) Create(context.Context, feedback.CreateInput) (feedback.Feedback, error) {
	return feedback.Feedback{}, nil
}

func feedbackCoverageHandler(t *testing.T, store feedback.Store) (http.Handler, string) {
	t.Helper()
	service := auth.NewService(newMemoryAuthRepository())
	session, err := service.RegisterStudent(context.Background(), "coverage@iitk.ac.in", "long-secure-password")
	if err != nil {
		t.Fatal(err)
	}
	return NewServer(store, service), session.AccessToken
}

func serveFeedbackRequest(handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestFeedbackErrorMessageCoversValidationErrors(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{feedback.ErrInvalidCourseID, feedback.ErrInvalidCourseID.Error()},
		{feedback.ErrInvalidCourseTitle, feedback.ErrInvalidCourseTitle.Error()},
		{feedback.ErrInvalidProfessor, feedback.ErrInvalidProfessor.Error()},
		{feedback.ErrInvalidYear, feedback.ErrInvalidYear.Error()},
		{feedback.ErrInvalidCategory, feedback.ErrInvalidCategory.Error()},
		{feedback.ErrInvalidRating, feedback.ErrInvalidRating.Error()},
		{feedback.ErrInvalidContent, feedback.ErrInvalidContent.Error()},
		{errors.New("other validation failure"), "invalid feedback"},
	}
	for _, test := range tests {
		if got := feedbackErrorMessage(test.err); got != test.want {
			t.Errorf("feedbackErrorMessage(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestFeedbackReaderEndpointsCoverOptionalAndErrorPaths(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		store      *feedbackCoverageStore
		wantStatus int
	}{
		{"explore empty normalizes to array", "/api/v1/feedback/explore?rating=bad", &feedbackCoverageStore{}, http.StatusOK},
		{"explore storage failure", "/api/v1/feedback/explore?rating=4", &feedbackCoverageStore{exploreErr: errors.New("private query error")}, http.StatusInternalServerError},
		{"ratings success", "/api/v1/feedback/ratings?courseId=CS455", &feedbackCoverageStore{ratings: []feedback.CourseRating{{CourseID: "CS455", ReviewCount: 1}}}, http.StatusOK},
		{"ratings storage failure", "/api/v1/feedback/ratings", &feedbackCoverageStore{ratingsErr: errors.New("private aggregate error")}, http.StatusInternalServerError},
		{"status not found", "/api/v1/feedback/status/missing", &feedbackCoverageStore{reviewErr: feedback.ErrReviewNotFound}, http.StatusNotFound},
		{"status storage failure", "/api/v1/feedback/status/error", &feedbackCoverageStore{reviewErr: errors.New("private lookup error")}, http.StatusInternalServerError},
		{"status success", "/api/v1/feedback/status/known", &feedbackCoverageStore{review: feedback.Feedback{ReferenceCode: "known", Status: feedback.StatusPublished}}, http.StatusOK},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, token := feedbackCoverageHandler(t, test.store)
			response := serveFeedbackRequest(handler, http.MethodGet, test.path, "", token)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.wantStatus, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "private") {
				t.Fatalf("response leaked internal store error: %s", response.Body.String())
			}
			if test.name == "explore empty normalizes to array" && response.Body.String() != "[]\n" {
				t.Fatalf("empty explore response = %q, want empty array", response.Body.String())
			}
		})
	}

	handler, token := feedbackCoverageHandler(t, feedbackOnlyStore{})
	for _, path := range []string{"/api/v1/feedback/explore", "/api/v1/feedback/ratings", "/api/v1/feedback/status/ref"} {
		response := serveFeedbackRequest(handler, http.MethodGet, path, "", token)
		if response.Code != http.StatusNotImplemented {
			t.Errorf("%s without a reader status = %d, want 501", path, response.Code)
		}
	}
}

func TestCreateFeedbackAndAppealFailurePaths(t *testing.T) {
	validBody := `{"courseId":"CS455","courseTitle":"Software Engineering","professor":"Prof. Rao","year":2026,"category":"teaching","rating":4,"content":"Useful examples."}`
	handler, token := feedbackCoverageHandler(t, &feedbackCoverageStore{createErr: errors.New("private insert error")})
	response := serveFeedbackRequest(handler, http.MethodPost, "/api/v1/feedback", validBody, token)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "private insert error") {
		t.Fatalf("create failure response = (%d, %s)", response.Code, response.Body.String())
	}

	for _, test := range []struct {
		name string
		err  error
		code int
	}{
		{"already appealed", feedback.ErrAppealAlreadyExists, http.StatusConflict},
		{"not eligible", feedback.ErrAppealNotEligible, http.StatusConflict},
		{"review missing", feedback.ErrReviewNotFound, http.StatusNotFound},
		{"store failure", errors.New("private appeal error"), http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, token := feedbackCoverageHandler(t, &feedbackCoverageStore{appealErr: test.err})
			response := serveFeedbackRequest(handler, http.MethodPost, "/api/v1/feedback/status/CE-TEST123/appeal", `{"reason":"Please reconsider this review."}`, token)
			if response.Code != test.code {
				t.Fatalf("status = %d, want %d; body: %s", response.Code, test.code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "private appeal error") {
				t.Fatalf("response leaked internal error: %s", response.Body.String())
			}
		})
	}
	handler, token = feedbackCoverageHandler(t, &feedbackCoverageStore{})
	for _, body := range []string{`{"reason":"  "}`, `{"reason":"` + strings.Repeat("x", 2001) + `"}`} {
		response := serveFeedbackRequest(handler, http.MethodPost, "/api/v1/feedback/status/CE-TEST123/appeal", body, token)
		if response.Code != http.StatusBadRequest {
			t.Errorf("invalid appeal body status = %d, want 400", response.Code)
		}
	}
}
