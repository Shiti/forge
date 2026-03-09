package supervisor

import (
	"math/rand"
	"time"
)

const (
	BaseDelay  = 1 * time.Second
	MaxDelay   = 30 * time.Second
	StableTime = 60 * time.Second // Time running before resets
	MaxRetries = 10
)

// ComputeBackoff returns an exponential backoff duration with ±25% jitter.
// Returns 0 when max retries are exceeded.
func ComputeBackoff(attempt int) time.Duration {
	if attempt > MaxRetries {
		return 0
	}

	shift := attempt - 1
	if shift > 10 {
		shift = 10
	}

	delay := BaseDelay * time.Duration(1<<shift)

	if delay > MaxDelay {
		delay = MaxDelay
	}

	jitterMax := int64(delay) / 4
	if jitterMax > 0 {
		jitter := rand.Int63n(jitterMax*2) - jitterMax
		delay = time.Duration(int64(delay) + jitter)
	}

	return delay
}
