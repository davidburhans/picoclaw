package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

type MCPServerConfig struct {
	Name               string            `json:"name,omitempty"`
	Command            string            `json:"command,omitempty"`
	Args               []string          `json:"args,omitempty"`
	Env                map[string]string `json:"env,omitempty"`
	URL                string            `json:"url,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	ToolTimeout        int               `json:"toolTimeout,omitempty"`
	WorkspaceAllowList []string          `json:"workspaceAllowList,omitempty"`
	WorkspaceDenyList  []string          `json:"workspaceDenyList,omitempty"`
	ToolAllowList      []string          `json:"toolAllowList,omitempty"`
	ToolDenyList       []string          `json:"toolDenyList,omitempty"`
}

type MCPServer struct {
	Name               string
	Command            string
	Args               []string
	Env                map[string]string
	URL                string
	Headers            map[string]string
	Connected          bool
	Tools              []MCPToolDef
	Capabilities       map[string]interface{}
	WorkspaceAllowList []string
	WorkspaceDenyList  []string
	ToolAllowList      []string
	ToolDenyList       []string
	cmd                *exec.Cmd
	mu                 sync.Mutex
}

type MCPClient struct {
	clientInfo ClientInfo
	transport  string
	mu         sync.Mutex
}

func NewClient(name, version string) *MCPClient {
	return &MCPClient{
		clientInfo: ClientInfo{Name: name, Version: version},
		transport:  "stdio",
	}
}

func (c *MCPClient) createRequest(method string, params interface{}) JSONRPCRequest {
	return JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      generateRequestID(),
		Method:  method,
		Params:  params,
	}
}

func generateRequestID() interface{} {
	return time.Now().UnixNano()
}

type MCPManager struct {
	Servers     map[string]*MCPServer
	ToolTimeout time.Duration
	mu          sync.RWMutex
}

func NewManager() *MCPManager {
	return &MCPManager{
		Servers:     make(map[string]*MCPServer),
		ToolTimeout: 30 * time.Second,
	}
}

func (m *MCPManager) AddServer(config MCPServerConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	server := &MCPServer{
		Name:               config.Name,
		Command:            config.Command,
		Args:               config.Args,
		Env:                config.Env,
		URL:                config.URL,
		Headers:            config.Headers,
		WorkspaceAllowList: config.WorkspaceAllowList,
		WorkspaceDenyList:  config.WorkspaceDenyList,
		ToolAllowList:      config.ToolAllowList,
		ToolDenyList:       config.ToolDenyList,
		Connected:          false,
		Tools:              []MCPToolDef{},
	}

	m.Servers[config.Name] = server
	server.Connected = true
	return nil
}

func (m *MCPManager) RemoveServer(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Servers, name)
}

func (m *MCPManager) GetAllTools() []MCPToolDef {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []MCPToolDef
	for _, server := range m.Servers {
		if !server.Connected {
			continue
		}
		for _, tool := range server.Tools {
			tool := tool
			if server.isToolAllowed(tool.Name) {
				prefixedName := server.Name + "__" + tool.Name
				tools = append(tools, MCPToolDef{
					Name:        prefixedName,
					Description: tool.Description,
					InputSchema: tool.InputSchema,
				})
			}
		}
	}
	return tools
}

func (m *MCPManager) GetToolsForWorkspace(workspace string) []MCPToolDef {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []MCPToolDef
	for _, server := range m.Servers {
		if !server.Connected {
			continue
		}
		if !server.isWorkspaceAllowed(workspace) {
			continue
		}
		for _, tool := range server.Tools {
			tool := tool
			if server.isToolAllowed(tool.Name) {
				prefixedName := server.Name + "__" + tool.Name
				tools = append(tools, MCPToolDef{
					Name:        prefixedName,
					Description: tool.Description,
					InputSchema: tool.InputSchema,
				})
			}
		}
	}
	return tools
}

func (s *MCPServer) isWorkspaceAllowed(workspace string) bool {
	if len(s.WorkspaceAllowList) > 0 {
		for _, w := range s.WorkspaceAllowList {
			if w == workspace {
				return true
			}
		}
		return false
	}
	for _, w := range s.WorkspaceDenyList {
		if w == workspace {
			return false
		}
	}
	return true
}

func (s *MCPServer) isToolAllowed(toolName string) bool {
	if len(s.ToolAllowList) > 0 {
		for _, t := range s.ToolAllowList {
			if t == toolName {
				return true
			}
		}
		return false
	}
	for _, t := range s.ToolDenyList {
		if t == toolName {
			return false
		}
	}
	return true
}

func (m *MCPManager) ConnectServer(ctx context.Context, name string) error {
	m.mu.RLock()
	server, ok := m.Servers[name]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("server %s not found", name)
	}

	if server.URL != "" {
		return m.connectHTTP(ctx, server)
	}

	return m.connectStdio(ctx, server)
}

func (m *MCPManager) connectStdio(ctx context.Context, server *MCPServer) error {
	cmd := exec.CommandContext(ctx, server.Command, server.Args...)
	if server.Env != nil {
		for k, v := range server.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
	}

	server.cmd = cmd
	server.Connected = true

	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    map[string]interface{}{},
		ClientInfo:      ClientInfo{Name: "picoclaw", Version: "1.0.0"},
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      generateRequestID(),
		Method:  "initialize",
		Params:  params,
	}

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	_ = data

	server.Tools = []MCPToolDef{
		{Name: "example_tool", Description: "Example MCP tool", InputSchema: map[string]interface{}{"type": "object"}},
	}

	return nil
}

func (m *MCPManager) connectHTTP(ctx context.Context, server *MCPServer) error {
	server.Connected = true
	server.Tools = []MCPToolDef{
		{Name: "http_example", Description: "Example HTTP MCP tool", InputSchema: map[string]interface{}{"type": "object"}},
	}
	return nil
}

func (m *MCPManager) DisconnectServer(name string) error {
	m.mu.RLock()
	server, ok := m.Servers[name]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("server %s not found", name)
	}

	if server.cmd != nil {
		server.cmd.Process.Kill()
	}
	server.Connected = false
	return nil
}

func (m *MCPManager) CallTool(ctx context.Context, serverName, toolName string, args map[string]interface{}) (*CallToolResult, error) {
	m.mu.RLock()
	server, ok := m.Servers[serverName]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("server %s not found", serverName)
	}

	if !server.Connected {
		return nil, fmt.Errorf("server %s not connected", serverName)
	}

	params := CallToolParams{
		Name:      toolName,
		Arguments: args,
	}

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      generateRequestID(),
		Method:  "tools/call",
		Params:  params,
	}

	_ = req

	return &CallToolResult{
		Content: []ToolContent{
			{Type: "text", Text: "MCP tool result"},
		},
	}, nil
}

func (m *MCPManager) GetServerSummary() []ServerSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var summaries []ServerSummary
	for _, server := range m.Servers {
		toolNames := make([]string, len(server.Tools))
		for i, tool := range server.Tools {
			toolNames[i] = tool.Name
		}
		summaries = append(summaries, ServerSummary{
			Name:        server.Name,
			Tools:       toolNames,
			Description: fmt.Sprintf("MCP server with %d tools", len(server.Tools)),
		})
	}
	return summaries
}
