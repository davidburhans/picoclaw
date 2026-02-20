// PicoClaw - Ultra-lightweight personal AI agent
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package providers

import (
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/utils"
)

// CreateProvider creates a provider based on the configuration.
// It uses the model_list configuration to create providers.
// Returns the provider, the model ID to use, and any error.
func CreateProvider(cfg *config.Config) (LLMProvider, string, error) {
	model := cfg.Agents.Defaults.Model

	// 1. Try resolve from model_list first
	modelCfg, err := cfg.GetModelConfig(model)
	if err == nil {
		// Found in model_list
		if modelCfg.Workspace == "" {
			modelCfg.Workspace = cfg.WorkspacePath()
		}

		protocol, _ := ExtractProtocol(modelCfg.Model)
		if protocol == "overflow" {
			return NewOverflowProvider(cfg, modelCfg.Providers), model, nil
		}

		return CreateProviderFromConfig(modelCfg)
	}

	// 2. Not in model_list, try virtual providers
	providerType, providerName := config.ResolveProvider(model)

	switch providerType {
	case "schedule":
		schedConfig, ok := cfg.Schedules[providerName]
		if !ok {
			return nil, "", fmt.Errorf("schedule %q not found in schedules", providerName)
		}
		return NewScheduleProvider(cfg, &schedConfig, nil), model, nil

	case "overflow":
		list := strings.Split(providerName, ",")
		return NewOverflowProvider(cfg, list), model, nil

	case "claude-cli":
		workspace := cfg.Agents.Defaults.Workspace
		if workspace == "" {
			workspace = "."
		}
		return NewClaudeCliProvider(utils.ExpandHome(workspace)), "claude-code", nil

	case "codex-code":
		workspace := cfg.Agents.Defaults.Workspace
		if workspace == "" {
			workspace = "."
		}
		return NewCodexCliProvider(utils.ExpandHome(workspace)), "codex", nil
	}

	return nil, "", fmt.Errorf("model %q not found in model_list and not a recognized virtual provider", model)
}
