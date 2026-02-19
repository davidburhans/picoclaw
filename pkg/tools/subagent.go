package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type SubagentTask struct {
	ID            string
	Task          string
	Label         string
	OriginChannel string
	OriginChatID  string
	Status        string
	Result        string
	Created       int64
}

type SubagentManager struct {
	tasks         map[string]*SubagentTask
	mu            sync.RWMutex
	provider      providers.LLMProvider
	defaultModel  string
	bus           *bus.MessageBus
	workspace     string
	tools         *ToolRegistry
	maxIterations int
	maxDepth      int
	maxTokens     int
	temperature   float64
	nextID        int
}

func NewSubagentManager(provider providers.LLMProvider, defaultModel, workspace string, bus *bus.MessageBus) *SubagentManager {
	cleanWS, _ := filepath.Abs(workspace)
	return &SubagentManager{
		tasks:         make(map[string]*SubagentTask),
		provider:      provider,
		defaultModel:  defaultModel,
		bus:           bus,
		workspace:     cleanWS,
		tools:         NewToolRegistry(),
		maxIterations: 10,
		maxDepth:      5,
		maxTokens:     4096,
		temperature:   0.7,
		nextID:        1,
	}
}

func (sm *SubagentManager) SetMaxIterations(n int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxIterations = n
}

func (sm *SubagentManager) GetMaxIterations() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.maxIterations
}

func (sm *SubagentManager) SetMaxDepth(depth int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxDepth = depth
}

func (sm *SubagentManager) SetMaxTokens(tokens int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.maxTokens = tokens
}

func (sm *SubagentManager) GetMaxTokens() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.maxTokens
}

func (sm *SubagentManager) SetTemperature(temp float64) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.temperature = temp
}

func (sm *SubagentManager) GetTemperature() float64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.temperature
}

// SetTools sets the tool registry for subagent execution.
// If not set, subagent will have access to the provided tools.
func (sm *SubagentManager) SetTools(tools *ToolRegistry) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.tools = tools
}

// RegisterTool registers a tool for subagent execution.
func (sm *SubagentManager) RegisterTool(tool Tool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.tools.Register(tool)
}

func (sm *SubagentManager) Spawn(ctx context.Context, task, label, role string, contextFiles []string, originChannel, originChatID string, callback AsyncCallback) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	taskID := fmt.Sprintf("subagent-%d", sm.nextID)
	sm.nextID++

	subagentTask := &SubagentTask{
		ID:            taskID,
		Task:          task,
		Label:         label,
		OriginChannel: originChannel,
		OriginChatID:  originChatID,
		Status:        "running",
		Created:       time.Now().UnixMilli(),
	}
	sm.tasks[taskID] = subagentTask

	// Check depth limit
	currentDepth := getSubagentDepth(ctx)
	if currentDepth >= sm.maxDepth {
		return "", fmt.Errorf("maximum sub-agent nesting depth (%d) exceeded", sm.maxDepth)
	}
	ctx = withSubagentDepth(ctx, currentDepth+1)

	// Start task in background with context cancellation support
	go sm.runTask(ctx, subagentTask, role, contextFiles, callback)

	if label != "" {
		return fmt.Sprintf("Spawned subagent '%s' for task: %s", label, task), nil
	}
	return fmt.Sprintf("Spawned subagent for task: %s", task), nil
}

func (sm *SubagentManager) runTask(ctx context.Context, task *SubagentTask, role string, contextFiles []string, callback AsyncCallback) {
	task.Status = "running"
	task.Created = time.Now().UnixMilli()

	// Build system prompt for subagent
	systemPrompt := buildSubagentPrompt(role, task.Task)

	// Inject context files
	initialMessage := task.Task
	if len(contextFiles) > 0 {
		initialMessage = sm.injectContextFiles(task.Task, contextFiles)
	}

	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: initialMessage,
		},
	}

	// Check if context is already cancelled before starting
	select {
	case <-ctx.Done():
		sm.mu.Lock()
		task.Status = "cancelled"
		task.Result = "Task cancelled before execution"
		sm.mu.Unlock()
		return
	default:
	}

	// Run tool loop with access to tools
	sm.mu.RLock()
	tools := sm.tools
	maxIter := sm.maxIterations
	maxTokens := sm.maxTokens
	temperature := sm.temperature
	sm.mu.RUnlock()

	loopResult, err := RunToolLoop(ctx, ToolLoopConfig{
		Provider:      sm.provider,
		Model:         sm.defaultModel,
		Tools:         tools,
		MaxIterations: maxIter,
		StopTool:      "report_completion",
		LLMOptions: map[string]any{
			"max_tokens":  maxTokens,
			"temperature": temperature,
		},
	}, messages, task.OriginChannel, task.OriginChatID)

	sm.mu.Lock()
	var result *ToolResult
	defer func() {
		sm.mu.Unlock()
		// Call callback if provided and result is set
		if callback != nil && result != nil {
			callback(ctx, result)
		}
	}()

	if err != nil {
		task.Status = "failed"
		task.Result = fmt.Sprintf("Error: %v", err)
		// Check if it was cancelled
		if ctx.Err() != nil {
			task.Status = "cancelled"
			task.Result = "Task cancelled during execution"
		}
		result = &ToolResult{
			ForLLM:  task.Result,
			ForUser: "",
			Silent:  false,
			IsError: true,
			Async:   false,
			Err:     err,
		}
	} else {
		task.Status = "completed"
		task.Result = loopResult.Content
		result = &ToolResult{
			ForLLM:  fmt.Sprintf("Subagent '%s' completed (iterations: %d): %s", task.Label, loopResult.Iterations, loopResult.Content),
			ForUser: loopResult.Content,
			Silent:  false,
			IsError: false,
			Async:   false,
		}
	}

	// Send announce message back to main agent
	if sm.bus != nil {
		announceContent := fmt.Sprintf("Task '%s' completed.\n\nResult:\n%s", task.Label, task.Result)
		sm.bus.PublishInbound(ctx, bus.InboundMessage{
			Channel:  "system",
			SenderID: fmt.Sprintf("subagent:%s", task.ID),
			// Format: "original_channel:original_chat_id" for routing back
			ChatID:  fmt.Sprintf("%s:%s", task.OriginChannel, task.OriginChatID),
			Content: announceContent,
		})
	}
}

func (sm *SubagentManager) GetTask(taskID string) (*SubagentTask, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	task, ok := sm.tasks[taskID]
	return task, ok
}

func (sm *SubagentManager) ListTasks() []*SubagentTask {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	tasks := make([]*SubagentTask, 0, len(sm.tasks))
	for _, task := range sm.tasks {
		tasks = append(tasks, task)
	}
	return tasks
}

// SubagentTool executes a subagent task synchronously and returns the result.
// Unlike SpawnTool which runs tasks asynchronously, SubagentTool waits for completion
// and returns the result directly in the ToolResult.
type SubagentTool struct {
	manager       *SubagentManager
	originChannel string
	originChatID  string
	workspace     string
	restrict      bool
	mu            sync.RWMutex
}

func NewSubagentTool(manager *SubagentManager, workspace string, restrict bool) *SubagentTool {
	return &SubagentTool{
		manager:       manager,
		originChannel: "cli",
		originChatID:  "direct",
		workspace:     workspace,
		restrict:      restrict,
	}
}

func (t *SubagentTool) Name() string {
	return "subagent"
}

func (t *SubagentTool) Description() string {
	return "Execute a subagent task synchronously and return the result. Use this for delegating specific tasks to an independent agent instance. Returns execution summary to user and full details to LLM."
}

func (t *SubagentTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"task": map[string]interface{}{
				"type":        "string",
				"description": "The task for subagent to complete",
			},
			"label": map[string]interface{}{
				"type":        "string",
				"description": "Optional short label for the task (for display)",
			},
			"role": map[string]interface{}{
				"type":        "string",
				"description": "The persona for the sub-agent, e.g. 'Senior Go Engineer', 'Security Auditor'. Shapes behavior.",
			},
			"context_files": map[string]interface{}{
				"type":        "array",
				"items":       map[string]interface{}{"type": "string"},
				"description": "File paths to read and inject as context before the task starts.",
			},
		},
		"required": []string{"task"},
	}
}

func (t *SubagentTool) SetContext(channel, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.originChannel = channel
	t.originChatID = chatID
}

func (t *SubagentTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	task, ok := args["task"].(string)
	if !ok {
		return ErrorResult("task is required").WithError(fmt.Errorf("task parameter is required"))
	}

	label, _ := args["label"].(string)
	role, _ := args["role"].(string)
	var contextFiles []string
	if cf, ok := args["context_files"].([]interface{}); ok {
		for _, f := range cf {
			if s, ok := f.(string); ok {
				contextFiles = append(contextFiles, s)
			}
		}
	}

	if t.manager == nil {
		return ErrorResult("Subagent manager not configured").WithError(fmt.Errorf("manager is nil"))
	}

	// Check depth limit
	currentDepth := getSubagentDepth(ctx)

	// BUG-3 FIX: Protect maxDepth read with RLock
	t.manager.mu.RLock()
	maxDepth := t.manager.maxDepth
	t.manager.mu.RUnlock()

	if currentDepth >= maxDepth {
		return ErrorResult(fmt.Sprintf("Maximum sub-agent nesting depth (%d) exceeded", maxDepth))
	}
	ctx = withSubagentDepth(ctx, currentDepth+1)

	// Build subagent prompt
	systemPrompt := buildSubagentPrompt(role, task)

	// Inject context files
	initialMessage := task
	if len(contextFiles) > 0 {
		initialMessage = t.manager.injectContextFiles(task, contextFiles)
	}

	// Build messages for subagent
	messages := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: initialMessage,
		},
	}

	// Use RunToolLoop to execute with tools (same as async SpawnTool)
	sm := t.manager
	sm.mu.RLock()
	tools := sm.tools
	maxIter := sm.maxIterations
	maxTokens := sm.maxTokens
	temperature := sm.temperature
	sm.mu.RUnlock()

	t.mu.RLock()
	originChannel := t.originChannel
	originChatID := t.originChatID
	t.mu.RUnlock()

	loopResult, err := RunToolLoop(ctx, ToolLoopConfig{
		Provider:      sm.provider,
		Model:         sm.defaultModel,
		Tools:         tools,
		MaxIterations: maxIter,
		StopTool:      "report_completion",
		LLMOptions: map[string]any{
			"max_tokens":  maxTokens,
			"temperature": temperature,
		},
	}, messages, originChannel, originChatID)

	if err != nil {
		return ErrorResult(fmt.Sprintf("Subagent execution failed: %v", err)).WithError(err)
	}

	// ForUser: Brief summary for user (truncated if too long)
	userContent := loopResult.Content
	maxUserLen := 500
	if len(userContent) > maxUserLen {
		userContent = userContent[:maxUserLen] + "..."
	}

	// ForLLM: Full execution details
	labelStr := label
	if labelStr == "" {
		labelStr = "(unnamed)"
	}
	llmContent := fmt.Sprintf("Subagent task completed:\nLabel: %s\nIterations: %d\nResult: %s",
		labelStr, loopResult.Iterations, loopResult.Content)

	return &ToolResult{
		ForLLM:  llmContent,
		ForUser: userContent,
		Silent:  false,
		IsError: false,
		Async:   false,
	}
}

func buildSubagentPrompt(role, goal string) string {
	if role == "" {
		role = "a capable worker"
	}
	return fmt.Sprintf(`You are a specialized sub-agent.
ROLE: %s
OBJECTIVE: %s

RULES:
1. You have access to tools — use them as needed.
2. Execute autonomously. Do NOT ask for clarification.
3. When finished, call the 'report_completion' tool with your final summary.
4. Stay focused on the objective.`, role, goal)
}

func (sm *SubagentManager) injectContextFiles(task string, files []string) string {
	var sb strings.Builder
	sb.WriteString(task)
	sb.WriteString("\n\n--- Context Files ---\n")
	for _, f := range files {
		// Use the unified validatePath check
		absPath, err := validatePath(f, sm.workspace, true)
		if err != nil {
			sb.WriteString(fmt.Sprintf("\n[%s]: (error: %v)\n", f, err))
			continue
		}

		data, err := os.ReadFile(absPath)
		if err != nil {
			sb.WriteString(fmt.Sprintf("\n[%s]: (error reading: %v)\n", f, err))
		} else {
			content := string(data)
			if len(content) > 10000 {
				content = content[:10000] + "\n... (truncated)"
			}
			sb.WriteString(fmt.Sprintf("\n### %s\n```\n%s\n```\n", f, content))
		}
	}
	return sb.String()
}

type contextKey string

const subagentDepthKey contextKey = "subagent_depth"

func getSubagentDepth(ctx context.Context) int {
	if v, ok := ctx.Value(subagentDepthKey).(int); ok {
		return v
	}
	return 0
}

func withSubagentDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, subagentDepthKey, depth)
}
