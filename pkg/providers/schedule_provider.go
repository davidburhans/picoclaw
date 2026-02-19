package providers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

type ScheduleProvider struct {
	cfg      *config.Config
	schedule *config.ScheduleConfig
	location *time.Location
	nowFunc  func() time.Time
}

func NewScheduleProvider(cfg *config.Config, schedule *config.ScheduleConfig, location *time.Location) *ScheduleProvider {
	if location == nil {
		location = time.Local
	}
	return &ScheduleProvider{
		cfg:      cfg,
		schedule: schedule,
		location: location,
		nowFunc:  time.Now,
	}
}

func (p *ScheduleProvider) matchRule(t time.Time) (*config.ScheduleRule, bool) {
	// Convert time to the configured timezone
	t = t.In(p.location)
	weekday := strings.ToLower(t.Weekday().String()[:3]) // mon, tue, etc.

	for _, rule := range p.schedule.Rules {
		// Check days match
		if len(rule.Days) > 0 {
			dayMatch := false
			for _, d := range rule.Days {
				d = strings.ToLower(d)
				if d == weekday {
					dayMatch = true
					break
				}
				if d == "weekday" && weekday != "sat" && weekday != "sun" {
					dayMatch = true
					break
				}
				if d == "weekend" && (weekday == "sat" || weekday == "sun") {
					dayMatch = true
					break
				}
			}
			if !dayMatch {
				continue
			}
		}

		// Check hours match
		if rule.Hours != nil {
			nowMins := t.Hour()*60 + t.Minute()

			start, err := time.Parse("15:04", rule.Hours.Start)
			if err != nil {
				logger.ErrorCF("schedule_provider", "Invalid start time", map[string]interface{}{"error": err, "time": rule.Hours.Start})
				continue
			}
			end, err := time.Parse("15:04", rule.Hours.End)
			if err != nil {
				logger.ErrorCF("schedule_provider", "Invalid end time", map[string]interface{}{"error": err, "time": rule.Hours.End})
				continue
			}

			startMins := start.Hour()*60 + start.Minute()
			endMins := end.Hour()*60 + end.Minute()

			if startMins <= endMins {
				// Same day span (e.g. 09:00 to 17:00)
				if nowMins < startMins || nowMins >= endMins {
					continue
				}
			} else {
				// Overnight span (e.g. 22:00 to 06:00)
				// Match if we are AFTER start OR BEFORE end
				if nowMins < startMins && nowMins >= endMins {
					continue
				}
			}
		}

		return &rule, true
	}

	return nil, false
}

func (p *ScheduleProvider) resolveProvider(t time.Time) (LLMProvider, string, error) {
	var providerType, model string

	rule, ok := p.matchRule(t)
	if ok {
		providerType = rule.Provider
		model = rule.Model
	} else {
		providerType = p.schedule.Default.Provider
		model = p.schedule.Default.Model
	}

	if strings.HasPrefix(providerType, "schedule") {
		return nil, "", fmt.Errorf("recursive schedule provider not allowed")
	}

	// Create a shallow clone of config data to avoid modifying the original.
	// We manually copy fields instead of struct assignment to avoid copying the sync.RWMutex.
	cfgClone := &config.Config{
		Agents:     p.cfg.Agents,
		Channels:   p.cfg.Channels,
		Workspaces: p.cfg.Workspaces,
		Providers:  p.cfg.Providers,
		Gateway:    p.cfg.Gateway,
		Tools:      p.cfg.Tools,
		MCP:        p.cfg.MCP,
		Heartbeat:  p.cfg.Heartbeat,
		Devices:    p.cfg.Devices,
	}
	cfgClone.Agents.Defaults.Provider = providerType

	// Create provider will look up the provider config.
	// If model is resolved from rule/default, we set it here.
	if model != "" {
		cfgClone.Agents.Defaults.Model = model
	} else {
		// If empty, we want CreateProvider to use the provider's default model
		// However, cfg.Agents.Defaults.Model overrides provider defaults in CreateProvider.
		// So we must ensure it's empty in our clone if we want fallback.
		cfgClone.Agents.Defaults.Model = ""
	}

	provider, err := CreateProvider(cfgClone)
	if err != nil {
		return nil, "", err
	}

	return provider, model, nil
}

func (p *ScheduleProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error) {
	provider, ruleModel, err := p.resolveProvider(p.nowFunc())
	if err != nil {
		return nil, err
	}

	targetModel := model
	if ruleModel != "" {
		targetModel = ruleModel
	}
	// If still empty, ask provider for default
	if targetModel == "" {
		targetModel = provider.GetDefaultModel()
	}

	return provider.Chat(ctx, messages, tools, targetModel, options)
}

func (p *ScheduleProvider) GetDefaultModel() string {
	provider, model, err := p.resolveProvider(p.nowFunc())
	if err != nil || provider == nil {
		return "schedule-error"
	}
	if model != "" {
		return model
	}
	return provider.GetDefaultModel()
}

func (p *ScheduleProvider) GetMaxTokens() int {
	provider, _, err := p.resolveProvider(p.nowFunc())
	if err != nil || provider == nil {
		return 0
	}
	return provider.GetMaxTokens()
}

func (p *ScheduleProvider) GetTemperature() float64 {
	provider, _, err := p.resolveProvider(p.nowFunc())
	if err != nil || provider == nil {
		return 0
	}
	return provider.GetTemperature()
}

func (p *ScheduleProvider) GetMaxToolIterations() int {
	provider, _, err := p.resolveProvider(p.nowFunc())
	if err != nil || provider == nil {
		return 0
	}
	return provider.GetMaxToolIterations()
}

func (p *ScheduleProvider) GetTimeout() int {
	provider, _, err := p.resolveProvider(p.nowFunc())
	if err != nil || provider == nil {
		return 0
	}
	return provider.GetTimeout()
}

func (p *ScheduleProvider) GetMaxConcurrent() int {
	provider, _, err := p.resolveProvider(p.nowFunc())
	if err != nil || provider == nil {
		return 1
	}
	return provider.GetMaxConcurrent()
}
