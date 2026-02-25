package metrics

import (
	"context"
	"time"
)

// AgentType defines the source of an LLM request or tool execution.
type AgentType string

const (
	AgentTypeMain      AgentType = "main"
	AgentTypeSubagent  AgentType = "subagent"
	AgentTypeHeartbeat AgentType = "heartbeat"
	AgentTypeCron      AgentType = "cron"
)

type contextKey string

const agentTypeKey contextKey = "picoclaw_agent_type"

// WithAgentType returns a new context with the specified agent type.
func WithAgentType(ctx context.Context, at AgentType) context.Context {
	return context.WithValue(ctx, agentTypeKey, string(at))
}

// AgentTypeFromContext extracts the agent type from the context, defaulting to "main".
func AgentTypeFromContext(ctx context.Context) string {
	if at, ok := ctx.Value(agentTypeKey).(string); ok {
		return at
	}
	return string(AgentTypeMain)
}

// Recorder provides high-level methods for recording metrics.
type Recorder struct {
	startTime time.Time
}

var defaultRecorder = &Recorder{startTime: time.Now()}

// DefaultRecorder returns the singleton recorder instance.
func DefaultRecorder() *Recorder {
	return defaultRecorder
}

// LLMUsageInfo matches the structure in providers package to avoid circular imports.
type LLMUsageInfo struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// RecordLLMCall records duration, tokens, and errors for an LLM request.
func (r *Recorder) RecordLLMCall(model, provider, apiBase, agentType, status string, duration time.Duration, usage *LLMUsageInfo, contextSize int) {
	llmRequests.WithLabelValues(model, provider, agentType).Inc()
	llmRequestDuration.WithLabelValues(model, provider, apiBase, agentType, status).Observe(duration.Seconds())

	if usage != nil {
		llmTokensPrompt.WithLabelValues(model, provider, apiBase, agentType).Add(float64(usage.PromptTokens))
		llmTokensCompletion.WithLabelValues(model, provider, apiBase, agentType).Add(float64(usage.CompletionTokens))
	}

	llmContextSize.WithLabelValues(model, agentType).Observe(float64(contextSize))
}

// RecordLLMTokens records token usage for an LLM call.
func (r *Recorder) RecordLLMTokens(model, tokenType string, count int) {
	// provider, apiBase, agentType labels are missing here, so we use "unknown" or simplify metric
	// Better to use RecordLLMCall which has all context.
	// But if called separately:
	if tokenType == "prompt" {
		llmTokensPrompt.WithLabelValues(model, "unknown", "unknown", "unknown").Add(float64(count))
	} else {
		llmTokensCompletion.WithLabelValues(model, "unknown", "unknown", "unknown").Add(float64(count))
	}
}

// RecordLLMError records an LLM error with classification.
func (r *Recorder) RecordLLMError(model, provider, apiBase, errorType, agentType string) {
	llmErrors.WithLabelValues(model, provider, apiBase, errorType, agentType).Inc()
}

// RecordToolCall records tool execution metrics.
func (r *Recorder) RecordToolCall(name, agentType, status string, duration time.Duration, resultSize int) {
	toolCalls.WithLabelValues(name, agentType, status).Inc()
	toolDuration.WithLabelValues(name, agentType).Observe(duration.Seconds())
	if resultSize > 0 {
		toolResultSize.WithLabelValues(name).Observe(float64(resultSize))
	}
}

// RecordToolError records a tool execution error.
func (r *Recorder) RecordToolError(name, errorType string) {
	toolErrors.WithLabelValues(name, errorType).Inc()
}

// RecordMessage records a message bus event.
func (r *Recorder) RecordMessage(direction, channel, msgType string) {
	messagesTotal.WithLabelValues(channel, direction, msgType).Inc()
}

// RecordBusDrop records a message dropped due to bus congestion.
func (r *Recorder) RecordBusDrop(direction string) {
	busDrops.WithLabelValues(direction).Inc()
}

// RecordAgentTurn records end-to-end turn metrics.
func (r *Recorder) RecordAgentTurn(model, channel, workspace, agentType string, duration time.Duration, iterations, tools int) {
	agentTurns.WithLabelValues(model, channel, workspace, agentType).Inc()
	agentResponseDuration.WithLabelValues(model, channel, workspace, agentType).Observe(duration.Seconds())
	agentIterations.WithLabelValues(model, agentType).Observe(float64(iterations))
	agentToolsPerTurn.WithLabelValues(model, agentType).Observe(float64(tools))
}

// RecordSubagentDuration records subagent execution duration.
func (r *Recorder) RecordSubagentDuration(model, role, subType, status string, duration time.Duration) {
	subagentDuration.WithLabelValues(model, role, subType, status).Observe(duration.Seconds())
}

// RecordHeartbeat records heartbeat processing metrics.
func (r *Recorder) RecordHeartbeat(status, workspace string, duration time.Duration) {
	heartbeatTotal.WithLabelValues(status, workspace).Inc()
	heartbeatDuration.WithLabelValues(workspace).Observe(duration.Seconds())
}

// RecordCronExecution records cron job execution metrics.
func (r *Recorder) RecordCronExecution(jobName, status, payloadKind string, duration time.Duration) {
	cronExecutions.WithLabelValues(jobName, status, payloadKind).Inc()
	cronDuration.WithLabelValues(jobName).Observe(duration.Seconds())
}

// UpdateUptime updates the application uptime metric.
func (r *Recorder) UpdateUptime() {
	uptimeGauge.Set(time.Since(r.startTime).Seconds())
}

// SetConcurrency updates concurrency gauges.
func (r *Recorder) SetConcurrency(providerID string, active, queueDepth int) {
	concurrencyActive.WithLabelValues(providerID).Set(float64(active))
	concurrencyQueueDepth.WithLabelValues(providerID).Set(float64(queueDepth))
}

// RecordConcurrencyWait records queue wait duration.
func (r *Recorder) RecordConcurrencyWait(providerID string, duration time.Duration) {
	concurrencyWait.WithLabelValues(providerID).Observe(duration.Seconds())
}

// RecordConcurrencyRejection records a rejection due to limit.
func (r *Recorder) RecordConcurrencyRejection(providerID string) {
	concurrencyRejections.WithLabelValues(providerID).Inc()
}

// RecordFallback records a model fallback attempt.
func (r *Recorder) RecordFallback(provider, model, reason string, duration time.Duration, skipped bool) {
	skippedStr := "false"
	if skipped {
		skippedStr = "true"
	}
	fallbackAttempts.WithLabelValues(provider, model, reason, skippedStr).Inc()
}

// RecordFallbackExhaustion records when all models in a chain fail.
func (r *Recorder) RecordFallbackExhaustion() {
	fallbackExhausted.Inc()
}
