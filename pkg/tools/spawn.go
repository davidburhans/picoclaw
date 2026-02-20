package tools

import (
	"context"
	"fmt"
	"sync"
)

type SpawnTool struct {
	manager        *SubagentManager
	originChannel  string
	originChatID   string
	allowlistCheck func(targetAgentID string) bool
	callback       AsyncCallback // For async completion notification
	mu             sync.RWMutex
}

func NewSpawnTool(manager *SubagentManager) *SpawnTool {
	return &SpawnTool{
		manager:       manager,
		originChannel: "cli",
		originChatID:  "direct",
	}
}

// SetCallback implements AsyncTool interface for async completion notification
func (t *SpawnTool) SetCallback(cb AsyncCallback) {
	t.callback = cb
}

func (t *SpawnTool) Name() string {
	return "spawn"
}

func (t *SpawnTool) Description() string {
	return "Spawn a subagent to handle a task in the background. Use this for complex or time-consuming tasks that can run independently. If the role matches an installed skill, the sub-agent is initialized with that skill's knowledge. The subagent will complete the task and report back when done."
}

func (t *SpawnTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task for subagent to complete",
			},
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Optional short label for the task (for display)",
			},
			"role": map[string]interface{}{
				"type":        "string",
				"description": "The role for the sub-agent. If this matches an installed skill name (e.g. 'summarize', 'skill-creator'), the sub-agent is initialized with that skill's knowledge and a listing of its available resources. Otherwise it is used as a free-text persona (e.g. 'Senior Go Engineer'). Check the <skills> block in your context before choosing a role.",
			},
			"context_files": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "File paths to read and inject as context before the task starts.",
			},
			"agent_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional target agent ID to delegate the task to",
			},
		},
		"required": []string{"task"},
	}
}

func (t *SpawnTool) SetContext(channel, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.originChannel = channel
	t.originChatID = chatID
}

func (t *SpawnTool) SetAllowlistChecker(check func(targetAgentID string) bool) {
	t.allowlistCheck = check
}

func (t *SpawnTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	task, ok := args["task"].(string)
	if !ok {
		return ErrorResult("task is required")
	}

	label, _ := args["label"].(string)
	role, _ := args["role"].(string)
	var contextFiles []string
	if cf, ok := args["context_files"].([]interface{}); ok {
		for _, f := range cf {
			if s, ok := f.(string); ok {
				contextFiles = append(contextFiles, s)
			}
		}
	}
	agentID, _ := args["agent_id"].(string)

	// Check allowlist if targeting a specific agent
	if agentID != "" && t.allowlistCheck != nil {
		if !t.allowlistCheck(agentID) {
			return ErrorResult(fmt.Sprintf("not allowed to spawn agent '%s'", agentID))
		}
	}

	if t.manager == nil {
		return ErrorResult("Subagent manager not configured")
	}

	t.mu.RLock()
	originChannel := t.originChannel
	originChatID := t.originChatID
	t.mu.RUnlock()

	// Pass callback to manager for async completion notification
	result, err := t.manager.Spawn(ctx, task, label, role, agentID, contextFiles, originChannel, originChatID, t.callback)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to spawn subagent: %v", err))
	}

	// Return AsyncResult since the task runs in background
	return AsyncResult(result)
}
