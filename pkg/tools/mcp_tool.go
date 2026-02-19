package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/mcp"
)

// MCPToolAdapter wraps an MCP tool as a picoclaw Tool
type MCPToolAdapter struct {
	serverName  string
	toolName    string
	description string
	inputSchema map[string]interface{}
	client      *mcp.MCPClient
}

// NewMCPToolAdapter creates a new MCP tool adapter
func NewMCPToolAdapter(serverName, toolName, description string, inputSchema map[string]interface{}, client *mcp.MCPClient) *MCPToolAdapter {
	return &MCPToolAdapter{
		serverName:  serverName,
		toolName:    toolName,
		description: description,
		inputSchema: inputSchema,
		client:      client,
	}
}

// Name returns the namespaced tool name (mcp_<server>_<tool>)
func (t *MCPToolAdapter) Name() string {
	// Sanitize names to avoid issues
	server := strings.ReplaceAll(t.serverName, "-", "_")
	tool := strings.ReplaceAll(t.toolName, "-", "_")
	return fmt.Sprintf("mcp_%s_%s", server, tool)
}

// Description returns the tool description
func (t *MCPToolAdapter) Description() string {
	if t.description != "" {
		return fmt.Sprintf("[MCP:%s] %s", t.serverName, t.description)
	}
	return fmt.Sprintf("[MCP:%s] %s", t.serverName, t.toolName)
}

// Parameters returns the tool input schema
func (t *MCPToolAdapter) Parameters() map[string]interface{} {
	return t.inputSchema
}

// Execute calls the MCP tool
func (t *MCPToolAdapter) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	result, err := t.client.CallTool(ctx, t.toolName, args)
	if err != nil {
		return ErrorResult(fmt.Sprintf("MCP tool call failed: %v", err))
	}

	if result.IsError {
		// Extract error message from content
		var errMsg string
		if len(result.Content) > 0 && result.Content[0].Type == "text" {
			errMsg = result.Content[0].Text
		} else {
			errMsg = "Unknown error"
		}
		return ErrorResult(fmt.Sprintf("MCP tool error: %s", errMsg))
	}

	// Build response from content
	var responseParts []string
	for _, content := range result.Content {
		if content.Type == "text" && content.Text != "" {
			responseParts = append(responseParts, content.Text)
		}
	}

	response := strings.Join(responseParts, "\n")
	if response == "" {
		response = "MCP tool completed successfully (no output)"
	}

	return &ToolResult{
		ForLLM:  response,
		ForUser: response,
	}
}
