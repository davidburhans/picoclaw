package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// --- LLM Performance Metrics ---
	llmRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_llm_request_duration_seconds",
		Help:    "Duration of LLM requests.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 20, 30, 60},
	}, []string{"model", "provider", "api_base", "agent_type", "status"})

	llmTokensPrompt = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_llm_tokens_prompt_total",
		Help: "Total prompt tokens consumed.",
	}, []string{"model", "provider", "api_base", "agent_type"})

	llmTokensCompletion = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_llm_tokens_completion_total",
		Help: "Total completion tokens generated.",
	}, []string{"model", "provider", "api_base", "agent_type"})

	llmContextSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_llm_context_size_tokens",
		Help:    "Estimated context size in tokens before LLM call.",
		Buckets: []float64{100, 500, 1000, 2000, 4000, 8000, 16000, 32000, 64000, 128000},
	}, []string{"model", "agent_type"})

	llmContextUtilization = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_llm_context_utilization_ratio",
		Help:    "Ratio of tokens used vs context window.",
		Buckets: []float64{0.1, 0.25, 0.5, 0.75, 0.9, 1.0},
	}, []string{"model"})

	llmErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_llm_errors_total",
		Help: "Total LLM call errors.",
	}, []string{"model", "provider", "api_base", "error_type", "agent_type"})

	llmRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_llm_requests_total",
		Help: "Total LLM requests attempted.",
	}, []string{"model", "provider", "agent_type"})

	// --- Tool Usage Metrics ---
	toolCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_tool_calls_total",
		Help: "Total tool executions.",
	}, []string{"tool_name", "agent_type", "status"})

	toolDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_tool_duration_seconds",
		Help:    "Duration of tool executions.",
		Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30},
	}, []string{"tool_name", "agent_type"})

	toolErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_tool_errors_total",
		Help: "Total tool execution errors.",
	}, []string{"tool_name", "error_type"})

	toolResultSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_tool_result_size_bytes",
		Help:    "Size of tool execution results in bytes.",
		Buckets: []float64{100, 1000, 5000, 10000, 50000, 100000},
	}, []string{"tool_name"})

	// --- Agent Turn Metrics ---
	agentResponseDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_agent_response_duration_seconds",
		Help:    "End-to-end duration for agent to respond to user message.",
		Buckets: []float64{1, 5, 10, 20, 30, 60, 120, 300},
	}, []string{"model", "channel", "workspace", "agent_type"})

	agentIterations = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_agent_iterations_per_turn",
		Help:    "Number of LLM + tool iterations in a single turn.",
		Buckets: []float64{1, 2, 3, 5, 10, 20},
	}, []string{"model", "agent_type"})

	agentToolsPerTurn = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_agent_tools_per_turn",
		Help:    "Number of tool calls in a single turn.",
		Buckets: []float64{0, 1, 2, 5, 10, 20},
	}, []string{"model", "agent_type"})

	agentTurns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_agent_turns_total",
		Help: "Total agent response cycles.",
	}, []string{"model", "channel", "workspace", "agent_type"})

	agentTimeToFirstTool = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_agent_time_to_first_tool_seconds",
		Help:    "Time from user message to first tool call.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 60},
	}, []string{"model"})

	// --- Subagent Metrics ---
	subagentSpawns = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_subagent_spawns_total",
		Help: "Total subagents spawned.",
	}, []string{"model", "role", "skill_matched", "type", "workspace"})

	subagentDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_subagent_duration_seconds",
		Help:    "Duration of subagent execution.",
		Buckets: []float64{5, 10, 30, 60, 120, 300, 600},
	}, []string{"model", "role", "type", "status"})

	subagentDepth = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "picoclaw_subagent_depth",
		Help:    "Nesting depth of subagents.",
		Buckets: []float64{0, 1, 2, 3, 4, 5},
	})

	subagentActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "picoclaw_subagent_active",
		Help: "Number of currently active subagent tasks.",
	}, []string{"workspace"})

	// --- Heartbeat Metrics ---
	heartbeatTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_heartbeat_total",
		Help: "Total heartbeat events.",
	}, []string{"status", "workspace"})

	heartbeatDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "picoclaw_heartbeat_duration_seconds",
		Help: "Duration of heartbeat processing.",
	}, []string{"workspace"})

	// --- Cron Metrics ---
	cronExecutions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_cron_executions_total",
		Help: "Total cron job executions.",
	}, []string{"job_name", "status", "payload_kind"})

	cronDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "picoclaw_cron_execution_duration_seconds",
		Help: "Duration of cron job execution.",
	}, []string{"job_name"})

	cronJobsActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "picoclaw_cron_jobs_active_total",
		Help: "Total currently enabled cron jobs.",
	})

	cronMissed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "picoclaw_cron_missed_total",
		Help: "Total missed cron job runs.",
	})

	// --- Concurrency & Queuing ---
	concurrencyActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "picoclaw_concurrency_active",
		Help: "Number of active LLM sessions per provider.",
	}, []string{"provider_id"})

	concurrencyQueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "picoclaw_concurrency_queue_depth",
		Help: "Number of requests waiting for a slot per provider.",
	}, []string{"provider_id"})

	concurrencyWait = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "picoclaw_concurrency_wait_seconds",
		Help:    "Time spent waiting for a concurrency slot.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
	}, []string{"provider_id"})

	concurrencyRejections = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_concurrency_rejections_total",
		Help: "Total requests rejected due to concurrency limits.",
	}, []string{"provider_id"})

	// --- Session & Context ---
	sessionActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "picoclaw_session_active",
		Help: "Number of active sessions.",
	}, []string{"workspace"})

	sessionRotations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_session_rotations_total",
		Help: "Total session rotation events.",
	}, []string{"workspace"})

	contextCompressions = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_context_compressions_total",
		Help: "Total context compression events.",
	}, []string{"type", "model"})

	summarizationDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "picoclaw_summarization_duration_seconds",
		Help: "Duration of history summarization.",
	}, []string{"model"})

	// --- Message Bus ---
	messagesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_messages_total",
		Help: "Total messages flowing through the bus.",
	}, []string{"channel", "direction", "type"})

	busDrops = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_bus_drops_total",
		Help: "Total messages dropped by the bus.",
	}, []string{"direction"})

	// --- Fallback & Reliability ---
	fallbackAttempts = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_fallback_attempts_total",
		Help: "Total model fallback attempts.",
	}, []string{"provider", "model", "reason", "skipped"})

	fallbackExhausted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "picoclaw_fallback_exhausted_total",
		Help: "Total fallback chain exhaustions (all models failed).",
	})

	cooldownActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "picoclaw_cooldown_active",
		Help: "Number of providers/models currently in cooldown.",
	}, []string{"provider", "model"})

	// --- User & Workspace ---
	userRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_user_requests_total",
		Help: "Total requests per user.",
	}, []string{"user_id", "channel", "workspace", "agent_id"})

	workspaceRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "picoclaw_workspace_requests_total",
		Help: "Total requests per workspace.",
	}, []string{"workspace", "agent_id"})

	// --- System Health ---
	uptimeGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "picoclaw_uptime_seconds",
		Help: "Application uptime in seconds.",
	})

	memoryArchiveDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "picoclaw_memory_archive_duration_seconds",
		Help: "Duration of session archiving to Qdrant.",
	}, []string{"workspace"})

	memorySearchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name: "picoclaw_memory_search_duration_seconds",
		Help: "Duration of vector memory searches.",
	})
)
