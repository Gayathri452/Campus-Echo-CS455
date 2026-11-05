package database

import (
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
)

func TestSummaryFromDocDerivesLegacyLockExpiry(t *testing.T) {
	lockedAt := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)

	summary := summaryFromDoc(appealSummaryDoc{
		Status:   appeals.StatusLocked,
		LockedAt: &lockedAt,
	})

	want := lockedAt.Add(appeals.LockDuration)
	if summary.LockExpiresAt == nil || !summary.LockExpiresAt.Equal(want) {
		t.Fatalf("lock expiry = %v, want %v", summary.LockExpiresAt, want)
	}
}

func TestSummaryFromDocPreservesExplicitLockExpiry(t *testing.T) {
	lockedAt := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	explicitExpiry := lockedAt.Add(appeals.LockDuration + time.Hour)

	summary := summaryFromDoc(appealSummaryDoc{
		Status:        appeals.StatusLocked,
		LockedAt:      &lockedAt,
		LockExpiresAt: &explicitExpiry,
	})

	if summary.LockExpiresAt == nil || !summary.LockExpiresAt.Equal(explicitExpiry) {
		t.Fatalf("lock expiry = %v, want explicit expiry %v", summary.LockExpiresAt, explicitExpiry)
	}
}
