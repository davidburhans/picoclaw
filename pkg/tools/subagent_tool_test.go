package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/skills"
)

// MockLLMProvider is a test implementation of LLMProvider
type MockLLMProvider struct {
	lastOptions map[string]interface{}
}

func (m *MockLLMProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, options map[string]interface{}) (*providers.LLMResponse, error) {
	m.lastOptions = options
	// Specialized mock responses for testing
	for _, msg := range messages {
		if msg.Role == "system" {
			if strings.Contains(msg.Content, "ROLE: Security Auditor") {
				return &providers.LLMResponse{Content: "Security audit complete. No issues found."}, nil
			}
		}
		if msg.Role == "user" && strings.Contains(msg.Content, "### test.go") {
			return &providers.LLMResponse{Content: "Analyzed test.go context."}, nil
		}
	}

	// Find the last user message to generate a response
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return &providers.LLMResponse{
				Content: "Task completed: " + messages[i].Content,
			}, nil
		}
	}
	return &providers.LLMResponse{Content: "No task provided"}, nil
}

func (m *MockLLMProvider) GetID() string {
	return "mock"
}

func (m *MockLLMProvider) GetDefaultModel() string {
	return "test-model"
}

func (m *MockLLMProvider) GetMaxTokens() int {
	return 4096
}

func (m *MockLLMProvider) GetTemperature() float64 {
	return 0.7
}

func (m *MockLLMProvider) GetMaxToolIterations() int {
	return 10
}

func (m *MockLLMProvider) GetTimeout() int {
	return 120
}

func (m *MockLLMProvider) GetMaxConcurrent() int {
	return 0 // default
}

func TestSubagentManager_SetLLMOptions_AppliesToRunToolLoop(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)
	manager.SetLLMOptions(2048, 0.6)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)
	tool.SetContext("cli", "direct")

	ctx := context.Background()
	args := map[string]interface{}{"task": "Do something"}
	result := tool.Execute(ctx, args)

	if result == nil || result.IsError {
		t.Fatalf("Expected successful result, got: %+v", result)
	}

	if provider.lastOptions == nil {
		t.Fatal("Expected LLM options to be passed, got nil")
	}
	if provider.lastOptions["max_tokens"] != 2048 {
		t.Fatalf("max_tokens = %v, want %d", provider.lastOptions["max_tokens"], 2048)
	}
	if provider.lastOptions["temperature"] != 0.6 {
		t.Fatalf("temperature = %v, want %v", provider.lastOptions["temperature"], 0.6)
	}
}

// TestSubagentTool_Name verifies tool name
func TestSubagentTool_Name(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	if tool.Name() != "subagent" {
		t.Errorf("Expected name 'subagent', got '%s'", tool.Name())
	}
}

// TestSubagentTool_Description verifies tool description
func TestSubagentTool_Description(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	desc := tool.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
	if !strings.Contains(desc, "subagent") {
		t.Errorf("Description should mention 'subagent', got: %s", desc)
	}
}

// TestSubagentTool_Parameters verifies tool parameters schema
func TestSubagentTool_Parameters(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	params := tool.Parameters()
	if params == nil {
		t.Error("Parameters should not be nil")
	}

	// Check type
	if params["type"] != "object" {
		t.Errorf("Expected type 'object', got: %v", params["type"])
	}

	// Check properties
	props, ok := params["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Properties should be a map")
	}

	// Verify task parameter
	task, ok := props["task"].(map[string]interface{})
	if !ok {
		t.Fatal("Task parameter should exist")
	}
	if task["type"] != "string" {
		t.Errorf("Task type should be 'string', got: %v", task["type"])
	}

	// Verify label parameter
	label, ok := props["label"].(map[string]interface{})
	if !ok {
		t.Fatal("Label parameter should exist")
	}
	if label["type"] != "string" {
		t.Errorf("Label type should be 'string', got: %v", label["type"])
	}

	// Check required fields
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatal("Required should be a string array")
	}
	if len(required) != 1 || required[0] != "task" {
		t.Errorf("Required should be ['task'], got: %v", required)
	}
}

// TestSubagentTool_SetContext verifies context setting
func TestSubagentTool_SetContext(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	tool.SetContext("test-channel", "test-chat")

	// Verify context is set (we can't directly access private fields,
	// but we can verify it doesn't crash)
	// The actual context usage is tested in Execute tests
}

// TestSubagentTool_Execute_Success tests successful execution
func TestSubagentTool_Execute_Success(t *testing.T) {
	provider := &MockLLMProvider{}
	msgBus := bus.NewMessageBus()
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, msgBus)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)
	tool.SetContext("telegram", "chat-123")

	ctx := context.Background()
	args := map[string]interface{}{
		"task":  "Write a haiku about coding",
		"label": "haiku-task",
	}

	result := tool.Execute(ctx, args)

	// Verify basic ToolResult structure
	if result == nil {
		t.Fatal("Result should not be nil")
	}

	// Verify no error
	if result.IsError {
		t.Errorf("Expected success, got error: %s", result.ForLLM)
	}

	// Verify not async
	if result.Async {
		t.Error("SubagentTool should be synchronous, not async")
	}

	// Verify not silent
	if result.Silent {
		t.Error("SubagentTool should not be silent")
	}

	// Verify ForUser contains brief summary (not empty)
	if result.ForUser == "" {
		t.Error("ForUser should contain result summary")
	}
	if !strings.Contains(result.ForUser, "Task completed") {
		t.Errorf("ForUser should contain task completion, got: %s", result.ForUser)
	}

	// Verify ForLLM contains full details
	if result.ForLLM == "" {
		t.Error("ForLLM should contain full details")
	}
	if !strings.Contains(result.ForLLM, "haiku-task") {
		t.Errorf("ForLLM should contain label 'haiku-task', got: %s", result.ForLLM)
	}
	if !strings.Contains(result.ForLLM, "Task completed:") {
		t.Errorf("ForLLM should contain task result, got: %s", result.ForLLM)
	}
}

// TestSubagentTool_Execute_NoLabel tests execution without label
func TestSubagentTool_Execute_NoLabel(t *testing.T) {
	provider := &MockLLMProvider{}
	msgBus := bus.NewMessageBus()
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, msgBus)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	ctx := context.Background()
	args := map[string]interface{}{
		"task": "Test task without label",
	}

	result := tool.Execute(ctx, args)

	if result.IsError {
		t.Errorf("Expected success without label, got error: %s", result.ForLLM)
	}

	// ForLLM should show (unnamed) for missing label
	if !strings.Contains(result.ForLLM, "(unnamed)") {
		t.Errorf("ForLLM should show '(unnamed)' for missing label, got: %s", result.ForLLM)
	}
}

// TestSubagentTool_Execute_MissingTask tests error handling for missing task
func TestSubagentTool_Execute_MissingTask(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	ctx := context.Background()
	args := map[string]interface{}{
		"label": "test",
	}

	result := tool.Execute(ctx, args)

	// Should return error
	if !result.IsError {
		t.Error("Expected error for missing task parameter")
	}

	// ForLLM should contain error message
	if !strings.Contains(result.ForLLM, "task is required") {
		t.Errorf("Error message should mention 'task is required', got: %s", result.ForLLM)
	}

	// Err should be set
	if result.Err == nil {
		t.Error("Err should be set for validation failure")
	}
}

// TestSubagentTool_Execute_NilManager tests error handling for nil manager
func TestSubagentTool_Execute_NilManager(t *testing.T) {
	tool := NewSubagentTool(nil, "/tmp/test", nil, false)

	ctx := context.Background()
	args := map[string]interface{}{
		"task": "test task",
	}

	result := tool.Execute(ctx, args)

	// Should return error
	if !result.IsError {
		t.Error("Expected error for nil manager")
	}

	if !strings.Contains(result.ForLLM, "Subagent manager not configured") {
		t.Errorf("Error message should mention manager not configured, got: %s", result.ForLLM)
	}
}

// TestSubagentTool_Execute_ContextPassing verifies context is properly used
func TestSubagentTool_Execute_ContextPassing(t *testing.T) {
	provider := &MockLLMProvider{}
	msgBus := bus.NewMessageBus()
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, msgBus)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	// Set context
	channel := "test-channel"
	chatID := "test-chat"
	tool.SetContext(channel, chatID)

	ctx := context.Background()
	args := map[string]interface{}{
		"task": "Test context passing",
	}

	result := tool.Execute(ctx, args)

	// Should succeed
	if result.IsError {
		t.Errorf("Expected success with context, got error: %s", result.ForLLM)
	}

	// The context is used internally; we can't directly test it
	// but execution success indicates context was handled properly
}

// TestSubagentTool_ForUserTruncation verifies long content is truncated for user
func TestSubagentTool_ForUserTruncation(t *testing.T) {
	// Create a mock provider that returns very long content
	provider := &MockLLMProvider{}
	msgBus := bus.NewMessageBus()
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, msgBus)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	ctx := context.Background()

	// Create a task that will generate long response
	longTask := strings.Repeat("This is a very long task description. ", 100)
	args := map[string]interface{}{
		"task":  longTask,
		"label": "long-test",
	}

	result := tool.Execute(ctx, args)

	// ForUser should be truncated to 500 chars + "..."
	maxUserLen := 500
	if len(result.ForUser) > maxUserLen+3 { // +3 for "..."
		t.Errorf("ForUser should be truncated to ~%d chars, got: %d", maxUserLen, len(result.ForUser))
	}

	// ForLLM should have full content
	if !strings.Contains(result.ForLLM, longTask[:50]) {
		t.Error("ForLLM should contain reference to original task")
	}
}

func TestSubagentTool_RoleInjection(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	ctx := context.Background()
	args := map[string]interface{}{
		"task": "Review security",
		"role": "Security Auditor",
	}

	result := tool.Execute(ctx, args)
	if result.IsError {
		t.Fatalf("Execute failed: %v", result.ForLLM)
	}

	if !strings.Contains(result.ForLLM, "Security audit complete") {
		t.Errorf("Expected role-specific response, got: %s", result.ForLLM)
	}
}

func TestSubagentTool_DepthLimit(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)
	manager.SetMaxDepth(2)
	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	ctx := context.Background()
	// Simulate depth 2 already reached
	ctx = withSubagentDepth(ctx, 2)

	args := map[string]interface{}{
		"task": "Nested task",
	}

	result := tool.Execute(ctx, args)
	if !result.IsError {
		t.Error("Expected error due to depth limit, but got success")
	}
	if !strings.Contains(result.ForLLM, "Maximum sub-agent nesting depth") {
		t.Errorf("Expected depth limit error message, got: %s", result.ForLLM)
	}
}

// --- Skill-backed role tests ---

// makeTestSkillDir creates a minimal valid skill in a temp directory and returns
// a SkillsLoader pointing at it.
func makeTestSkillDir(t *testing.T, skillName string, extraFiles map[string]string) (*skills.SkillsLoader, string) {
	t.Helper()
	base := t.TempDir()
	wsSkills := filepath.Join(base, "skills")
	skillDir := filepath.Join(wsSkills, skillName)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: " + skillName + "\ndescription: Test skill " + skillName + "\n---\n\n# " + skillName + "\n\nDo the thing."
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatal(err)
	}
	for rel, content := range extraFiles {
		full := filepath.Join(skillDir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	loader := skills.NewSkillsLoader(base, "", "")
	return loader, base
}

func TestSubagentTool_SkillBackedRole(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)

	loader, _ := makeTestSkillDir(t, "test-skill", map[string]string{
		"scripts/helper.sh": "#!/bin/sh\necho hello",
	})
	manager.SetSkillsLoader(loader)

	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	// The mock provider checks for "ROLE:" in the system prompt to give a
	// role-specific response. For skill-backed roles the system prompt
	// embeds skill knowledge, not a bare ROLE: line — verify it succeeds
	// and does NOT produce a "no skill matched" hint.
	ctx := context.Background()
	args := map[string]interface{}{
		"task":  "Do the thing",
		"role":  "test-skill",
		"label": "skill-test",
	}

	result := tool.Execute(ctx, args)
	if result.IsError {
		t.Fatalf("Expected success for skill-backed role, got error: %s", result.ForLLM)
	}
	if strings.Contains(result.ForLLM, "No skill matched role") {
		t.Errorf("Should not contain 'No skill matched' hint when skill is found, got: %s", result.ForLLM)
	}
}

func TestSubagentTool_FreeTextRoleFallback(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)

	// Load an empty skill registry — role won't match anything
	loader, _ := makeTestSkillDir(t, "some-other-skill", nil)
	manager.SetSkillsLoader(loader)

	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	ctx := context.Background()
	args := map[string]interface{}{
		"task":  "Do stuff",
		"role":  "Senior Wizard",
		"label": "wizard-task",
	}

	result := tool.Execute(ctx, args)
	if result.IsError {
		t.Fatalf("Expected success for free-text role fallback, got error: %s", result.ForLLM)
	}
	// Should include the "no skill matched" hint since the role didn't match a skill
	if !strings.Contains(result.ForLLM, "No skill matched role 'Senior Wizard'") {
		t.Errorf("Expected 'No skill matched' hint for unmatched role, got: %s", result.ForLLM)
	}
}

func TestSubagentTool_NoRoleNoHint(t *testing.T) {
	provider := &MockLLMProvider{}
	manager := NewSubagentManager(provider, "test-model", "/tmp/test", nil, nil)

	tool := NewSubagentTool(manager, "/tmp/test", nil, false)

	ctx := context.Background()
	args := map[string]interface{}{
		"task":  "Generic task",
		"label": "no-role",
	}

	result := tool.Execute(ctx, args)
	if result.IsError {
		t.Fatalf("Expected success for no-role task, got error: %s", result.ForLLM)
	}
	// No role provided — should not include the skill hint
	if strings.Contains(result.ForLLM, "No skill matched role") {
		t.Errorf("Should not include skill hint when no role is specified, got: %s", result.ForLLM)
	}
}

