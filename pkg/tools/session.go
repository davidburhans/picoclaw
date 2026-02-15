package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/session"
)

type ReadSessionTool struct {
	workspace string
}

func NewReadSessionTool(workspace string) *ReadSessionTool {
	return &ReadSessionTool{
		workspace: workspace,
	}
}

func (t *ReadSessionTool) Name() string {
	return "read_session"
}

func (t *ReadSessionTool) Description() string {
	return "Reads the content of a past chat session by its session key. Use this to reference previous conversations."
}

func (t *ReadSessionTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"session_key": map[string]interface{}{
				"type":        "string",
				"description": "The session key (filename without extension) to read, e.g., 'discord_12345_v1_task'",
			},
		},
		"required": []string{"session_key"},
	}
}

func (t *ReadSessionTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	key, ok := args["session_key"].(string)
	if !ok {
		return ErrorResult("session_key must be a string")
	}

	// Sanitize key (basic check)
	if strings.Contains(key, "..") || strings.Contains(key, "/") || strings.Contains(key, "\\") {
		return ErrorResult("invalid session key")
	}

	path := filepath.Join(t.workspace, "sessions", key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrorResult(fmt.Sprintf("session file not found: %s", key))
		}
		return ErrorResult(fmt.Sprintf("failed to read session file: %v", err))
	}

	var sess session.Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return ErrorResult(fmt.Sprintf("failed to parse session file: %v", err))
	}

	// Format output
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Session: %s\n", sess.Key))
	sb.WriteString(fmt.Sprintf("Created: %s\n", sess.Created))
	if sess.Summary != "" {
		sb.WriteString(fmt.Sprintf("Summary: %s\n", sess.Summary))
	}
	sb.WriteString("\nMessages:\n")
	for _, msg := range sess.Messages {
		role := msg.Role
		content := msg.Content
		// Truncate content slightly if huge? Maybe not.
		sb.WriteString(fmt.Sprintf("[%s] %s\n", role, content))
	}

	return NewToolResult(sb.String())
}
