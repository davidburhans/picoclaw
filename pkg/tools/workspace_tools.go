package tools

import (
	"github.com/sipeed/picoclaw/pkg/config"
)

type ToolFilter struct {
	allowedTools    []string
	restrictedTools []string
}

func NewToolFilter(allowedTools, restrictedTools []string) *ToolFilter {
	return &ToolFilter{
		allowedTools:    allowedTools,
		restrictedTools: restrictedTools,
	}
}

func (f *ToolFilter) IsAllowed(toolName string) bool {
	if len(f.allowedTools) > 0 {
		for _, t := range f.allowedTools {
			if t == toolName {
				return !f.isRestricted(toolName)
			}
		}
		return false
	}
	return !f.isRestricted(toolName)
}

func (f *ToolFilter) isRestricted(toolName string) bool {
	for _, t := range f.restrictedTools {
		if t == toolName {
			return true
		}
	}
	return false
}

func GetWorkspaceToolConfig(cfg *config.Config, workspaceName string) (allowed, restricted []string) {
	workspace, exists := cfg.Workspaces[workspaceName]
	if !exists {
		return nil, nil
	}
	return workspace.AllowedTools, workspace.RestrictedTools
}
