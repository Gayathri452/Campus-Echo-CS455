package httpapi

// Person B owns this file: the detail, release and resolve handlers
// (SCRUM-12, SCRUM-13). Replace each stub body; keep the method names, because
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

// getAppeal handles GET /api/v1/appeals/{id} (read the id with
// r.PathValue("id")). Responds 200 with the full appeals.Ticket only for the
// moderator holding an unexpired lock; 403 for appeals.ErrNotLockHolder (this
// includes an expired lock, so the page can show "no longer assigned to you")
// and 404 for appeals.ErrNotFound.
func (s *Server) getAppeal(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authenticated user unavailable"})
		return
	}
	ticket, err := s.appealStore.Get(r.Context(), r.PathValue("id"), user.ID)
	if err != nil {
		s.writeAppealResolveError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ticket)
}

// releaseAppeal handles POST /api/v1/appeals/{id}/release (no body).
// Responds 204 on success, 403 for appeals.ErrNotLockHolder.
func (s *Server) releaseAppeal(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authenticated user unavailable"})
		return
	}
	if err := s.appealStore.Release(r.Context(), r.PathValue("id"), user.ID); err != nil {
		s.writeAppealResolveError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveAppeal handles POST /api/v1/appeals/{id}/resolve with
// {"action": "restore" | "delete", "comment": "..."}. Decode it with
// decodeRequestJSON. Responds 204 on success; 400 for appeals.ErrInvalidAction
// or appeals.ErrCommentRequired (delete needs a non-empty comment); 403 for
// appeals.ErrNotLockHolder.
func (s *Server) resolveAppeal(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action  string `json:"action"`
		Comment string `json:"comment"`
	}
	if err := decodeRequestJSON(w, r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one valid JSON object"})
		return
	}
	user, ok := userFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authenticated user unavailable"})
		return
	}
	if err := s.appealStore.Resolve(r.Context(), r.PathValue("id"), user.ID, input.Action, input.Comment); err != nil {
		s.writeAppealResolveError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeAppealResolveError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, appeals.ErrNotLockHolder):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "This appeal is no longer assigned to you"})
	case errors.Is(err, appeals.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "appeal ticket not found"})
	case errors.Is(err, appeals.ErrInvalidAction), errors.Is(err, appeals.ErrCommentRequired):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		log.Printf("moderator appeal operation: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not complete appeal operation"})
	}
}
