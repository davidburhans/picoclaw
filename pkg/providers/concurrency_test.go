package providers

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConcurrencyTracker(t *testing.T) {
	tracker := GlobalConcurrencyTracker()
	tracker.Reset()

	id := "test-provider"
	max := 2

	// Acquire up to max
	assert.True(t, tracker.Acquire(id, max))
	assert.Equal(t, 1, tracker.GetCount(id))

	assert.True(t, tracker.Acquire(id, max))
	assert.Equal(t, 2, tracker.GetCount(id))

	// Should fail now
	assert.False(t, tracker.Acquire(id, max))
	assert.Equal(t, 2, tracker.GetCount(id))

	// Release one
	tracker.Release(id)
	assert.Equal(t, 1, tracker.GetCount(id))

	// Should succeed again
	assert.True(t, tracker.Acquire(id, max))
	assert.Equal(t, 2, tracker.GetCount(id))
}

func TestConcurrencyTracker_NoLimit(t *testing.T) {
	tracker := GlobalConcurrencyTracker()
	tracker.Reset()

	id := "unlimited"
	max := 0 // or -1

	for i := 0; i < 100; i++ {
		assert.True(t, tracker.Acquire(id, max))
	}
	assert.Equal(t, 100, tracker.GetCount(id))
}

func TestConcurrencyTracker_ThreadSafe(t *testing.T) {
	tracker := GlobalConcurrencyTracker()
	tracker.Reset()
	id := "concurrent"
	max := 100

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.Acquire(id, max)
		}()
	}
	wg.Wait()
	assert.Equal(t, 100, tracker.GetCount(id))

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.Release(id)
		}()
	}
	wg.Wait()
	assert.Equal(t, 0, tracker.GetCount(id))
}
