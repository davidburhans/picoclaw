package providers

import (
	"context"
	"time"

	"github.com/sipeed/picoclaw/pkg/metrics"
)

// MetricsWrapper decorates an LLMProvider to record metrics.
type MetricsWrapper struct {
	LLMProvider
}

// WrapWithMetrics wraps a provider with metrics collection.
func WrapWithMetrics(p LLMProvider) LLMProvider {
	return &MetricsWrapper{p}
}

func (w *MetricsWrapper) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error) {
	start := time.Now()
	resp, err := w.LLMProvider.Chat(ctx, messages, tools, model, options)
	duration := time.Since(start)

	// Record metrics
	agentType := metrics.AgentTypeFromContext(ctx)
	providerID := w.GetID()

	status := "success"
	if err != nil {
		status = "error"
		// Skip advanced classification for now to avoid complexity
	}

	var usage *metrics.LLMUsageInfo
	if resp != nil && resp.Usage != nil {
		usage = &metrics.LLMUsageInfo{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
	metrics.DefaultRecorder().RecordLLMCall(model, providerID, w.GetAPIBase(), string(agentType), status, duration, usage, 0)

	return resp, err
}
