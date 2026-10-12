package httpapi

// Person A owns this file: the queue, claim and active-appeals handlers
// (SCRUM-10, SCRUM-11). Replace each stub body; keep the method names, because
// moderator_routes.go refers to them. Request and response shapes are in
// docs/moderator-api.md.
//
// Use the signed-in moderator with userFromContext(r.Context()).ID and the
// store with s.appealStore. Reuse writeJSON and decodeRequestJSON.

import (
	"errors"
	"log"
	"net/http"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
)

// listAppeals handles GET /api/v1/appeals?sort=asc|desc.
// Default sort is asc (oldest first). Reject any other sort value with 400.
// Responds 200 with {"appeals": []appeals.Summary}; an empty queue is an empty
// array, never null.
func (s *Server) listAppeals(w http.ResponseWriter, r *http.Request) {
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = appeals.SortOldestFirst
	}
	if sort != appeals.SortOldestFirst && sort != appeals.SortNewestFirst {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid sort"})
		return
	}

	items, err := s.appealStore.ListUnclaimed(r.Context(), sort)
	if err != nil {
		log.Printf("list appeals: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not list appeals"})
		return
	}

	writeJSON(w, http.StatusOK, map[string][]appeals.Summary{"appeals": items})
}

// listMyAppeals handles GET /api/v1/appeals/mine.
// Responds 200 with {"appeals": []appeals.Summary} for the signed-in moderator.
func (s *Server) listMyAppeals(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authenticated user unavailable"})
		return
	}

	items, err := s.appealStore.ListMine(r.Context(), user.ID)
	if err != nil {
		log.Printf("list my appeals: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not list appeals"})
		return
	}

	writeJSON(w, http.StatusOK, map[string][]appeals.Summary{"appeals": items})
}

// claimAppeal handles POST /api/v1/appeals/{id}/claim (read the id with
// r.PathValue("id")). Responds 200 with the claimed appeals.Summary,
// 409 {"error": "This ticket has already been claimed"} when
// appeals.ErrAlreadyClaimed, and 404 when appeals.ErrNotFound.
func (s *Server) claimAppeal(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authenticated user unavailable"})
		return
	}

	ticketID := r.PathValue("id")
	claimed, err := s.appealStore.Claim(r.Context(), ticketID, user.ID)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, claimed)
	case errors.Is(err, appeals.ErrAlreadyClaimed):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "This ticket has already been claimed"})
	case errors.Is(err, appeals.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": appeals.ErrNotFound.Error()})
	default:
		log.Printf("claim appeal: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not claim appeal"})
	}
}
