package appeals

import (
	"context"
	"log"
	"time"
)

// DefaultSweepInterval is how often RunExpiryWorker looks for expired locks.
const DefaultSweepInterval = time.Minute

// RunExpiryWorker releases tickets whose lock is older than LockDuration until
// ctx is cancelled. It is started once from cmd/api. Queries must also treat
// expired locks as claimable, so a slow sweep never leaves a ticket stuck.
func RunExpiryWorker(ctx context.Context, store ResolveStore, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultSweepInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			released, err := store.ReleaseExpired(ctx, now.UTC())
			switch {
			case err == ErrNotImplemented:
				// Scaffold stub: nothing to sweep until ReleaseExpired is written.
			case err != nil:
				log.Printf("release expired appeal locks: %v", err)
			case released > 0:
				log.Printf("released %d expired appeal lock(s)", released)
			}
		}
	}
}
