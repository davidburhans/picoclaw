package providers

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestCreateProvider_Schedule(t *testing.T) {
	cfg := &config.Config{
		Providers: config.ProvidersConfig{
			Schedule: config.ScheduleEntries{
				"work": config.ScheduleConfig{
					Timezone: "UTC",
					Default: config.ScheduleDefault{
						Provider: "openai",
						Model:    "gpt-4",
					},
				},
				"personal": config.ScheduleConfig{
					Timezone: "UTC",
					Default: config.ScheduleDefault{
						Provider: "anthropic",
						Model:    "claude-3",
					},
				},
			},
		},
	}

	// Test case 1: schedule/work
	cfg.Agents.Defaults.Provider = "schedule/work"
	provider, err := CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider(schedule/work) failed: %v", err)
	}
	if _, ok := provider.(*ScheduleProvider); !ok {
		t.Errorf("Expected *ScheduleProvider, got %T", provider)
	}

	// Test case 2: schedule/personal
	cfg.Agents.Defaults.Provider = "schedule/personal"
	provider, err = CreateProvider(cfg)
	if err != nil {
		t.Fatalf("CreateProvider(schedule/personal) failed: %v", err)
	}
	if _, ok := provider.(*ScheduleProvider); !ok {
		t.Errorf("Expected *ScheduleProvider, got %T", provider)
	}

	// Test case 3: schedule/missing
	cfg.Agents.Defaults.Provider = "schedule/missing"
	_, err = CreateProvider(cfg)
	if err == nil {
		t.Error("Expected error for missing schedule instance, got nil")
	}
}

func TestScheduleProvider_RecursionCheck(t *testing.T) {
	// Setup a recursive schedule configuration
	// Note: We can't easily trigger the recursion check without mocking time match
	// But we can check ResolveProvider directly via CreateProvider logic if we could simulate internal call.
	// Instead, let's verify that creating a schedule provider with a recursive rule *inside* it works initially,
	// but fails during runtime resolution.

	cfg := &config.Config{
		Providers: config.ProvidersConfig{
			Schedule: config.ScheduleEntries{
				"recursive": config.ScheduleConfig{
					Default: config.ScheduleDefault{
						Provider: "schedule/recursive",
					},
				},
			},
		},
	}

	schedConfig := cfg.Providers.Schedule["recursive"]
	sched := NewScheduleProvider(cfg, &schedConfig, nil)

	// Try resolveProvider (private method, accessible in test if in same package)
	// But we are in providers_test package or providers package?
	// New file is package providers.

	t.Run("ResolveProvider_Recursion", func(t *testing.T) {
		// We expect resolveProvider to fail
		_, _, err := sched.resolveProvider(sched.nowFunc())
		if err == nil {
			t.Error("Expected recursion error, got nil")
		} else if err.Error() != "recursive schedule provider not allowed" {
			t.Errorf("Expected 'recursive schedule provider not allowed', got %v", err)
		}
	})
}
