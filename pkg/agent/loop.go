// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/constants"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/mailbox"
	"github.com/sipeed/picoclaw/pkg/mcp"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/memory/embedding"
	"github.com/sipeed/picoclaw/pkg/memory/qdrant"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
	"github.com/sipeed/picoclaw/pkg/state"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/utils"
)

type workspaceContext struct {
	path           string
	tools          *tools.ToolRegistry
	sessions       *session.SessionManager
	state          *state.Manager
	contextBuilder *ContextBuilder
	mu             sync.Mutex // protects lazy init
}

type AgentLoop struct {
	bus            *bus.MessageBus
	provider       providers.LLMProvider
	config         *config.Config
	defaultWS      string
	model          string
	contextWindow  int // Maximum context window size in tokens
	maxIterations  int
	timeout        time.Duration // Session timeout
	workspaces     map[string]*workspaceContext
	wsMu           sync.RWMutex // protects workspaces map
	running        atomic.Bool
	summarizing    sync.Map // Tracks which sessions are currently being summarized
	sessionLocks   sync.Map // Per-session mutexes to prevent concurrent processing
	channelManager *channels.Manager
	memoryManager  *memory.Manager
	mailboxClient  *mailbox.Client

	// Concurrency control
	wg               sync.WaitGroup
	sem              chan struct{}
	summarizationSem chan struct{}
}

type workspaceSessionController struct {
	al   *AgentLoop
	wctx *workspaceContext
}

func (wsc *workspaceSessionController) RotateSession(ctx context.Context, baseKey, archiveName string) (string, string, error) {
	return wsc.al.RotateSession(ctx, baseKey, archiveName, wsc.wctx)
}

func (wsc *workspaceSessionController) GetSessionManager() *session.SessionManager {
	return wsc.wctx.sessions
}

func (wsc *workspaceSessionController) GetActiveSession(baseKey string) string {
	return wsc.wctx.state.GetActiveSession(baseKey)
}

// processOptions configures how a message is processed
type processOptions struct {
	SessionKey      string // Session identifier for history/context
	Channel         string // Target channel for tool execution
	ChatID          string // Target chat ID for tool execution
	UserMessage     string // User message content (may include prefix)
	DefaultResponse string // Response when LLM returns empty
	EnableSummary   bool   // Whether to trigger summarization
	SendResponse    bool   // Whether to send response via bus
	NoHistory       bool   // If true, don't load session history (for heartbeat)
	Metadata        map[string]string
}

// createToolRegistry creates a tool registry with common tools.
// This is shared between main agent and subagents.
func createToolRegistry(workspace string, allowedPaths []string, restrict bool, cfg *config.Config, msgBus *bus.MessageBus, memoryManager *memory.Manager) *tools.ToolRegistry {
	registry := tools.NewToolRegistry()

	// File system tools
	registry.Register(tools.NewReadFileTool(workspace, allowedPaths, restrict))
	registry.Register(tools.NewWriteFileTool(workspace, allowedPaths, restrict))
	registry.Register(tools.NewListDirTool(workspace, allowedPaths, restrict))
	registry.Register(tools.NewEditFileTool(workspace, allowedPaths, restrict))
	registry.Register(tools.NewAppendFileTool(workspace, allowedPaths, restrict))

	// Shell execution
	registry.Register(tools.NewExecTool(workspace, allowedPaths, restrict))

	// Session management
	registry.Register(tools.NewReadSessionTool(workspace))

	if searchTool := tools.NewWebSearchTool(tools.WebSearchToolOptions{
		BraveAPIKey:          cfg.Tools.Web.Brave.APIKey,
		BraveMaxResults:      cfg.Tools.Web.Brave.MaxResults,
		BraveEnabled:         cfg.Tools.Web.Brave.Enabled,
		DuckDuckGoMaxResults: cfg.Tools.Web.DuckDuckGo.MaxResults,
		DuckDuckGoEnabled:    cfg.Tools.Web.DuckDuckGo.Enabled,
		SearXNGURL:           cfg.Tools.Web.SearXNG.BaseURL,
		SearXNGMaxResults:    cfg.Tools.Web.SearXNG.MaxResults,
		SearXNGEnabled:       cfg.Tools.Web.SearXNG.Enabled,
		PerplexityAPIKey:     cfg.Tools.Web.Perplexity.APIKey,
		PerplexityMaxResults: cfg.Tools.Web.Perplexity.MaxResults,
		PerplexityEnabled:    cfg.Tools.Web.Perplexity.Enabled,
	}); searchTool != nil {
		registry.Register(searchTool)
	}
	registry.Register(tools.NewWebFetchTool(50000))

	// Hardware tools (I2C, SPI) - Linux only, returns error on other platforms
	registry.Register(tools.NewI2CTool())
	registry.Register(tools.NewSPITool())

	// Message tool - available to both agent and subagent
	// Subagent uses it to communicate directly with user
	messageTool := tools.NewMessageTool()
	messageTool.SetSendCallback(func(channel, chatID, content string) error {
		msgBus.PublishOutbound(context.Background(), bus.OutboundMessage{
			Channel: channel,
			ChatID:  chatID,
			Content: content,
		})
		return nil
	})
	registry.Register(messageTool)

	// MCP tools - register tools from all enabled MCP servers
	if cfg.MCP != nil && len(cfg.MCP) > 0 {
		mcpMgr := mcp.NewMCPManager(cfg.MCP, workspace)
		ctx := context.Background()
		if err := mcpMgr.StartAll(ctx, cfg.MCP); err != nil {
			logger.ErrorCF("mcp", "Failed to start MCP servers",
				map[string]interface{}{"error": err.Error()})
		} else if mcpMgr.Count() > 0 {
			// Get all tools from all servers
			allTools := mcpMgr.GetAllTools(ctx)
			totalTools := 0
			for serverName, serverTools := range allTools {
				for _, toolDef := range serverTools {
					client := mcpMgr.GetClient(serverName)
					if client != nil {
						adapter := tools.NewMCPToolAdapter(
							serverName,
							toolDef.Name,
							toolDef.Description,
							toolDef.InputSchema,
							client,
						)
						registry.Register(adapter)
						totalTools++
					}
				}
			}
			logger.InfoCF("mcp", "Registered MCP tools",
				map[string]interface{}{
					"servers": mcpMgr.Count(),
					"tools":   totalTools,
				})
		}
	}

	// Long-term memory tools
	if memoryManager != nil && memoryManager.IsEnabled() {
		registry.Register(tools.NewMemorySearchTool(memoryManager, workspace))
		registry.Register(tools.NewMemoryBrowseTool(memoryManager, workspace))
	}

	return registry
}

func NewAgentLoop(cfg *config.Config, msgBus *bus.MessageBus, provider providers.LLMProvider) *AgentLoop {
	defaultWS := cfg.WorkspacePath()
	os.MkdirAll(defaultWS, 0755)

	// Resolve configuration values (fallback to provider if unset in config)
	model := cfg.Agents.Defaults.Model
	if model == "" {
		model = provider.GetDefaultModel()
	}

	maxTokens := provider.GetMaxTokens()
	if cfg.Agents.Defaults.MaxTokens != nil {
		maxTokens = *cfg.Agents.Defaults.MaxTokens
	}

	maxToolIterations := provider.GetMaxToolIterations()
	if cfg.Agents.Defaults.MaxToolIterations != nil {
		maxToolIterations = *cfg.Agents.Defaults.MaxToolIterations
	}

	timeoutSec := provider.GetTimeout()
	if cfg.Agents.Defaults.Timeout != nil {
		timeoutSec = *cfg.Agents.Defaults.Timeout
	}
	timeout := time.Duration(timeoutSec) * time.Second

	// Check for provider-specific overrides (now using pointers)
	providerType, instanceName := config.ResolveProvider(cfg.Agents.Defaults.Provider)
	if pCfg, ok := cfg.Providers.Get(providerType, instanceName); ok {
		if pCfg.Model != "" {
			model = pCfg.Model
		}
		if pCfg.MaxTokens != nil {
			maxTokens = *pCfg.MaxTokens
		}
		if pCfg.Temperature != nil {
			// temperature is handled in subagent or directly in Chat calls
		}
		if pCfg.MaxToolIterations != nil {
			maxToolIterations = *pCfg.MaxToolIterations
		}
		if pCfg.Timeout != nil {
			timeout = time.Duration(*pCfg.Timeout) * time.Second
		}
	}

	maxConcurrentSessions := provider.GetMaxConcurrent()
	if maxConcurrentSessions <= 0 {
		maxConcurrentSessions = 1
	}

	al := &AgentLoop{
		bus:           msgBus,
		provider:      provider,
		config:        cfg,
		defaultWS:     defaultWS,
		model:         model,
		contextWindow: maxTokens,
		maxIterations: maxToolIterations,
		timeout:       timeout,
		workspaces:    make(map[string]*workspaceContext),
		summarizing:   sync.Map{},
		sessionLocks:     sync.Map{},
		sem:              make(chan struct{}, maxConcurrentSessions),
		summarizationSem: make(chan struct{}, 3), // Limit concurrent summarization tasks to 3
	}

	// Initialize memory manager if enabled
	if cfg.Memory.Enabled {
		providerType := strings.ToLower(cfg.Memory.Provider)
		var db memory.VectorDB
		var err error

		if providerType == "qdrant" {
			db, err = qdrant.NewClient(cfg.Memory.Qdrant.URL, cfg.Memory.Qdrant.APIKey)
			if err != nil {
				logger.ErrorCF("agent", "Failed to initialize Qdrant client", map[string]interface{}{"error": err})
			}
		}

		if db != nil {
			embedder := embedding.NewClient(cfg.Memory.Embedding)
			al.memoryManager = memory.NewManager(cfg.Memory, db, embedder)
			logger.InfoCF("agent", "Long-term memory enabled", map[string]interface{}{"provider": providerType})
		}
	}

	// Pre-initialize default workspace
	al.mailboxClient = mailbox.NewClient(cfg.ResolveMailboxPath())
	al.getOrCreateWorkspaceContext("")

	return al
}

func (al *AgentLoop) getOrCreateWorkspaceContext(senderID string) *workspaceContext {
	path := al.config.ResolveWorkspace(senderID)
	return al.getOrCreateWorkspaceContextByPath(path)
}

func (al *AgentLoop) getOrCreateWorkspaceContextByPath(path string) *workspaceContext {
	al.wsMu.Lock()
	wctx, ok := al.workspaces[path]
	if !ok {
		wctx = &workspaceContext{path: path}
		al.workspaces[path] = wctx
	}
	al.wsMu.Unlock()

	wctx.mu.Lock()
	defer wctx.mu.Unlock()

	if wctx.tools == nil {
		al.initWorkspaceContext(wctx)
	}

	return wctx
}

func (al *AgentLoop) initWorkspaceContext(wctx *workspaceContext) {
	workspace := wctx.path
	os.MkdirAll(workspace, 0755)
	
	restrict := true
	if al.config.Agents.Defaults.RestrictToWorkspace != nil {
		restrict = *al.config.Agents.Defaults.RestrictToWorkspace
	}

	var allowedPaths []string
	// Check for workspace-specific override
	for _, ws := range al.config.Workspaces {
		if config.ExpandHome(ws.Path) == workspace {
			if ws.RestrictToWorkspace != nil {
				restrict = *ws.RestrictToWorkspace
			}
			allowedPaths = ws.AllowedExternalPaths
			break
		}
	}

	// Create tool registry for main agent
	toolsRegistry := createToolRegistry(workspace, allowedPaths, restrict, al.config, al.bus, al.memoryManager)

	// Create subagent manager
	subagentManager := tools.NewSubagentManager(al.provider, al.model, workspace, allowedPaths, al.bus)

	// Register mailbox tools
	wsName := al.config.ResolveWorkspaceName(workspace)
	toolsRegistry.Register(tools.NewSendMessageTool(al.mailboxClient, wsName))
	toolsRegistry.Register(tools.NewListWorkspacesTool(al.config))

	// Apply subagent config
	maxIterations := al.maxIterations
	if al.config.Agents.Defaults.Subagent.MaxIterations != nil {
		maxIterations = *al.config.Agents.Defaults.Subagent.MaxIterations
	}
	subagentManager.SetMaxIterations(maxIterations)

	maxDepth := 5
	if al.config.Agents.Defaults.Subagent.MaxDepth != nil {
		maxDepth = *al.config.Agents.Defaults.Subagent.MaxDepth
	}
	subagentManager.SetMaxDepth(maxDepth)

	maxTokens := al.contextWindow
	if al.config.Agents.Defaults.Subagent.MaxTokens != nil {
		maxTokens = *al.config.Agents.Defaults.Subagent.MaxTokens
	}
	subagentManager.SetMaxTokens(maxTokens)

	temp := al.provider.GetTemperature()
	if al.config.Agents.Defaults.Subagent.Temperature != nil {
		temp = *al.config.Agents.Defaults.Subagent.Temperature
	}
	subagentManager.SetTemperature(temp)

	subagentTools := createToolRegistry(workspace, allowedPaths, restrict, al.config, al.bus, al.memoryManager)
	subagentTools.Register(&tools.ReportCompletionTool{})
	subagentManager.SetTools(subagentTools)

	// Register spawn tool (for main agent)
	spawnTool := tools.NewSpawnTool(subagentManager)
	toolsRegistry.Register(spawnTool)

	// Register subagent tool (synchronous execution)
	subagentTool := tools.NewSubagentTool(subagentManager, workspace, allowedPaths, restrict)
	toolsRegistry.Register(subagentTool)

	wctx.sessions = session.NewSessionManager(filepath.Join(workspace, "sessions"))
	wctx.state = state.NewManager(workspace)

	// Create MCP manager and start servers
	mcpMgr := mcp.NewMCPManager(al.config.MCP, workspace)
	ctx := context.Background()
	if err := mcpMgr.StartAll(ctx, al.config.MCP); err != nil {
		logger.ErrorCF("mcp", "Failed to start MCP servers",
			map[string]interface{}{"error": err.Error(), "workspace": workspace})
	}

	// Register MCP tools in the workspace agent's registry
	if mcpMgr.Count() > 0 {
		allTools := mcpMgr.GetAllTools(ctx)
		totalTools := 0
		for serverName, serverTools := range allTools {
			for _, toolDef := range serverTools {
				client := mcpMgr.GetClient(serverName)
				if client != nil {
					adapter := tools.NewMCPToolAdapter(
						serverName,
						toolDef.Name,
						toolDef.Description,
						toolDef.InputSchema,
						client,
					)
					toolsRegistry.Register(adapter)
					totalTools++
				}
			}
		}
		logger.InfoCF("mcp", "Registered MCP tools in workspace",
			map[string]interface{}{
				"workspace": workspace,
				"servers":   mcpMgr.Count(),
				"tools":     totalTools,
			})
	}

	// Create context builder and set tools registry
	wctx.contextBuilder = NewContextBuilder(workspace, al.config.Agents.Defaults.Name)
	wctx.contextBuilder.SetToolsRegistry(toolsRegistry)
	wctx.contextBuilder.SetMCPManager(mcpMgr)

	// Wire skill registry into sub-agent manager for skill-backed role resolution
	subagentManager.SetSkillsLoader(wctx.contextBuilder.GetSkillsLoader())

	wctx.tools = toolsRegistry

	// Register session control tool with workspace context wrapper
	toolsRegistry.Register(tools.NewSessionControlTool(&workspaceSessionController{al: al, wctx: wctx}))
}

func (al *AgentLoop) Run(ctx context.Context) error {
	al.running.Store(true)

	for al.running.Load() {
		msg, ok := al.bus.ConsumeInbound(ctx)
		if !ok {
			// Check if we're stopping
			if !al.running.Load() || ctx.Err() != nil {
				break
			}
			continue
		}

		// Acquire semaphore to limit concurrency
		select {
		case al.sem <- struct{}{}:
			al.wg.Add(1)
			go func(m bus.InboundMessage) {
				defer func() {
					<-al.sem
					al.wg.Done()
				}()
				al.handleInboundMessage(ctx, m)
			}(msg)
		case <-ctx.Done():
			return nil
		}
	}

	return nil
}

func (al *AgentLoop) handleInboundMessage(ctx context.Context, msg bus.InboundMessage) {
	// 2. Update tool contexts (resets sentInRound flag)
	wctx := al.getOrCreateWorkspaceContext(msg.SenderID)
	al.updateToolContexts(wctx.tools, msg.Channel, msg.ChatID)

	response, err := al.processMessage(ctx, msg)
	if err != nil {
		response = fmt.Sprintf("Error processing message: %v", err)
	}

	if response != "" {
		// Check if the message tool already sent a response during this round.
		// If so, skip publishing to avoid duplicate messages to the user.
		alreadySent := false
		if tool, ok := wctx.tools.Get("message"); ok {
			if mt, ok := tool.(*tools.MessageTool); ok {
				alreadySent = mt.HasSentInRound()
			}
		}

		if !alreadySent {
			al.bus.PublishOutbound(ctx, bus.OutboundMessage{
				Channel: msg.Channel,
				ChatID:  msg.ChatID,
				Content: response,
			})
		}
	}
}

func (al *AgentLoop) Stop() {
	al.running.Store(false)

	// Wait for all active message processing to complete
	al.wg.Wait()

	// Close long-term memory connection
	if al.memoryManager != nil {
		al.memoryManager.Close()
	}

	// Clean up all MCP clients in all workspaces
	al.wsMu.RLock()
	defer al.wsMu.RUnlock()

	for _, wctx := range al.workspaces {
		wctx.mu.Lock()
		if wctx.contextBuilder != nil && wctx.contextBuilder.mcpManager != nil {
			wctx.contextBuilder.mcpManager.StopAll()
		}
		wctx.mu.Unlock()
	}
}

func (al *AgentLoop) RegisterTool(tool tools.Tool) {
	// Register to default workspace context by default
	wctx := al.getOrCreateWorkspaceContext("")
	wctx.tools.Register(tool)
}

func (al *AgentLoop) SetChannelManager(cm *channels.Manager) {
	al.channelManager = cm
}

func (al *AgentLoop) SetMemoryManager(mm *memory.Manager) {
	al.memoryManager = mm
}

// RecordLastChannel records the last active channel for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChannel(wctx *workspaceContext, channel string) error {
	return wctx.state.SetLastChannel(channel)
}

// RecordLastChatID records the last active chat ID for this workspace.
// This uses the atomic state save mechanism to prevent data loss on crash.
func (al *AgentLoop) RecordLastChatID(wctx *workspaceContext, chatID string) error {
	return wctx.state.SetLastChatID(chatID)
}

func (al *AgentLoop) ProcessDirect(ctx context.Context, content, sessionKey string) (string, error) {
	return al.ProcessDirectWithChannel(ctx, content, sessionKey, "cli", "direct")
}

func (al *AgentLoop) ProcessDirectWithChannel(ctx context.Context, content, sessionKey, channel, chatID string) (string, error) {
	msg := bus.InboundMessage{
		Channel:    channel,
		SenderID:   "cron",
		ChatID:     chatID,
		Content:    content,
		SessionKey: sessionKey,
	}

	return al.processMessage(ctx, msg)
}

func (al *AgentLoop) ProcessHeartbeat(ctx context.Context, content, channel, chatID string) (string, error) {
	// Heartbeat always uses default workspace if called this way
	return al.ProcessHeartbeatForWorkspace(ctx, content, channel, chatID, al.defaultWS)
}

// ProcessHeartbeatForWorkspace processes a heartbeat request for a specific workspace.
func (al *AgentLoop) ProcessHeartbeatForWorkspace(ctx context.Context, content, channel, chatID, workspacePath string) (string, error) {
	wctx := al.getOrCreateWorkspaceContextByPath(workspacePath)

	return al.runAgentLoop(ctx, processOptions{
		SessionKey:      "heartbeat",
		Channel:         channel,
		ChatID:          chatID,
		UserMessage:     content,
		DefaultResponse: "HEARTBEAT_OK",
		EnableSummary:   false,
		SendResponse:    false,
		NoHistory:       true, // Don't load session history for heartbeat
	}, wctx)
}

func (al *AgentLoop) processMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Add message preview to log (show full content for error messages)
	var logContent string
	if strings.Contains(msg.Content, "Error:") || strings.Contains(msg.Content, "error") {
		logContent = msg.Content // Full content for errors
	} else {
		logContent = utils.Truncate(msg.Content, 80)
	}
	logger.InfoCF("agent", fmt.Sprintf("Processing message from %s:%s: %s", msg.Channel, msg.SenderID, logContent),
		map[string]interface{}{
			"channel":     msg.Channel,
			"chat_id":     msg.ChatID,
			"sender_id":   msg.SenderID,
			"session_key": msg.SessionKey,
		})

	// Route system messages to processSystemMessage
	if msg.Channel == "system" {
		return al.processSystemMessage(ctx, msg)
	}

	// Check for !new command to start a fresh session
	trimmedContent := strings.TrimSpace(msg.Content)
	if strings.HasPrefix(trimmedContent, "!new") {
		var archiveName string
		parts := strings.Fields(trimmedContent)
		if len(parts) > 1 {
			archiveName = strings.Join(parts[1:], "_")
		}

		wctx := al.getOrCreateWorkspaceContext(msg.SenderID)
		newKey, archivedKey, err := al.RotateSession(ctx, msg.SessionKey, archiveName, wctx)
		if err != nil {
			return fmt.Sprintf("Error starting new session: %v", err), nil
		}

		return fmt.Sprintf("🚀 Started new session: `%s`.\nArchived previous session as: `%s`.", newKey, archivedKey), nil
	}

	// Check for commands
	if response, handled := al.handleCommand(ctx, msg); handled {
		return response, nil
	}

	wctx := al.getOrCreateWorkspaceContext(msg.SenderID)

	// Resolve active session key (if any)
	// This ensures we continue writing to the active session file instead of the base key
	activeSession := wctx.state.GetActiveSession(msg.SessionKey)
	effectiveSessionKey := msg.SessionKey
	if activeSession != "" {
		effectiveSessionKey = activeSession
	}

	// Process as user message
	return al.runAgentLoop(ctx, processOptions{
		SessionKey:      effectiveSessionKey,
		Channel:         msg.Channel,
		ChatID:          msg.ChatID,
		UserMessage:     msg.Content,
		DefaultResponse: "I've completed processing but have no response to give.",
		EnableSummary:   true,
		SendResponse:    false,
		Metadata:        msg.Metadata,
	}, wctx)
}

func (al *AgentLoop) generateSessionSummary(ctx context.Context, history []providers.Message) (string, error) {
	if len(history) == 0 {
		return "", fmt.Errorf("empty history")
	}

	// Prepare prompt
	prompt := "Create a VERY short session title (max 20 characters, 2-3 words, no special chars, use underscores). Format: [title]. Example: bug_fix_auth. Output ONLY the title."

	// Create new message list for summarization
	msgs := []providers.Message{
		{Role: "system", Content: prompt},
	}

	// Limit history and filter out tool messages to avoid orphan results
	// which cause 400 errors from strict providers like Anthropic.
	filteredHistory := make([]providers.Message, 0, len(history))
	for _, m := range history {
		if m.Role == "tool" || len(m.ToolCalls) > 0 {
			continue
		}
		filteredHistory = append(filteredHistory, m)
	}

	start := 0
	if len(filteredHistory) > 20 {
		start = len(filteredHistory) - 20
	}
	msgs = append(msgs, filteredHistory[start:]...)

	// Disable tools for summary generation
	resp, err := al.provider.Chat(ctx, msgs, []providers.ToolDefinition{}, al.model, map[string]interface{}{
		"max_tokens":  50,
		"temperature": 0.5,
	})
	if err != nil {
		return "", err
	}

	summary := strings.TrimSpace(resp.Content)

	// Basic sanitization
	summary = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, summary)

	// Deduplicate underscores
	for strings.Contains(summary, "__") {
		summary = strings.ReplaceAll(summary, "__", "_")
	}
	summary = strings.Trim(summary, "_")

	// Explicit truncation
	if len(summary) > 30 {
		summary = summary[:30]
		// Cut at last underscore if possible to avoid partial words
		if lastUnderscore := strings.LastIndex(summary, "_"); lastUnderscore > 10 {
			summary = summary[:lastUnderscore]
		}
	}

	return summary, nil
}

// RotateSession archives the current session and starts a new one.
// Returns the new session key, the archived session key, and any error.
func (al *AgentLoop) RotateSession(ctx context.Context, baseKey, archiveName string, wctx *workspaceContext) (string, string, error) {
	logger.InfoCF("agent", "RotateSession starting", map[string]interface{}{"baseKey": baseKey, "archiveName": archiveName, "workspace": wctx.path})

	// 1. Identify the current active session key (old key)
	oldKey := wctx.state.GetActiveSession(baseKey)
	if oldKey == "" {
		oldKey = baseKey // Fallback if no active session tracking yet
	}
	logger.InfoCF("agent", "RotateSession: oldKey identified", map[string]interface{}{"oldKey": oldKey})

	// 2. Auto-generate name if none provided
	if archiveName == "" {
		// Get history to generate summary
		history := wctx.sessions.GetHistory(oldKey)
		logger.InfoCF("agent", "RotateSession: history length", map[string]interface{}{"len": len(history)})
		if len(history) > 0 {
			logger.InfoCF("agent", "Generating session summary for archive", map[string]interface{}{"key": oldKey})
			generated, err := al.generateSessionSummary(ctx, history)
			if err == nil && generated != "" {
				archiveName = generated
			} else {
				logger.WarnCF("agent", "Failed to generate summary", map[string]interface{}{"error": err})
				// Fallback to timestamp ID
				archiveName = time.Now().Format("20060102_150405")
			}
		} else {
			// Empty history, just use timestamp
			archiveName = time.Now().Format("20060102_150405")
		}
	}
	logger.InfoCF("agent", "RotateSession: archiveName final", map[string]interface{}{"name": archiveName})

	// 3. Rename the OLD session file
	newArchiveKey := fmt.Sprintf("%s_%s", oldKey, archiveName)
	logger.InfoCF("agent", "RotateSession: renaming", map[string]interface{}{"old": oldKey, "new": newArchiveKey})
	if err := wctx.sessions.RenameSession(oldKey, newArchiveKey); err != nil {
		logger.WarnCF("agent", "Failed to rename archived session", map[string]interface{}{
			"error": err.Error(),
			"old":   oldKey,
			"new":   newArchiveKey,
		})
		// If rename fails (e.g. file not found), we keep oldKey as valid reference
		newArchiveKey = oldKey
	}

	// 4. Start NEW session
	newKey, err := wctx.state.StartNewSession(baseKey, "")
	if err != nil {
		logger.ErrorCF("agent", "Failed to start new session", map[string]interface{}{
			"error":       err.Error(),
			"session_key": baseKey,
		})
		return "", "", err
	}

	// BUG-2 FIX: Clean up the lock for the old session key to prevent memory leak
	// The lock map grows indefinitely otherwise.
	al.sessionLocks.Delete(oldKey)

	// 5. Archive session to long-term memory if enabled
	if al.memoryManager != nil {
		history := wctx.sessions.GetHistory(newArchiveKey)
		if len(history) > 0 {
			go func() {
				archiveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				if err := al.memoryManager.ArchiveSession(archiveCtx, wctx.path, newArchiveKey, history); err != nil {
					logger.WarnCF("agent", "Failed to archive session to long-term memory", map[string]interface{}{"error": err, "session": newArchiveKey})
				} else {
					logger.InfoCF("agent", "Session archived to long-term memory", map[string]interface{}{"session": newArchiveKey})
				}
			}()
		}
	}

	logger.InfoCF("agent", "RotateSession: completed", map[string]interface{}{"newKey": newKey, "archived": newArchiveKey})
	return newKey, newArchiveKey, nil
}

func (al *AgentLoop) processSystemMessage(ctx context.Context, msg bus.InboundMessage) (string, error) {
	// Verify this is a system message
	if msg.Channel != "system" {
		return "", fmt.Errorf("processSystemMessage called with non-system message channel: %s", msg.Channel)
	}

	logger.InfoCF("agent", "Processing system message",
		map[string]interface{}{
			"sender_id": msg.SenderID,
			"chat_id":   msg.ChatID,
		})

	// Resolve workspace for this system message
	wctx := al.getOrCreateWorkspaceContext(msg.SenderID)

	// Parse origin channel from chat_id (format: "channel:chat_id")
	var originChannel string
	if idx := strings.Index(msg.ChatID, ":"); idx > 0 {
		originChannel = msg.ChatID[:idx]
	} else {
		// Fallback
		originChannel = "cli"
	}

	// Extract subagent result from message content
	// Format: "Task 'label' completed.\n\nResult:\n<actual content>"
	content := msg.Content
	if idx := strings.Index(content, "Result:\n"); idx >= 0 {
		content = content[idx+8:] // Extract just the result part
	}

	// Skip internal channels - only log, don't send to user
	if constants.IsInternalChannel(originChannel) {
		logger.InfoCF("agent", "Subagent completed (internal channel)",
			map[string]interface{}{
				"sender_id":   msg.SenderID,
				"content_len": len(content),
				"channel":     originChannel,
			})
		return "", nil
	}

	// Agent acts as dispatcher only - subagent handles user interaction via message tool
	// Don't forward result here, subagent should use message tool to communicate with user
	logger.InfoCF("agent", "Subagent completed",
		map[string]interface{}{
			"sender_id":   msg.SenderID,
			"channel":     originChannel,
			"content_len": len(content),
			"workspace":   wctx.path,
		})

	// Agent only logs, does not respond to user
	return "", nil
}

// getSessionLock returns (or creates) a mutex for a specific session
func (al *AgentLoop) getSessionLock(sessionKey string) *sync.Mutex {
	mu, _ := al.sessionLocks.LoadOrStore(sessionKey, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

func (al *AgentLoop) runAgentLoop(ctx context.Context, opts processOptions, wctx *workspaceContext) (string, error) {
	// 0. Acquire session lock to prevent concurrent processing of the same session
	mu := al.getSessionLock(opts.SessionKey)
	mu.Lock()
	defer mu.Unlock()

	// 0. Record last channel for heartbeat notifications (skip internal channels)
	if opts.Channel != "" && opts.ChatID != "" {
		// Don't record internal channels (cli, system, subagent)
		if !constants.IsInternalChannel(opts.Channel) {
			channelKey := fmt.Sprintf("%s:%s", opts.Channel, opts.ChatID)
			if err := al.RecordLastChannel(wctx, channelKey); err != nil {
				logger.WarnCF("agent", "Failed to record last channel: %v", map[string]interface{}{"error": err.Error()})
			}
		}
	}

	// 1.5. Check for mailbox messages and inject as system context
	mailboxContext := ""
	workspaceName := al.config.ResolveWorkspaceName(wctx.path)
	messages_inbox, err := al.mailboxClient.List(workspaceName)
	if err == nil && len(messages_inbox) > 0 {
		var sb strings.Builder
		sb.WriteString("SYSTEM: You have the following unread inter-agent messages in your mailbox:\n")
		for _, m := range messages_inbox {
			sb.WriteString(fmt.Sprintf("- From %s (%s): %s\n", m.From, m.Timestamp.Format("2006-01-02 15:04"), m.Content))
			// Auto-mark as read once injected into context
			al.mailboxClient.MarkRead(workspaceName, m.Filename)
		}
		sb.WriteString("[End of Mailbox Messages]\n")
		mailboxContext = sb.String()
	}

	// 2. Build messages (skip history for heartbeat)
	var history []providers.Message
	var summary string
	if !opts.NoHistory {
		history = wctx.sessions.GetHistory(opts.SessionKey)
		summary = wctx.sessions.GetSummary(opts.SessionKey)
	}

	// Inject mailbox messages at the beginning of context if present
	if mailboxContext != "" {
		history = append([]providers.Message{{Role: "system", Content: mailboxContext}}, history...)
	}

	messages := wctx.contextBuilder.BuildMessages(
		history,
		summary,
		opts.UserMessage,
		nil,
		opts.Channel,
		opts.ChatID,
	)

	// 3. Save user message to session
	wctx.sessions.AddMessage(opts.SessionKey, "user", opts.UserMessage)

	// 10. Clear reactions (remove gear) in a defer to ensure it happens on failure too
	defer func() {
		if opts.Channel == "discord" && opts.Metadata["message_id"] != "" {
			al.bus.PublishOutbound(ctx, bus.OutboundMessage{
				Channel: opts.Channel,
				ChatID:  opts.ChatID,
				Type:    bus.MessageTypeReaction,
				Metadata: map[string]string{
					"action":     "remove",
					"emoji":      "⚙️",
					"message_id": opts.Metadata["message_id"],
				},
			})
		}
	}()

	// 4. Run LLM iteration loop
	finalContent, iteration, err := al.runLLMIteration(ctx, messages, opts, wctx)
	if err != nil {
		// Even if iteration fails, we should record an assistant response to maintain role balance
		errContent := fmt.Sprintf("Error processing message: %v", err)
		wctx.sessions.AddMessage(opts.SessionKey, "assistant", errContent)
		wctx.sessions.Save(opts.SessionKey)
		return "", err
	}

	// 5. Handle empty response
	if finalContent == "" {
		finalContent = opts.DefaultResponse
	}

	// 6. Save final assistant message to session
	wctx.sessions.AddMessage(opts.SessionKey, "assistant", finalContent)
	wctx.sessions.Save(opts.SessionKey)

	// 7. Optional: summarization
	if opts.EnableSummary {
		al.maybeSummarize(wctx, opts.SessionKey, opts.Channel, opts.ChatID)
	}

	// 8. Optional: send response via bus
	if opts.SendResponse {
		al.bus.PublishOutbound(ctx, bus.OutboundMessage{
			Channel: opts.Channel,
			ChatID:  opts.ChatID,
			Content: finalContent,
		})
	}

	// 9. Log response
	responsePreview := utils.Truncate(finalContent, 120)
	logger.InfoCF("agent", fmt.Sprintf("Response: %s", responsePreview),
		map[string]interface{}{
			"session_key":  opts.SessionKey,
			"iterations":   iteration,
			"final_length": len(finalContent),
		})

	return finalContent, nil
}

func (al *AgentLoop) GetSessionManager() *session.SessionManager {
	// For backward compatibility and testing, returns default workspace sessions
	wctx := al.getOrCreateWorkspaceContext("")
	return wctx.sessions
}

func (al *AgentLoop) GetStateManager() *state.Manager {
	// For backward compatibility and testing, returns default workspace state
	wctx := al.getOrCreateWorkspaceContext("")
	return wctx.state
}

func (al *AgentLoop) GetActiveSession(baseKey string) string {
	// For backward compatibility and testing, returns default workspace active session
	wctx := al.getOrCreateWorkspaceContext("")
	return wctx.state.GetActiveSession(baseKey)
}

func (al *AgentLoop) startTypingLoop(ctx context.Context, channel, chatID string) {
	defer al.wg.Done()
	// Don't send typing indicator for internal channels
	if constants.IsInternalChannel(channel) {
		return
	}

	ticker := time.NewTicker(8 * time.Second)
	defer ticker.Stop()

	// Send initial typing indicator
	al.bus.PublishOutbound(ctx, bus.OutboundMessage{
		Channel: channel,
		ChatID:  chatID,
		Type:    bus.MessageTypeTyping,
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			al.bus.PublishOutbound(ctx, bus.OutboundMessage{
				Channel: channel,
				ChatID:  chatID,
				Type:    bus.MessageTypeTyping,
			})
		}
	}
}

// runLLMIteration executes the LLM call loop with tool handling.
// Returns the final content, iteration count, and any error.
func (al *AgentLoop) runLLMIteration(ctx context.Context, messages []providers.Message, opts processOptions, wctx *workspaceContext) (string, int, error) {
	// Start typing indicator loop
	typingCtx, cancelTyping := context.WithCancel(ctx)
	defer cancelTyping()
	al.wg.Add(1)
	go al.startTypingLoop(typingCtx, opts.Channel, opts.ChatID)

	iteration := 0
	var finalContent string

	for iteration < al.maxIterations {
		iteration++

		logger.DebugCF("agent", "LLM iteration",
			map[string]interface{}{
				"iteration": iteration,
				"max":       al.maxIterations,
			})

		// Build tool definitions
		providerToolDefs := wctx.tools.ToProviderDefs()

		// Log LLM request details
		logger.DebugCF("agent", "LLM request",
			map[string]interface{}{
				"iteration":         iteration,
				"model":             al.model,
				"messages_count":    len(messages),
				"tools_count":       len(providerToolDefs),
				"max_tokens":        8192,
				"temperature":       0.7,
				"system_prompt_len": len(messages[0].Content),
			})

		// Log full messages (detailed)
		logger.DebugCF("agent", "Full LLM request",
			map[string]interface{}{
				"iteration":     iteration,
				"messages_json": formatMessagesForLog(messages),
				"tools_json":    formatToolsForLog(providerToolDefs),
			})

		var response *providers.LLMResponse
		var err error

		// Define queue notification for Discord
		onWait := func(info providers.WaiterInfo) {
			if opts.Channel == "discord" && opts.Metadata["message_id"] != "" {
				al.bus.PublishOutbound(ctx, bus.OutboundMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Type:    bus.MessageTypeReaction,
					Content: fmt.Sprintf("%d", info.Position),
					Metadata: map[string]string{
						"action":     "add",
						"emoji":      "queue",
						"message_id": opts.Metadata["message_id"],
					},
				})
			}
		}

		// Retry loop for context/token errors
		maxRetries := 2
		for retry := 0; retry <= maxRetries; retry++ {
			response, err = al.provider.Chat(ctx, messages, providerToolDefs, al.model, map[string]interface{}{
				"max_tokens":  8192,
				"temperature": 0.7,
				"wait":        true, // Wait for concurrency slots rather than retrying manually
				"on_wait":     onWait,
			})

			// If we were waiting in queue, clear queue reactions and add gear
			if opts.Channel == "discord" && opts.Metadata["message_id"] != "" {
				al.bus.PublishOutbound(ctx, bus.OutboundMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Type:    bus.MessageTypeReaction,
					Metadata: map[string]string{
						"action":     "clear_queue",
						"message_id": opts.Metadata["message_id"],
					},
				})
				al.bus.PublishOutbound(ctx, bus.OutboundMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Type:    bus.MessageTypeReaction,
					Metadata: map[string]string{
						"action":     "add",
						"emoji":      "⚙️",
						"message_id": opts.Metadata["message_id"],
					},
				})
			}

			if err == nil {
				break // Success
			}

			// Classify error using robust classification logic
			fErr := providers.ClassifyError(err, al.provider.GetID(), al.model)
			isTokenLimit := fErr != nil && fErr.Reason == providers.FailoverTokenLimit

			if isTokenLimit && retry < maxRetries {
					logger.WarnCF("agent", "Context window error detected, attempting compression", map[string]interface{}{
						"error": err.Error(),
						"retry": retry,
					})

					// Notify user on first retry only
					if retry == 0 && !constants.IsInternalChannel(opts.Channel) && opts.SendResponse {
						al.bus.PublishOutbound(ctx, bus.OutboundMessage{
							Channel: opts.Channel,
							ChatID:  opts.ChatID,
							Content: "⚠️ Context window exceeded. Compressing history and retrying...",
						})
					}

					// COMPRESSION LOGIC REFACTOR:
					// To avoid fragile state reloading, we perform a local compression on history
					// and rebuild the messages list for the retry.
					history := wctx.sessions.GetHistory(opts.SessionKey)
					if len(history) > 4 {
						// Perform emergency compression on the history slice (dropped oldest 50%)
						// We must maintain tool call/result parity!
						system := history[0]
						conversation := history[1 : len(history)-1]
						last := history[len(history)-1]
						
						mid := len(conversation) / 2
						
						// Back up mid if it points to a tool result orphan
						for mid > 0 && conversation[mid].Role == "tool" {
							mid--
						}
						
						droppedCount := mid
						newHist := make([]providers.Message, 0, len(history))
						newHist = append(newHist, system) // System
						newHist = append(newHist, providers.Message{
							Role:    "system",
							Content: fmt.Sprintf("[System: Emergency compression dropped %d oldest messages due to context limit]", droppedCount),
						})
						newHist = append(newHist, conversation[mid:]...)
						newHist = append(newHist, last) // Last message

						// Update session manager so future iterations/rounds don't hit the same limit
						wctx.sessions.SetHistory(opts.SessionKey, newHist)
						wctx.sessions.Save(opts.SessionKey)

						// Rebuild messages for CURRENT iteration retry
						newSummary := wctx.sessions.GetSummary(opts.SessionKey)
						messages = wctx.contextBuilder.BuildMessages(
							newHist,
							newSummary,
							"", // History already has the user message if iteration > 1 (or we just added it)
							nil,
							opts.Channel,
							opts.ChatID,
						)
					}

					continue
				}

			// Real error or success, break loop
			break
		}

		if err != nil {
			logger.ErrorCF("agent", "LLM call failed",
				map[string]interface{}{
					"iteration":       iteration,
					"error":           err.Error(),
					"message_summary": getMessageSummary(messages),
				})
			return "", iteration, fmt.Errorf("LLM call failed after retries: %w", err)
		}

		// Check if no tool calls - we're done
		if len(response.ToolCalls) == 0 {
			finalContent = response.Content
			logger.InfoCF("agent", "LLM response without tool calls (direct answer)",
				map[string]interface{}{
					"iteration":     iteration,
					"content_chars": len(finalContent),
				})
			break
		}

		// Log tool calls
		toolNames := make([]string, 0, len(response.ToolCalls))
		for _, tc := range response.ToolCalls {
			toolNames = append(toolNames, tc.Name)
		}
		logger.InfoCF("agent", "LLM requested tool calls",
			map[string]interface{}{
				"tools":     toolNames,
				"count":     len(response.ToolCalls),
				"iteration": iteration,
			})

		// Build assistant message with tool calls
		assistantMsg := providers.Message{
			Role:    "assistant",
			Content: response.Content,
		}
		for _, tc := range response.ToolCalls {
			argumentsJSON, _ := json.Marshal(tc.Arguments)
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, providers.ToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: &providers.FunctionCall{
					Name:      tc.Name,
					Arguments: string(argumentsJSON),
				},
			})
		}
		messages = append(messages, assistantMsg)

		// Save assistant message with tool calls to session
		wctx.sessions.AddFullMessage(opts.SessionKey, assistantMsg)

		// Execute tool calls
		if len(response.ToolCalls) > 0 {
			// Add tool reaction for Discord
			if opts.Channel == "discord" && opts.Metadata["message_id"] != "" {
				al.bus.PublishOutbound(ctx, bus.OutboundMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Type:    bus.MessageTypeReaction,
					Metadata: map[string]string{
						"action":     "add",
						"emoji":      "🛠️",
						"message_id": opts.Metadata["message_id"],
					},
				})
				// Remove reaction at the end of tool execution block (or iteration)
				defer al.bus.PublishOutbound(ctx, bus.OutboundMessage{
					Channel: opts.Channel,
					ChatID:  opts.ChatID,
					Type:    bus.MessageTypeReaction,
					Metadata: map[string]string{
						"action":     "remove",
						"emoji":      "🛠️",
						"message_id": opts.Metadata["message_id"],
					},
				})
			}

			for _, tc := range response.ToolCalls {
				// Log tool call with arguments preview
				argsJSON, _ := json.Marshal(tc.Arguments)
				argsPreview := utils.Truncate(string(argsJSON), 200)
				logger.InfoCF("agent", fmt.Sprintf("Tool call: %s(%s)", tc.Name, argsPreview),
					map[string]interface{}{
						"tool":      tc.Name,
						"iteration": iteration,
					})

				// Create async callback for tools that implement AsyncTool
				asyncCallback := func(callbackCtx context.Context, result *tools.ToolResult) {
					if !result.Silent && result.ForUser != "" {
						logger.InfoCF("agent", "Async tool completed, agent will handle notification",
							map[string]interface{}{
								"tool":        tc.Name,
								"content_len": len(result.ForUser),
							})
					}
				}

				toolResult := wctx.tools.ExecuteWithContext(ctx, tc.Name, tc.Arguments, opts.Channel, opts.ChatID, asyncCallback)

				// Send ForUser content to user immediately if not Silent
				if !toolResult.Silent && toolResult.ForUser != "" && opts.SendResponse {
					al.bus.PublishOutbound(ctx, bus.OutboundMessage{
						Channel: opts.Channel,
						ChatID:  opts.ChatID,
						Content: toolResult.ForUser,
					})
				}

				// Determine content for LLM based on tool result
				contentForLLM := toolResult.ForLLM
				if contentForLLM == "" && toolResult.Err != nil {
					contentForLLM = toolResult.Err.Error()
				}

				toolResultMsg := providers.Message{
					Role:       "tool",
					Content:    contentForLLM,
					ToolCallID: tc.ID,
				}
				messages = append(messages, toolResultMsg)

				// Save tool result message to session
				wctx.sessions.AddFullMessage(opts.SessionKey, toolResultMsg)
			}
		}
	}

	return finalContent, iteration, nil
}

// updateToolContexts updates the context for tools that need channel/chatID info.
func (al *AgentLoop) updateToolContexts(registry *tools.ToolRegistry, channel, chatID string) {
	if registry == nil {
		return
	}

	// Use ContextualTool interface instead of type assertions
	if tool, ok := registry.Get("message"); ok {
		if mt, ok := tool.(tools.ContextualTool); ok {
			mt.SetContext(channel, chatID)
		}
	}
	if tool, ok := registry.Get("spawn"); ok {
		if st, ok := tool.(tools.ContextualTool); ok {
			st.SetContext(channel, chatID)
		}
	}
	if tool, ok := registry.Get("subagent"); ok {
		if st, ok := tool.(tools.ContextualTool); ok {
			st.SetContext(channel, chatID)
		}
	}
}

// maybeSummarize triggers summarization if the session history exceeds thresholds.
func (al *AgentLoop) maybeSummarize(wctx *workspaceContext, sessionKey, channel, chatID string) {
	newHistory := wctx.sessions.GetHistory(sessionKey)
	tokenEstimate := al.estimateTokens(newHistory)
	threshold := al.contextWindow * 75 / 100

	if tokenEstimate > threshold {
		if _, loading := al.summarizing.LoadOrStore(sessionKey, true); !loading {
			al.wg.Add(1)
			go func() {
				defer al.wg.Done()
				defer al.summarizing.Delete(sessionKey)

				// Acquire summarization semaphore
				select {
				case al.summarizationSem <- struct{}{}:
					defer func() { <-al.summarizationSem }()
				case <-time.After(10 * time.Second):
					logger.WarnCF("agent", "Timed out waiting for summarization semaphore", map[string]interface{}{
						"session_key": sessionKey,
					})
					return
				}

				// Notify user about optimization if not an internal channel
				if !constants.IsInternalChannel(channel) {
					al.bus.PublishOutbound(context.Background(), bus.OutboundMessage{
						Channel: channel,
						ChatID:  chatID,
						Content: "⚠️ Memory threshold reached. Optimizing conversation history...",
					})
				}
				al.summarizeSession(wctx, sessionKey)
			}()
		}
	}
}

// forceCompression aggressively reduces context when the limit is hit.
// It drops the oldest 50% of messages (keeping system prompt and last user message).
func (al *AgentLoop) forceCompression(sessionKey string, wctx *workspaceContext) {
	history := wctx.sessions.GetHistory(sessionKey)
	if len(history) <= 4 {
		return
	}

	// Keep system prompt (usually [0]) and the very last message (user's trigger)
	// We want to drop the oldest half of the *conversation*
	// Assuming [0] is system, [1:] is conversation
	system := history[0]
	conversation := history[1 : len(history)-1]
	last := history[len(history)-1]

	if len(conversation) == 0 {
		return
	}

	// Helper to find the mid-point of the conversation
	mid := len(conversation) / 2

	// BUG-1 FIX: Ensure we don't drop the tool call (assistant role) if we kept the result (tool role)
	// Protocol requires [assistant (tool_call), tool (result)] pairs.
	// We back up mid if it points to a tool result, ensuring we either drop both or keep both.
	for mid > 0 && conversation[mid].Role == "tool" {
		mid--
	}

	droppedCount := mid
	keptConversation := conversation[mid:]

	newHistory := make([]providers.Message, 0)
	newHistory = append(newHistory, system)

	// Add a note about compression
	compressionNote := fmt.Sprintf("[System: Emergency compression dropped %d oldest messages due to context limit]", droppedCount)

	// We only modify the messages list here
	newHistory = append(newHistory, providers.Message{
		Role:    "system",
		Content: compressionNote,
	})

	newHistory = append(newHistory, keptConversation...)
	newHistory = append(newHistory, last)

	// Update session
	wctx.sessions.SetHistory(sessionKey, newHistory)
	wctx.sessions.Save(sessionKey)

	logger.WarnCF("agent", "Forced compression executed", map[string]interface{}{
		"session_key":  sessionKey,
		"dropped_msgs": droppedCount,
		"new_count":    len(newHistory),
	})
}


// getMessageSummary returns a concise summary of messages for logging
func getMessageSummary(messages []providers.Message) []map[string]interface{} {
	summary := make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		item := map[string]interface{}{
			"role": m.Role,
		}
		if len(m.ToolCalls) > 0 {
			toolNames := make([]string, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				toolNames[j] = tc.Name
			}
			item["tool_calls"] = toolNames
		}
		if m.ToolCallID != "" {
			item["tool_call_id"] = m.ToolCallID
		}
		summary[i] = item
	}
	return summary
}

// GetStartupInfo returns information about loaded tools and skills for the default workspace.
func (al *AgentLoop) GetStartupInfo() map[string]interface{} {
	info := make(map[string]interface{})
	wctx := al.getOrCreateWorkspaceContext("")

	// Tools info
	tools := wctx.tools.List()
	info["tools"] = map[string]interface{}{
		"count": len(tools),
		"names": tools,
	}

	// Skills info
	info["skills"] = wctx.contextBuilder.GetSkillsInfo()

	return info
}

// formatMessagesForLog formats messages for logging
func formatMessagesForLog(messages []providers.Message) string {
	if len(messages) == 0 {
		return "[]"
	}

	var result string
	result += "[\n"
	for i, msg := range messages {
		result += fmt.Sprintf("  [%d] Role: %s\n", i, msg.Role)
		if len(msg.ToolCalls) > 0 {
			result += "  ToolCalls:\n"
			for _, tc := range msg.ToolCalls {
				result += fmt.Sprintf("    - ID: %s, Type: %s, Name: %s\n", tc.ID, tc.Type, tc.Name)
				if tc.Function != nil {
					result += fmt.Sprintf("      Arguments: %s\n", utils.Truncate(tc.Function.Arguments, 200))
				}
			}
		}
		if msg.Content != "" {
			content := utils.Truncate(msg.Content, 200)
			result += fmt.Sprintf("  Content: %s\n", content)
		}
		if msg.ToolCallID != "" {
			result += fmt.Sprintf("  ToolCallID: %s\n", msg.ToolCallID)
		}
		result += "\n"
	}
	result += "]"
	return result
}

// formatToolsForLog formats tool definitions for logging
func formatToolsForLog(tools []providers.ToolDefinition) string {
	if len(tools) == 0 {
		return "[]"
	}

	var result string
	result += "[\n"
	for i, tool := range tools {
		result += fmt.Sprintf("  [%d] Type: %s, Name: %s\n", i, tool.Type, tool.Function.Name)
		result += fmt.Sprintf("      Description: %s\n", tool.Function.Description)
		if len(tool.Function.Parameters) > 0 {
			result += fmt.Sprintf("      Parameters: %s\n", utils.Truncate(fmt.Sprintf("%v", tool.Function.Parameters), 200))
		}
	}
	result += "]"
	return result
}

// summarizeSession summarizes the conversation history for a session.
func (al *AgentLoop) summarizeSession(wctx *workspaceContext, sessionKey string) {
	timeout := al.timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	history := wctx.sessions.GetHistory(sessionKey)
	summary := wctx.sessions.GetSummary(sessionKey)

	// Keep last 4 messages for continuity
	if len(history) <= 4 {
		return
	}

	toSummarize := history[:len(history)-4]

	// Archive to long-term memory before truncating
	if al.memoryManager != nil && al.memoryManager.IsEnabled() {
		if err := al.memoryManager.ArchiveSession(ctx, wctx.path, sessionKey, toSummarize); err != nil {
			logger.WarnCF("agent", "Failed to archive session segment during summarization", map[string]interface{}{"error": err, "session": sessionKey})
		} else {
			logger.InfoCF("agent", "Archived session segment to long-term memory", map[string]interface{}{"session": sessionKey, "messages": len(toSummarize)})
		}
	}

	// Oversized Message Guard
	// Skip messages larger than 50% of context window to prevent summarizer overflow
	maxMessageTokens := al.contextWindow / 2
	validMessages := make([]providers.Message, 0)
	omitted := false

	for _, m := range toSummarize {
		// BUG-8 FIX: Include all message types (tool calls/results too)
		// so DiscardFirst doesn't drop content that was never summarized.
		// Estimate tokens for this message
		msgTokens := len(m.Content) / 2 // Use safer estimate here too (2.5 -> 2 for integer division safety)
		if msgTokens > maxMessageTokens {
			omitted = true
			continue
		}
		validMessages = append(validMessages, m)
	}

	if len(validMessages) == 0 {
		return
	}

	// Multi-Part Summarization
	// Split into two parts if history is significant
	var finalSummary string
	if len(validMessages) > 10 {
		mid := len(validMessages) / 2
		part1 := validMessages[:mid]
		part2 := validMessages[mid:]

		s1, _ := al.summarizeBatch(ctx, part1, "")
		s2, _ := al.summarizeBatch(ctx, part2, "")

		// Merge them
		mergePrompt := fmt.Sprintf("Merge these two conversation summaries into one cohesive summary:\n\n1: %s\n\n2: %s", s1, s2)
		resp, err := al.provider.Chat(ctx, []providers.Message{{Role: "user", Content: mergePrompt}}, nil, al.model, map[string]interface{}{
			"max_tokens":  1024,
			"temperature": 0.3,
		})
		if err == nil {
			finalSummary = resp.Content
		} else {
			finalSummary = s1 + " " + s2
		}
	} else {
		finalSummary, _ = al.summarizeBatch(ctx, validMessages, summary)
	}

	if omitted && finalSummary != "" {
		finalSummary += "\n[Note: Some oversized messages were omitted from this summary for efficiency.]"
	}

	if finalSummary != "" {
		wctx.sessions.SetSummary(sessionKey, finalSummary)
		wctx.sessions.DiscardFirst(sessionKey, len(toSummarize))
		wctx.sessions.Save(sessionKey)
	}
}

// summarizeBatch summarizes a batch of messages.
func (al *AgentLoop) summarizeBatch(ctx context.Context, batch []providers.Message, existingSummary string) (string, error) {
	prompt := "Provide a concise summary of this conversation segment, preserving core context and key points.\n"
	if existingSummary != "" {
		prompt += "Existing context: " + existingSummary + "\n"
	}
	prompt += "\nCONVERSATION:\n"
	for _, m := range batch {
		prompt += fmt.Sprintf("%s: %s\n", m.Role, m.Content)
	}

	response, err := al.provider.Chat(ctx, []providers.Message{{Role: "user", Content: prompt}}, nil, al.model, map[string]interface{}{
		"max_tokens":  1024,
		"temperature": 0.3,
	})
	if err != nil {
		return "", err
	}
	return response.Content, nil
}

// estimateTokens estimates the number of tokens in a message list.
// Uses a safe heuristic of 2.5 characters per token to account for CJK and other
// overheads better than the previous 3 chars/token.
func (al *AgentLoop) estimateTokens(messages []providers.Message) int {
	totalChars := 0
	for _, m := range messages {
		totalChars += utf8.RuneCountInString(m.Content)
	}
	// 2.5 chars per token = totalChars * 2 / 5
	return totalChars * 2 / 5
}

func (al *AgentLoop) handleCommand(ctx context.Context, msg bus.InboundMessage) (string, bool) {
	content := strings.TrimSpace(msg.Content)
	if !strings.HasPrefix(content, "/") {
		return "", false
	}

	parts := strings.Fields(content)
	if len(parts) == 0 {
		return "", false
	}

	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "/show":
		if len(args) < 1 {
			return "Usage: /show [model|channel]", true
		}
		switch args[0] {
		case "model":
			return fmt.Sprintf("Current model: %s", al.model), true
		case "channel":
			return fmt.Sprintf("Current channel: %s", msg.Channel), true
		default:
			return fmt.Sprintf("Unknown show target: %s", args[0]), true
		}

	case "/list":
		if len(args) < 1 {
			return "Usage: /list [models|channels]", true
		}
		switch args[0] {
		case "models":
			// TODO: Fetch available models dynamically if possible
			return "Available models: glm-4.7, claude-3-5-sonnet, gpt-4o (configured in config.json/env)", true
		case "channels":
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			channels := al.channelManager.GetEnabledChannels()
			if len(channels) == 0 {
				return "No channels enabled", true
			}
			return fmt.Sprintf("Enabled channels: %s", strings.Join(channels, ", ")), true
		default:
			return fmt.Sprintf("Unknown list target: %s", args[0]), true
		}

	case "/switch":
		if len(args) < 3 || args[1] != "to" {
			return "Usage: /switch [model|channel] to <name>", true
		}
		target := args[0]
		value := args[2]

		switch target {
		case "model":
			oldModel := al.model
			al.model = value
			return fmt.Sprintf("Switched model from %s to %s", oldModel, value), true
		case "channel":
			// This changes the 'default' channel for some operations, or effectively redirects output?
			// For now, let's just validate if the channel exists
			if al.channelManager == nil {
				return "Channel manager not initialized", true
			}
			if _, exists := al.channelManager.GetChannel(value); !exists && value != "cli" {
				return fmt.Sprintf("Channel '%s' not found or not enabled", value), true
			}

			// If message came from CLI, maybe we want to redirect CLI output to this channel?
			// That would require state persistence about "redirected channel"
			// For now, just acknowledged.
			return fmt.Sprintf("Switched target channel to %s (Note: this currently only validates existence)", value), true
		default:
			return fmt.Sprintf("Unknown switch target: %s", target), true
		}
	}

	return "", false
}
