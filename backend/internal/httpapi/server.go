package httpapi

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

type Server struct {
	feedbackStore feedback.Store
	authService   *auth.Service
	appealStore   appeals.Store
}

// NewServer builds the API handler. Optional dependencies (see WithAppealStore)
// are passed as trailing options so existing callers keep compiling.
func NewServer(feedbackStore feedback.Store, authService *auth.Service, options ...Option) http.Handler {
	server := &Server{feedbackStore: feedbackStore, authService: authService}
	for _, option := range options {
		option(server)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", server.health)
	mux.HandleFunc("POST /api/v1/auth/register", server.registerStudent)
	mux.HandleFunc("POST /api/v1/auth/login", server.login)
	mux.HandleFunc("POST /api/v1/auth/logout", server.logout)
	mux.Handle("GET /api/v1/auth/me", server.requireUser(http.HandlerFunc(server.currentUser)))
	mux.Handle("POST /api/v1/admin/moderators", server.requireRole(auth.RoleAdministrator, http.HandlerFunc(server.provisionModerator)))
	mux.Handle("POST /api/v1/feedback", server.requireRole(auth.RoleStudent, http.HandlerFunc(server.createFeedback)))
	server.registerModeratorRoutes(mux)
	return mux
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON response: %v", err)
	}
}
