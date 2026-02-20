package providers

import (
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestCreateProvider_MultiInstance(t *testing.T) {
	cfg := config.DefaultConfig()

	// Setup multiple ollama instances via ModelList
	cfg.ModelList = []config.ModelConfig{
		{
			ModelName: "ollama",
			Model:     "ollama/llama3",
			APIBase:   "http://localhost:11434/v1",
		},
		{
			ModelName: "ollama/gpt-oss",
			Model:     "ollama/gpt-oss-120b",
			APIBase:   "http://localhost:1234/v1",
		},
	}

	t.Run("resolve explicit instance", func(t *testing.T) {
		cfg.Agents.Defaults.Model = "ollama/gpt-oss"

		p, _, err := CreateProvider(cfg)
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

	t.Run("resolve default instance (model name match)", func(t *testing.T) {
		cfg.Agents.Defaults.Model = "ollama"

		p, _, err := CreateProvider(cfg)
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
