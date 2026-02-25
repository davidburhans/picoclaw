package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/sipeed/picoclaw/pkg/mcp"
	"github.com/sipeed/picoclaw/pkg/orchestrator/family"
	"github.com/sipeed/picoclaw/pkg/orchestrator/mailbox"
)

var (
	mailboxStore = mailbox.NewMemoryStore()
	familyStore  = family.NewFamilyStore()
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		var req mcp.JSONRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("Failed to unmarshal request: %v", err)
			continue
		}

		ctx := context.Background()
		var resp *mcp.JSONRPCResponse

		switch req.Method {
		case "initialize":
			resp = handleInitialize(req)
		case "tools/list":
			resp = handleToolsList(req)
		case "tools/call":
			resp = handleToolsCall(ctx, req)
		default:
			resp = &mcp.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &mcp.JSONRPCError{
					Code:    -32601,
					Message: "Method not found",
				},
			}
		}

		if resp != nil {
			out, _ := json.Marshal(resp)
			fmt.Println(string(out))
		}
	}
}

func handleInitialize(req mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
	return &mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: mcp.InitializeResult{
			ProtocolVersion: "2024-11-05", // Example standard version
			Capabilities: map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			ServerInfo: mcp.ServerInfo{
				Name:    "picoclaw-orchestrator",
				Version: "1.0.0",
			},
		},
	}
}

func handleToolsList(req mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
	return &mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: mcp.ToolsListResult{
			Tools: []mcp.MCPToolDef{
				{
					Name:        "send_message",
					Description: "Send a message to another family member's mailbox.",
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"from":    map[string]interface{}{"type": "string", "description": "Who is sending it"},
							"to":      map[string]interface{}{"type": "string", "description": "Who it is going to"},
							"content": map[string]interface{}{"type": "string", "description": "The message body"},
						},
						"required": []string{"from", "to", "content"},
					},
				},
				{
					Name:        "list_messages",
					Description: "List messages in your mailbox.",
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"user": map[string]interface{}{"type": "string", "description": "User whose inbox to list"},
						},
						"required": []string{"user"},
					},
				},
				// Add chores, lists, etc. missing later if needed
			},
		},
	}
}

func handleToolsCall(ctx context.Context, req mcp.JSONRPCRequest) *mcp.JSONRPCResponse {
	// Parse params
	var params mcp.CallToolParams
	paramsBytes, _ := json.Marshal(req.Params)
	json.Unmarshal(paramsBytes, &params)

	var result string
	var isError bool

	switch params.Name {
	case "send_message":
		from, _ := params.Arguments["from"].(string)
		to, _ := params.Arguments["to"].(string)
		content, _ := params.Arguments["content"].(string)
		id, err := mailboxStore.SendMessage(ctx, from, to, content)
		if err != nil {
			result = err.Error()
			isError = true
		} else {
			result = fmt.Sprintf("Message sent with ID: %s", id)
		}

	case "list_messages":
		user, _ := params.Arguments["user"].(string)
		msgs, err := mailboxStore.ListMessages(ctx, user)
		if err != nil {
			result = err.Error()
			isError = true
		} else {
			b, _ := json.Marshal(msgs)
			result = string(b)
		}

	default:
		result = fmt.Sprintf("Unknown tool %s", params.Name)
		isError = true
	}

	return &mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: mcp.CallToolResult{
			Content: []mcp.ToolContent{
				{
					Type: "text",
					Text: result,
				},
			},
			IsError: isError,
		},
	}
}
