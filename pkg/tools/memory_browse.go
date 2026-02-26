package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/memory"
)

type MemoryBrowseTool struct {
	manager     *memory.Manager
	workspaceID string
}

func NewMemoryBrowseTool(manager *memory.Manager, workspaceID string) *MemoryBrowseTool {
	return &MemoryBrowseTool{
		manager:     manager,
		workspaceID: workspaceID,
	}
}

func (t *MemoryBrowseTool) Name() string {
	return "memory_browse"
}

func (t *MemoryBrowseTool) Description() string {
	return `Find past sessions related to a topic, sorted chronologically. Useful for questions like "when did we first discuss X?" or "what's the most recent session about Y?"

Unlike memory_search (which ranks by relevance), results here are ordered by date so you can see how a topic evolved over time.`
}

func (t *MemoryBrowseTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The topic to find sessions about.",
			},
			"order": map[string]interface{}{
				"type":        "string",
				"description": "Date sort order: 'desc' for most recent first (default), 'asc' for oldest first.",
				"enum":        []string{"asc", "desc"},
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of results to return (default: 5).",
			},
		},
		"required": []string{"query"},
	}
}

func (t *MemoryBrowseTool) Execute(ctx context.Context, input map[string]interface{}) *ToolResult {
	if t.manager == nil {
		return SilentResult("Long-term memory is not enabled.")
	}

	query, _ := input["query"].(string)
	if query == "" {
		return ErrorResult("query is required for memory_browse")
	}

	order := "desc"
	if o, ok := input["order"].(string); ok && (o == "asc" || o == "desc") {
		order = o
	}

	limit := 5
	if l, ok := input["limit"].(float64); ok {
		limit = int(l)
	}

	results, err := t.manager.SearchByDate(ctx, t.workspaceID, query, limit, order)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to browse memory: %v", err))
	}

	if len(results) == 0 {
		return UserResult(fmt.Sprintf("No sessions found related to %q.", query))
	}

	direction := "most recent first"
	if order == "asc" {
		direction = "oldest first"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d sessions related to %q (%s):\n\n", len(results), query, direction))
	for i, r := range results {
		content, _ := r.Payload["content"].(string)
		sessionID, _ := r.Payload["session_id"].(string)
		timestampStr := formatTimestamp(r.Payload["timestamp"])
		sb.WriteString(fmt.Sprintf("--- Session %d (ID: %s, Date: %s) ---\n", i+1, sessionID, timestampStr))
		sb.WriteString(content)
		sb.WriteString("\n\n")
	}

	return UserResult(sb.String())
}
