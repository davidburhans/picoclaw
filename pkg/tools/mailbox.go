package tools

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/mailbox"
)

type SendMessageTool struct {
	client *mailbox.Client
	from   string
}

func NewSendMessageTool(client *mailbox.Client, from string) *SendMessageTool {
	return &SendMessageTool{client: client, from: from}
}

func (t *SendMessageTool) Name() string {
	return "send_message"
}

func (t *SendMessageTool) Description() string {
	return "Sends a message to another workspace or family member."
}

func (t *SendMessageTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"to": map[string]interface{}{
				"type":        "string",
				"description": "Recipient workspace name (e.g., 'default', 'mom', 'kids').",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Content of the message.",
			},
		},
		"required": []string{"to", "content"},
	}
}

func (t *SendMessageTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	to, _ := args["to"].(string)
	content, _ := args["content"].(string)

	msg := mailbox.Message{
		From:    t.from,
		To:      to,
		Content: content,
	}

	if err := t.client.Send(msg); err != nil {
		return &ToolResult{Err: fmt.Errorf("failed to send message: %w", err)}
	}

	return &ToolResult{
		ForUser: fmt.Sprintf("✅ Message sent to %s", to),
		ForLLM:  fmt.Sprintf("Message successfully sent to %s.", to),
	}
}

type ListWorkspacesTool struct {
	cfg *config.Config
}

func NewListWorkspacesTool(cfg *config.Config) *ListWorkspacesTool {
	return &ListWorkspacesTool{cfg: cfg}
}

func (t *ListWorkspacesTool) Name() string {
	return "list_workspaces"
}

func (t *ListWorkspacesTool) Description() string {
	return "Lists all available workspaces for inter-agent communication."
}

func (t *ListWorkspacesTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *ListWorkspacesTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	names := t.cfg.GetWorkspaceNames()
	return &ToolResult{
		ForLLM: fmt.Sprintf("Available workspaces: %v", names),
	}
}
