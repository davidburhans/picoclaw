package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/sipeed/picoclaw/pkg/session"
)

// SessionController is the interface for session management in the agent.
type SessionController interface {
	RotateSession(ctx context.Context, baseKey, archiveName string) (string, string, error)
	GetSessionManager() *session.SessionManager
	GetActiveSession(baseKey string) string
}

// SessionControlTool allows the agent to manage its own sessions.
type SessionControlTool struct {
	controller SessionController
	channel    string
	chatID     string
	mu         sync.RWMutex
}

// NewSessionControlTool creates a new SessionControlTool.
func NewSessionControlTool(controller SessionController) *SessionControlTool {
	return &SessionControlTool{
		controller: controller,
	}
}

// Name returns the tool name.
func (t *SessionControlTool) Name() string {
	return "session_control"
}

// Description returns the tool description.
func (t *SessionControlTool) Description() string {
	return "Control and query chat sessions. Use 'start_new_session' to archive the current conversation and start a fresh one (equivalent to !new command). Use 'get_session_info' to see message count and summary of the current session."
}

// Parameters returns the tool parameters schema.
func (t *SessionControlTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"start_new_session", "get_session_info"},
				"description": "Action to perform.",
			},
			"archive_name": map[string]interface{}{
				"type":        "string",
				"description": "Optional name for the archived session (for start_new_session).",
			},
		},
		"required": []string{"action"},
	}
}

// SetContext sets the current channel/chat context.
func (t *SessionControlTool) SetContext(channel, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.channel = channel
	t.chatID = chatID
}

// Execute runs the tool with the given arguments.
func (t *SessionControlTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	action, ok := args["action"].(string)
	if !ok {
		return ErrorResult("action is required")
	}

	t.mu.RLock()
	channel := t.channel
	chatID := t.chatID
	t.mu.RUnlock()

	if channel == "" || chatID == "" {
		return ErrorResult("no session context available")
	}

	// Base key is usually channel:chatID
	baseKey := fmt.Sprintf("%s:%s", channel, chatID)

	switch action {
	case "start_new_session":
		archiveName, _ := args["archive_name"].(string)
		newKey, archivedKey, err := t.controller.RotateSession(ctx, baseKey, archiveName)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to rotate session: %v", err))
		}
		return NewToolResult(fmt.Sprintf("Successfully started new session: %s. Previous session archived as: %s", newKey, archivedKey))

	case "get_session_info":
		sm := t.controller.GetSessionManager()
		activeKey := t.controller.GetActiveSession(baseKey)
		if activeKey == "" {
			activeKey = baseKey
		}

		history := sm.GetHistory(activeKey)
		summary := sm.GetSummary(activeKey)

		return NewToolResult(fmt.Sprintf("Current Session: %s\nMessages: %d\nSummary: %s", activeKey, len(history), summary))

	default:
		return ErrorResult(fmt.Sprintf("unknown action: %s", action))
	}
}
