package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/mcp"
)

type MCPTool struct {
	manager *mcp.MCPManager
	toolDef mcp.MCPToolDef
}

func NewMCPTool(manager *mcp.MCPManager, toolDef mcp.MCPToolDef) *MCPTool {
	return &MCPTool{
		manager: manager,
		toolDef: toolDef,
	}
}

func (t *MCPTool) Name() string {
	return t.toolDef.Name
}

func (t *MCPTool) Description() string {
	return t.toolDef.Description
}

func (t *MCPTool) Parameters() map[string]any {
	if t.toolDef.InputSchema != nil {
		return t.toolDef.InputSchema
	}
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *MCPTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	parts := strings.SplitN(t.toolDef.Name, "__", 2)
	if len(parts) != 2 {
		return &ToolResult{Err: fmt.Errorf("invalid MCP tool name format")}
	}

	serverName, origToolName := parts[0], parts[1]
	res, err := t.manager.CallTool(ctx, serverName, origToolName, args)
	if err != nil {
		return &ToolResult{Err: err}
	}

	var sb strings.Builder
	for _, c := range res.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
			sb.WriteString("\n")
		}
	}

	content := strings.TrimSpace(sb.String())
	if res.IsError {
		return &ToolResult{Err: fmt.Errorf("MCP tool error: %s", content)}
	}

	return &ToolResult{ForLLM: content}
}
