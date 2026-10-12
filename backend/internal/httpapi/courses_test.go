package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/courses"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

// courseFakeStore is a feedback store that also implements courses.Store, the
// way *database.Mongo does.
type courseFakeStore struct {
	courses    map[string]courses.Course
	lastSearch string
	lastLimit  int
	failWith   error
}

func newCourseFakeStore(initial ...courses.Course) *courseFakeStore {
	store := &courseFakeStore{courses: map[string]courses.Course{}}
	for _, course := range initial {
		store.courses[course.CourseID] = course
	}
	return store
}

func (s *courseFakeStore) Create(context.Context, feedback.CreateInput) (feedback.Feedback, error) {
	return feedback.Feedback{}, errors.New("not used by course tests")
}

func (s *courseFakeStore) ListCourses(_ context.Context, search string, limit int) ([]courses.Course, error) {
	s.lastSearch, s.lastLimit = search, limit
	if s.failWith != nil {
		return nil, s.failWith
	}
	var list []courses.Course
	for _, course := range s.courses {
		if search == "" || strings.Contains(strings.ToLower(course.CourseID+" "+course.Title), strings.ToLower(search)) {
			list = append(list, course)
		}
	}
	return list, nil
}

func (s *courseFakeStore) CreateCourse(_ context.Context, course courses.Course) (courses.Course, error) {
	if _, exists := s.courses[course.CourseID]; exists {
		return courses.Course{}, courses.ErrCourseExists
	}
	s.courses[course.CourseID] = course
	return course, nil
}

func (s *courseFakeStore) DeleteCourse(_ context.Context, courseID string) error {
	if _, exists := s.courses[courseID]; !exists {
		return courses.ErrCourseNotFound
	}
	delete(s.courses, courseID)
	return nil
}

type courseTestTokens struct {
	admin, moderator, student string
}

func newCourseTestServer(t *testing.T, store *courseFakeStore) (http.Handler, courseTestTokens) {
	t.Helper()
	ctx := context.Background()
	service := auth.NewService(newMemoryAuthRepository())
	const password = "long-secure-password"

	if _, err := service.CreateAdministrator(ctx, "courses-admin@iitk.ac.in", password); err != nil {
		t.Fatalf("create administrator: %v", err)
	}
	admin, err := service.Login(ctx, "courses-admin@iitk.ac.in", password)
	if err != nil {
		t.Fatalf("login administrator: %v", err)
	}
	if _, err := service.ProvisionModerator(ctx, "courses-moderator@iitk.ac.in", password); err != nil {
		t.Fatalf("provision moderator: %v", err)
	}
	moderator, err := service.Login(ctx, "courses-moderator@iitk.ac.in", password)
	if err != nil {
		t.Fatalf("login moderator: %v", err)
	}
	student, err := service.RegisterStudent(ctx, "courses-student@iitk.ac.in", password)
	if err != nil {
		t.Fatalf("register student: %v", err)
	}

	var feedbackStore feedback.Store
	if store != nil {
		feedbackStore = store
	}
	return NewServer(feedbackStore, service), courseTestTokens{
		admin: admin.AccessToken, moderator: moderator.AccessToken, student: student.AccessToken,
	}
}

func courseRequest(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestListCoursesForEverySignedInRole(t *testing.T) {
	store := newCourseFakeStore(courses.Course{CourseID: "CS455", Title: "Software Engineering"})
	handler, tokens := newCourseTestServer(t, store)

	if got := courseRequest(handler, http.MethodGet, "/api/v1/courses", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("no token = %d, want %d", got, http.StatusUnauthorized)
	}
	for role, token := range map[string]string{"student": tokens.student, "moderator": tokens.moderator, "administrator": tokens.admin} {
		response := courseRequest(handler, http.MethodGet, "/api/v1/courses", token, "")
		if response.Code != http.StatusOK {
			t.Fatalf("%s list = %d, body %s", role, response.Code, response.Body)
		}
		var body struct {
			Courses []courses.Course `json:"courses"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Courses) != 1 || body.Courses[0].CourseID != "CS455" {
			t.Fatalf("%s got %+v", role, body.Courses)
		}
	}
}

func TestListCoursesPassesSearchAndLimit(t *testing.T) {
	store := newCourseFakeStore()
	handler, tokens := newCourseTestServer(t, store)

	response := courseRequest(handler, http.MethodGet, "/api/v1/courses?search=%20soft%20&limit=20", tokens.student, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if store.lastSearch != "soft" || store.lastLimit != 20 {
		t.Fatalf("store got search %q limit %d", store.lastSearch, store.lastLimit)
	}
	if !strings.Contains(response.Body.String(), `"courses":[]`) {
		t.Fatalf("empty list body = %s, want []", response.Body)
	}
	for _, query := range []string{"?limit=0", "?limit=501", "?limit=many"} {
		if got := courseRequest(handler, http.MethodGet, "/api/v1/courses"+query, tokens.student, "").Code; got != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", query, got)
		}
	}
}

func TestCreateCourseIsAdministratorOnly(t *testing.T) {
	store := newCourseFakeStore()
	handler, tokens := newCourseTestServer(t, store)
	body := `{"courseId":"cs455","title":"Software Engineering"}`

	if got := courseRequest(handler, http.MethodPost, "/api/v1/admin/courses", tokens.student, body).Code; got != http.StatusForbidden {
		t.Errorf("student create = %d, want 403", got)
	}
	if got := courseRequest(handler, http.MethodPost, "/api/v1/admin/courses", tokens.moderator, body).Code; got != http.StatusForbidden {
		t.Errorf("moderator create = %d, want 403", got)
	}
	if len(store.courses) != 0 {
		t.Fatal("a non-administrator created a course")
	}
}

func TestCreateCourseNormalizesAndRejectsDuplicates(t *testing.T) {
	store := newCourseFakeStore()
	handler, tokens := newCourseTestServer(t, store)

	response := courseRequest(handler, http.MethodPost, "/api/v1/admin/courses", tokens.admin,
		`{"courseId":"  cs455 ","title":" Software   Engineering ","instructor":"Prof. Rao"}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("create = %d, body %s", response.Code, response.Body)
	}
	var created courses.Course
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.CourseID != "CS455" || created.Title != "Software Engineering" || created.Instructor != "Prof. Rao" {
		t.Fatalf("created = %+v", created)
	}

	duplicate := courseRequest(handler, http.MethodPost, "/api/v1/admin/courses", tokens.admin, `{"courseId":"CS455","title":"Again"}`)
	if duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", duplicate.Code)
	}
}

func TestCreateCourseValidation(t *testing.T) {
	handler, tokens := newCourseTestServer(t, newCourseFakeStore())
	for _, body := range []string{
		`{"courseId":"","title":"x"}`,
		`{"courseId":"CS455","title":""}`,
		`{"courseId":"CS<455>","title":"x"}`,
		`{"courseId":"CS455","title":"x","extra":true}`,
		`not json`,
	} {
		if got := courseRequest(handler, http.MethodPost, "/api/v1/admin/courses", tokens.admin, body).Code; got != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", body, got)
		}
	}
}

func TestDeleteCourse(t *testing.T) {
	store := newCourseFakeStore(courses.Course{CourseID: "CS455", Title: "Software Engineering"})
	handler, tokens := newCourseTestServer(t, store)

	if got := courseRequest(handler, http.MethodDelete, "/api/v1/admin/courses/CS455", tokens.student, "").Code; got != http.StatusForbidden {
		t.Fatalf("student delete = %d, want 403", got)
	}
	if got := courseRequest(handler, http.MethodDelete, "/api/v1/admin/courses/cs455", tokens.admin, "").Code; got != http.StatusNoContent {
		t.Fatalf("admin delete = %d, want 204", got)
	}
	if _, exists := store.courses["CS455"]; exists {
		t.Fatal("course was not deleted")
	}
	if got := courseRequest(handler, http.MethodDelete, "/api/v1/admin/courses/CS455", tokens.admin, "").Code; got != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", got)
	}
}

func TestCourseRoutesWithoutCatalogStore(t *testing.T) {
	handler, tokens := newCourseTestServer(t, nil)
	if got := courseRequest(handler, http.MethodGet, "/api/v1/courses", tokens.student, "").Code; got != http.StatusNotImplemented {
		t.Fatalf("list without store = %d, want 501", got)
	}
}

func TestCourseListErrorIsServerError(t *testing.T) {
	store := newCourseFakeStore()
	store.failWith = errors.New("boom")
	handler, tokens := newCourseTestServer(t, store)
	if got := courseRequest(handler, http.MethodGet, "/api/v1/courses", tokens.student, "").Code; got != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", got)
	}
}
