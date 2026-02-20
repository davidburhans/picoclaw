package providers

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrConcurrencyLimit is returned when the concurrency limit for a provider is reached.
var ErrConcurrencyLimit = errors.New("concurrency limit reached")

// WaiterInfo contains information about a request's position in the concurrency queue.
type WaiterInfo struct {
	Position int
	Total    int
}

// ConcurrencyTracker tracks active sessions across provider instances.
type ConcurrencyTracker struct {
	mu           sync.Mutex
	counts       map[string]int
	waiters      map[string][]chan WaiterInfo
	globalSignal chan struct{}
}

var (
	globalTracker *ConcurrencyTracker
	trackerOnce   sync.Once
)

// GlobalConcurrencyTracker returns the singleton instance of ConcurrencyTracker.
func GlobalConcurrencyTracker() *ConcurrencyTracker {
	trackerOnce.Do(func() {
		globalTracker = &ConcurrencyTracker{
			counts:       make(map[string]int),
			waiters:      make(map[string][]chan WaiterInfo),
			globalSignal: make(chan struct{}),
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

// WaitAcquire blocks until a slot is available for the given provider ID or ctx is cancelled.
// notifyFunc is called whenever the request's position in the queue changes.
func (ct *ConcurrencyTracker) WaitAcquire(ctx context.Context, id string, max int, notifyFunc func(WaiterInfo)) error {
	msgChan := make(chan WaiterInfo, 1)

	ct.mu.Lock()
	ct.waiters[id] = append(ct.waiters[id], msgChan)
	ct.mu.Unlock()

	defer func() {
		ct.mu.Lock()
		ct.removeWaiter(id, msgChan)
		ct.mu.Unlock()
	}()

	for {
		ct.mu.Lock()
		if max <= 0 || ct.counts[id] < max {
			ct.counts[id]++
			ct.mu.Unlock()
			return nil
		}
		
		// Determine position
		pos := -1
		for i, w := range ct.waiters[id] {
			if w == msgChan {
				pos = i + 1
				break
			}
		}
		total := len(ct.waiters[id])
		sig := ct.globalSignal
		ct.mu.Unlock()

		if notifyFunc != nil && pos > 0 {
			notifyFunc(WaiterInfo{Position: pos, Total: total})
		}

		select {
		case <-sig:
			// Something was released, check again
		case info := <-msgChan:
			if notifyFunc != nil {
				notifyFunc(info)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// WaitAny attempts to acquire any of the provided IDs. Blocks until at least one is acquired or ctx is cancelled.
// Returns the ID that was acquired.
func (ct *ConcurrencyTracker) WaitAny(ctx context.Context, ids []string, maxes []int, notifyFunc func(WaiterInfo)) (string, error) {
	if len(ids) == 0 {
		return "", errors.New("no IDs provided to WaitAny")
	}
	for {
		ct.mu.Lock()
		for i, id := range ids {
			max := 0
			if i < len(maxes) {
				max = maxes[i]
			}
			if max <= 0 || ct.counts[id] < max {
				ct.counts[id]++
				ct.mu.Unlock()
				return id, nil
			}
		}
		sig := ct.globalSignal
		ct.mu.Unlock()

		if notifyFunc != nil {
			// Notify that we are in a generic wait state.
			// Since we're waiting for ANY, the concept of "position" is fuzzy.
			// We'll report Position 1 to trigger the hourglass reaction.
			notifyFunc(WaiterInfo{Position: 1, Total: 1})
		}

		select {
		case <-sig:
			// Something released, try again
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

// Release decrements the active session count for the given provider ID.
func (ct *ConcurrencyTracker) Release(id string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.counts[id] > 0 {
		ct.counts[id]--
		
		// Notify waiters for THIS id about position changes
		for i, w := range ct.waiters[id] {
			select {
			case w <- WaiterInfo{Position: i + 1, Total: len(ct.waiters[id])}:
			default:
			}
		}

		// Broadcast release to all waiters (especially those in WaitAny or other IDs)
		close(ct.globalSignal)
		ct.globalSignal = make(chan struct{})
	}
}

// removeWaiter removes a waiter from the list. Must be called with mu held.
func (ct *ConcurrencyTracker) removeWaiter(id string, w chan WaiterInfo) {
	waiters := ct.waiters[id]
	for i, waiter := range waiters {
		if waiter == w {
			ct.waiters[id] = append(waiters[:i], waiters[i+1:]...)
			break
		}
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
	ct.waiters = make(map[string][]chan WaiterInfo)
	// Also reset signal to be safe
	close(ct.globalSignal)
	ct.globalSignal = make(chan struct{})
}
// ConcurrencyWrapper decorates an LLMProvider with concurrency tracking.
type ConcurrencyWrapper struct {
	LLMProvider
}

func (w *ConcurrencyWrapper) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error) {
	id := w.LLMProvider.GetID()
	max := w.LLMProvider.GetMaxConcurrent()

	if id != "" && max > 0 {
		wait, _ := options["wait"].(bool)
		if wait {
			onWait, _ := options["on_wait"].(func(WaiterInfo))
			if err := GlobalConcurrencyTracker().WaitAcquire(ctx, id, max, onWait); err != nil {
				return nil, err
			}
		} else {
			if !GlobalConcurrencyTracker().Acquire(id, max) {
				return nil, fmt.Errorf("%w for provider %s", ErrConcurrencyLimit, id)
			}
		}
		defer GlobalConcurrencyTracker().Release(id)
	}

	return w.LLMProvider.Chat(ctx, messages, tools, model, options)
}

func NewConcurrencyWrapper(provider LLMProvider) *ConcurrencyWrapper {
	if provider == nil {
		return nil
	}
	// Don't double-wrap
	if _, ok := provider.(*ConcurrencyWrapper); ok {
		return provider.(*ConcurrencyWrapper)
	}
	return &ConcurrencyWrapper{LLMProvider: provider}
}
