package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sipeed/picoclaw/pkg/logger"
)

// MCPClient represents a connection to a single MCP server
type MCPClient struct {
	name      string
	transport string // "stdio" or "sse"

	// stdio transport
	cmd    *exec.Cmd
	stdin  io.Writer
	stdout *bufio.Reader
	stderr io.Reader

	// SSE transport
	sseURL string

	// state
	initialized atomic.Bool
	nextID      atomic.Int64
	mu          sync.Mutex
	pendingReqs map[int64]chan *JSONRPCResponse
}

// NewStdioClient creates an MCP client using stdio transport
func NewStdioClient(ctx context.Context, name, command string, args []string, cwd string, env map[string]string) (*MCPClient, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if cwd != "" {
		cmd.Dir = cwd
	}

	// Set env vars
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), envMapToSlice(env)...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start MCP server: %w", err)
	}

	client := &MCPClient{
		name:        name,
		transport:   "stdio",
		cmd:         cmd,
		stdin:       stdin,
		stdout:      bufio.NewReader(stdout),
		stderr:      stderr,
		pendingReqs: make(map[int64]chan *JSONRPCResponse),
	}

	// Start goroutine to read responses
	go client.readLoop()

	// Start goroutine to log stderr
	go client.logStderr()

	return client, nil
}

// NewSSEClient creates an MCP client using SSE transport
func NewSSEClient(ctx context.Context, name, url string) (*MCPClient, error) {
	// TODO: implement SSE transport
	return nil, fmt.Errorf("SSE transport not yet implemented")
}

// Initialize performs MCP handshake
func (c *MCPClient) Initialize(ctx context.Context) error {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		ClientInfo: ClientInfo{
			Name:    "picoclaw",
			Version: "1.0.0",
		},
	}

	var result InitializeResult
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	c.initialized.Store(true)

	logger.InfoCF("mcp", "MCP server initialized",
		map[string]interface{}{
			"server":  c.name,
			"version": result.ProtocolVersion,
			"info":    result.ServerInfo.Name,
		})

	return nil
}

// ListTools retrieves available tools from the server
func (c *MCPClient) ListTools(ctx context.Context) ([]MCPToolDef, error) {
	if !c.initialized.Load() {
		return nil, fmt.Errorf("client not initialized")
	}

	var result ToolsListResult
	if err := c.call(ctx, "tools/list", nil, &result); err != nil {
		return nil, fmt.Errorf("tools/list failed: %w", err)
	}

	logger.InfoCF("mcp", "Listed MCP tools",
		map[string]interface{}{
			"server": c.name,
			"count":  len(result.Tools),
		})

	return result.Tools, nil
}

// CallTool invokes a tool on the server
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]interface{}) (*CallToolResult, error) {
	if !c.initialized.Load() {
		return nil, fmt.Errorf("client not initialized")
	}

	params := CallToolParams{
		Name:      name,
		Arguments: args,
	}

	var result CallToolResult
	if err := c.call(ctx, "tools/call", params, &result); err != nil {
		return nil, fmt.Errorf("tools/call failed: %w", err)
	}

	return &result, nil
}

// Close terminates the MCP server connection
func (c *MCPClient) Close() error {
	c.mu.Lock()
	for id, ch := range c.pendingReqs {
		close(ch)
		delete(c.pendingReqs, id)
	}
	c.mu.Unlock()

	if c.cmd != nil && c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

// call sends a JSON-RPC request and waits for response
func (c *MCPClient) call(ctx context.Context, method string, params interface{}, result interface{}) error {
	id := c.nextID.Add(1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Create response channel
	respChan := make(chan *JSONRPCResponse, 1)
	c.mu.Lock()
	c.pendingReqs[id] = respChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pendingReqs, id)
		c.mu.Unlock()
	}()

	// Send request
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.transport == "stdio" {
		if _, err := c.stdin.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("failed to write request: %w", err)
		}
	}

	// Wait for response
	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return fmt.Errorf("JSON-RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}

		// Unmarshal result
		if result != nil {
			resultData, err := json.Marshal(resp.Result)
			if err != nil {
				return fmt.Errorf("failed to marshal result: %w", err)
			}
			if err := json.Unmarshal(resultData, result); err != nil {
				return fmt.Errorf("failed to unmarshal result: %w", err)
			}
		}

		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// readLoop reads JSON-RPC responses from stdout
func (c *MCPClient) readLoop() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logger.ErrorCF("mcp", "Error reading from MCP server",
					map[string]interface{}{
						"server": c.name,
						"error":  err.Error(),
					})
			}
			c.Close()
			return
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			logger.ErrorCF("mcp", "Failed to parse JSON-RPC response",
				map[string]interface{}{
					"server": c.name,
					"error":  err.Error(),
					"line":   string(line),
				})
			continue
		}

		// Route to waiting request
		if resp.ID != nil {
			var id int64
			switch v := resp.ID.(type) {
			case float64:
				id = int64(v)
			case int64:
				id = v
			default:
				logger.ErrorCF("mcp", "Invalid response ID type",
					map[string]interface{}{
						"server": c.name,
						"id":     resp.ID,
					})
				continue
			}

			c.mu.Lock()
			ch, ok := c.pendingReqs[id]
			c.mu.Unlock()

			if ok {
				ch <- &resp
			}
		}
	}
}

// logStderr logs stderr output from the MCP server
func (c *MCPClient) logStderr() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		lower := strings.ToLower(line)
		if strings.Contains(lower, "error") || strings.Contains(lower, "panic") || strings.Contains(lower, "exception") {
			logger.WarnCF("mcp", "MCP server error on stderr",
				map[string]interface{}{
					"server": c.name,
					"line":   line,
				})
		} else {
			logger.DebugCF("mcp", "MCP server stderr",
				map[string]interface{}{
					"server": c.name,
					"line":   line,
				})
		}
	}
}

// envMapToSlice converts map to KEY=VALUE slice
func envMapToSlice(m map[string]string) []string {
	result := make([]string, 0, len(m))
	for k, v := range m {
		result = append(result, fmt.Sprintf("%s=%s", k, v))
	}
	return result
}
