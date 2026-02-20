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
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".picoclaw")
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
	messages := []providers.Message{}

	systemPrompt := cb.BuildSystemPrompt()

	// Add Current Session info if provided
	if channel != "" && chatID != "" {
		systemPrompt += fmt.Sprintf("\n\n## Current Session\nChannel: %s\nChat ID: %s", channel, chatID)
	}

	// Log system prompt summary for debugging (debug mode only)
	logger.DebugCF("agent", "System prompt built",
		map[string]interface{}{
			"total_chars":   len(systemPrompt),
			"total_lines":   strings.Count(systemPrompt, "\n") + 1,
			"section_count": strings.Count(systemPrompt, "\n\n---\n\n") + 1,
		})

	// Log preview of system prompt (avoid logging huge content)
	preview := systemPrompt
	if len(preview) > 500 {
		preview = preview[:500] + "... (truncated)"
	}
	logger.DebugCF("agent", "System prompt preview",
		map[string]interface{}{
			"preview": preview,
		})

	if summary != "" {
		systemPrompt += "\n\n## Summary of Previous Conversation\n\n" + summary
	}

	messages = append(messages, providers.Message{
		Role:    "system",
		Content: systemPrompt,
	})

	// Add history and current message
	rawMessages := append([]providers.Message{}, history...)
	rawMessages = append(rawMessages, providers.Message{
		Role:    "user",
		Content: currentMessage,
	})

	// Sanitize messages to ensure strict role alternation
	// 1. Remove orphaned tool messages at the start (already handled partly by previous fix, but let's be thorough)
	// 2. Merge consecutive messages with the same role
	sanitized := []providers.Message{messages[0]} // Start with system prompt
	for _, m := range rawMessages {
		if len(sanitized) == 0 {
			sanitized = append(sanitized, m)
			continue
		}

		last := &sanitized[len(sanitized)-1]

		// Role alternation logic
		if m.Role == last.Role && m.Role != "tool" {
			// Merge consecutive roles (except system and tool)
			if m.Role != "system" {
				if last.Content != "" && m.Content != "" {
					last.Content += "\n\n" + m.Content
				} else if m.Content != "" {
					last.Content = m.Content
				}
				// Merge tool calls if any
				if len(m.ToolCalls) > 0 {
					last.ToolCalls = append(last.ToolCalls, m.ToolCalls...)
				}
				logger.DebugCF("agent", "Merged consecutive messages with same role", map[string]interface{}{"role": m.Role})
				continue
			}
		}

		// Handle Tool message requirements: must follow assistant message with tool calls
		// OR follow another tool message that eventually follows an assistant message with tool calls
		if m.Role == "tool" {
			// Find the nearest preceding non-tool message
			foundAssistant := false
			for j := len(sanitized) - 1; j >= 0; j-- {
				if sanitized[j].Role == "assistant" {
					if len(sanitized[j].ToolCalls) > 0 {
						foundAssistant = true
					}
					break
				}
				if sanitized[j].Role != "tool" {
					break // Hit something else (user/system) before assistant
				}
			}

			if !foundAssistant {
				logger.WarnCF("agent", "Removing orphaned tool message from prompt", map[string]interface{}{"tool_call_id": m.ToolCallID})
				continue
			}
		}

		sanitized = append(sanitized, m)
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
