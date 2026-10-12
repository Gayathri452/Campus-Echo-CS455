package httpapi

import (
	"net/http"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
)

// Option configures optional Server dependencies.
type Option func(*Server)

// WithAppealStore supplies the moderator appeal store.
func WithAppealStore(store appeals.Store) Option {
	return func(s *Server) { s.appealStore = store }
}

// registerModeratorRoutes is the single place the moderator endpoints are
// registered. Do not add routes elsewhere, and do not edit server.go again for
// moderator work: change the handler in your own file instead.
//
// Every route here is moderator-only; the role check is enforced by the
// backend (NFR-8), not the UI. "mine" is more specific than "{id}", so Go's
// ServeMux routes GET /appeals/mine to listMyAppeals.
func (s *Server) registerModeratorRoutes(mux *http.ServeMux) {
	moderatorOnly := func(handler http.HandlerFunc) http.Handler {
		return s.requireRole(auth.RoleModerator, handler)
	}

	// Pages 1 and 3 (Person A) - appeals_queue.go
	mux.Handle("GET /api/v1/appeals", moderatorOnly(s.listAppeals))
	mux.Handle("GET /api/v1/appeals/mine", moderatorOnly(s.listMyAppeals))
	mux.Handle("POST /api/v1/appeals/{id}/claim", moderatorOnly(s.claimAppeal))

	// Page 2 (Person B) - appeals_resolve.go
	mux.Handle("GET /api/v1/appeals/{id}", moderatorOnly(s.getAppeal))
	mux.Handle("POST /api/v1/appeals/{id}/release", moderatorOnly(s.releaseAppeal))
	mux.Handle("POST /api/v1/appeals/{id}/resolve", moderatorOnly(s.resolveAppeal))
}

// writeNotImplemented is what scaffold stubs return until a handler is written.
func writeNotImplemented(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "not implemented"})
}
