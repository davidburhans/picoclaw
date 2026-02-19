package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

type OverflowProvider struct {
	cfg       *config.Config
	providers []string
}

func NewOverflowProvider(cfg *config.Config, providers []string) *OverflowProvider {
	return &OverflowProvider{
		cfg:       cfg,
		providers: providers,
	}
}

func (p *OverflowProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error) {
	var lastErr error

	for _, providerStr := range p.providers {
		// Resolve provider
		providerType, _ := config.ResolveProvider(providerStr)

		// Check for recursion (simple depth check or type check)
		if strings.ToLower(providerType) == "overflow" {
			// Prevent overflow provider calling another overflow provider directly for now to avoid loops
			logger.WarnCF("overflow_provider", "Skipping recursive overflow provider in chain", map[string]interface{}{
				"provider": providerStr,
			})
			continue
		}

		// Create the sub-provider
		// We need to construct a config that points to this provider.
		// However, CreateProvider relies on cfg.Agents.Defaults to know what to create if we don't pass specific info.
		// Actually, CreateProvider accepts a cfg object. We can modify a clone of cfg to target this provider.
		
		cfgClone := *p.cfg // Shallow copy
		// We explicitly want to create a specific provider instance.
		// CreateProvider uses cfg.Agents.Defaults.Provider if we don't have a direct way to instantiate by name.
		// Wait, CreateProvider looks at cfg.Agents.Defaults.Provider.
		// We should update the clone's default provider to be the current one in the list.
		cfgClone.Agents.Defaults.Provider = providerStr
		// We should also clear the model if we want the provider's default, OR pass the requested model?
		// The requested 'model' might be relevant for the sub-provider.
		// If 'model' is generic (e.g. "gpt-4"), we pass it down.
		// If 'model' was specific to the overflow provider (e.g. "overflow"), we might want to let the sub-provider decide.
		// BUT, strictly speaking, CreateProvider uses the config to instantiate.
		
		subProvider, err := CreateProvider(&cfgClone)
		if err != nil {
			logger.ErrorCF("overflow_provider", "Failed to create sub-provider", map[string]interface{}{
				"provider": providerStr,
				"error":    err,
			})
			lastErr = err
			continue
		}

		// Try to chat
		response, err := subProvider.Chat(ctx, messages, tools, model, options)
		if err == nil {
			return response, nil
		}

		// Check if error is concurrency related
		// We use errors.Is to check for the sentinel error
		if errors.Is(err, ErrConcurrencyLimit) {
			logger.InfoCF("overflow_provider", "Provider at capacity, failing over", map[string]interface{}{
				"provider": providerStr,
			})
			lastErr = err
			continue
		}

		// If it's another error, we return it immediately (no failover on logic/API errors)
		return nil, err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all overflow providers failed: %w", lastErr)
	}

	return nil, fmt.Errorf("no providers configured for overflow")
}

func (p *OverflowProvider) GetDefaultModel() string {
	// Try to get from first provider
	if len(p.providers) > 0 {
		// This is expensive to create just for checking model, but necessary without cache
		cfgClone := *p.cfg
		cfgClone.Agents.Defaults.Provider = p.providers[0]
		if provider, err := CreateProvider(&cfgClone); err == nil {
			return provider.GetDefaultModel()
		}
	}
	return "overflow-model"
}

func (p *OverflowProvider) GetMaxTokens() int {
	// Return max of all (or just a large number)
	return 128000
}

func (p *OverflowProvider) GetTemperature() float64 {
	return 0.7
}

func (p *OverflowProvider) GetMaxToolIterations() int {
	return 20
}

func (p *OverflowProvider) GetTimeout() int {
	return 300
}

func (p *OverflowProvider) GetMaxConcurrent() int {
	// We return a high number because the overflow provider itself doesn't limit concurrency,
	// it delegates that to sub-providers.
	return 1000
}
