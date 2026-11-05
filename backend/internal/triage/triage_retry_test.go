package triage

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestDelayedRetryDoesNotOccupyWorker(t *testing.T) {
	firstAttempt := make(chan struct{})
	secondJob := make(chan struct{})
	var retryCalls atomic.Int32
	pool := NewWorkerPool(1, 2, 2*time.Second, 1, func(_ context.Context, job Job) error {
		if job.ReviewID == "retry" {
			if retryCalls.Add(1) == 1 {
				close(firstAttempt)
				return errors.New("temporary failure")
			}
			return nil
		}
		close(secondJob)
		return nil
	})
	defer pool.Stop()

	if err := pool.Enqueue(context.Background(), Job{ReviewID: "retry"}); err != nil {
		t.Fatalf("enqueue retry job: %v", err)
	}
	select {
	case <-firstAttempt:
	case <-time.After(time.Second):
		t.Fatal("first attempt did not start")
	}
	if err := pool.Enqueue(context.Background(), Job{ReviewID: "second"}); err != nil {
		t.Fatalf("enqueue second job: %v", err)
	}
	select {
	case <-secondJob:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second job was blocked by the retry backoff")
	}
	if got := retryCalls.Load(); got != 1 {
		t.Fatalf("retry attempts before backoff elapsed = %d, want 1", got)
	}
}
