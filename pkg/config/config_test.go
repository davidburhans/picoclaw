package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestDefaultConfig_HeartbeatEnabled verifies heartbeat is enabled by default
func TestDefaultConfig_HeartbeatEnabled(t *testing.T) {
	cfg := DefaultConfig()

	if !cfg.Heartbeat.Enabled {
		t.Error("Heartbeat should be enabled by default")
	}
}

// TestDefaultConfig_WorkspacePath verifies workspace path is correctly set
func TestDefaultConfig_WorkspacePath(t *testing.T) {
	cfg := DefaultConfig()

	// Just verify the workspace is set, don't compare exact paths
	// since expandHome behavior may differ based on environment
	if cfg.Agents.Defaults.Workspace == "" {
		t.Error("Workspace should not be empty")
	}
}

// TestDefaultConfig_Model verifies model is set
func TestDefaultConfig_Model(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Agents.Defaults.Model == "" {
		t.Error("Model should not be empty")
	}
}

// TestDefaultConfig_MaxTokens verifies max tokens (0 means provider default)
func TestDefaultConfig_MaxTokens(t *testing.T) {
	cfg := DefaultConfig()
	if *cfg.Agents.Defaults.MaxTokens != 0 {
		t.Errorf("Expected MaxTokens 0, got %d", *cfg.Agents.Defaults.MaxTokens)
	}
}

// TestDefaultConfig_MaxToolIterations verifies max tool iterations (0 means provider default)
func TestDefaultConfig_MaxToolIterations(t *testing.T) {
	cfg := DefaultConfig()
	if *cfg.Agents.Defaults.MaxToolIterations != 0 {
		t.Errorf("Expected MaxToolIterations 0, got %d", *cfg.Agents.Defaults.MaxToolIterations)
	}
}

// TestDefaultConfig_Temperature verifies temperature (0 means provider default)
func TestDefaultConfig_Temperature(t *testing.T) {
	cfg := DefaultConfig()
	if *cfg.Agents.Defaults.Temperature != 0 {
		t.Errorf("Expected Temperature 0, got %f", *cfg.Agents.Defaults.Temperature)
	}
}

// TestDefaultConfig_Gateway verifies gateway defaults
func TestDefaultConfig_Gateway(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Gateway.Host != "0.0.0.0" {
		t.Error("Gateway host should have default value")
	}
	if cfg.Gateway.Port == 0 {
		t.Error("Gateway port should have default value")
	}
}

// TestDefaultConfig_Providers verifies provider structure
func TestDefaultConfig_Providers(t *testing.T) {
	cfg := DefaultConfig()

	// Verify all providers have a default empty entry
	check := func(name string, e ProviderEntries) {
		if len(e) != 1 {
			t.Errorf("%s should have exactly 1 entry by default, got %d", name, len(e))
		}
		if _, ok := e[""]; !ok {
			t.Errorf("%s should have a default entry with empty key", name)
		}
	}

	check("Anthropic", cfg.Providers.Anthropic)
	check("OpenAI", cfg.Providers.OpenAI)
	check("OpenRouter", cfg.Providers.OpenRouter)
	check("Groq", cfg.Providers.Groq)
	check("Zhipu", cfg.Providers.Zhipu)
	check("VLLM", cfg.Providers.VLLM)
	check("Gemini", cfg.Providers.Gemini)
}

// TestDefaultConfig_Channels verifies channels are disabled by default
func TestDefaultConfig_Channels(t *testing.T) {
	cfg := DefaultConfig()

	// Verify all channels are disabled by default
	if cfg.Channels.WhatsApp.Enabled {
		t.Error("WhatsApp should be disabled by default")
	}
	if cfg.Channels.Telegram.Enabled {
		t.Error("Telegram should be disabled by default")
	}
	if cfg.Channels.Feishu.Enabled {
		t.Error("Feishu should be disabled by default")
	}
	if cfg.Channels.Discord.Enabled {
		t.Error("Discord should be disabled by default")
	}
	if cfg.Channels.MaixCam.Enabled {
		t.Error("MaixCam should be disabled by default")
	}
	if cfg.Channels.QQ.Enabled {
		t.Error("QQ should be disabled by default")
	}
	if cfg.Channels.DingTalk.Enabled {
		t.Error("DingTalk should be disabled by default")
	}
	if cfg.Channels.Slack.Enabled {
		t.Error("Slack should be disabled by default")
	}
}

// TestDefaultConfig_WebTools verifies web tools config
func TestDefaultConfig_WebTools(t *testing.T) {
	cfg := DefaultConfig()

	// Verify web tools defaults
	if cfg.Tools.Web.Brave.MaxResults != 5 {
		t.Error("Expected Brave MaxResults 5, got ", cfg.Tools.Web.Brave.MaxResults)
	}
	if cfg.Tools.Web.Brave.APIKey != "" {
		t.Error("Brave API key should be empty by default")
	}
	if cfg.Tools.Web.DuckDuckGo.MaxResults != 5 {
		t.Error("Expected DuckDuckGo MaxResults 5, got ", cfg.Tools.Web.DuckDuckGo.MaxResults)
	}
}

func TestSaveConfig_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file permission bits are not enforced on Windows")
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	if err := SaveConfig(path, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("config file has permission %04o, want 0600", perm)
	}
}

// TestConfig_Complete verifies all config fields are set
func TestConfig_Complete(t *testing.T) {
	cfg := DefaultConfig()

	// Verify complete config structure
	if cfg.Agents.Defaults.Workspace == "" {
		t.Error("Workspace should not be empty")
	}
	if cfg.Agents.Defaults.Model == "" {
		t.Error("Model should not be empty")
	}
	if *cfg.Agents.Defaults.Temperature < 0 {
		t.Error("Temperature should not be negative")
	}
	if *cfg.Agents.Defaults.MaxTokens < 0 {
		t.Error("MaxTokens should not be negative")
	}
	if *cfg.Agents.Defaults.MaxToolIterations < 0 {
		t.Error("MaxToolIterations should not be negative")
	}
	if cfg.Gateway.Host != "0.0.0.0" {
		t.Error("Gateway host should have default value")
	}
	if cfg.Gateway.Port == 0 {
		t.Error("Gateway port should have default value")
	}
	if !cfg.Heartbeat.Enabled {
		t.Error("Heartbeat should be enabled by default")
	}
}

func TestResolveProvider(t *testing.T) {
	tests := []struct {
		input    string
		wantType string
		wantInst string
	}{
		{"ollama", "ollama", ""},
		{"ollama/llama", "ollama", "llama"},
		{"ollama/gpt-oss", "ollama", "gpt-oss"},
		{"openai", "openai", ""},
		{"openai.test", "openai", "test"},
		{"", "", ""},
	}

	for _, tt := range tests {
		gotType, gotInst := ResolveProvider(tt.input)
		if gotType != tt.wantType || gotInst != tt.wantInst {
			t.Errorf("ResolveProvider(%q) = (%q, %q), want (%q, %q)", tt.input, gotType, gotInst, tt.wantType, tt.wantInst)
		}
	}
}

func TestProviderEntries_Unmarshal(t *testing.T) {
	t.Run("SingleObject", func(t *testing.T) {
		data := []byte(`{"api_key": "test-key", "model": "test-model"}`)
		var p ProviderEntries
		if err := p.UnmarshalJSON(data); err != nil {
			t.Fatalf("UnmarshalJSON failed: %v", err)
		}
		if len(p) != 1 || p[""].APIKey != "test-key" || p[""].Model != "test-model" {
			t.Errorf("Unexpected entries: %+v", p)
		}
	})

	t.Run("MapObject", func(t *testing.T) {
		data := []byte(`{
			"inst1": {"api_key": "key1", "model": "mod1"},
			"inst2": {"api_key": "key2", "model": "mod2"}
		}`)
		var p ProviderEntries
		if err := p.UnmarshalJSON(data); err != nil {
			t.Fatalf("UnmarshalJSON failed: %v", err)
		}
		if len(p) != 2 || p["inst1"].APIKey != "key1" || p["inst2"].APIKey != "key2" {
			t.Errorf("Unexpected entries: %+v", p)
		}
	})

	t.Run("MapWithPotentialConflicts", func(t *testing.T) {
		// If it's a map but doesn't look like a single config, treat as map
		data := []byte(`{"llama": {"model": "llama3"}}`)
		var p ProviderEntries
		if err := p.UnmarshalJSON(data); err != nil {
			t.Fatalf("UnmarshalJSON failed: %v", err)
		}
		if len(p) != 1 || p["llama"].Model != "llama3" {
			t.Errorf("Unexpected entries: %+v", p)
		}
	})
}

func TestResolveWorkspace(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agents.Defaults.Workspace = "~/default"
	cfg.Workspaces = map[string]WorkspaceConfig{
		"dave": {
			Path:  "~/dave",
			Users: []string{"user1", "user2"},
		},
		"wife": {
			Path:  "~/wife",
			Users: []string{"user3"},
		},
	}

	tests := []struct {
		senderID string
		want     string
	}{
		{"user1", ExpandHome("~/dave")},
		{"user2", ExpandHome("~/dave")},
		{"user3", ExpandHome("~/wife")},
		{"unknown", ExpandHome("~/default")},
		{"", ExpandHome("~/default")},
	}

	for _, tt := range tests {
		got := cfg.ResolveWorkspace(tt.senderID)
		if got != tt.want {
			t.Errorf("ResolveWorkspace(%q) = %q, want %q", tt.senderID, got, tt.want)
		}
	}
}

func TestWorkspaceConfig_Unmarshal(t *testing.T) {
	data := []byte(`{
		"agents": {
			"defaults": {
				"workspace": "~/default"
			}
		},
		"workspaces": {
			"dave": {
				"path": "~/dave",
				"users": ["user1", 123]
			}
		}
	}`)

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(cfg.Workspaces) != 1 {
		t.Fatalf("Expected 1 workspace, got %d", len(cfg.Workspaces))
	}

	ws, ok := cfg.Workspaces["dave"]
	if !ok {
		t.Fatal("Workspace 'dave' not found")
	}

	if ws.Path != "~/dave" {
		t.Errorf("Expected path ~/dave, got %s", ws.Path)
	}

	if len(ws.Users) != 2 {
		t.Fatalf("Expected 2 users, got %d", len(ws.Users))
	}

	if ws.Users[0] != "user1" || ws.Users[1] != "123" {
		t.Errorf("Unexpected users: %v", ws.Users)
	}
}

func TestResolveRestrictToWorkspace(t *testing.T) {
	cfg := DefaultConfig()
	// Global default
	cfg.Agents.Defaults.RestrictToWorkspace = BoolPtr(false)

	trueVal := true
	cfg.Workspaces = map[string]WorkspaceConfig{
		"restricted": {
			Path:                "~/restricted",
			Users:               []string{"user_restricted"},
			RestrictToWorkspace: &trueVal,
		},
		"unrestricted": {
			Path:  "~/unrestricted",
			Users: []string{"user_unrestricted"},
			// Nil RestrictToWorkspace -> fallback to global
		},
	}

	tests := []struct {
		senderID string
		want     bool
	}{
		{"user_restricted", true},
		{"user_unrestricted", false},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		got := cfg.ResolveRestrictToWorkspace(tt.senderID)
		if got != tt.want {
			t.Errorf("ResolveRestrictToWorkspace(%q) = %v, want %v", tt.senderID, got, tt.want)
		}
	}
}

func TestExpandHome_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on non-Windows OS")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~\\test\\path", filepath.Join(home, "test", "path")},
		{"~\\file.txt", filepath.Join(home, "file.txt")},
	}

	for _, tt := range tests {
		got := ExpandHome(tt.input)
		if got != tt.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
