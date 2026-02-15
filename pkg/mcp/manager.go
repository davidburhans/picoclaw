package mcp

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// MCPManager manages all configured MCP servers
type MCPManager struct {
	clients    map[string]*MCPClient
	defaultCwd string
}

// NewMCPManager creates a new MCP manager
func NewMCPManager(cfg map[string]config.MCPServerConfig, defaultCwd string) *MCPManager {
	return &MCPManager{
		clients:    make(map[string]*MCPClient),
		defaultCwd: defaultCwd,
	}
}

// StartAll initializes all enabled MCP servers
func (m *MCPManager) StartAll(ctx context.Context, cfg map[string]config.MCPServerConfig) error {
	for name, serverCfg := range cfg {
		if !serverCfg.Enabled {
			logger.DebugCF("mcp", "Skipping disabled MCP server",
				map[string]interface{}{"server": name})
			continue
		}
		
		var client *MCPClient
		var err error
		
		// Determine transport
		if serverCfg.Command != "" {
			// stdio transport
			cwd := serverCfg.Cwd
			if cwd == "" {
				cwd = m.defaultCwd
			}
			
			client, err = NewStdioClient(ctx, name, serverCfg.Command, serverCfg.Args, cwd, serverCfg.Env)
			if err != nil {
				logger.ErrorCF("mcp", "Failed to create stdio MCP client",
					map[string]interface{}{
						"server": name,
						"error":  err.Error(),
					})
				continue
			}
		} else if serverCfg.URL != "" {
			// SSE transport
			client, err = NewSSEClient(ctx, name, serverCfg.URL)
			if err != nil {
				logger.ErrorCF("mcp", "Failed to create SSE MCP client",
					map[string]interface{}{
						"server": name,
						"error":  err.Error(),
					})
				continue
			}
		} else {
			logger.WarnCF("mcp", "MCP server has no command or URL",
				map[string]interface{}{"server": name})
			continue
		}
		
		// Initialize the client
		if err := client.Initialize(ctx); err != nil {
			logger.ErrorCF("mcp", "Failed to initialize MCP server",
				map[string]interface{}{
					"server": name,
					"error":  err.Error(),
				})
			client.Close()
			continue
		}
		
		m.clients[name] = client
		
		logger.InfoCF("mcp", "MCP server started",
			map[string]interface{}{"server": name})
	}
	
	return nil
}

// StopAll shuts down all MCP clients
func (m *MCPManager) StopAll() {
	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			logger.ErrorCF("mcp", "Failed to close MCP client",
				map[string]interface{}{
					"server": name,
					"error":  err.Error(),
				})
		}
	}
	m.clients = make(map[string]*MCPClient)
}

// GetAllTools returns all tools from all servers, keyed by server name
func (m *MCPManager) GetAllTools(ctx context.Context) map[string][]MCPToolDef {
	result := make(map[string][]MCPToolDef)
	
	for name, client := range m.clients {
		tools, err := client.ListTools(ctx)
		if err != nil {
			logger.ErrorCF("mcp", "Failed to list tools from MCP server",
				map[string]interface{}{
					"server": name,
					"error":  err.Error(),
				})
			continue
		}
		result[name] = tools
	}
	
	return result
}

// GetClient returns the MCP client for a given server name
func (m *MCPManager) GetClient(name string) *MCPClient {
	return m.clients[name]
}

// GetServerSummaries returns summary info for all servers (for prompt generation)
func (m *MCPManager) GetServerSummaries(ctx context.Context) []ServerSummary {
	summaries := make([]ServerSummary, 0, len(m.clients))
	
	for name, client := range m.clients {
		tools, err := client.ListTools(ctx)
		if err != nil {
			logger.ErrorCF("mcp", "Failed to list tools for server summary",
				map[string]interface{}{
					"server": name,
					"error":  err.Error(),
				})
			continue
		}
		
		toolNames := make([]string, len(tools))
		for i, tool := range tools {
			toolNames[i] = fmt.Sprintf("mcp_%s_%s", name, tool.Name)
		}
		
		summaries = append(summaries, ServerSummary{
			Name:        name,
			Tools:       toolNames,
			Description: fmt.Sprintf("%d tools available", len(tools)),
		})
	}
	
	return summaries
}

// Count returns the number of active MCP servers
func (m *MCPManager) Count() int {
	return len(m.clients)
}
