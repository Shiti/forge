package leader

import (
	"context"
	"testing"
	"time"
)

func TestSingleNodeElector_ImmediateLeadership(t *testing.T) {
	e := NewSingleNodeElector()
	if e.IsLeader() {
		t.Fatal("expected not leader before Acquire")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- e.Acquire(ctx)
	}()

	// Watch should emit true once leadership is gained.
	select {
	case got := <-e.Watch():
		if !got {
			t.Fatal("expected true from Watch after Acquire")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Watch")
	}

	if !e.IsLeader() {
		t.Fatal("expected IsLeader to return true")
	}

	// Cancel the context; Acquire should return context.Canceled.
	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Acquire to return")
	}

	if e.IsLeader() {
		t.Fatal("expected not leader after context cancelled")
	}
}

func TestSingleNodeElector_Resign(t *testing.T) {
	e := NewSingleNodeElector()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go e.Acquire(ctx) //nolint:errcheck

	// Wait for leadership.
	select {
	case got := <-e.Watch():
		if !got {
			t.Fatal("expected true from Watch")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for leadership")
	}

	if !e.IsLeader() {
		t.Fatal("expected leader before Resign")
	}

	if err := e.Resign(ctx); err != nil {
		t.Fatalf("Resign failed: %v", err)
	}

	if e.IsLeader() {
		t.Fatal("expected not leader after Resign")
	}

	// Watch should emit false after Resign.
	select {
	case got := <-e.Watch():
		if got {
			t.Fatal("expected false from Watch after Resign")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Watch after Resign")
	}
}
