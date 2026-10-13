package httpapi

// Course catalog: administrators define the list of courses, and every
// signed-in user can read it so the student pages can suggest courses in a
// dropdown. The catalog is advisory: review submission does not check it.
//
//	GET    /api/v1/courses?search=cs&limit=50   any signed-in user
//	POST   /api/v1/admin/courses                administrator
//	DELETE /api/v1/admin/courses/{courseId}     administrator
//
// Like the existing feedback handlers, these find their store by checking
// whether the server's feedback store also implements courses.Store
// (*database.Mongo does), so no existing wiring changes.

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/courses"
)

func (s *Server) registerCourseRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/courses", s.requireUser(http.HandlerFunc(s.listCourses)))
	mux.Handle("POST /api/v1/admin/courses", s.requireRole(auth.RoleAdministrator, http.HandlerFunc(s.createCourse)))
	mux.Handle("DELETE /api/v1/admin/courses/{courseId}", s.requireRole(auth.RoleAdministrator, http.HandlerFunc(s.deleteCourse)))
}

func (s *Server) courseStore(w http.ResponseWriter) (courses.Store, bool) {
	store, ok := s.feedbackStore.(courses.Store)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "the course catalog is unavailable"})
	}
	return store, ok
}

func (s *Server) listCourses(w http.ResponseWriter, r *http.Request) {
	store, ok := s.courseStore(w)
	if !ok {
		return
	}
	limit := courses.MaxListLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > courses.MaxListLimit {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a whole number from 1 to 500"})
			return
		}
		limit = parsed
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	if len(search) > courses.MaxTitleLength {
		search = search[:courses.MaxTitleLength]
	}

	list, err := store.ListCourses(r.Context(), search, limit)
	if err != nil {
		log.Printf("list courses: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load courses"})
		return
	}
	if list == nil {
		list = []courses.Course{}
	}
	writeJSON(w, http.StatusOK, map[string][]courses.Course{"courses": list})
}

func (s *Server) createCourse(w http.ResponseWriter, r *http.Request) {
	store, ok := s.courseStore(w)
	if !ok {
		return
	}
	var input courses.CreateInput
	if err := decodeRequestJSON(w, r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one valid JSON object"})
		return
	}
	input = courses.Normalize(input)
	if err := courses.Validate(input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	created, err := store.CreateCourse(r.Context(), courses.Course{
		CourseID:   input.CourseID,
		Title:      input.Title,
		Instructor: input.Instructor,
	})
	if errors.Is(err, courses.ErrCourseExists) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		log.Printf("create course: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save the course"})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) deleteCourse(w http.ResponseWriter, r *http.Request) {
	store, ok := s.courseStore(w)
	if !ok {
		return
	}
	courseID := courses.NormalizeCourseID(r.PathValue("courseId"))
	if courseID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": courses.ErrInvalidCourseID.Error()})
		return
	}
	err := store.DeleteCourse(r.Context(), courseID)
	if errors.Is(err, courses.ErrCourseNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		log.Printf("delete course: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete the course"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
