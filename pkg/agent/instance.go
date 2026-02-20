package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/routing"
	"github.com/sipeed/picoclaw/pkg/session"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/utils"
)

// AgentInstance represents a fully configured agent with its own workspace,
// session manager, context builder, and tool registry.
type AgentInstance struct {
	ID             string
	Name           string
	Model          string
	Fallbacks      []string
	Workspace      string
	MaxIterations  int
	MaxTokens      int
	Temperature    float64
	ContextWindow  int
	Provider       providers.LLMProvider
	Sessions       *session.SessionManager
	ContextBuilder *ContextBuilder
	Tools          *tools.ToolRegistry
	Subagents      *config.SubagentsConfig
	SkillsFilter   []string
	Candidates     []providers.FallbackCandidate
}

// NewAgentInstance creates an agent instance from config.
func NewAgentInstance(
	agentCfg *config.AgentConfig,
	defaults *config.AgentDefaults,
	cfg *config.Config,
	provider providers.LLMProvider,
) *AgentInstance {
	workspace, allowedPaths := resolveAgentWorkspace(agentCfg, defaults, cfg)
	os.MkdirAll(workspace, 0755)

	model := resolveAgentModel(agentCfg, defaults)
	fallbacks := resolveAgentFallbacks(agentCfg, defaults)

	restrict := true
	if defaults.RestrictToWorkspace != nil {
		restrict = *defaults.RestrictToWorkspace
	}
	toolsRegistry := tools.NewToolRegistry()
	toolsRegistry.Register(tools.NewReadFileTool(workspace, allowedPaths, restrict))
	toolsRegistry.Register(tools.NewWriteFileTool(workspace, allowedPaths, restrict))
	toolsRegistry.Register(tools.NewListDirTool(workspace, allowedPaths, restrict))
	toolsRegistry.Register(tools.NewExecToolWithConfig(workspace, allowedPaths, restrict, cfg))
	toolsRegistry.Register(tools.NewEditFileTool(workspace, allowedPaths, restrict))
	toolsRegistry.Register(tools.NewAppendFileTool(workspace, allowedPaths, restrict))

	sessionsDir := filepath.Join(workspace, "sessions")
	sessionsManager := session.NewSessionManager(sessionsDir)

	agentID := routing.DefaultAgentID
	agentName := ""
	var subagents *config.SubagentsConfig
	var skillsFilter []string

	if agentCfg != nil {
		agentID = routing.NormalizeAgentID(agentCfg.ID)
		agentName = agentCfg.Name
		subagents = agentCfg.Subagents
		skillsFilter = agentCfg.Skills
	}

	contextBuilder := NewContextBuilder(workspace, agentName)
	contextBuilder.SetToolsRegistry(toolsRegistry)

	maxIter := 20
	if defaults.MaxToolIterations != nil {
		maxIter = *defaults.MaxToolIterations
	}

	maxTokens := 8192
	if defaults.MaxTokens != nil && *defaults.MaxTokens > 0 {
		maxTokens = *defaults.MaxTokens
	}

	temperature := 0.7
	if defaults.Temperature != nil {
		temperature = *defaults.Temperature
	}

	// Resolve fallback candidates
	modelCfg := providers.ModelConfig{
		Primary:   model,
		Fallbacks: fallbacks,
	}
	candidates := providers.ResolveCandidates(modelCfg, defaults.Provider)

	return &AgentInstance{
		ID:             agentID,
		Name:           agentName,
		Model:          model,
		Fallbacks:      fallbacks,
		Workspace:      workspace,
		MaxIterations:  maxIter,
		MaxTokens:      maxTokens,
		Temperature:    temperature,
		ContextWindow:  maxTokens,
		Provider:       provider,
		Sessions:       sessionsManager,
		ContextBuilder: contextBuilder,
		Tools:          toolsRegistry,
		Subagents:      subagents,
		SkillsFilter:   skillsFilter,
		Candidates:     candidates,
	}
}

// resolveAgentWorkspace determines the workspace directory for an agent and any allowed external paths.
func resolveAgentWorkspace(agentCfg *config.AgentConfig, defaults *config.AgentDefaults, cfg *config.Config) (string, []string) {
	wsName := ""
	if agentCfg != nil && strings.TrimSpace(agentCfg.Workspace) != "" {
		wsName = strings.TrimSpace(agentCfg.Workspace)
	} else if agentCfg == nil || agentCfg.Default || agentCfg.ID == "" || routing.NormalizeAgentID(agentCfg.ID) == "main" {
		wsName = defaults.Workspace
	}

	if wsName != "" {
		// Check if it's a named workspace in config
		if wsCfg, ok := cfg.Workspaces[wsName]; ok {
			return utils.ExpandHome(wsCfg.Path), wsCfg.AllowedExternalPaths
		}
		// Otherwise treat as a path
		return utils.ExpandHome(wsName), nil
	}

	home := utils.ExpandHome("~")
	id := routing.NormalizeAgentID(agentCfg.ID)
	return filepath.Join(home, ".picoclaw", "workspace-"+id), nil
}

// resolveAgentModel resolves the primary model for an agent.
func resolveAgentModel(agentCfg *config.AgentConfig, defaults *config.AgentDefaults) string {
	if agentCfg != nil && agentCfg.Model != nil && strings.TrimSpace(agentCfg.Model.Primary) != "" {
		return strings.TrimSpace(agentCfg.Model.Primary)
	}
	return defaults.Model
}

// resolveAgentFallbacks resolves the fallback models for an agent.
func resolveAgentFallbacks(agentCfg *config.AgentConfig, defaults *config.AgentDefaults) []string {
	if agentCfg != nil && agentCfg.Model != nil && agentCfg.Model.Fallbacks != nil {
		return agentCfg.Model.Fallbacks
	}
	return defaults.ModelFallbacks
}
