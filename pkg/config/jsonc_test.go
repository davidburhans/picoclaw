package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig_JSONC(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	// JSONC content with comments and trailing commas
	content := `{
		// Line comment
		"agents": {
			"defaults": {
				"name": "test-agent", // Trailing comment
				"workspace": "~/.picoclaw/workspace"
			}
		},
		/* Block comment */
		"heartbeat": {
			"enabled": true,
		}
	}`

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Agents.Defaults.Name != "test-agent" {
		t.Errorf("Expected agent name test-agent, got %s", cfg.Agents.Defaults.Name)
	}
	if !cfg.Heartbeat.Enabled {
		t.Error("Expected heartbeat enabled")
	}
}
