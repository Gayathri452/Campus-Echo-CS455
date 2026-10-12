// Package appeals holds the shared contract for the moderator appeal workflow:
// ticket shapes, statuses, errors, the store interfaces and the 30-minute lock
// rule. It uses only the standard library so every other package can import it.
//
// This file is the agreed contract between the two people building the
// moderator pages. Change it only by agreement, in its own small PR, because
// both sides depend on it.
package appeals

import (
	"context"
	"errors"
	"time"
)

// Ticket statuses. A ticket is created by the student-appeal feature
// (FR-9) as StatusUnderAppeal.
//
//	under_appeal --claim--> locked --restore--> restored
//	     ^                    |   \--delete---> deleted
//	     +----release/30 min--+
const (
	StatusUnderAppeal = "under_appeal"
	StatusLocked      = "locked"
	StatusRestored    = "restored"
	StatusDeleted     = "deleted"
)

// Sort orders for the queue (FR-10 default is oldest first).
const (
	SortOldestFirst = "asc"
	SortNewestFirst = "desc"
)

// Resolve actions accepted by POST /api/v1/appeals/{id}/resolve.
const (
	ActionRestore = "restore"
	ActionDelete  = "delete"
)

// LockDuration is how long a moderator may hold a ticket without resolving it
// (FR-12). After this the ticket is available to every moderator again.
const LockDuration = 30 * time.Minute

// Errors returned by stores. Each HTTP handler maps them to a status code.
var (
	ErrNotImplemented  = errors.New("not implemented")
	ErrNotFound        = errors.New("appeal ticket not found")
	ErrAlreadyClaimed  = errors.New("appeal ticket has already been claimed")
	ErrNotLockHolder   = errors.New("appeal ticket is not locked by this moderator")
	ErrCommentRequired = errors.New("a comment is required to delete a review")
	ErrInvalidAction   = errors.New("action must be restore or delete")
)

// Summary is the list shape used by the queue and the active-appeals page.
// It is also what a successful claim returns.
type Summary struct {
	ID            string     `json:"id" bson:"_id,omitempty"`
	ReferenceCode string     `json:"referenceCode" bson:"referenceCode"`
	CourseID      string     `json:"courseId" bson:"courseId"`
	Year          int        `json:"year" bson:"year"`
	Professor     string     `json:"professor" bson:"professor"`
	Status        string     `json:"status" bson:"status"`
	LockedAt      *time.Time `json:"lockedAt,omitempty" bson:"lockedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt" bson:"createdAt"`
}

// Ticket is the full stored document and the detail-page shape. Display fields
// are copied onto the ticket when the appeal is created, so the moderator pages
// do not depend on how the feedback collection evolves. It deliberately holds
// no student identity.
type Ticket struct {
	Summary           `bson:",inline"`
	ReviewID          string     `json:"reviewId" bson:"reviewId"`
	FeedbackText      string     `json:"feedbackText" bson:"feedbackText"`
	FlagReason        string     `json:"flagReason" bson:"flagReason"`
	AppealComment     string     `json:"appealComment" bson:"appealComment"`
	LockedBy          string     `json:"-" bson:"lockedBy,omitempty"`
	ResolutionComment string     `json:"-" bson:"resolutionComment,omitempty"`
	ResolvedAt        *time.Time `json:"-" bson:"resolvedAt,omitempty"`
}

// QueueStore is implemented by Person A (pages 1 and 3: queue, claim, active).
type QueueStore interface {
	// ListUnclaimed returns claimable tickets in the requested order: status
	// under_appeal, plus locked tickets whose lock has expired. sort is
	// SortOldestFirst or SortNewestFirst.
	ListUnclaimed(ctx context.Context, sort string) ([]Summary, error)
	// Claim locks the ticket to moderatorID in one atomic operation. Exactly one
	// concurrent caller succeeds; the others get ErrAlreadyClaimed. An unknown
	// ID returns ErrNotFound.
	Claim(ctx context.Context, ticketID, moderatorID string) (Summary, error)
	// ListMine returns tickets locked by moderatorID whose lock has not expired.
	ListMine(ctx context.Context, moderatorID string) ([]Summary, error)
}

// ResolveStore is implemented by Person B (page 2: detail, release, resolve,
// and the expiry sweep).
type ResolveStore interface {
	// Get returns the full ticket, only if it is locked by moderatorID and the
	// lock has not expired; otherwise ErrNotLockHolder (or ErrNotFound).
	Get(ctx context.Context, ticketID, moderatorID string) (Ticket, error)
	// Release returns a ticket held by moderatorID to the queue.
	Release(ctx context.Context, ticketID, moderatorID string) error
	// Resolve closes a ticket held by moderatorID. ActionRestore publishes the
	// review; ActionDelete removes it and needs a non-empty comment
	// (ErrCommentRequired), which the student can later read.
	Resolve(ctx context.Context, ticketID, moderatorID, action, comment string) error
	// ReleaseExpired returns every ticket whose lock is older than LockDuration
	// to the queue and reports how many it released.
	ReleaseExpired(ctx context.Context, now time.Time) (int64, error)
}

// Store is the full set of operations; *database.Mongo implements both halves.
type Store interface {
	QueueStore
	ResolveStore
}

// LockExpiryCutoff returns the moment before which a lock counts as expired at
// time now. Stores use it in queries; never recompute "30 minutes" elsewhere.
func LockExpiryCutoff(now time.Time) time.Time {
	return now.Add(-LockDuration)
}

// IsLockExpired reports whether a lock taken at lockedAt has expired at now. A
// lock expires exactly LockDuration after it was taken.
func IsLockExpired(lockedAt, now time.Time) bool {
	return !lockedAt.After(LockExpiryCutoff(now))
}
