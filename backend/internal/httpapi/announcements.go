package httpapi

// Announcements: administrators post notices that students see on their
// dashboard (for example a feedback window opening for a course).
//
//	GET    /api/v1/announcements             any signed-in user; active only
//	GET    /api/v1/admin/announcements       administrator; includes expired
//	POST   /api/v1/admin/announcements       administrator
//	DELETE /api/v1/admin/announcements/{id}  administrator
//
// Like the existing feedback handlers, these find their store by checking
// whether the server's feedback store also implements announcements.Store
// (*database.Mongo does), so no existing wiring changes.

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/announcements"
	"github.com/Kishore200524/CS455-Project/backend/internal/auth"
)

func (s *Server) registerAnnouncementRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/announcements", s.requireUser(http.HandlerFunc(s.listActiveAnnouncements)))
	mux.Handle("GET /api/v1/admin/announcements", s.requireRole(auth.RoleAdministrator, http.HandlerFunc(s.listAllAnnouncements)))
	mux.Handle("POST /api/v1/admin/announcements", s.requireRole(auth.RoleAdministrator, http.HandlerFunc(s.createAnnouncement)))
	mux.Handle("DELETE /api/v1/admin/announcements/{id}", s.requireRole(auth.RoleAdministrator, http.HandlerFunc(s.deleteAnnouncement)))
}

func (s *Server) announcementStore(w http.ResponseWriter) (announcements.Store, bool) {
	store, ok := s.feedbackStore.(announcements.Store)
	if !ok {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "announcements are unavailable"})
	}
	return store, ok
}

func (s *Server) listActiveAnnouncements(w http.ResponseWriter, r *http.Request) {
	s.writeAnnouncements(w, r, true, announcements.MaxStudentList)
}

func (s *Server) listAllAnnouncements(w http.ResponseWriter, r *http.Request) {
	s.writeAnnouncements(w, r, false, announcements.MaxAdminList)
}

func (s *Server) writeAnnouncements(w http.ResponseWriter, r *http.Request, activeOnly bool, limit int) {
	store, ok := s.announcementStore(w)
	if !ok {
		return
	}
	list, err := store.ListAnnouncements(r.Context(), time.Now().UTC(), activeOnly, limit)
	if err != nil {
		log.Printf("list announcements: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load announcements"})
		return
	}
	if list == nil {
		list = []announcements.Announcement{}
	}
	writeJSON(w, http.StatusOK, map[string][]announcements.Announcement{"announcements": list})
}

func (s *Server) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	store, ok := s.announcementStore(w)
	if !ok {
		return
	}
	var input announcements.CreateInput
	if err := decodeRequestJSON(w, r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request must contain one valid JSON object"})
		return
	}
	now := time.Now().UTC()
	input = announcements.Normalize(input)
	if err := announcements.Validate(input, now); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	created, err := store.CreateAnnouncement(r.Context(), announcements.Announcement{
		Title:     input.Title,
		Message:   input.Message,
		CourseID:  input.CourseID,
		Pinned:    input.Pinned,
		ExpiresAt: input.ExpiresAt,
		CreatedAt: now,
	})
	if err != nil {
		log.Printf("create announcement: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save the announcement"})
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) deleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	store, ok := s.announcementStore(w)
	if !ok {
		return
	}
	err := store.DeleteAnnouncement(r.Context(), r.PathValue("id"))
	if errors.Is(err, announcements.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		log.Printf("delete announcement: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete the announcement"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
