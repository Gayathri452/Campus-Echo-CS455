package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/announcements"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
)

// announcementFakeStore is a feedback store that also implements
// announcements.Store, the way *database.Mongo does.
type announcementFakeStore struct {
	items          []announcements.Announcement
	lastActiveOnly bool
	lastLimit      int
	failWith       error
}

func (s *announcementFakeStore) Create(context.Context, feedback.CreateInput) (feedback.Feedback, error) {
	return feedback.Feedback{}, errors.New("not used by announcement tests")
}

func (s *announcementFakeStore) ListAnnouncements(_ context.Context, now time.Time, activeOnly bool, limit int) ([]announcements.Announcement, error) {
	s.lastActiveOnly, s.lastLimit = activeOnly, limit
	if s.failWith != nil {
		return nil, s.failWith
	}
	var list []announcements.Announcement
	for _, item := range s.items {
		if activeOnly && item.ExpiresAt != nil && !item.ExpiresAt.After(now) {
			continue
		}
		list = append(list, item)
	}
	return list, nil
}

func (s *announcementFakeStore) CreateAnnouncement(_ context.Context, item announcements.Announcement) (announcements.Announcement, error) {
	item.ID = fmt.Sprintf("a%d", len(s.items)+1)
	s.items = append(s.items, item)
	return item, nil
}

func (s *announcementFakeStore) DeleteAnnouncement(_ context.Context, id string) error {
	for i, item := range s.items {
		if item.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return nil
		}
	}
	return announcements.ErrNotFound
}

func decodeAnnouncements(t *testing.T, body string) []announcements.Announcement {
	t.Helper()
	var payload struct {
		Announcements []announcements.Announcement `json:"announcements"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Announcements
}

func newAnnouncementTestServer(t *testing.T, store *announcementFakeStore) (http.Handler, courseTestTokens) {
	t.Helper()
	var feedbackStore feedback.Store
	if store != nil {
		feedbackStore = store
	}
	return newRoleTestServer(t, feedbackStore)
}

func TestStudentsSeeOnlyActiveAnnouncements(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	future := time.Now().UTC().Add(time.Hour)
	store := &announcementFakeStore{items: []announcements.Announcement{
		{ID: "a1", Title: "Live", Message: "x"},
		{ID: "a2", Title: "Expired", Message: "x", ExpiresAt: &past},
		{ID: "a3", Title: "Until later", Message: "x", ExpiresAt: &future},
	}}
	handler, tokens := newAnnouncementTestServer(t, store)

	if got := courseRequest(handler, http.MethodGet, "/api/v1/announcements", "", "").Code; got != http.StatusUnauthorized {
		t.Fatalf("no token = %d, want 401", got)
	}
	response := courseRequest(handler, http.MethodGet, "/api/v1/announcements", tokens.student, "")
	if response.Code != http.StatusOK {
		t.Fatalf("student list = %d", response.Code)
	}
	list := decodeAnnouncements(t, response.Body.String())
	if len(list) != 2 || !store.lastActiveOnly || store.lastLimit != announcements.MaxStudentList {
		t.Fatalf("student got %+v (activeOnly %v, limit %d)", list, store.lastActiveOnly, store.lastLimit)
	}
}

func TestAdministratorSeesAllAnnouncements(t *testing.T) {
	past := time.Now().UTC().Add(-time.Hour)
	store := &announcementFakeStore{items: []announcements.Announcement{
		{ID: "a1", Title: "Live", Message: "x"},
		{ID: "a2", Title: "Expired", Message: "x", ExpiresAt: &past},
	}}
	handler, tokens := newAnnouncementTestServer(t, store)

	if got := courseRequest(handler, http.MethodGet, "/api/v1/admin/announcements", tokens.student, "").Code; got != http.StatusForbidden {
		t.Fatalf("student admin list = %d, want 403", got)
	}
	response := courseRequest(handler, http.MethodGet, "/api/v1/admin/announcements", tokens.admin, "")
	if response.Code != http.StatusOK || len(decodeAnnouncements(t, response.Body.String())) != 2 || store.lastActiveOnly {
		t.Fatalf("admin list = %d %s", response.Code, response.Body)
	}
}

func TestCreateAnnouncement(t *testing.T) {
	store := &announcementFakeStore{}
	handler, tokens := newAnnouncementTestServer(t, store)
	expires := time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339)
	body := `{"title":"  Feedback window open ","message":"Share your CS455A feedback by Friday.","courseId":"cs455a","pinned":true,"expiresAt":"` + expires + `"}`

	for name, token := range map[string]string{"student": tokens.student, "moderator": tokens.moderator} {
		if got := courseRequest(handler, http.MethodPost, "/api/v1/admin/announcements", token, body).Code; got != http.StatusForbidden {
			t.Fatalf("%s create = %d, want 403", name, got)
		}
	}
	response := courseRequest(handler, http.MethodPost, "/api/v1/admin/announcements", tokens.admin, body)
	if response.Code != http.StatusCreated {
		t.Fatalf("admin create = %d, body %s", response.Code, response.Body)
	}
	var created announcements.Announcement
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Title != "Feedback window open" || created.CourseID != "CS455A" || !created.Pinned || created.ExpiresAt == nil || created.CreatedAt.IsZero() {
		t.Fatalf("created = %+v", created)
	}
}

func TestCreateAnnouncementValidation(t *testing.T) {
	handler, tokens := newAnnouncementTestServer(t, &announcementFakeStore{})
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	for _, body := range []string{
		`{"title":"","message":"x"}`,
		`{"title":"x","message":""}`,
		`{"title":"x","message":"x","courseId":"CS<455>"}`,
		`{"title":"x","message":"x","expiresAt":"` + past + `"}`,
		`{"title":"x","message":"x","expiresAt":"tomorrow"}`,
		`{"title":"x","message":"x","audience":"all"}`,
		`{"title":"` + strings.Repeat("t", 121) + `","message":"x"}`,
	} {
		if got := courseRequest(handler, http.MethodPost, "/api/v1/admin/announcements", tokens.admin, body).Code; got != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", body, got)
		}
	}
}

func TestDeleteAnnouncement(t *testing.T) {
	store := &announcementFakeStore{items: []announcements.Announcement{{ID: "a1", Title: "x", Message: "x"}}}
	handler, tokens := newAnnouncementTestServer(t, store)

	if got := courseRequest(handler, http.MethodDelete, "/api/v1/admin/announcements/a1", tokens.student, "").Code; got != http.StatusForbidden {
		t.Fatalf("student delete = %d, want 403", got)
	}
	if got := courseRequest(handler, http.MethodDelete, "/api/v1/admin/announcements/a1", tokens.admin, "").Code; got != http.StatusNoContent {
		t.Fatalf("admin delete = %d, want 204", got)
	}
	if got := courseRequest(handler, http.MethodDelete, "/api/v1/admin/announcements/a1", tokens.admin, "").Code; got != http.StatusNotFound {
		t.Fatalf("second delete = %d, want 404", got)
	}
}

func TestAnnouncementsWithoutStoreAndOnError(t *testing.T) {
	handler, tokens := newAnnouncementTestServer(t, nil)
	if got := courseRequest(handler, http.MethodGet, "/api/v1/announcements", tokens.student, "").Code; got != http.StatusNotImplemented {
		t.Fatalf("without store = %d, want 501", got)
	}
	failing, failingTokens := newAnnouncementTestServer(t, &announcementFakeStore{failWith: errors.New("boom")})
	if got := courseRequest(failing, http.MethodGet, "/api/v1/announcements", failingTokens.student, "").Code; got != http.StatusInternalServerError {
		t.Fatalf("store error = %d, want 500", got)
	}
}
