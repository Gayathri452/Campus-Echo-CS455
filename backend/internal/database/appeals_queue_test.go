package database

import (
	"testing"
	"time"

	"github.com/Kishore200524/CS455-Project/backend/internal/appeals"
	"github.com/Kishore200524/CS455-Project/backend/internal/feedback"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func TestFeedbackRecordConvertsAllPublicFields(t *testing.T) {
	created := time.Date(2026, time.November, 6, 12, 0, 0, 0, time.UTC)
	published := created.Add(time.Hour)
	id := primitive.NewObjectID()
	record := feedbackRecord{
		ID:                     id,
		ReferenceCode:          "CE-TEST123",
		CourseID:               "CS455",
		CourseTitle:            "Software Engineering",
		Professor:              "Prof. Rao",
		Year:                   2026,
		Category:               "teaching",
		Rating:                 5,
		Content:                "Helpful review",
		Status:                 feedback.StatusPublished,
		SemanticEmbedding:      []float32{0.1, 0.2},
		SemanticEmbeddingModel: "test-model",
		FlagReason:             "flag reason",
		ProcessingError:        "processing error",
		TriageAttempts:         2,
		TriageCycleAttempts:    3,
		TriageFailureCategory:  "timeout",
		TriageRetryAt:          &published,
		CreatedAt:              created,
		PublishedAt:            &published,
	}

	converted := record.feedback()
	if converted.ID != id.Hex() || converted.ReferenceCode != record.ReferenceCode ||
		converted.CourseID != record.CourseID || converted.CourseTitle != record.CourseTitle ||
		converted.Professor != record.Professor || converted.Year != record.Year ||
		converted.Category != record.Category || converted.Rating != record.Rating ||
		converted.Content != record.Content || converted.Status != record.Status ||
		converted.SemanticEmbeddingModel != record.SemanticEmbeddingModel ||
		converted.FlagReason != record.FlagReason || converted.ProcessingError != record.ProcessingError ||
		converted.TriageAttempts != record.TriageAttempts || converted.TriageCycleAttempts != record.TriageCycleAttempts ||
		converted.TriageFailureCategory != record.TriageFailureCategory ||
		!converted.CreatedAt.Equal(record.CreatedAt) || converted.PublishedAt == nil ||
		!converted.PublishedAt.Equal(*record.PublishedAt) {
		t.Fatalf("feedback conversion lost fields: %+v", converted)
	}
}
