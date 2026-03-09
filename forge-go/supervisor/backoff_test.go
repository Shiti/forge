package supervisor

import (
	"testing"
	"time"
)

func TestComputeBackoff(t *testing.T) {
	// Attempt 1 -> 1s delay
	d1 := ComputeBackoff(1)
	if d1 < 750*time.Millisecond || d1 > 1250*time.Millisecond {
		t.Errorf("expected ~1s, got %v", d1)
	}

	// Attempt 2 -> 2s delay
	d2 := ComputeBackoff(2)
	if d2 < 1500*time.Millisecond || d2 > 2500*time.Millisecond {
		t.Errorf("expected ~2s, got %v", d2)
	}

	// Attempt 11 -> expected 0
	d11 := ComputeBackoff(11)
	if d11 != 0 {
		t.Errorf("expected 0 for attempt > MaxRetries, got %v", d11)
	}
}
