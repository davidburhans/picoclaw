# PicoClaw Feature Porting Plan

## Why This Plan Exists

PicoClaw is forked from [OpenClaw](https://github.com/openclaw/openclaw), rewritten in Go for
simplicity, performance, and full control. The upstream `og/main` branch provides the clean
architecture (protocol-based providers, simplified agent loop, single-binary deployment), while
`origin/main` (our fork) contains all the production features we've built over weeks of iteration.

**The goal is to port battle-tested features from `origin/main` onto the cleaner `og/main`
foundation** — keeping the architectural improvements while restoring full functionality.

### Why Not Pivot?

We evaluated OpenClaw (TypeScript), IronClaw (Rust), NanoClaw (TypeScript/Claude SDK), ZeroClaw
(Rust), and nanobot (Python). None are viable because: (1) our codebase and expertise is Go,
(2) our custom features (family safety, mailbox, Qdrant memory, metrics/dashboard) don't exist
upstream, and (3) we already have working code to port.

### Multi-Instance Architecture Decision

> [!IMPORTANT]
> **We chose multi-instance over multi-workspace.** Instead of one PicoClaw process managing
> multiple workspaces with context switching, each user/family-member runs their own PicoClaw
> container with its own config and safety level. An **Orchestrator MCP Server** (separate
> container in the same docker-compose) provides inter-instance communication (mailbox) and
> shared family tools (chores, lists) via MCP. This keeps each PicoClaw instance simple,
> avoids complex workspace routing, and makes the fork resilient to upstream changes.

---

## What Already Exists on og/main

These features are **already on the current branch** and do NOT need porting:

| Feature | Implementation | Description |
|---------|---------------|-------------|
| Model failover | `pkg/providers/fallback.go` | `FallbackChain` tries each candidate model in order; `CooldownTracker` temporarily disables providers that return errors. Respects error classification to avoid retrying non-retriable errors. |
| Concurrency control | `pkg/providers/cooldown.go` | `CooldownTracker` puts providers on timed cooldown after errors. Exponential backoff. |
| Subagents (sync) | `pkg/tools/subagent.go` | `SubagentTool` — runs a task synchronously using the same LLM provider. Returns result directly. Has access to the same tool registry. |
| Subagents (async) | `pkg/tools/spawn.go` | `SpawnTool` — spawns a background goroutine via `SubagentManager.Spawn()`. Returns immediately with a task ID. Supports `AsyncCallback` for completion notification (e.g., Discord message). |
| Skills platform | `pkg/skills/` | `SkillsLoader` reads skill YAML+markdown from workspace, global, and builtin directories. `ClawHubRegistry` searches/installs skills from the public ClawHub API. Skills are injected into the system prompt. |
| File-based memory | `pkg/agent/memory.go` | `MemoryStore` — long-term memory in `memory/MEMORY.md`, daily notes in `memory/YYYYMM/YYYYMMDD.md`. Injected into agent prompt via `GetMemoryContext()`. |
| Session routing | `pkg/routing/session_key.go` | Generates unique session keys per channel: `discord:<channelID>`, `telegram:<chatID>`, etc. |
| Error classification | `pkg/providers/error_classifier.go` | Classifies LLM errors as token limit, rate limit, timeout, auth, or unknown. Used by `FallbackChain` to decide whether to retry. |

---

## Features to Port (Priority Order)

### Phase 1: MCP Support

**Why**: MCP (Model Context Protocol) is the standard for connecting LLM agents to external
tools. Without it, every integration must be a native Go tool. With MCP, users connect to
hundreds of existing servers (filesystem, databases, GitHub, etc.) with just config.

**Status**: ✅ IN PROGRESS

**Config format**: We adopt the `mcpServers` format used by Claude Desktop, Cursor, and nanobot.
Users can copy MCP configs directly from any MCP server's README:

```json
{
  "tools": {
    "mcpServers": {
      "filesystem": {
        "command": "npx",
        "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path"]
      },
      "remote-api": {
        "url": "https://example.com/mcp/",
        "headers": { "Authorization": "Bearer xxxxx" }
      }
    }
  }
}
```

#### What's in origin/main

```
pkg/mcp/
├── types.go    # MCPServerConfig, MCPTool, MCPToolResult structs
├── client.go   # Stdio client: spawns subprocess, JSON-RPC over stdin/stdout
└── manager.go  # Manages multiple MCP server connections, tool discovery
```

**How it works**: `Manager.Start()` iterates over configured MCP servers, spawns each as a
subprocess (via `command` + `args`), performs the MCP `initialize` handshake, then calls
`tools/list` to discover available tools. These tools are wrapped as native PicoClaw `Tool`
interfaces and registered in the tool registry alongside built-in tools. When the LLM calls
an MCP tool, the manager routes the call to the appropriate subprocess via `tools/call`.

#### Files Ported/Created
1. [x] `pkg/mcp/types.go` — 14 tests passing
2. [x] `pkg/mcp/client.go` — 14 tests passing
3. [x] `pkg/config/config.go` — MCP config added

#### Remaining
- [ ] Register MCP tools in `pkg/agent/loop.go`
- [ ] Add MCP section to `config/config.example.json`

---

### Phase 2: Safety Filter

**Why**: For family use, different PicoClaw instances need age-appropriate content filtering.
A child's instance should block explicit content; a parent's instance runs unfiltered. Since
we're using multi-instance architecture, safety is just a per-instance config setting.

#### What's in origin/main

```
pkg/safety/
├── filter.go       # Content filtering engine
├── filter_test.go
└── prompt.go       # System prompt injection for safety context
```

**How `Filter` works**: `NewFilter(level, birthYear)` creates a filter. The filter has four
levels and two checks:

**Input check** (`CheckContent`): Keyword-based blocking. Tests if user input contains
blocked terms.
- `off` — no filtering
- `low` — blocks adult keywords (violence, weapons, drugs, explicit, etc.)
- `medium` — blocks adult keywords + dangerous keywords (suicide, murder, hack, steal, etc.)
- `high` — blocks all of the above + age-specific topics for young users (<13: dating,
  politics, religion)

**Response check** (`CheckResponse`): Returns a `CheckResult` struct with:
- `Safe` / `Blocked` / `NeedsApproval` flags
- `BlockedMessage` — age-appropriate replacement text
- `Rewrite` flag for future LLM-based content rewriting

**System prompt** (`GetSystemPrompt`): Injects safety context into the agent's system prompt
based on birth year. For young users (<13): "Use simple vocabulary, short sentences." For
teens (13-17): "Be helpful but mindful of age-appropriate content."

**Key types**:
```go
type Filter struct {
    level     string  // "off", "low", "medium", "high"
    birthYear int     // Used to compute age for dynamic filtering
}

type CheckResult struct {
    Safe           bool
    Blocked        bool
    NeedsApproval  bool   // High safety + young user = flag for parent review
    Reason         string
    BlockedMessage string // "Ask a parent or guardian..."
}
```

#### Config
```json
{
  "agents": {
    "defaults": {
      "safety_level": "medium",
      "birth_year": 2015
    }
  }
}
```

#### Files to Port
1. `pkg/safety/filter.go` — Copy from origin/main
2. `pkg/safety/filter_test.go` — Copy from origin/main
3. `pkg/safety/prompt.go` — Copy from origin/main
4. Update `pkg/config/config.go` — Add `SafetyLevel` and `BirthYear` to agent config
5. Update `pkg/agent/loop.go` — Run `CheckContent` on input, `CheckResponse` on output,
   inject `GetSystemPrompt` into system message

---

### Phase 3: Orchestrator MCP Server _(separate project)_

**Why**: With containerized multi-instance architecture (no shared filesystem), inter-instance
communication and shared family data need a network service. Rather than building custom IPC
into PicoClaw, we use MCP — which PicoClaw already supports after Phase 1. The orchestrator
is a standalone MCP server running in its own container in the same `docker-compose.yml`.

> [!IMPORTANT]
> **This is NOT a PicoClaw code change.** `pkg/mailbox/` and `pkg/family/` from `origin/main`
> are **not ported** to PicoClaw. Instead, their functionality moves to a separate orchestrator
> service. Each PicoClaw instance connects to it via its `mcpServers` config.

#### Architecture

```
┌─────────────────────────────────────────────────────┐
│ docker-compose.yml                                  │
│                                                     │
│  ┌──────────────┐  ┌──────────────┐                 │
│  │  picoclaw-dad │  │ picoclaw-kid │  ...more        │
│  │  (container)  │  │  (container) │                 │
│  │              │  │              │                 │
│  │  mcpServers: │  │  mcpServers: │                 │
│  │   orchestrator│  │   orchestrator│                │
│  └──────┬───────┘  └──────┬───────┘                 │
│         │    MCP (stdio/  │                          │
│         │    network)     │                          │
│         └────────┬────────┘                          │
│           ┌──────▼──────┐                            │
│           │ orchestrator │                           │
│           │  (container) │                           │
│           │              │                           │
│           │  MCP Server  │                           │
│           │  - mailbox   │                           │
│           │  - chores    │                           │
│           │  - lists     │                           │
│           │              │                           │
│           │  SQLite/JSON │                           │
│           └──────────────┘                           │
│                                                     │
│  ┌──────────┐  ┌─────────┐  ┌──────────┐            │
│  │prometheus │  │ grafana │  │  qdrant  │            │
│  └──────────┘  └─────────┘  └──────────┘            │
└─────────────────────────────────────────────────────┘
```

#### MCP Tools Provided by Orchestrator

**Mailbox** (inter-instance messaging):
| Tool | Description |
|------|-------------|
| `send_message` | Send a message to another instance's inbox |
| `list_messages` | List messages in own inbox |
| `read_message` | Read a specific message by ID |
| `list_instances` | List all registered PicoClaw instances |

**Family — Chores**:
| Tool | Description |
|------|-------------|
| `assign_chore` | Assign a chore to a family member (instance) |
| `list_chores` | View chores (own or all, depending on permissions) |
| `complete_chore` | Mark a chore as done |
| `verify_chore` | Parent approval of completed chore |

**Family — Shared Lists**:
| Tool | Description |
|------|-------------|
| `create_list` | Create a new shared list |
| `add_to_list` | Add item to a list |
| `get_list` | View list items |
| `remove_from_list` | Remove item from list |
| `delete_list` | Delete a list |

#### PicoClaw Config (per instance)

Each PicoClaw instance just needs the orchestrator in its MCP config:

```json
{
  "tools": {
    "mcpServers": {
      "orchestrator": {
        "url": "http://orchestrator:8080/mcp",
        "headers": { "X-Instance-ID": "dad" }
      }
    }
  }
}
```

The `X-Instance-ID` header identifies which instance is calling, so the orchestrator
can scope mailbox and chore queries to the correct inbox.

#### Implementation Notes
- Orchestrator can be Go, Python, or any language with MCP server support
- Storage: SQLite (single file, easy to back up) or JSON files in its own volume
- Instance registration: auto-register on first MCP connection, or static config
- Permissions: orchestrator knows which instances are "parents" (can verify chores,
  see all messages) vs "children" (scoped access) based on config

---

### Phase 4: Schedule Provider

**Why**: Different times of day warrant different cost/quality tradeoffs. Use expensive models
during work hours when quality matters, cheap models overnight for background tasks. The
schedule provider routes LLM requests to different providers based on day-of-week and
time-of-day rules.

> [!NOTE]
> **Dropped from origin/main**: The `OverflowProvider` and `ConcurrencyTracker` are **not
> being ported**. Their functionality (failover when a provider is at capacity or errors out)
> is already handled by `FallbackChain` + `CooldownTracker` on `og/main`, which is a cleaner
> implementation with proper error classification.

#### What's in origin/main

```
pkg/providers/
├── schedule_provider.go       # Time-based provider routing
└── schedule_provider_test.go
```

**How it works**: `ScheduleProvider` implements `LLMProvider`. On every `Chat()` call, it
checks the current time against an ordered list of rules to decide which real provider +
model to delegate to.

```go
type ScheduleProvider struct {
    cfg      *config.Config
    schedule *config.ScheduleConfig
    location *time.Location     // Timezone for rule matching
    nowFunc  func() time.Time   // Injectable clock for testing
}
```

**Rule matching** (`matchRule(time)`):
1. Converts current time to the configured timezone
2. Iterates over `schedule.Rules` in order
3. For each rule, checks `Days` — supports individual days (`"mon"`, `"tue"`...`"sun"`),
   plus shortcuts `"weekday"` (mon-fri) and `"weekend"` (sat-sun)
4. Checks `Hours` — start/end in `"HH:MM"` format. Supports:
   - Same-day spans: `"09:00"` to `"17:00"` — matches if current time is within range
   - Overnight spans: `"22:00"` to `"06:00"` — matches if current time is AFTER start
     OR BEFORE end
5. First matching rule wins → returns that rule's `provider` and `model`
6. If no rule matches → uses `schedule.Default`

**Provider resolution**: The matched provider string is used to create a real `LLMProvider`
via `CreateProvider()`. A shallow clone of config is made to avoid mutating the original.
Recursive schedule providers (schedule referencing schedule) are explicitly rejected.

**Config example**:
```json
{
  "agents": {
    "defaults": {
      "provider": "schedule",
      "schedule": {
        "timezone": "America/Chicago",
        "default": { "provider": "openrouter", "model": "claude-sonnet-4-20250514" },
        "rules": [
          {
            "days": ["weekday"],
            "hours": { "start": "09:00", "end": "17:00" },
            "provider": "anthropic",
            "model": "claude-sonnet-4-20250514"
          },
          {
            "days": ["weekend"],
            "provider": "openrouter",
            "model": "deepseek/deepseek-chat"
          }
        ]
      }
    }
  }
}
```

**Config types needed**:
```go
type ScheduleConfig struct {
    Timezone string         `json:"timezone"`
    Default  ScheduleRule   `json:"default"`
    Rules    []ScheduleRule `json:"rules"`
}

type ScheduleRule struct {
    Days     []string       `json:"days,omitempty"`     // "mon".."sun", "weekday", "weekend"
    Hours    *ScheduleHours `json:"hours,omitempty"`
    Provider string         `json:"provider"`
    Model    string         `json:"model,omitempty"`
}

type ScheduleHours struct {
    Start string `json:"start"` // "HH:MM" format
    End   string `json:"end"`   // "HH:MM" format
}
```

#### Files to Port
1. `pkg/providers/schedule_provider.go` — Copy from origin/main
2. `pkg/providers/schedule_provider_test.go` — Copy from origin/main
3. Update `pkg/providers/factory.go` — Register `schedule` provider type
4. Update `pkg/config/config.go` — Add `ScheduleConfig` to agent config

---

### Phase 5: Metrics & Observability

**Why**: Without metrics you can't tell which providers are slow, which channels get the most
traffic, or whether the agent is hitting rate limits. The Prometheus + Grafana stack provides
real-time production visibility.

#### What's in origin/main

```
pkg/metrics/
├── metrics.go       # 40+ Prometheus metric definitions (promauto)
├── recorder.go      # High-level recording methods (RecordLLMCall, RecordToolCall, etc.)
└── metrics_test.go

pkg/providers/
└── metrics_wrapper.go  # LLMProvider decorator for automatic metrics collection

pkg/dashboard/
├── server.go        # HTTP server: /health, /ready, /metrics, /api/status, /api/activity
├── config_api.go    # Config read/write API
├── schema.go        # Dashboard JSON schema
├── dashboard_test.go
└── static/          # Embedded web UI (HTML/CSS/JS)
    ├── index.html
    ├── dashboard.css
    └── dashboard.js

config/
├── prometheus.yml                             # Prometheus scrape config
├── grafana/provisioning/dashboards/picoclaw.yml  # Grafana auto-provisioning
docker-compose.yml                             # Prometheus + Grafana containers
Dockerfile                                     # PicoClaw with metrics endpoint
```

#### Metrics Collected (Prometheus)

**LLM Performance** (labeled by model, provider, agent_type):
- `picoclaw_llm_request_duration_seconds` — Histogram of LLM call latencies
- `picoclaw_llm_tokens_prompt_total` / `completion_total` — Token usage counters
- `picoclaw_llm_context_size_tokens` — Context window utilization
- `picoclaw_llm_errors_total` — Errors by type (rate_limit, timeout, auth, etc.)
- `picoclaw_llm_requests_total` — Total request counter

**Tool Usage**:
- `picoclaw_tool_calls_total` — Tool executions by name and status
- `picoclaw_tool_duration_seconds` — Tool execution latency
- `picoclaw_tool_result_size_bytes` — Result payload sizes

**Agent Turns** (labeled by model, channel, workspace):
- `picoclaw_agent_response_duration_seconds` — End-to-end user→response latency
- `picoclaw_agent_iterations_per_turn` — LLM+tool loop iterations per turn
- `picoclaw_agent_tools_per_turn` — Tool calls per turn

**Concurrency & Queuing**:
- `picoclaw_concurrency_active` — Active sessions per provider (gauge)
- `picoclaw_concurrency_queue_depth` — Waiting requests (gauge)
- `picoclaw_concurrency_wait_seconds` — Queue wait time
- `picoclaw_concurrency_rejections_total` — Rejected requests

**Subagents, Heartbeat, Cron, Sessions, Fallback** — each with their own counters,
histograms, and gauges. ~40 metrics total.

#### MetricsWrapper

A `LLMProvider` decorator in `metrics_wrapper.go`. Wraps any provider to automatically
record call duration, token usage, and error status on every `Chat()` call:

```go
type MetricsWrapper struct { LLMProvider }

func (w *MetricsWrapper) Chat(ctx, messages, tools, model, options) (*LLMResponse, error) {
    start := time.Now()
    resp, err := w.LLMProvider.Chat(...)
    metrics.DefaultRecorder().RecordLLMCall(model, providerID, apiBase, agentType, status, duration, usage, 0)
    return resp, err
}
```

#### Recorder

Singleton `DefaultRecorder()` provides typed methods: `RecordLLMCall()`, `RecordToolCall()`,
`RecordAgentTurn()`, `RecordHeartbeat()`, `RecordCronExecution()`, `SetConcurrency()`,
`RecordFallback()`, etc. Each method updates the appropriate Prometheus counter/histogram.

#### Dashboard Server

HTTP server on configurable port. Endpoints:
- `/health`, `/ready` — Liveness/readiness probes
- `/metrics` — Prometheus scrape endpoint (`promhttp.Handler()`)
- `/api/status` — JSON status (uptime, version)
- `/api/activity` — Ring buffer of recent message bus events (inbound/outbound)
- `/api/config` — Read/write config API
- `/dashboard/static/` — Embedded web UI (HTML/CSS/JS SPA)

The `ActivityBuffer` subscribes to the `MessageBus` and records the last 100 events in a
thread-safe ring buffer.

#### Files to Port
1. `pkg/metrics/` — Copy entire package
2. `pkg/providers/metrics_wrapper.go` — Copy from origin/main
3. `pkg/dashboard/` — Copy entire package (including `static/`)
4. `config/prometheus.yml`, `config/grafana/`, `docker-compose.yml`, `Dockerfile`
5. Update `pkg/agent/loop.go` — Wrap provider with `MetricsWrapper`, record agent turns
6. Update `pkg/tools/toolloop.go` — Record tool call metrics

---

### Phase 6: Qdrant Long-Term Memory

**Why**: File-based memory (`MEMORY.md`) works for short-term context but doesn't scale.
Qdrant vector search lets the agent recall relevant information from months-old conversations.
This is the difference between an agent that forgets each session and one that knows you.

#### What's in origin/main

```
pkg/memory/
├── types.go                # VectorDB and Embedder interfaces
├── manager.go              # Memory lifecycle: archive, search, search-by-date
├── manager_test.go
├── qdrant/
│   ├── client.go           # Qdrant gRPC client
│   └── client_test.go
└── embedding/
    └── client.go           # OpenAI-compatible embedding API client
```

#### Interfaces

```go
// VectorDB — abstraction over any vector database (Qdrant is the only impl)
type VectorDB interface {
    Store(ctx, collection string, record VectorRecord) error
    Search(ctx, collection string, vector []float32, limit, offset int, filters map[string]interface{}) ([]SearchResult, error)
    EnsureCollection(ctx, name string, dimension int) error
    Close() error
}

// Embedder — abstraction over any embedding API
type Embedder interface {
    Embed(ctx, text string) ([]float32, error)
    Dimension() int  // 1536 for text-embedding-3-small, 3072 for text-embedding-3-large
}
```

#### Manager — Session Archiving

`ArchiveSession(ctx, workspaceID, sessionID, messages)`:
1. Concatenates non-system messages into text
2. Chunks text with **sliding window** (default 4096 chars, 10% overlap)
3. Generates embeddings for each chunk via `Embedder`
4. Auto-creates Qdrant collection with correct dimension on first use
5. Stores each chunk as a Qdrant point with payload: `workspace_id`, `session_id`,
   `content`, `timestamp`, `chunk_index`, `total_chunks`
6. Uses **deterministic UUIDs** (`uuid.NewMD5`) for point IDs — Qdrant requires UUIDs
   or uint64, not arbitrary strings

#### Manager — Search

Two search modes (separate tools to avoid Qdrant `order_by` index errors):

**`Search(ctx, workspaceID, query, limit, offset)`** — Pure vector similarity search.
Embeds the query, searches Qdrant with workspace filter, returns by similarity score.

**`SearchByDate(ctx, workspaceID, query, limit, order)`** — Hybrid: fetches 10x candidates
by similarity, then re-sorts client-side by timestamp (asc/desc). Needed because Qdrant
can't do `order_by` + vector search in the same query.

#### Qdrant Client Details

- Parses URL to extract host/port, auto-switches HTTP port (6333) to gRPC port (6334)
- Creates collection with timestamp payload index for efficient date filtering
- Workspace-scoped searches (each query filtered by `workspace_id`)

#### Agent Tools

- `memory_search` — Vector similarity search. Input: `query`, `limit`. Returns scored results
  with content snippets.
- `memory_browse` — Chronological browse. Input: `limit`, `order` (asc/desc). No query
  needed — just lists recent/oldest sessions.

#### Key Lessons Learned (Must Carry Forward)
- Use `uuid.NewMD5` for deterministic point IDs (not sequential ints)
- Parse Qdrant URL to extract host/port; auto-switch 6333→6334 for gRPC
- Filter tool messages during session summary (they're noise for embeddings)
- `memory_search` and `memory_browse` MUST be separate tools

#### Files to Port
1. `pkg/memory/` — Copy entire package
2. Port `memory_search` and `memory_browse` tools
3. Update `pkg/config/config.go` — Add `MemoryConfig` with Qdrant/embedding settings
4. Update `pkg/session/manager.go` — Call `ArchiveSession` on session rotate

---

### Phase 7: Session Key Verification

**Status**: ✅ PARTIALLY EXISTS — `pkg/routing/session_key.go` is on `og/main`.

Verify all channel implementations correctly pass session keys to `HandleMessage`:
- [ ] Discord: `discord:<channelID>`
- [ ] Telegram: `telegram:<chatID>`
- [ ] Slack: `slack:<channelID>`
- [ ] WhatsApp: `whatsapp:<chatID>`
- [ ] All others

---

### Phase 8: Webhooks (New Feature)

**Why**: Cron handles scheduled tasks; webhooks handle reactive events (GitHub pushes, deploy
alerts, payment notifications). Adds a `/webhook/:id` endpoint to the gateway that routes
incoming HTTP POSTs to a configured agent as messages.

**Status**: ❌ NOT BUILT — new feature inspired by OpenClaw and IronClaw.

```json
{
  "webhooks": {
    "github-deploy": {
      "secret": "whsec_...",
      "agent": "default"
    }
  }
}
```

---

### Phase 9: Group Message Routing (New Feature)

**Why**: Group chats (Discord servers, Telegram groups) need different behavior than DMs:
mention gating (only respond when @mentioned), reply tracking, per-group session isolation.

**Status**: ❌ NOT BUILT — needs design.

---

### Phase 10: Voice (TTS/STT) — Deferred

Explicitly deferred. If re-added: check origin/main commits `5088676`, `85d98ad`, `9e933cb`.

---

## Implementation Checklist

### Pre-requisites
- [x] Create branch: `git checkout -b feature/port-from-fork`
- [x] Reset to og/main: `git reset --hard og/main`
- [x] Test clean build: `go build ./cmd/picoclaw/`

### Phase 1: MCP _(in progress)_
- [x] Port pkg/mcp/ (types, client) — 28 tests passing
- [x] Add MCP config to pkg/config/config.go
- [x] Register MCP tools in pkg/agent/loop.go
- [x] Add mcpServers to config/config.example.json

### Phase 2: Safety Filter
- [x] Copy pkg/safety/ from origin/main
- [x] Add SafetyLevel + BirthYear to agent config
- [x] Integrate CheckContent/CheckResponse in agent loop
- [x] Inject GetSystemPrompt into system message

### Phase 3: Orchestrator MCP Server _(separate project)_
- [x] Choose language/framework for MCP server
- [x] Implement mailbox (send/list/read)
- [x] Implement chores (assign/list/complete/verify)
- [x] Implement shared lists (create/add/update/delete)
- [x] Add to docker-compose as sidecar
- [x] Add orchestrator container to docker-compose.yml
- [x] Configure PicoClaw instances with orchestrator MCP connection

### Phase 4: Schedule Provider
- [x] Copy schedule_provider.go + tests from origin/main
- [x] Register schedule provider type in factory
- [x] Add ScheduleConfig to config

### Phase 5: Metrics & Dashboard
- [ ] Copy pkg/metrics/ and pkg/dashboard/ from origin/main
- [ ] Copy metrics_wrapper.go from origin/main
- [ ] Port config/prometheus.yml, config/grafana/, docker-compose.yml, Dockerfile
- [ ] Wire MetricsWrapper + recorder calls into agent loop and tool loop

### Phase 6: Qdrant Long-Term Memory
- [ ] Copy pkg/memory/ from origin/main
- [ ] Port memory_search and memory_browse tools
- [ ] Add MemoryConfig to config
- [ ] Add session archiving to session manager
- [ ] Verify UUID point IDs and URL parsing fixes

### Phase 7: Session Keys
- [ ] Audit all channel impls for correct session key passing

### Phase 8: Webhooks
- [ ] Design and implement /webhook/:id gateway endpoint

### Phase 9: Group Routing
- [ ] Design mention gating and per-group isolation

### Final
- [ ] `go test ./...`
- [ ] `golangci-lint run`
- [ ] `make build-all`

---

## Key Design Decisions

1. **Multi-instance, not multi-workspace** — Each user/family-member runs their own PicoClaw
   container. No shared filesystem. An orchestrator container provides inter-instance
   communication and shared state via MCP.
2. **Orchestrator as MCP server** — Mailbox, chores, and lists are NOT PicoClaw core features.
   They're MCP tools served by a separate orchestrator service. `pkg/mailbox/` and
   `pkg/family/` from origin/main are **not ported**.
3. **MCP config**: `mcpServers` format compatible with Claude Desktop / Cursor
4. **Model failover**: `FallbackChain` + `CooldownTracker` on og/main handle all failover.
   `OverflowProvider` and `ConcurrencyTracker` from origin/main are **dropped** (redundant).
   `ScheduleProvider` is ported separately — it's time-based routing, not failover.
5. **Subagents**: Already working — sync `SubagentTool` + async `SpawnTool`, skill-backed
6. **Testing**: TDD — tests before each phase

---

## References

### Commits
- MCP: `e8e0fe0`, `df72a4a`, `403e048`
- Voice: `5088676`, `85d98ad`, `9e933cb` (deferred)
- Family: `b447bd8`

### Key Files (origin/main)
- `pkg/config/config.go` — Full config with all features
- `pkg/agent/loop.go` — Agent with MCP, safety integration
- `pkg/tools/registry.go` — Tool registration
- `pkg/mailbox/` — Reference for orchestrator mailbox implementation
- `pkg/family/` — Reference for orchestrator chore/list implementation

### Key Files (og/main — current)
- `pkg/providers/fallback.go` — FallbackChain + CooldownTracker
- `pkg/tools/subagent.go` + `spawn.go` — Subagent system
- `pkg/skills/` — Skills platform with ClawHub
- `pkg/agent/memory.go` — File-based memory
- `pkg/routing/session_key.go` — Session key generation
