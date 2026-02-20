package providers

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestCreateProvider_MultiInstance(t *testing.T) {
	cfg := config.DefaultConfig()

	// Setup multiple ollama instances
	cfg.Providers.Ollama = config.ProviderEntries{
		"": {
			Model:   "llama3",
			APIBase: "http://localhost:11434/v1",
		},
		"gpt-oss": {
			Model:   "gpt-oss-120b",
			APIBase: "http://localhost:1234/v1",
		},
	}

	t.Run("resolve explicit instance", func(t *testing.T) {
		cfg.Agents.Defaults.Provider = "ollama/gpt-oss"
		cfg.Agents.Defaults.Model = "" // Should use model from config

		p, err := CreateProvider(cfg)
		if err != nil {
			t.Fatalf("CreateProvider failed: %v", err)
		}

		tempProvider := p
		if cw, ok := p.(*ConcurrencyWrapper); ok {
			tempProvider = cw.LLMProvider
		}
		hp, ok := tempProvider.(*HTTPProvider)
		if !ok {
			t.Fatalf("Expected HTTPProvider, got %T", p)
		}

		if hp.apiBase != "http://localhost:1234/v1" {
			t.Errorf("Expected apiBase http://localhost:1234/v1, got %s", hp.apiBase)
		}
	})

	t.Run("resolve default instance (explicit empty)", func(t *testing.T) {
		cfg.Agents.Defaults.Provider = "ollama"
		cfg.Agents.Defaults.Model = ""

		p, err := CreateProvider(cfg)
		if err != nil {
			t.Fatalf("CreateProvider failed: %v", err)
		}

		tempProvider := p
		if cw, ok := p.(*ConcurrencyWrapper); ok {
			tempProvider = cw.LLMProvider
		}
		hp, ok := tempProvider.(*HTTPProvider)
		if !ok {
			t.Fatalf("Expected HTTPProvider, got %T", p)
		}

		if hp.apiBase != "http://localhost:11434/v1" {
			t.Errorf("Expected apiBase http://localhost:11434/v1, got %s", hp.apiBase)
		}
	})

	t.Run("fallback by model name prefix", func(t *testing.T) {
		cfg.Agents.Defaults.Provider = ""
		cfg.Agents.Defaults.Model = "ollama/llama3"

		p, err := CreateProvider(cfg)
		if err != nil {
			t.Fatalf("CreateProvider failed: %v", err)
		}

		tempProvider := p
		if cw, ok := p.(*ConcurrencyWrapper); ok {
			tempProvider = cw.LLMProvider
		}
		hp, ok := tempProvider.(*HTTPProvider)
		if !ok {
			t.Fatalf("Expected HTTPProvider, got %T", p)
		}

		if hp.apiBase != "http://localhost:11434/v1" {
			t.Errorf("Expected apiBase http://localhost:11434/v1, got %s", hp.apiBase)
		}
	})
}

func TestCreateProvider_LegacyCompatibility(t *testing.T) {
	cfg := config.DefaultConfig()

	// Setup legacy single-provider config (unmarshalled into "")
	cfg.Providers.OpenAI = config.ProviderEntries{
		"": {
			APIKey: "sk-legacy-key",
			Model:  "gpt-4",
		},
	}

	t.Run("legacy provider string", func(t *testing.T) {
		cfg.Agents.Defaults.Provider = "openai"
		cfg.Agents.Defaults.Model = "gpt-4"

		p, err := CreateProvider(cfg)
		if err != nil {
			t.Fatalf("CreateProvider failed: %v", err)
		}

		tempProvider := p
		if cw, ok := p.(*ConcurrencyWrapper); ok {
			tempProvider = cw.LLMProvider
		}
		hp, ok := tempProvider.(*HTTPProvider)
		if !ok {
			t.Fatalf("Expected HTTPProvider, got %T", p)
		}

		if hp.apiKey != "sk-legacy-key" {
			t.Errorf("Expected apiKey sk-legacy-key, got %s", hp.apiKey)
		}
	})
}
