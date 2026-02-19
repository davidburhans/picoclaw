package providers

import (
	"errors"
	"sync"
)

// ErrConcurrencyLimit is returned when the concurrency limit for a provider is reached.
var ErrConcurrencyLimit = errors.New("concurrency limit reached")

// ConcurrencyTracker tracks active sessions across provider instances.
type ConcurrencyTracker struct {
	mu     sync.Mutex
	counts map[string]int
}

var (
	globalTracker *ConcurrencyTracker
	trackerOnce   sync.Once
)

// GlobalConcurrencyTracker returns the singleton instance of ConcurrencyTracker.
func GlobalConcurrencyTracker() *ConcurrencyTracker {
	trackerOnce.Do(func() {
		globalTracker = &ConcurrencyTracker{
			counts: make(map[string]int),
		}
	})
	return globalTracker
}

// Acquire tries to increment the active session count for the given provider ID.
// Returns true if the count was incremented (limit not reached), false otherwise.
func (ct *ConcurrencyTracker) Acquire(id string, max int) bool {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if max <= 0 {
		// No limit
		ct.counts[id]++
		return true
	}

	if ct.counts[id] >= max {
		return false
	}

	ct.counts[id]++
	return true
}

// Release decrements the active session count for the given provider ID.
func (ct *ConcurrencyTracker) Release(id string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.counts[id] > 0 {
		ct.counts[id]--
	}
}

// GetCount returns the current active session count for the given provider ID.
func (ct *ConcurrencyTracker) GetCount(id string) int {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.counts[id]
}

// Reset clears all counts (mostly for testing).
func (ct *ConcurrencyTracker) Reset() {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.counts = make(map[string]int)
}
