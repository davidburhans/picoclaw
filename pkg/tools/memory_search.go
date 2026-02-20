package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/memory"
)

type MemorySearchTool struct {
	manager     *memory.Manager
	workspaceID string
}

func NewMemorySearchTool(manager *memory.Manager, workspaceID string) *MemorySearchTool {
	return &MemorySearchTool{
		manager:     manager,
		workspaceID: workspaceID,
	}
}

func (t *MemorySearchTool) Name() string {
	return "memory_search"
}

func (t *MemorySearchTool) Description() string {
	return `Search past sessions by semantic similarity. Use this when the user references something from a previous conversation that is not in the current context. Results are ranked by relevance to your query.

Use memory_browse instead if you want results ordered by date (most recent or oldest first) rather than by relevance.`
}

func (t *MemorySearchTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The topic or question to search for in past sessions.",
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 5).",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MemorySearchTool) Execute(ctx context.Context, input map[string]interface{}) *ToolResult {
	if t.manager == nil {
		return SilentResult("Long-term memory is not enabled.")
	}

	query, _ := input["query"].(string)
	if query == "" {
		return ErrorResult("query is required for memory_search; use memory_browse to list sessions chronologically")
	}

	limit := 5
	if l, ok := input["limit"].(float64); ok {
		limit = int(l)
	}

	results, err := t.manager.Search(ctx, t.workspaceID, query, limit, 0)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to search memory: %v", err))
	}

	if len(results) == 0 {
		return UserResult("No relevant memories found.")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d relevant memories:\n\n", len(results)))
	for i, r := range results {
		content, _ := r.Payload["content"].(string)
		sessionID, _ := r.Payload["session_id"].(string)
		timestampStr := formatTimestamp(r.Payload["timestamp"])
		sb.WriteString(fmt.Sprintf("--- Memory %d (Session: %s, Score: %.3f, Date: %s) ---\n", i+1, sessionID, r.Score, timestampStr))
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}

	return UserResult(sb.String())
}

// formatTimestamp converts a Qdrant payload timestamp (int64, float64, or string) to a human-readable string.
func formatTimestamp(ts interface{}) string {
	if ts == nil {
		return "unknown"
	}
	var unixTs int64
	switch v := ts.(type) {
	case int64:
		unixTs = v
	case float64:
		unixTs = int64(v)
	case int:
		unixTs = int64(v)
	case string:
		return v
	}
	if unixTs > 0 {
		return time.Unix(unixTs, 0).Format("2006-01-02 15:04:05")
	}
	return "unknown"
}
