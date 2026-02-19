package agent

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// slowMockProvider simulates a slow LLM response to ensure overlap
type slowMockProvider struct {
	delay time.Duration
}

func (m *slowMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	select {
	case <-time.After(m.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &providers.LLMResponse{
		Content:   "Mock response",
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *slowMockProvider) GetDefaultModel() string { return "mock-model" }
func (m *slowMockProvider) GetMaxTokens() int { return 4096 }
func (m *slowMockProvider) GetTemperature() float64 { return 0.7 }
func (m *slowMockProvider) GetMaxToolIterations() int { return 10 }
func (m *slowMockProvider) GetTimeout() int { return 10 } // Short timeout for test
func (m *slowMockProvider) GetMaxConcurrent() int { return 10 }

func TestAgentLoop_ConcurrentSummarization(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-crash-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	msgBus := bus.NewMessageBus()
	// Slow provider to ensure summarization takes time
	provider := &slowMockProvider{delay: 100 * time.Millisecond}
	al := NewAgentLoop(cfg, msgBus, provider)

	sessionKey := "concurrent_session"
	
	// 1. Populate session with many messages to trigger summarization
	// Threshold is > 20 messages
	history := make([]providers.Message, 0, 25)
	history = append(history, providers.Message{Role: "system", Content: "System prompt"})
	for i := 0; i < 22; i++ {
		history = append(history, providers.Message{
			Role:    "user",
			Content: fmt.Sprintf("Message %d", i),
		})
	}
	al.GetSessionManager().SetHistory(sessionKey, history)

	// 2. Trigger summarization via a new message
	// This should start the summarization goroutine
	ctx := context.Background()
	go func() {
		_, err := al.ProcessDirectWithChannel(ctx, "Trigger summarization", sessionKey, "cli", "chat1")
		if err != nil {
			t.Logf("ProcessDirect error (expected if cancelled): %v", err)
		}
	}()

	// 3. Immediately send multiple concurrent messages to trigger race conditions
	// We want to hit the session manager while summarization is running (reading/writing)
	// and while the main loop is attempting to update history
	for i := 0; i < 10; i++ {
		go func(i int) {
			_, err := al.ProcessDirectWithChannel(ctx, fmt.Sprintf("Concurrent msg %d", i), sessionKey, "cli", "chat1")
			if err != nil {
				t.Logf("Concurrent msg error: %v", err)
			}
		}(i)
	}

	// Wait for a bit to let things run/crash
	select {
	case <-time.After(2 * time.Second):
		t.Log("Test finished without crash")
	}
}
