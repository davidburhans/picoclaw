package mcp

import (
	"context"
	"testing"
	"time"
)

func TestJSONRPCRequest(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
		Params:  nil,
	}

	if req.JSONRPC != "2.0" {
		t.Errorf("expected JSONRPC 2.0, got %s", req.JSONRPC)
	}
	if req.Method != "tools/list" {
		t.Errorf("expected method tools/list, got %s", req.Method)
	}
}

func TestJSONRPCResponse(t *testing.T) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  "test result",
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("expected JSONRPC 2.0, got %s", resp.JSONRPC)
	}
	if resp.Error != nil {
		t.Errorf("expected no error, got %v", resp.Error)
	}
}

func TestJSONRPCError(t *testing.T) {
	err := JSONRPCError{
		Code:    -32600,
		Message: "Invalid Request",
	}

	if err.Code != -32600 {
		t.Errorf("expected code -32600, got %d", err.Code)
	}
	if err.Message != "Invalid Request" {
		t.Errorf("expected message 'Invalid Request', got %s", err.Message)
	}
}

func TestMCPToolDefinition(t *testing.T) {
	tool := MCPToolDef{
		Name:        "echo",
		Description: "Echo back the input",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{"message"},
		},
	}

	if tool.Name != "echo" {
		t.Errorf("expected tool name 'echo', got %s", tool.Name)
	}
	if tool.Description != "Echo back the input" {
		t.Errorf("expected description, got %s", tool.Description)
	}
}

func TestCallToolParams(t *testing.T) {
	params := CallToolParams{
		Name: "echo",
		Arguments: map[string]interface{}{
			"message": "hello",
		},
	}

	if params.Name != "echo" {
		t.Errorf("expected name 'echo', got %s", params.Name)
	}
	if params.Arguments["message"] != "hello" {
		t.Errorf("expected message 'hello', got %v", params.Arguments["message"])
	}
}

func TestCallToolResult(t *testing.T) {
	result := CallToolResult{
		Content: []ToolContent{
			{Type: "text", Text: "hello"},
		},
		IsError: false,
	}

	if len(result.Content) != 1 {
		t.Errorf("expected 1 content item, got %d", len(result.Content))
	}
	if result.Content[0].Text != "hello" {
		t.Errorf("expected text 'hello', got %s", result.Content[0].Text)
	}
	if result.IsError {
		t.Errorf("expected no error")
	}
}

func TestToolContentTypes(t *testing.T) {
	tests := []struct {
		name     string
		content  ToolContent
		expected string
	}{
		{
			name:     "text content",
			content:  ToolContent{Type: "text", Text: "hello"},
			expected: "text",
		},
		{
			name:     "image content",
			content:  ToolContent{Type: "image", Data: "base64...", MimeType: "image/png"},
			expected: "image",
		},
		{
			name:     "resource content",
			content:  ToolContent{Type: "resource", Text: "file content"},
			expected: "resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.content.Type != tt.expected {
				t.Errorf("expected type %s, got %s", tt.expected, tt.content.Type)
			}
		})
	}
}

func TestInitializeParams(t *testing.T) {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]interface{}{
			"tools":     struct{}{},
			"resources": struct{}{},
		},
		ClientInfo: ClientInfo{
			Name:    "picoclaw",
			Version: "0.1.0",
		},
	}

	if params.ProtocolVersion != "2024-11-05" {
		t.Errorf("expected protocol version 2024-11-05, got %s", params.ProtocolVersion)
	}
	if params.ClientInfo.Name != "picoclaw" {
		t.Errorf("expected client name picoclaw, got %s", params.ClientInfo.Name)
	}
}

func TestInitializeResult(t *testing.T) {
	result := InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]interface{}{
			"tools": struct{}{},
		},
		ServerInfo: ServerInfo{
			Name:    "test-server",
			Version: "1.0.0",
		},
	}

	if result.ServerInfo.Name != "test-server" {
		t.Errorf("expected server name test-server, got %s", result.ServerInfo.Name)
	}
}

func TestServerSummary(t *testing.T) {
	summary := ServerSummary{
		Name:        "filesystem",
		Tools:       []string{"read_file", "write_file", "list_directory"},
		Description: "Filesystem access tools",
	}

	if summary.Name != "filesystem" {
		t.Errorf("expected name filesystem, got %s", summary.Name)
	}
	if len(summary.Tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(summary.Tools))
	}
}

func TestToolsListResult(t *testing.T) {
	result := ToolsListResult{
		Tools: []MCPToolDef{
			{Name: "tool1", Description: "First tool"},
			{Name: "tool2", Description: "Second tool"},
		},
	}

	if len(result.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(result.Tools))
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan bool)
	go func() {
		select {
		case <-ctx.Done():
			done <- true
		case <-time.After(100 * time.Millisecond):
			done <- false
		}
	}()

	cancel()

	if !<-done {
		t.Error("context was not cancelled properly")
	}
}

func TestContextWithTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	select {
	case <-ctx.Done():
		// Expected - timeout should trigger
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout was not triggered")
	}
}
