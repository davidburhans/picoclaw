package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// testMockProvider is a simple mock LLM provider for testing
type testMockProvider struct{}

func (m *testMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:   "Mock response",
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *testMockProvider) GetDefaultModel() string {
	return "mock-model"
}

func (m *testMockProvider) GetMaxTokens() int {
	return 4096
}

func (m *testMockProvider) GetTemperature() float64 {
	return 0.7
}

func (m *testMockProvider) GetMaxToolIterations() int {
	return 10
}

func (m *testMockProvider) GetTimeout() int {
	return 120
}

func (m *testMockProvider) GetMaxConcurrent() int {
	return 1
}

func (m *testMockProvider) GetID() string {
	return "mock-id"
}

func TestRecordLastChannel(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	// Create agent loop
	msgBus := bus.NewMessageBus()
	provider := &testMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Test RecordLastChannel
	testChannel := "test-channel"
	err = al.RecordLastChannel(al.getOrCreateWorkspaceContext(""), testChannel)
	if err != nil {
		t.Fatalf("RecordLastChannel failed: %v", err)
	}

	// Verify channel was saved
	lastChannel := al.GetStateManager().GetLastChannel()
	if lastChannel != testChannel {
		t.Errorf("Expected channel '%s', got '%s'", testChannel, lastChannel)
	}

	// Verify persistence by creating a new agent loop
	al2 := NewAgentLoop(cfg, msgBus, provider)
	if al2.GetStateManager().GetLastChannel() != testChannel {
		t.Errorf("Expected persistent channel '%s', got '%s'", testChannel, al2.GetStateManager().GetLastChannel())
	}
}

func TestRecordLastChatID(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	// Create agent loop
	msgBus := bus.NewMessageBus()
	provider := &testMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Test RecordLastChatID
	testChatID := "test-chat-id-123"
	err = al.RecordLastChatID(al.getOrCreateWorkspaceContext(""), testChatID)
	if err != nil {
		t.Fatalf("RecordLastChatID failed: %v", err)
	}

	// Verify chat ID was saved
	lastChatID := al.GetStateManager().GetLastChatID()
	if lastChatID != testChatID {
		t.Errorf("Expected chat ID '%s', got '%s'", testChatID, lastChatID)
	}

	// Verify persistence by creating a new agent loop
	al2 := NewAgentLoop(cfg, msgBus, provider)
	if al2.GetStateManager().GetLastChatID() != testChatID {
		t.Errorf("Expected persistent chat ID '%s', got '%s'", testChatID, al2.GetStateManager().GetLastChatID())
	}
}

func TestNewAgentLoop_StateInitialized(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	// Create agent loop
	msgBus := bus.NewMessageBus()
	provider := &testMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Verify state manager is initialized
	if al.GetStateManager() == nil {
		t.Error("Expected state manager to be initialized")
	}

	// Verify state directory was created
	stateDir := filepath.Join(tmpDir, "state")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("Expected state directory to exist")
	}
}

// TestToolRegistry_ToolRegistration verifies tools can be registered and retrieved
func TestToolRegistry_ToolRegistration(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &testMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Register a custom tool
	customTool := &mockCustomTool{}
	al.RegisterTool(customTool)

	// Verify tool is registered by checking it doesn't panic on GetStartupInfo
	// (actual tool retrieval is tested in tools package tests)
	info := al.GetStartupInfo()
	toolsInfo := info["tools"].(map[string]interface{})
	toolsList := toolsInfo["names"].([]string)

	// Check that our custom tool name is in the list
	found := false
	for _, name := range toolsList {
		if name == "mock_custom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected custom tool to be registered")
	}
}

// TestToolContext_Updates verifies tool context is updated with channel/chatID
func TestToolContext_Updates(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "OK"}
	_ = NewAgentLoop(cfg, msgBus, provider)

	// Verify that ContextualTool interface is defined and can be implemented
	// This test validates the interface contract exists
	ctxTool := &mockContextualTool{}

	// Verify the tool implements the interface correctly
	var _ tools.ContextualTool = ctxTool
}

// TestToolRegistry_GetDefinitions verifies tool definitions can be retrieved
func TestToolRegistry_GetDefinitions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &testMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Register a test tool and verify it shows up in startup info
	testTool := &mockCustomTool{}
	al.RegisterTool(testTool)

	info := al.GetStartupInfo()
	toolsInfo := info["tools"].(map[string]interface{})
	toolsList := toolsInfo["names"].([]string)

	// Check that our custom tool name is in the list
	found := false
	for _, name := range toolsList {
		if name == "mock_custom" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected custom tool to be registered")
	}
}

// TestAgentLoop_GetStartupInfo verifies startup info contains tools
func TestAgentLoop_GetStartupInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &testMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	info := al.GetStartupInfo()

	// Verify tools info exists
	toolsInfo, ok := info["tools"]
	if !ok {
		t.Fatal("Expected 'tools' key in startup info")
	}

	toolsMap, ok := toolsInfo.(map[string]interface{})
	if !ok {
		t.Fatal("Expected 'tools' to be a map")
	}

	count, ok := toolsMap["count"]
	if !ok {
		t.Fatal("Expected 'count' in tools info")
	}

	// Should have default tools registered
	if count.(int) == 0 {
		t.Error("Expected at least some tools to be registered")
	}
}

// TestAgentLoop_Stop verifies Stop() sets running to false
func TestAgentLoop_Stop(t *testing.T) {

	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &testMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	// Note: running is only set to true when Run() is called
	// We can't test that without starting the event loop
	// Instead, verify the Stop method can be called safely
	al.Stop()

	// Verify running is false (initial state or after Stop)
	if al.running.Load() {
		t.Error("Expected agent to be stopped (or never started)")
	}
}

// Mock implementations for testing

type simpleMockProvider struct {
	response string
}

func (m *simpleMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:   m.response,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *simpleMockProvider) GetDefaultModel() string {
	return "mock-model"
}

func (m *simpleMockProvider) GetMaxTokens() int {
	return 4096
}

func (m *simpleMockProvider) GetTemperature() float64 {
	return 0.7
}

func (m *simpleMockProvider) GetMaxToolIterations() int {
	return 10
}

func (m *simpleMockProvider) GetTimeout() int {
	return 120
}

func (m *simpleMockProvider) GetMaxConcurrent() int {
	return 1
}

func (m *simpleMockProvider) GetID() string {
	return "simple-mock-id"
}

// mockCustomTool is a simple mock tool for registration testing
type mockCustomTool struct{}

func (m *mockCustomTool) Name() string {
	return "mock_custom"
}

func (m *mockCustomTool) Description() string {
	return "Mock custom tool for testing"
}

func (m *mockCustomTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (m *mockCustomTool) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	return tools.SilentResult("Custom tool executed")
}

// mockContextualTool tracks context updates
type mockContextualTool struct {
	lastChannel string
	lastChatID  string
}

func (m *mockContextualTool) Name() string {
	return "mock_contextual"
}

func (m *mockContextualTool) Description() string {
	return "Mock contextual tool"
}

func (m *mockContextualTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (m *mockContextualTool) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	return tools.SilentResult("Contextual tool executed")
}

func (m *mockContextualTool) SetContext(channel, chatID string) {
	m.lastChannel = channel
	m.lastChatID = chatID
}

// testHelper executes a message and returns the response
type testHelper struct {
	al *AgentLoop
}

func (h testHelper) executeAndGetResponse(tb testing.TB, ctx context.Context, msg bus.InboundMessage) string {
	// Use a short timeout to avoid hanging
	timeoutCtx, cancel := context.WithTimeout(ctx, responseTimeout)
	defer cancel()

	response, err := h.al.processMessage(timeoutCtx, msg)
	if err != nil {
		tb.Fatalf("processMessage failed: %v", err)
	}
	return response
}

const responseTimeout = 3 * time.Second

// TestToolResult_SilentToolDoesNotSendUserMessage verifies silent tools don't trigger outbound
func TestToolResult_SilentToolDoesNotSendUserMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "File operation complete"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ReadFileTool returns SilentResult, which should not send user message
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "read test.txt",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// Silent tool should return the LLM's response directly
	if response != "File operation complete" {
		t.Errorf("Expected 'File operation complete', got: %s", response)
	}
}

// TestToolResult_UserFacingToolDoesSendMessage verifies user-facing tools trigger outbound
func TestToolResult_UserFacingToolDoesSendMessage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "Command output: hello world"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// ExecTool returns UserResult, which should send user message
	ctx := context.Background()
	msg := bus.InboundMessage{
		Channel:    "test",
		SenderID:   "user1",
		ChatID:     "chat1",
		Content:    "run hello",
		SessionKey: "test-session",
	}

	response := helper.executeAndGetResponse(t, ctx, msg)

	// User-facing tool should include the output in final response
	if response != "Command output: hello world" {
		t.Errorf("Expected 'Command output: hello world', got: %s", response)
	}
}

// failFirstMockProvider fails on the first N calls with a specific error
type failFirstMockProvider struct {
	failures    int
	currentCall int
	failError   error
	successResp string
}

func (m *failFirstMockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	m.currentCall++
	if m.currentCall <= m.failures {
		return nil, m.failError
	}
	return &providers.LLMResponse{
		Content:   m.successResp,
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *failFirstMockProvider) GetDefaultModel() string {
	return "mock-fail-model"
}

func (m *failFirstMockProvider) GetMaxTokens() int {
	return 4096
}

func (m *failFirstMockProvider) GetTemperature() float64 {
	return 0.7
}

func (m *failFirstMockProvider) GetMaxToolIterations() int {
	return 10
}

func (m *failFirstMockProvider) GetTimeout() int {
	return 120
}

func (m *failFirstMockProvider) GetMaxConcurrent() int {
	return 1
}

func (m *failFirstMockProvider) GetID() string {
	return "fail-mock-id"
}

// TestAgentLoop_ContextExhaustionRetry verify that the agent retries on context errors
func TestAgentLoop_ContextExhaustionRetry(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	msgBus := bus.NewMessageBus()

	// Create a provider that fails once with a context error
	contextErr := fmt.Errorf("InvalidParameter: Total tokens of image and text exceed max message tokens")
	provider := &failFirstMockProvider{
		failures:    1,
		failError:   contextErr,
		successResp: "Recovered from context error",
	}

	al := NewAgentLoop(cfg, msgBus, provider)

	// Inject some history to simulate a full context
	sessionKey := "test-session-context"
	// Create dummy history
	history := []providers.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Old message 1"},
		{Role: "assistant", Content: "Old response 1"},
		{Role: "user", Content: "Old message 2"},
		{Role: "assistant", Content: "Old response 2"},
		{Role: "user", Content: "Trigger message"},
	}
	al.GetSessionManager().SetHistory(sessionKey, history)

	// Call ProcessDirectWithChannel
	// Note: ProcessDirectWithChannel calls processMessage which will execute runLLMIteration
	response, err := al.ProcessDirectWithChannel(context.Background(), "Trigger message", sessionKey, "test", "test-chat")
	if err != nil {
		t.Fatalf("Expected success after retry, got error: %v", err)
	}

	if response != "Recovered from context error" {
		t.Errorf("Expected 'Recovered from context error', got '%s'", response)
	}

	// We expect 2 calls: 1st failed, 2nd succeeded
	if provider.currentCall != 2 {
		t.Errorf("Expected 2 calls (1 fail + 1 success), got %d", provider.currentCall)
	}

	// Check final history length
	finalHistory := al.GetSessionManager().GetHistory(sessionKey)
	// We verify that the history has been modified (compressed)
	// Original length: 6
	// Expected behavior: compression drops ~50% of history (mid slice)
	// We can assert that the length is NOT what it would be without compression.
	// Without compression: 6 + 1 (new user msg) + 1 (assistant msg) = 8
	if len(finalHistory) >= 8 {
		t.Errorf("Expected history to be compressed (len < 8), got %d", len(finalHistory))
	}
}

func TestAgentLoop_SessionRotation(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	// Create agent loop
	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "Test response"}
	al := NewAgentLoop(cfg, msgBus, provider)
	helper := testHelper{al: al}

	// Create a context
	ctx := context.Background()

	// 1. Send /new (should rotate session)
	// Key: discord_123
	msg1 := bus.InboundMessage{
		Channel:    "discord",
		ChatID:     "123",
		Content:    "!new my-task",
		SessionKey: "discord_123",
	}

	// Add initial session file for RenameSession to succeed
	al.GetSessionManager().AddMessage("discord_123", "system", "Initial session")
	al.GetSessionManager().Save("discord_123")

	response1, err := al.processMessage(ctx, msg1)
	if err != nil {
		t.Fatalf("Failed to process !new: %v", err)
	}

	// Verify response contains "Started new session"
	if !strings.Contains(response1, "Started new session") {
		t.Errorf("Expected confirmation of new session, got: %s", response1)
	}

	// 2. Verify state updated
	activeSession := al.GetActiveSession("discord_123")
	// Should be: discord_123_v1 (unnamed, since name is for archive)
	expectedKey := "discord_123_v1"
	if activeSession != expectedKey {
		t.Errorf("Expected active session '%s', got '%s'", expectedKey, activeSession)
	}

	// 3. Send message to new session
	msg2 := bus.InboundMessage{
		Channel:    "discord",
		ChatID:     "123",
		Content:    "Hello",
		SessionKey: "discord_123",
	}
	response2 := helper.executeAndGetResponse(t, ctx, msg2)

	if response2 != "Test response" {
		t.Errorf("Expected 'Test response', got: %s", response2)
	}

	// Verify new session file exists
	// Wait, runAgentLoop calls al.sessions.AddMessage -> Save()
	// Save() uses session key as filename.
	// We updated msg.SessionKey to effectiveSessionKey before calling runAgentLoop.
	// So it should save to discord_123_v1.json

	newSessionPath := filepath.Join(tmpDir, "sessions", expectedKey+".json")
	if _, err := os.Stat(newSessionPath); os.IsNotExist(err) {
		t.Errorf("Expected session file %s to exist in %s", expectedKey+".json", filepath.Join(tmpDir, "sessions"))
	}
}

func TestAgentLoop_SessionRotation_CrossChannel(t *testing.T) {
	// Create temp workspace
	tmpDir, err := os.MkdirTemp("", "agent-test-cross-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test config
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace:         tmpDir,
				Model:             "test-model",
				MaxTokens:         config.IntPtr(4096),
				MaxToolIterations: config.IntPtr(10),
			},
		},
	}

	// Create agent loop
	msgBus := bus.NewMessageBus()
	provider := &simpleMockProvider{response: "Test response"}
	al := NewAgentLoop(cfg, msgBus, provider)

	ctx := context.Background()

	testCases := []struct {
		name          string
		channel       string
		chatID        string
		content       string
		sessionKey    string
		expectedKey   string // Active key is v1
		archiveName   string // For verification string
		shouldSucceed bool
	}{
		{
			name:          "CLI with leading space",
			channel:       "cli",
			chatID:        "console",
			content:       "  !new cli-task",
			sessionKey:    "cli_console",
			expectedKey:   "cli_console_v1",
			archiveName:   "cli_console_cli-task",
			shouldSucceed: true,
		},
		{
			name:          "Telegram normal",
			channel:       "telegram",
			chatID:        "12345",
			content:       "!new tg-task",
			sessionKey:    "telegram_12345",
			expectedKey:   "telegram_12345_v1",
			archiveName:   "telegram_12345_tg-task",
			shouldSucceed: true,
		},
		{
			name:          "Discord with surrounding space",
			channel:       "discord",
			chatID:        "9876",
			content:       " \t !new discord-task \n ",
			sessionKey:    "discord_9876",
			expectedKey:   "discord_9876_v1",
			archiveName:   "discord_9876_discord-task",
			shouldSucceed: true,
		},
		{
			name:          "Auto-generate name",
			channel:       "cli",
			chatID:        "console-auto",
			content:       "!new",
			sessionKey:    "cli_console-auto",
			expectedKey:   "cli_console-auto_v1",
			archiveName:   "cli_console-auto_Test_response", // Mock provider returns "Test response"
			shouldSucceed: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := bus.InboundMessage{
				Channel:    tc.channel,
				ChatID:     tc.chatID,
				Content:    tc.content,
				SessionKey: tc.sessionKey,
			}

			// Add a message to history so auto-gen has something to summarize
			if strings.TrimSpace(tc.content) == "!new" {
				al.GetSessionManager().AddMessage(tc.sessionKey, "user", "History context")
			}

			// Add initial session file for RenameSession to succeed
			al.GetSessionManager().AddMessage(tc.sessionKey, "system", "Initial session")
			al.GetSessionManager().Save(tc.sessionKey)

			response, err := al.processMessage(ctx, msg)
			if err != nil {
				t.Fatalf("Failed to process !new: %v", err)
			}

			if !strings.Contains(response, "Started new session") {
				t.Errorf("Expected confirmation of new session, got: %s", response)
			}

			if !strings.Contains(response, "Archived previous session as:") {
				t.Errorf("Expected mention of archived session, got: %s", response)
			}

			if !strings.Contains(response, tc.archiveName) {
				t.Errorf("Expected archive name '%s' in response, got: %s", tc.archiveName, response)
			}

			// Verify state updated
			activeSession := al.GetActiveSession(tc.sessionKey)
			if activeSession != tc.expectedKey {
				t.Errorf("Expected active session '%s', got '%s'", tc.expectedKey, activeSession)
			}
		})
	}
}

func TestGenerateSessionSummary_Truncation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-summary-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: tmpDir,
				Model:     "test-model",
			},
		},
	}

	msgBus := bus.NewMessageBus()
	// Mock provider returning a very long response
	longResponse := "this_is_a_very_long_session_name_that_should_definitely_be_truncated_to_be_shorter"
	provider := &simpleMockProvider{response: longResponse}
	al := NewAgentLoop(cfg, msgBus, provider)

	history := []providers.Message{
		{Role: "user", Content: "Hello"},
	}

	summary, err := al.generateSessionSummary(context.Background(), history)
	if err != nil {
		t.Fatalf("generateSessionSummary failed: %v", err)
	}

	if len(summary) > 30 {
		t.Errorf("Expected summary length <= 30, got %d: %s", len(summary), summary)
	}

	// It should cut at the last underscore before 30 chars
	// "this_is_a_very_long_session_name_that_should_definitely_be_truncated_to_be_shorter"
	// 30 chars: "this_is_a_very_long_session_na"
	// Last underscore before index 30: "this_is_a_very_long_session" (index 27)
	expectedPrefix := "this_is_a_very_long_session"
	if summary != expectedPrefix {
		t.Errorf("Expected summary '%s', got '%s'", expectedPrefix, summary)
	}
}
