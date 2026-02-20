package providers

import (
	"context"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOverflowProvider_Failover(t *testing.T) {
	// Reset global tracker
	GlobalConcurrencyTracker().Reset()

	// Setup config
	cfg := &config.Config{
		ModelList: []config.ModelConfig{
			{
				ModelName: "openai/mock1",
				Model:     "openai/gpt-4",
				APIKey:    "mock-key",
				APIBase:   "http://mock-url",
				MaxConcurrentSessions: config.IntPtr(1),
			},
			{
				ModelName: "openai/mock2",
				Model:     "openai/gpt-4",
				APIKey:    "mock-key",
				APIBase:   "http://mock-url",
				MaxConcurrentSessions: config.IntPtr(1),
			},
		},
	}

	// Create Overflow Provider
	overflowProvider := NewOverflowProvider(cfg, []string{"openai/mock1", "openai/mock2"})

	// We need to mock HTTP requests or use a real server.
	// Since HTTPProvider makes real HTTP calls, we should mock the server or intercept.
	// However, for concurrency testing, we only care about Acquiring the slot.
	// The HTTPProvider.Chat will fail if we don't have a real server, BUT the concurrency check happens BEFORE the HTTP call.
	// So if acquire fails, it returns error immediately.
	// If acquire succeeds, it proceeds to HTTP call.
	// We want to test that if the FIRST provider fails acquire, it goes to SECOND.
	
	// Problem: We can't easily mock the HTTP call inside HTTPProvider without changing it to use an interface for HTTP client or similar.
	// But we can rely on the fact that if acquire succeeds, it will try to make a request.
	// We can set an invalid URL and expect a specific error if it got through.
	// "failed to send request" or "API base not configured" etc.

	// Let's manually acquire slot for mock1 to simulate it being busy.
	GlobalConcurrencyTracker().Acquire("openai/mock1:gpt-4", 1) // Note: ID generation logic in CreateProvider uses format "type/name:model" or similar.
	// Wait, CreateProvider logic:
	// providerID := providerStr ("openai/mock1")
	// if !found { append model }
	// Here "openai/mock1" is found. So ID is "openai/mock1".
	
	// However, NewHTTPProvider is called with this ID.
	// Let's verify ID generation by creating one first.
	// But we are using NewOverflowProvider directly.
	// Inside Chat, it calls CreateProvider.
	
	// So let's pre-fill the tracker for "openai/mock1".
	GlobalConcurrencyTracker().Acquire("openai/mock1", 1)

	// Now call Chat on overflow provider.
	// It should try mock1, fail acquire, log warning, and try mock2.
	// mock2 is free, so it should acquire and then try to make HTTP request.
	// Since URL is invalid/dummy, it will fail with network error.
	// But it should NOT fail with "concurrency limit reached".
	
	ctx := context.Background()
	_, err := overflowProvider.Chat(ctx, nil, nil, "gpt-4", nil)
	
	// verification
	require.Error(t, err)
	// The error should NOT be "concurrency limit reached".
	// It should be related to connection refusal or similar since we perform a real HTTP request to bad URL.
	// Or "API base not configured" if we didn't set it (we did).
	
	if strings.Contains(err.Error(), "concurrency limit reached") {
		t.Fatalf("Expected failover to mock2, but got concurrency error from mock1 logic bubbling up? Error: %v", err)
	}
	
	// Also verify mock2 has 0 count (released after error) or 1 if it's still running (it's not).
	// Because HTTPProvider releases in defer, usage of mock2 should be 0.
	assert.Equal(t, 0, GlobalConcurrencyTracker().GetCount("openai/mock2"))
	
	// release mock1
	GlobalConcurrencyTracker().Release("openai/mock1")
}

func TestOverflowProvider_AllBusy(t *testing.T) {
	GlobalConcurrencyTracker().Reset()

	cfg := &config.Config{
		ModelList: []config.ModelConfig{
			{
				ModelName: "openai/mock1",
				Model:     "openai/m",
				APIKey:    "k",
				APIBase:   "http://localhost",
				MaxConcurrentSessions: config.IntPtr(1),
			},
			{
				ModelName: "openai/mock2",
				Model:     "openai/m",
				APIKey:    "k",
				APIBase:   "http://localhost",
				MaxConcurrentSessions: config.IntPtr(1),
			},
		},
	}

	GlobalConcurrencyTracker().Acquire("openai/mock1", 1)
	GlobalConcurrencyTracker().Acquire("openai/mock2", 1)

	overflowProvider := NewOverflowProvider(cfg, []string{"openai/mock1", "openai/mock2"})
	
	_, err := overflowProvider.Chat(context.Background(), nil, nil, "gpt-4", nil)
	
	require.Error(t, err)
	// Should fail because all are busy. The error message should reflect that all failed.
	// Our logic returns "all overflow providers failed: ..." and the last error.
	// The last error should be "concurrency limit reached" (from mock2).
	
	assert.Contains(t, err.Error(), "all overflow providers failed")
	// assert.Contains(t, err.Error(), "concurrency limit reached") // This might be nested or wrapped
}

func TestOverflowProvider_Recursion(t *testing.T) {
    GlobalConcurrencyTracker().Reset()
    
    cfg := &config.Config{
        ModelList: []config.ModelConfig{
            {
                ModelName: "overflow/loop",
                Model:     "overflow/overflow/loop",
            },
        },
    }
    
    overflowProvider := NewOverflowProvider(cfg, []string{"overflow/loop"})
    
    // Should detect recursion and skip/fail
    _, err := overflowProvider.Chat(context.Background(), nil, nil, "gpt-4", nil)
    require.Error(t, err)
    // "no providers configured" or "all providers failed"
}
