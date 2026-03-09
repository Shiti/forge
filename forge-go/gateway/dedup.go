package gateway

import (
	"sync"
	"time"
)

const evictionThreshold = 1000

// DedupCache tracks recently forwarded message IDs to prevent infinite broadcast loops.
type DedupCache struct {
	mu     sync.Mutex
	cache  map[uint64]time.Time
	maxTTL time.Duration
}

// NewDedupCache initializes a concurrent-safe deduplication tracker
func NewDedupCache(maxTTL time.Duration) *DedupCache {
	return &DedupCache{
		cache:  make(map[uint64]time.Time),
		maxTTL: maxTTL,
	}
}

// ShouldForward returns true if the message ID has NOT been seen recently.
// If unseen, it atomically records the ID to block subsequent identical checks.
func (d *DedupCache) ShouldForward(msgID uint64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.cache[msgID]; exists {
		return false
	}

	now := time.Now()
	d.cache[msgID] = now

	// Amortize eviction: only scan when the cache exceeds the threshold.
	if len(d.cache) > evictionThreshold {
		for id, timestamp := range d.cache {
			if now.Sub(timestamp) > d.maxTTL {
				delete(d.cache, id)
			}
		}
	}

	return true
}

func (d *DedupCache) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache = make(map[uint64]time.Time)
}
