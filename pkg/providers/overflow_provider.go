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
	wait, _ := options["wait"].(bool)

	for {
		var lastErr error
		subProviderIDs := make([]string, 0, len(p.providers))
		subProviderMaxes := make([]int, 0, len(p.providers))

		for _, providerStr := range p.providers {
			// Resolve provider
			providerType, _ := config.ResolveProvider(providerStr)

			// Check for recursion
			if strings.ToLower(providerType) == "overflow" {
				logger.WarnCF("overflow_provider", "Skipping recursive overflow provider in chain", map[string]interface{}{
					"provider": providerStr,
				})
				continue
			}

			cfgClone := *p.cfg // Shallow copy
			cfgClone.Agents.Defaults.Model = providerStr

			subProvider, _, err := CreateProvider(&cfgClone)
			if err != nil {
				logger.ErrorCF("overflow_provider", "Failed to create sub-provider", map[string]interface{}{
					"provider": providerStr,
					"error":    err,
				})
				lastErr = err
				continue
			}

			// Collect IDs for potential WaitAny
			subProviderIDs = append(subProviderIDs, subProvider.GetID())
			subProviderMaxes = append(subProviderMaxes, subProvider.GetMaxConcurrent())

			// Try to chat WITHOUT waiting first to explore the chain
			subOptions := make(map[string]interface{})
			for k, v := range options {
				subOptions[k] = v
			}
			subOptions["wait"] = false

			response, err := subProvider.Chat(ctx, messages, tools, model, subOptions)
			if err == nil {
				return response, nil
			}

			logger.DebugCF("overflow_provider", "Sub-provider chat failed", map[string]interface{}{
				"provider": providerStr,
				"error":    err,
			})

			// Check if error is concurrency related
			if errors.Is(err, ErrConcurrencyLimit) {
				logger.InfoCF("overflow_provider", "Provider at capacity, failing over", map[string]interface{}{
					"provider": providerStr,
				})
				lastErr = err
				continue
			}

			// If it's another error, we return it immediately
			return nil, err
		}

		// If we reach here, all providers were exhausted
		if wait && errors.Is(lastErr, ErrConcurrencyLimit) && len(subProviderIDs) > 0 {
			// Wait for ANY of the providers in the chain to become available
			logger.InfoCF("overflow_provider", "All providers busy, waiting for any slot in chain", map[string]interface{}{
				"chain": subProviderIDs,
			})
			onWait, _ := options["on_wait"].(func(WaiterInfo))
			targetID, err := GlobalConcurrencyTracker().WaitAny(ctx, subProviderIDs, subProviderMaxes, onWait)
			if err != nil {
				return nil, err
			}
			// Release the slot immediately so it can be re-acquired by the actual Chat call in the next iteration
			GlobalConcurrencyTracker().Release(targetID)
			continue
		}

		if lastErr != nil {
			return nil, fmt.Errorf("all overflow providers failed: %w", lastErr)
		}

		return nil, fmt.Errorf("no providers configured for overflow")
	}
}

func (p *OverflowProvider) GetID() string {
	return "overflow:" + strings.Join(p.providers, ",")
}

func (p *OverflowProvider) GetDefaultModel() string {
	// Try to get from first provider
	if len(p.providers) > 0 {
		// This is expensive to create just for checking model, but necessary without cache
		cfgClone := *p.cfg
		cfgClone.Agents.Defaults.Model = p.providers[0]
		if provider, _, err := CreateProvider(&cfgClone); err == nil {
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
