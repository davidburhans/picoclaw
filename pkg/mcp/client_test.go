package mcp

import (
	"testing"
	"time"
)

func TestMCPServerConfig(t *testing.T) {
	config := MCPServerConfig{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		Env:     nil,
	}

	if config.Command != "npx" {
		t.Errorf("expected command npx, got %s", config.Command)
	}
	if len(config.Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(config.Args))
	}
}

func TestMCPServerConfigWithURL(t *testing.T) {
	config := MCPServerConfig{
		URL:     "https://mcp.example.com/sse",
		Headers: map[string]string{"Authorization": "Bearer token"},
	}

	if config.URL != "https://mcp.example.com/sse" {
		t.Errorf("expected URL, got %s", config.URL)
	}
	if config.Headers["Authorization"] != "Bearer token" {
		t.Errorf("expected Bearer token, got %s", config.Headers["Authorization"])
	}
}

func TestMCPServerState(t *testing.T) {
	server := &MCPServer{
		Name:         "filesystem",
		Command:      "npx",
		Args:         []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"},
		Connected:    false,
		Tools:        nil,
		Capabilities: nil,
	}

	if server.Name != "filesystem" {
		t.Errorf("expected name filesystem, got %s", server.Name)
	}
	if server.Connected {
		t.Error("expected not connected")
	}
}

func TestMCPClientInitialize(t *testing.T) {
	client := NewClient("test-client", "1.0.0")

	if client.clientInfo.Name != "test-client" {
		t.Errorf("expected name test-client, got %s", client.clientInfo.Name)
	}
	if client.clientInfo.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", client.clientInfo.Version)
	}
}

func TestMCPClientCreateRequest(t *testing.T) {
	client := NewClient("test", "1.0.0")

	req := client.createRequest("tools/list", nil)

	if req.JSONRPC != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %s", req.JSONRPC)
	}
	if req.Method != "tools/list" {
		t.Errorf("expected method tools/list, got %s", req.Method)
	}
}

func TestMCPManagerAddServer(t *testing.T) {
	manager := NewManager()

	config := MCPServerConfig{
		Name:    "test-server",
		Command: "echo",
		Args:    []string{"hello"},
	}

	err := manager.AddServer(config)
	if err != nil {
		t.Errorf("failed to add server: %v", err)
	}

	if len(manager.Servers) != 1 {
		t.Errorf("expected 1 server, got %d", len(manager.Servers))
	}
}

func TestMCPManagerRemoveServer(t *testing.T) {
	manager := NewManager()

	config := MCPServerConfig{
		Name:    "test-server",
		Command: "echo",
		Args:    []string{"hello"},
	}

	manager.AddServer(config)
	manager.RemoveServer("test-server")

	if len(manager.Servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(manager.Servers))
	}
}

func TestMCPManagerGetServerTools(t *testing.T) {
	manager := NewManager()

	manager.Servers["test-server"] = &MCPServer{
		Name:      "test-server",
		Connected: true,
		Tools: []MCPToolDef{
			{Name: "read_file", Description: "Read a file", InputSchema: map[string]interface{}{}},
			{Name: "write_file", Description: "Write a file", InputSchema: map[string]interface{}{}},
		},
	}

	tools := manager.GetAllTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(tools))
	}
}

func TestMCPManagerGetToolsForWorkspace(t *testing.T) {
	manager := NewManager()

	manager.Servers["server1"] = &MCPServer{
		Name:      "server1",
		Connected: true,
		Tools: []MCPToolDef{
			{Name: "tool_a", Description: "Tool A", InputSchema: map[string]interface{}{}},
		},
		WorkspaceAllowList: []string{"workspace1", "workspace2"},
	}

	manager.Servers["server2"] = &MCPServer{
		Name:      "server2",
		Connected: true,
		Tools: []MCPToolDef{
			{Name: "tool_b", Description: "Tool B", InputSchema: map[string]interface{}{}},
		},
		WorkspaceAllowList: []string{"workspace1"},
	}

	tools := manager.GetToolsForWorkspace("workspace1")
	if len(tools) != 2 {
		t.Errorf("expected 2 tools for workspace1, got %d", len(tools))
	}

	tools = manager.GetToolsForWorkspace("workspace2")
	if len(tools) != 1 {
		t.Errorf("expected 1 tool for workspace2, got %d", len(tools))
	}

	tools = manager.GetToolsForWorkspace("workspace3")
	if len(tools) != 0 {
		t.Errorf("expected 0 tools for workspace3, got %d", len(tools))
	}
}

func TestMCPServerDenyList(t *testing.T) {
	manager := NewManager()

	manager.Servers["server1"] = &MCPServer{
		Name:      "server1",
		Connected: true,
		Tools: []MCPToolDef{
			{Name: "read_file", Description: "Read a file", InputSchema: map[string]interface{}{}},
			{Name: "write_file", Description: "Write a file", InputSchema: map[string]interface{}{}},
		},
		ToolDenyList: []string{"write_file"},
	}

	tools := manager.GetAllTools()
	if len(tools) != 1 {
		t.Errorf("expected 1 tool (write_file denied), got %d", len(tools))
	}
	if tools[0].Name != "server1__read_file" {
		t.Errorf("expected tool read_file, got %s", tools[0].Name)
	}
}

func TestToolNaming(t *testing.T) {
	manager := NewManager()

	manager.Servers["fs"] = &MCPServer{
		Name:      "fs",
		Connected: true,
		Tools: []MCPToolDef{
			{Name: "read_file", Description: "Read", InputSchema: map[string]interface{}{}},
		},
	}

	manager.Servers["db"] = &MCPServer{
		Name:      "db",
		Connected: true,
		Tools: []MCPToolDef{
			{Name: "read_file", Description: "Read", InputSchema: map[string]interface{}{}},
		},
	}

	tools := manager.GetAllTools()
	if len(tools) != 2 {
		t.Errorf("expected 2 tools with prefixed names, got %d", len(tools))
	}

	found := false
	for _, tool := range tools {
		if tool.Name == "fs__read_file" {
			found = true
		}
	}
	if !found {
		t.Error("expected prefixed tool name fs__read_file")
	}
}

func TestManagerTimeoutConfig(t *testing.T) {
	manager := NewManager()
	manager.ToolTimeout = 30 * time.Second

	if manager.ToolTimeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", manager.ToolTimeout)
	}
}
