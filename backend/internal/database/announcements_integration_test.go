package database

import (
	"errors"
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/announcements"
)

func TestAnnouncementsIntegration(t *testing.T) {
	store, ctx := queueIntegrationMongo(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	expired := now.Add(-time.Hour)
	later := now.Add(time.Hour)

	create := func(title string, pinned bool, expiresAt *time.Time, age time.Duration) announcements.Announcement {
		t.Helper()
		created, err := store.CreateAnnouncement(ctx, announcements.Announcement{
			Title: title, Message: "message for " + title, Pinned: pinned, ExpiresAt: expiresAt, CreatedAt: now.Add(-age),
		})
		if err != nil {
			t.Fatalf("create %s: %v", title, err)
		}
		if created.ID == "" {
			t.Fatalf("create %s returned no ID", title)
		}
		return created
	}
	create("old", false, nil, 3*time.Hour)
	create("new", false, &later, time.Hour)
	create("pinned", true, nil, 5*time.Hour)
	gone := create("expired", false, &expired, 2*time.Hour)

	active, err := store.ListAnnouncements(ctx, now, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, item := range active {
		titles = append(titles, item.Title)
	}
	if len(titles) != 3 || titles[0] != "pinned" || titles[1] != "new" || titles[2] != "old" {
		t.Fatalf("active order = %v, want [pinned new old]", titles)
	}

	all, err := store.ListAnnouncements(ctx, now, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("all = %d, want 4", len(all))
	}

	if err := store.DeleteAnnouncement(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteAnnouncement(ctx, gone.ID); !errors.Is(err, announcements.ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
	if err := store.DeleteAnnouncement(ctx, "not-an-id"); !errors.Is(err, announcements.ErrNotFound) {
		t.Fatalf("bad id delete = %v, want ErrNotFound", err)
	}
}
