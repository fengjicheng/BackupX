package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSupervisorShutdownCancelsAndWaits(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	started := make(chan struct{})
	finished := make(chan struct{})
	if !supervisor.Go(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	}) {
		t.Fatal("expected task to be accepted")
	}
	<-started

	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := supervisor.Shutdown(waitCtx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("Shutdown returned before the task finished")
	}
	if supervisor.Go(func(context.Context) {}) {
		t.Fatal("expected task submitted after shutdown to be rejected")
	}
}

func TestSupervisorShutdownHonorsWaitContext(t *testing.T) {
	supervisor := NewSupervisor(context.Background())
	release := make(chan struct{})
	if !supervisor.Go(func(context.Context) { <-release }) {
		t.Fatal("expected task to be accepted")
	}

	waitCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := supervisor.Shutdown(waitCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown error = %v, want context.Canceled", err)
	}
	close(release)
	if err := supervisor.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown returned error: %v", err)
	}
}
