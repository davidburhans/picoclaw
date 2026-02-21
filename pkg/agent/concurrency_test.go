package agent

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/stretchr/testify/assert"
)

// concurrencyMockProvider simulates a slow LLM call
type concurrencyMockProvider struct {
	delay time.Duration
	mu    sync.Mutex
	calls int
}

func (m *concurrencyMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()

	select {
	case <-time.After(m.delay):
		return &providers.LLMResponse{Content: "Slow response"}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *concurrencyMockProvider) GetDefaultModel() string   { return "slow-model" }
func (m *concurrencyMockProvider) GetMaxTokens() int         { return 4096 }
func (m *concurrencyMockProvider) GetTemperature() float64   { return 0.7 }
func (m *concurrencyMockProvider) GetMaxToolIterations() int { return 10 }
func (m *concurrencyMockProvider) GetTimeout() int           { return 120 }
func (m *concurrencyMockProvider) GetMaxConcurrent() int     { return 5 }
func (m *concurrencyMockProvider) GetID() string             { return "concurrency-mock-id" }

func TestAgentLoop_ConcurrentProcessing(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "agent-concurrency-*")
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: tmpDir,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &concurrencyMockProvider{delay: 500 * time.Millisecond}
	al := NewAgentLoop(cfg, msgBus, provider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start agent loop in background
	go al.Run(ctx)

	// Send 5 messages concurrently
	for i := 0; i < 5; i++ {
		msgBus.PublishInbound(ctx, bus.InboundMessage{
			Channel:    "test",
			SenderID:   fmt.Sprintf("user%d", i),
			ChatID:     "chat1",
			Content:    "hello",
			SessionKey: fmt.Sprintf("session%d", i),
		})
	}

	// Wait for a bit (shorter than sequential processing time: 5 * 500ms = 2.5s)
	// But it should take ~500ms + some overhead if concurrent.
	time.Sleep(1 * time.Second)

	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()

	assert.Equal(t, 5, calls, "Expected 5 concurrent calls to have started/finished")
}

func TestAgentLoop_GracefulShutdown(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "agent-shutdown-*")
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: tmpDir,
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &concurrencyMockProvider{delay: 1 * time.Second}
	al := NewAgentLoop(cfg, msgBus, provider)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go al.Run(ctx)

	// Trigger processing
	msgBus.PublishInbound(ctx, bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		Content:    "hello",
		SessionKey: "session1",
	})

	// Wait for it to start
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	al.Stop()
	duration := time.Since(start)

	// Should take at least ~800ms more to finish the slow provider call
	assert.GreaterOrEqual(t, duration, 500*time.Millisecond, "Stop() returned too early, didn't wait for goroutine")
}
