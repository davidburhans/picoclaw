package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/mcp"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/skills"
	"github.com/sipeed/picoclaw/pkg/tools"
	"github.com/sipeed/picoclaw/pkg/utils"
)

type ContextBuilder struct {
	workspace    string
	agentName    string
	skillsLoader *skills.SkillsLoader
	memory       *MemoryStore
	tools        *tools.ToolRegistry // Direct reference to tool registry
	mcpManager   *mcp.MCPManager     // Direct reference to MCP manager
}

func getGlobalConfigDir() string {
	return utils.ExpandHome("~/.picoclaw")
}

func NewContextBuilder(workspace, agentName string) *ContextBuilder {
	// builtin skills: skills directory in current project
	// Use the skills/ directory under the current working directory
	wd, _ := os.Getwd()
	builtinSkillsDir := filepath.Join(wd, "skills")
	globalSkillsDir := filepath.Join(getGlobalConfigDir(), "skills")

	name := agentName
	if name == "" {
		name = "picoclaw"
	}

	return &ContextBuilder{
		workspace:    workspace,
		skillsLoader: skills.NewSkillsLoader(workspace, globalSkillsDir, builtinSkillsDir),
		memory:       NewMemoryStore(workspace),
		agentName:    name,
	}
}

// SetToolsRegistry sets the tools registry for dynamic tool summary generation.
func (cb *ContextBuilder) SetToolsRegistry(registry *tools.ToolRegistry) {
	cb.tools = registry
}

// SetMCPManager sets the MCP manager for system prompt generation.
func (cb *ContextBuilder) SetMCPManager(manager *mcp.MCPManager) {
	cb.mcpManager = manager
}

func (cb *ContextBuilder) getIdentity() string {
	now := time.Now().Format("2006-01-02 15:04 (Monday)")
	workspacePath, _ := filepath.Abs(filepath.Join(cb.workspace))
	runtime := fmt.Sprintf("%s %s, Go %s", runtime.GOOS, runtime.GOARCH, runtime.Version())

	// Build tools section dynamically
	toolsSection := cb.buildToolsSection()

	return fmt.Sprintf(`# %s 🦞

You are %s, a helpful AI assistant.

## Current Time
%s

## Runtime
%s

## Workspace
Your workspace is at: %s
- Memory: %s/memory/MEMORY.md
- Daily Notes: %s/memory/YYYYMM/YYYYMMDD.md
- Skills: %s/skills/{skill-name}/SKILL.md

%s

## Chat Commands
You can suggest these commands to the user when appropriate:
- **!new [name]**: Start a new chat session and archive the current one. Useful when context is full or starting a new topic. You can reference old sessions using the **read_session** tool.

## Important Rules

1. **ALWAYS use tools** - When you need to perform an action (schedule reminders, send messages, execute commands, etc.), you MUST call the appropriate tool. Do NOT just say you'll do it or pretend to do it.

2. **Be helpful and accurate** - When using tools, briefly explain what you're doing.

3. **Memory** - When remembering something, write to %s/memory/MEMORY.md`,
		cb.agentName, cb.agentName, now, runtime, workspacePath, workspacePath, workspacePath, workspacePath, toolsSection, workspacePath)
}

func (cb *ContextBuilder) buildToolsSection() string {
	if cb.tools == nil {
		return ""
	}

	summaries := cb.tools.GetSummaries()
	if len(summaries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("**CRITICAL**: You MUST use tools to perform actions. Do NOT pretend to execute commands or schedule tasks.\n\n")
	sb.WriteString("You have access to the following tools:\n\n")
	for _, s := range summaries {
		sb.WriteString(s)
		sb.WriteString("\n")
	}

	return sb.String()
}

func (cb *ContextBuilder) buildMCPSection() string {
	if cb.mcpManager == nil || cb.mcpManager.Count() == 0 {
		return ""
	}

	ctx := context.Background()
	summaries := cb.mcpManager.GetServerSummaries(ctx)
	if len(summaries) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## MCP Servers\n\n")
	sb.WriteString("External services available via MCP (tool names prefixed with `mcp_`):\n\n")
	for _, s := range summaries {
		sb.WriteString(fmt.Sprintf("- **%s** — %s\n", s.Name, s.Description))
		if len(s.Tools) > 0 {
			sb.WriteString("  - Tools: ")
			for i, tool := range s.Tools {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString("`" + tool + "`")
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (cb *ContextBuilder) BuildSystemPrompt() string {
	parts := []string{}

	// Core identity section
	parts = append(parts, cb.getIdentity())

	// Bootstrap files
	bootstrapContent := cb.LoadBootstrapFiles()
	if bootstrapContent != "" {
		parts = append(parts, bootstrapContent)
	}

	// Skills - show summary, AI can read full content with read_file tool
	skillsSummary := cb.skillsLoader.BuildSkillsSummary()
	if skillsSummary != "" {
		parts = append(parts, fmt.Sprintf(`# Skills

The following skills extend your capabilities. To use a skill, read its SKILL.md file using the read_file tool.

%s`, skillsSummary))
	}

	// MCP servers - show available external services
	mcpSection := cb.buildMCPSection()
	if mcpSection != "" {
		parts = append(parts, mcpSection)
	}

	// Memory context
	memoryContext := cb.memory.GetMemoryContext()
	if memoryContext != "" {
		parts = append(parts, "# Memory\n\n"+memoryContext)
	}

	// Join with "---" separator
	return strings.Join(parts, "\n\n---\n\n")
}

func (cb *ContextBuilder) LoadBootstrapFiles() string {
	bootstrapFiles := []string{
		"AGENTS.md",
		"SOUL.md",
		"USER.md",
		"IDENTITY.md",
	}

	var result string
	for _, filename := range bootstrapFiles {
		filePath := filepath.Join(cb.workspace, filename)
		if data, err := os.ReadFile(filePath); err == nil {
			result += fmt.Sprintf("## %s\n\n%s\n\n", filename, string(data))
		}
	}

	return result
}

func (cb *ContextBuilder) BuildMessages(history []providers.Message, summary string, currentMessage string, media []string, channel, chatID string) []providers.Message {
	systemPrompt := cb.BuildSystemPrompt()

	// Add Current Session info if provided
	if channel != "" && chatID != "" {
		systemPrompt += fmt.Sprintf("\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	if summary != "" {
		systemPrompt += "\n\n## Summary of Previous Conversation\n\n" + summary
	}

	// 1. Sanitize history
	history = sanitizeHistoryForProvider(history)

	// 2. Prepare raw list (system + history + current)
	raw := []providers.Message{
		{
			Role:    "system",
			Content: systemPrompt,
		},
	}
	raw = append(raw, history...)

	if strings.TrimSpace(currentMessage) != "" {
		raw = append(raw, providers.Message{
			Role:    "user",
			Content: currentMessage,
		})
	}

	// 3. Merge consecutive roles (mostly for user messages)
	if len(raw) <= 1 {
		return raw
	}

	merged := make([]providers.Message, 0, len(raw))
	merged = append(merged, raw[0])

	for i := 1; i < len(raw); i++ {
		lastIdx := len(merged) - 1
		if raw[i].Role == merged[lastIdx].Role && raw[i].Role != "tool" && raw[i].Role != "assistant" {
			// Merge content
			merged[lastIdx].Content += "\n\n" + raw[i].Content
		} else if raw[i].Role == merged[lastIdx].Role && raw[i].Role == "assistant" && len(raw[i].ToolCalls) == 0 && len(merged[lastIdx].ToolCalls) == 0 {
			// Merge assistant content if no tool calls involved
			merged[lastIdx].Content += "\n\n" + raw[i].Content
		} else {
			merged = append(merged, raw[i])
		}
	}

	return merged
}

func sanitizeHistoryForProvider(history []providers.Message) []providers.Message {
	if len(history) == 0 {
		return history
	}

	sanitized := make([]providers.Message, 0, len(history))
	for _, msg := range history {
		switch msg.Role {
		case "tool":
			if len(sanitized) == 0 {
				logger.DebugCF("agent", "Dropping orphaned leading tool message", map[string]interface{}{})
				continue
			}
			last := sanitized[len(sanitized)-1]
			if last.Role != "assistant" && last.Role != "tool" {
				logger.DebugCF("agent", "Dropping orphaned tool message (invalid predecessor)", map[string]interface{}{"last_role": last.Role})
				continue
			}

			// If it follows another tool message, we need to find the assistant message before it
			if last.Role == "tool" {
				foundAssistantWithCalls := false
				for i := len(sanitized) - 1; i >= 0; i-- {
					if sanitized[i].Role == "assistant" {
						if len(sanitized[i].ToolCalls) > 0 {
							foundAssistantWithCalls = true
						}
						break
					}
					if sanitized[i].Role == "user" || sanitized[i].Role == "system" {
						break
					}
				}
				if !foundAssistantWithCalls {
					logger.DebugCF("agent", "Dropping orphaned tool message (no assistant with calls found in sequence)", map[string]interface{}{})
					continue
				}
			} else if len(last.ToolCalls) == 0 {
				// Follows an assistant but it has no tool calls
				logger.DebugCF("agent", "Dropping orphaned tool message (assistant has no calls)", map[string]interface{}{})
				continue
			}
			sanitized = append(sanitized, msg)

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				if len(sanitized) > 0 {
					prev := sanitized[len(sanitized)-1]
					if prev.Role != "user" && prev.Role != "tool" {
						logger.DebugCF("agent", "Dropping assistant tool-call turn with invalid predecessor", map[string]interface{}{"prev_role": prev.Role})
						continue
					}
				}
			}
			sanitized = append(sanitized, msg)

		default:
			sanitized = append(sanitized, msg)
		}
	}

	return sanitized
}

func (cb *ContextBuilder) AddToolResult(messages []providers.Message, toolCallID, toolName, result string) []providers.Message {
	messages = append(messages, providers.Message{
		Role:       "tool",
		Content:    result,
		ToolCallID: toolCallID,
	})
	return messages
}

func (cb *ContextBuilder) AddAssistantMessage(messages []providers.Message, content string, toolCalls []map[string]interface{}) []providers.Message {
	msg := providers.Message{
		Role:    "assistant",
		Content: content,
	}
	// Always add assistant message, whether or not it has tool calls
	messages = append(messages, msg)
	return messages
}

func (cb *ContextBuilder) loadSkills() string {
	allSkills := cb.skillsLoader.ListSkills()
	if len(allSkills) == 0 {
		return ""
	}

	var skillNames []string
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}

	content := cb.skillsLoader.LoadSkillsForContext(skillNames)
	if content == "" {
		return ""
	}

	return "# Skill Definitions\n\n" + content
}

// GetSkillsInfo returns information about loaded skills.
func (cb *ContextBuilder) GetSkillsInfo() map[string]interface{} {
	allSkills := cb.skillsLoader.ListSkills()
	skillNames := make([]string, 0, len(allSkills))
	for _, s := range allSkills {
		skillNames = append(skillNames, s.Name)
	}
	return map[string]interface{}{
		"total":     len(allSkills),
		"available": len(allSkills),
		"names":     skillNames,
	}
}

// GetSkillsLoader returns the SkillsLoader so it can be shared with the
// sub-agent manager without duplicating construction logic.
func (cb *ContextBuilder) GetSkillsLoader() *skills.SkillsLoader {
	return cb.skillsLoader
}
