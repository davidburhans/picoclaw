package metrics

import (
	"context"
	"testing"
	"time"
)

func TestRecorder_NoPanic(t *testing.T) {
	r := &Recorder{startTime: time.Now()}

	// Ensure recording calls don't panic even if uninitialized (using default global prometheus registerer)
	t.Run("RecordLLMCall", func(t *testing.T) {
		r.RecordLLMCall("gpt-4", "openai", "https://api.openai.com", "main", "success", 100*time.Millisecond, &LLMUsageInfo{TotalTokens: 100}, 50)
	})

	t.Run("RecordToolCall", func(t *testing.T) {
		r.RecordToolCall("test-tool", "main", "success", 50*time.Millisecond, 1024)
	})

	t.Run("RecordMessage", func(t *testing.T) {
		r.RecordMessage("inbound", "discord", "text")
	})

	t.Run("RecordBusDrop", func(t *testing.T) {
		r.RecordBusDrop("outbound")
	})

	t.Run("RecordAgentTurn", func(t *testing.T) {
		r.RecordAgentTurn("gpt-4", "discord", "default", "main", 1*time.Second, 3, 2)
	})
}

func TestWithAgentType(t *testing.T) {
	ctx := context.Background()
	ctx = WithAgentType(ctx, AgentTypeSubagent)

	val := AgentTypeFromContext(ctx)
	if val != string(AgentTypeSubagent) {
		t.Errorf("expected agent type %s, got %s", AgentTypeSubagent, val)
	}
}

func TestAgentTypeFromContext_Default(t *testing.T) {
	ctx := context.Background()
	val := AgentTypeFromContext(ctx)
	if val != string(AgentTypeMain) {
		t.Errorf("expected default agent type %s, got %s", AgentTypeMain, val)
	}
}
