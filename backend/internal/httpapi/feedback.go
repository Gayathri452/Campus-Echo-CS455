package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

const maxRequestBody = 16 * 1024

func (s *Server) createFeedback(w http.ResponseWriter, r *http.Request) {
	var input feedback.CreateInput
	if err := decodeRequestJSON(w, r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one valid JSON object"})
		return
	}

	input = feedback.Normalize(input)
	if err := feedback.Validate(input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": feedbackErrorMessage(err)})
		return
	}

	created, err := s.feedbackStore.Create(r.Context(), input)
	if err != nil {
		log.Printf("create feedback: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save feedback"})
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) exploreFeedback(w http.ResponseWriter, r *http.Request) {
	reader, ok := s.feedbackStore.(feedback.Reader)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "review exploration is unavailable"})
		return
	}
	rating, _ := strconv.Atoi(r.URL.Query().Get("rating"))
	results, err := reader.Explore(r.Context(), feedback.ExploreFilters{
		Search: r.URL.Query().Get("search"), Category: r.URL.Query().Get("category"),
		CourseID: r.URL.Query().Get("courseId"), Rating: rating,
	})
	if err != nil {
		log.Printf("explore feedback: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load published reviews"})
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) courseRatings(w http.ResponseWriter, r *http.Request) {
	reader, ok := s.feedbackStore.(feedback.Reader)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "course ratings are unavailable"})
		return
	}
	results, err := reader.CourseRatings(r.Context(), r.URL.Query().Get("courseId"))
	if err != nil {
		log.Printf("course ratings: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load course ratings"})
		return
	}
	writeJSON(w, http.StatusOK, results)
}

func (s *Server) reviewStatus(w http.ResponseWriter, r *http.Request) {
	reader, ok := s.feedbackStore.(feedback.Reader)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "review status is unavailable"})
		return
	}
	review, err := reader.FindByReference(r.Context(), r.PathValue("reference"))
	if errors.Is(err, feedback.ErrReviewNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "review reference was not found"})
		return
	}
	if err != nil {
		log.Printf("review status: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load review status"})
		return
	}
	writeJSON(w, http.StatusOK, review)
}

func (s *Server) createAppeal(w http.ResponseWriter, r *http.Request) {
	appeals, ok := s.feedbackStore.(feedback.AppealStore)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "appeals are unavailable"})
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if err := decodeRequestJSON(w, r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one valid JSON object"})
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || len(input.Reason) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "appeal reason must be between 1 and 2000 characters"})
		return
	}
	appeal, err := appeals.CreateAppeal(r.Context(), r.PathValue("reference"), input.Reason)
	if errors.Is(err, feedback.ErrAppealNotEligible) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if errors.Is(err, feedback.ErrReviewNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "review reference was not found"})
		return
	}
	if err != nil {
		log.Printf("create appeal: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create appeal"})
		return
	}
	writeJSON(w, http.StatusCreated, appeal)
}

func feedbackErrorMessage(err error) string {
	switch {
	case errors.Is(err, feedback.ErrInvalidCourseID):
		return feedback.ErrInvalidCourseID.Error()
	case errors.Is(err, feedback.ErrInvalidCourseTitle):
		return feedback.ErrInvalidCourseTitle.Error()
	case errors.Is(err, feedback.ErrInvalidCategory):
		return feedback.ErrInvalidCategory.Error()
	case errors.Is(err, feedback.ErrInvalidRating):
		return feedback.ErrInvalidRating.Error()
	case errors.Is(err, feedback.ErrInvalidContent):
		return feedback.ErrInvalidContent.Error()
	default:
		return "invalid feedback"
	}
}

func decodeRequestJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON value")
	}
	return nil
}
