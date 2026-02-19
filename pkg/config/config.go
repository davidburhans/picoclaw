package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/caarlos0/env/v11"
	"github.com/tidwall/jsonc"
)

// FlexibleStringSlice is a []string that also accepts JSON numbers,
// so allow_from can contain both "123" and 123.
type FlexibleStringSlice []string

func BoolPtr(v bool) *bool { return &v }
func IntPtr(v int) *int { return &v }
func FloatPtr(v float64) *float64 { return &v }

func (f *FlexibleStringSlice) UnmarshalJSON(data []byte) error {
	// Try []string first
	var ss []string
	if err := json.Unmarshal(data, &ss); err == nil {
		*f = ss
		return nil
	}

	// Try []interface{} to handle mixed types
	var raw []interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	result := make([]string, 0, len(raw))
	for _, v := range raw {
		switch val := v.(type) {
		case string:
			result = append(result, val)
		case float64:
			result = append(result, fmt.Sprintf("%.0f", val))
		default:
			result = append(result, fmt.Sprintf("%v", val))
		}
	}
	*f = result
	return nil
}

type Config struct {
	Agents     AgentsConfig               `json:"agents"`
	Channels   ChannelsConfig             `json:"channels"`
	Workspaces map[string]WorkspaceConfig `json:"workspaces"`
	Providers  ProvidersConfig            `json:"providers"`
	Gateway    GatewayConfig              `json:"gateway"`
	Tools      ToolsConfig                `json:"tools"`
	MCP        map[string]MCPServerConfig `json:"mcp"`
	Heartbeat  HeartbeatConfig            `json:"heartbeat"`
	Devices    DevicesConfig              `json:"devices"`
	mu         sync.RWMutex
}

type WorkspaceConfig struct {
	Path                string              `json:"path" env:"PICOCLAW_WORKSPACES_{{.Name}}_PATH"`
	Users               FlexibleStringSlice `json:"users" env:"PICOCLAW_WORKSPACES_{{.Name}}_USERS"`
	RestrictToWorkspace *bool               `json:"restrict_to_workspace"`
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
}

type AgentDefaults struct {
	Name                string         `json:"name" env:"PICOCLAW_AGENTS_DEFAULTS_NAME"`
	Workspace           string         `json:"workspace" env:"PICOCLAW_AGENTS_DEFAULTS_WORKSPACE"`
	RestrictToWorkspace *bool          `json:"restrict_to_workspace" env:"PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE"`
	Provider            string         `json:"provider" env:"PICOCLAW_AGENTS_DEFAULTS_PROVIDER"`
	Model               string         `json:"model" env:"PICOCLAW_AGENTS_DEFAULTS_MODEL"`
	MaxTokens           *int           `json:"max_tokens" env:"PICOCLAW_AGENTS_DEFAULTS_MAX_TOKENS"`
	Temperature         *float64       `json:"temperature" env:"PICOCLAW_AGENTS_DEFAULTS_TEMPERATURE"`
	MaxToolIterations   *int           `json:"max_tool_iterations" env:"PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS"`
	Timeout             *int           `json:"timeout" env:"PICOCLAW_AGENTS_DEFAULTS_TIMEOUT"` // seconds
	Subagent            SubagentConfig `json:"subagent"`
}

type SubagentConfig struct {
	MaxIterations *int     `json:"max_iterations" env:"PICOCLAW_SUBAGENT_MAX_ITERATIONS"`
	MaxTokens     *int     `json:"max_tokens" env:"PICOCLAW_SUBAGENT_MAX_TOKENS"`
	Temperature   *float64 `json:"temperature" env:"PICOCLAW_SUBAGENT_TEMPERATURE"`
	MaxDepth      *int     `json:"max_depth" env:"PICOCLAW_SUBAGENT_MAX_DEPTH"`
}

type ChannelsConfig struct {
	WhatsApp WhatsAppConfig `json:"whatsapp"`
	Telegram TelegramConfig `json:"telegram"`
	Feishu   FeishuConfig   `json:"feishu"`
	Discord  DiscordConfig  `json:"discord"`
	MaixCam  MaixCamConfig  `json:"maixcam"`
	QQ       QQConfig       `json:"qq"`
	DingTalk DingTalkConfig `json:"dingtalk"`
	Slack    SlackConfig    `json:"slack"`
	LINE     LINEConfig     `json:"line"`
	OneBot   OneBotConfig   `json:"onebot"`
}

type WhatsAppConfig struct {
	Enabled   bool                `json:"enabled" env:"PICOCLAW_CHANNELS_WHATSAPP_ENABLED"`
	BridgeURL string              `json:"bridge_url" env:"PICOCLAW_CHANNELS_WHATSAPP_BRIDGE_URL"`
	AllowFrom FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_WHATSAPP_ALLOW_FROM"`
}

type TelegramConfig struct {
	Enabled   bool                `json:"enabled" env:"PICOCLAW_CHANNELS_TELEGRAM_ENABLED"`
	Token     string              `json:"token" env:"PICOCLAW_CHANNELS_TELEGRAM_TOKEN"`
	Proxy     string              `json:"proxy" env:"PICOCLAW_CHANNELS_TELEGRAM_PROXY"`
	AllowFrom FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_TELEGRAM_ALLOW_FROM"`
}

type FeishuConfig struct {
	Enabled           bool                `json:"enabled" env:"PICOCLAW_CHANNELS_FEISHU_ENABLED"`
	AppID             string              `json:"app_id" env:"PICOCLAW_CHANNELS_FEISHU_APP_ID"`
	AppSecret         string              `json:"app_secret" env:"PICOCLAW_CHANNELS_FEISHU_APP_SECRET"`
	EncryptKey        string              `json:"encrypt_key" env:"PICOCLAW_CHANNELS_FEISHU_ENCRYPT_KEY"`
	VerificationToken string              `json:"verification_token" env:"PICOCLAW_CHANNELS_FEISHU_VERIFICATION_TOKEN"`
	AllowFrom         FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_FEISHU_ALLOW_FROM"`
}

type DiscordConfig struct {
	Enabled   bool                `json:"enabled" env:"PICOCLAW_CHANNELS_DISCORD_ENABLED"`
	Token     string              `json:"token" env:"PICOCLAW_CHANNELS_DISCORD_TOKEN"`
	AllowFrom FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_DISCORD_ALLOW_FROM"`
}

type MaixCamConfig struct {
	Enabled   bool                `json:"enabled" env:"PICOCLAW_CHANNELS_MAIXCAM_ENABLED"`
	Host      string              `json:"host" env:"PICOCLAW_CHANNELS_MAIXCAM_HOST"`
	Port      int                 `json:"port" env:"PICOCLAW_CHANNELS_MAIXCAM_PORT"`
	AllowFrom FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_MAIXCAM_ALLOW_FROM"`
}

type QQConfig struct {
	Enabled   bool                `json:"enabled" env:"PICOCLAW_CHANNELS_QQ_ENABLED"`
	AppID     string              `json:"app_id" env:"PICOCLAW_CHANNELS_QQ_APP_ID"`
	AppSecret string              `json:"app_secret" env:"PICOCLAW_CHANNELS_QQ_APP_SECRET"`
	AllowFrom FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_QQ_ALLOW_FROM"`
}

type DingTalkConfig struct {
	Enabled      bool                `json:"enabled" env:"PICOCLAW_CHANNELS_DINGTALK_ENABLED"`
	ClientID     string              `json:"client_id" env:"PICOCLAW_CHANNELS_DINGTALK_CLIENT_ID"`
	ClientSecret string              `json:"client_secret" env:"PICOCLAW_CHANNELS_DINGTALK_CLIENT_SECRET"`
	AllowFrom    FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_DINGTALK_ALLOW_FROM"`
}

type SlackConfig struct {
	Enabled   bool                `json:"enabled" env:"PICOCLAW_CHANNELS_SLACK_ENABLED"`
	BotToken  string              `json:"bot_token" env:"PICOCLAW_CHANNELS_SLACK_BOT_TOKEN"`
	AppToken  string              `json:"app_token" env:"PICOCLAW_CHANNELS_SLACK_APP_TOKEN"`
	AllowFrom FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_SLACK_ALLOW_FROM"`
}

type LINEConfig struct {
	Enabled            bool                `json:"enabled" env:"PICOCLAW_CHANNELS_LINE_ENABLED"`
	ChannelSecret      string              `json:"channel_secret" env:"PICOCLAW_CHANNELS_LINE_CHANNEL_SECRET"`
	ChannelAccessToken string              `json:"channel_access_token" env:"PICOCLAW_CHANNELS_LINE_CHANNEL_ACCESS_TOKEN"`
	WebhookHost        string              `json:"webhook_host" env:"PICOCLAW_CHANNELS_LINE_WEBHOOK_HOST"`
	WebhookPort        int                 `json:"webhook_port" env:"PICOCLAW_CHANNELS_LINE_WEBHOOK_PORT"`
	WebhookPath        string              `json:"webhook_path" env:"PICOCLAW_CHANNELS_LINE_WEBHOOK_PATH"`
	AllowFrom          FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_LINE_ALLOW_FROM"`
}

type OneBotConfig struct {
	Enabled            bool                `json:"enabled" env:"PICOCLAW_CHANNELS_ONEBOT_ENABLED"`
	WSUrl              string              `json:"ws_url" env:"PICOCLAW_CHANNELS_ONEBOT_WS_URL"`
	AccessToken        string              `json:"access_token" env:"PICOCLAW_CHANNELS_ONEBOT_ACCESS_TOKEN"`
	ReconnectInterval  int                 `json:"reconnect_interval" env:"PICOCLAW_CHANNELS_ONEBOT_RECONNECT_INTERVAL"`
	GroupTriggerPrefix []string            `json:"group_trigger_prefix" env:"PICOCLAW_CHANNELS_ONEBOT_GROUP_TRIGGER_PREFIX"`
	AllowFrom          FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_ONEBOT_ALLOW_FROM"`
}

type HeartbeatConfig struct {
	Enabled  bool `json:"enabled" env:"PICOCLAW_HEARTBEAT_ENABLED"`
	Interval int  `json:"interval" env:"PICOCLAW_HEARTBEAT_INTERVAL"` // minutes, min 5
}

type DevicesConfig struct {
	Enabled    bool `json:"enabled" env:"PICOCLAW_DEVICES_ENABLED"`
	MonitorUSB bool `json:"monitor_usb" env:"PICOCLAW_DEVICES_MONITOR_USB"`
}

type ProvidersConfig struct {
	Anthropic    ProviderEntries `json:"anthropic"`
	OpenAI       ProviderEntries `json:"openai"`
	OpenRouter   ProviderEntries `json:"openrouter"`
	Groq         ProviderEntries `json:"groq"`
	Zhipu        ProviderEntries `json:"zhipu"`
	VLLM         ProviderEntries `json:"vllm"`
	Gemini       ProviderEntries `json:"gemini"`
	Nvidia       ProviderEntries `json:"nvidia"`
	Ollama       ProviderEntries `json:"ollama"`
	Moonshot     ProviderEntries `json:"moonshot"`
	ShengSuanYun ProviderEntries `json:"shengsuanyun"`
	// Removed duplicate DeepSeek
	DeepSeek      ProviderEntries `json:"deepseek"`
	GitHubCopilot ProviderEntries `json:"github_copilot"`
	Schedule      ScheduleEntries `json:"schedule,omitempty"`
}

type ScheduleEntries map[string]ScheduleConfig

func (s *ScheduleEntries) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a map (new format)
	var m map[string]ScheduleConfig
	if err := json.Unmarshal(data, &m); err == nil {
		// Heuristic: if it's a map, check if keys are named instances or ScheduleConfig fields.
		// If the map contains known ScheduleConfig fields as keys, it's likely the old format.
		isOldFormat := false
		for k := range m {
			switch strings.ToLower(k) {
			case "timezone", "rules", "default":
				isOldFormat = true
			}
			if isOldFormat {
				break
			}
		}

		if !isOldFormat && len(m) > 0 {
			*s = m
			return nil
		}
	}

	// Try to unmarshal as a single config (old format)
	var single ScheduleConfig
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*s = ScheduleEntries{"": single}
	return nil
}

func (s ScheduleEntries) MarshalJSON() ([]byte, error) {
	// If it only contains the default entry, marshal as a single object
	if len(s) == 1 {
		if single, ok := s[""]; ok {
			return json.Marshal(single)
		}
	}
	// Otherwise marshal as a map
	return json.Marshal(map[string]ScheduleConfig(s))
}

type ProviderEntries map[string]ProviderConfig

func (p *ProviderEntries) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a map (new format)
	var m map[string]ProviderConfig
	if err := json.Unmarshal(data, &m); err == nil {
		// Heuristic: if it's a map, check if keys are named instances or ProviderConfig fields.
		// If the map contains known ProviderConfig fields as keys, it's likely the old format.
		isOldFormat := false
		for k := range m {
			switch strings.ToLower(k) {
			case "model", "api_key", "api_base", "proxy", "auth_method", "connect_mode", "max_tokens", "temperature", "max_tool_iterations", "timeout":
				isOldFormat = true
			}
			if isOldFormat {
				break
			}
		}

		if !isOldFormat && len(m) > 0 {
			*p = m
			return nil
		}
	}

	// Try to unmarshal as a single config (old format)
	var single ProviderConfig
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*p = ProviderEntries{"": single}
	return nil
}

func (p ProviderEntries) MarshalJSON() ([]byte, error) {
	// If it only contains the default entry, marshal as a single object
	if len(p) == 1 {
		if single, ok := p[""]; ok {
			return json.Marshal(single)
		}
	}
	// Otherwise marshal as a map
	return json.Marshal(map[string]ProviderConfig(p))
}

type ProviderConfig struct {
	Model             string   `json:"model,omitempty" env:"PICOCLAW_PROVIDERS_{{.Name}}_MODEL"`
	APIKey            string   `json:"api_key" env:"PICOCLAW_PROVIDERS_{{.Name}}_API_KEY"`
	APIBase           string   `json:"api_base" env:"PICOCLAW_PROVIDERS_{{.Name}}_API_BASE"`
	Proxy             string   `json:"proxy,omitempty" env:"PICOCLAW_PROVIDERS_{{.Name}}_PROXY"`
	AuthMethod        string   `json:"auth_method,omitempty" env:"PICOCLAW_PROVIDERS_{{.Name}}_AUTH_METHOD"`
	ConnectMode       string   `json:"connect_mode,omitempty" env:"PICOCLAW_PROVIDERS_{{.Name}}_CONNECT_MODE"` //only for Github Copilot, `stdio` or `grpc`
	MaxTokens         *int     `json:"max_tokens,omitempty"`
	Temperature       *float64 `json:"temperature,omitempty"`
	MaxToolIterations *int     `json:"max_tool_iterations,omitempty"`
	Timeout           *int     `json:"timeout,omitempty"`
	MaxConcurrentSessions int `json:"max_concurrent_sessions,omitempty"`
}

type ScheduleConfig struct {
	Timezone string          `json:"timezone,omitempty"` // IANA timezone, e.g. "America/Chicago"
	Rules    []ScheduleRule  `json:"rules"`
	Default  ScheduleDefault `json:"default"`
}

type ScheduleRule struct {
	Days     []string       `json:"days,omitempty"`  // e.g. ["mon","tue"]
	Hours    *ScheduleHours `json:"hours,omitempty"` // optional time window
	Provider string         `json:"provider"`        // provider to use
	Model    string         `json:"model,omitempty"` // optional model override
}

type ScheduleHours struct {
	Start string `json:"start"` // "HH:MM" 24h format
	End   string `json:"end"`   // "HH:MM" 24h format
}

type ScheduleDefault struct {
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
}

type GatewayConfig struct {
	Host string `json:"host" env:"PICOCLAW_GATEWAY_HOST"`
	Port int    `json:"port" env:"PICOCLAW_GATEWAY_PORT"`
}

// ResolveProvider splits a provider string like "ollama/llama" into ("ollama", "llama")
func ResolveProvider(providerStr string) (string, string) {
	if idx := strings.Index(providerStr, "/"); idx != -1 {
		return providerStr[:idx], providerStr[idx+1:]
	}
	if idx := strings.Index(providerStr, "."); idx != -1 {
		return providerStr[:idx], providerStr[idx+1:]
	}
	return providerStr, ""
}

// Get returns the ProviderConfig for the given provider type and instance name.
func (p *ProvidersConfig) Get(providerType, instanceName string) (ProviderConfig, bool) {
	var entries ProviderEntries
	switch strings.ToLower(providerType) {
	case "anthropic", "claude":
		entries = p.Anthropic
	case "openai", "gpt":
		entries = p.OpenAI
	case "openrouter":
		entries = p.OpenRouter
	case "groq":
		entries = p.Groq
	case "zhipu", "glm":
		entries = p.Zhipu
	case "vllm":
		entries = p.VLLM
	case "gemini", "google":
		entries = p.Gemini
	case "nvidia":
		entries = p.Nvidia
	case "ollama":
		entries = p.Ollama
	case "moonshot":
		entries = p.Moonshot
	case "shengsuanyun":
		entries = p.ShengSuanYun
	case "deepseek":
		entries = p.DeepSeek
	case "github_copilot", "copilot":
		entries = p.GitHubCopilot
	default:
		return ProviderConfig{}, false
	}

	cfg, ok := entries[instanceName]
	if !ok && instanceName == "" && len(entries) > 0 {
		// Fallback: if no instance name provided, use the first available one if "" is not set
		for _, v := range entries {
			return v, true
		}
	}
	return cfg, ok
}

type BraveConfig struct {
	Enabled    bool   `json:"enabled" env:"PICOCLAW_TOOLS_WEB_BRAVE_ENABLED"`
	APIKey     string `json:"api_key" env:"PICOCLAW_TOOLS_WEB_BRAVE_API_KEY"`
	MaxResults int    `json:"max_results" env:"PICOCLAW_TOOLS_WEB_BRAVE_MAX_RESULTS"`
}

type DuckDuckGoConfig struct {
	Enabled    bool `json:"enabled" env:"PICOCLAW_TOOLS_WEB_DUCKDUCKGO_ENABLED"`
	MaxResults int  `json:"max_results" env:"PICOCLAW_TOOLS_WEB_DUCKDUCKGO_MAX_RESULTS"`
}

type SearXNGConfig struct {
	Enabled    bool   `json:"enabled" env:"PICOCLAW_TOOLS_WEB_SEARXNG_ENABLED"`
	BaseURL    string `json:"base_url" env:"PICOCLAW_TOOLS_WEB_SEARXNG_BASE_URL"`
	MaxResults int    `json:"max_results" env:"PICOCLAW_TOOLS_WEB_SEARXNG_MAX_RESULTS"`
}

type PerplexityConfig struct {
	Enabled    bool   `json:"enabled" env:"PICOCLAW_TOOLS_WEB_PERPLEXITY_ENABLED"`
	APIKey     string `json:"api_key" env:"PICOCLAW_TOOLS_WEB_PERPLEXITY_API_KEY"`
	MaxResults int    `json:"max_results" env:"PICOCLAW_TOOLS_WEB_PERPLEXITY_MAX_RESULTS"`
}

type WebToolsConfig struct {
	Brave      BraveConfig      `json:"brave"`
	DuckDuckGo DuckDuckGoConfig `json:"duckduckgo"`
	SearXNG    SearXNGConfig    `json:"searxng"`
	Perplexity PerplexityConfig `json:"perplexity"`
}

type CronToolsConfig struct {
	ExecTimeoutMinutes int `json:"exec_timeout_minutes" env:"PICOCLAW_TOOLS_CRON_EXEC_TIMEOUT_MINUTES"` // 0 means no timeout
}

type ToolsConfig struct {
	Web  WebToolsConfig  `json:"web"`
	Cron CronToolsConfig `json:"cron"`
}

type MCPServerConfig struct {
	Enabled   bool              `json:"enabled"`
	Command   string            `json:"command,omitempty"` // stdio transport
	Args      []string          `json:"args,omitempty"`
	Cwd       string            `json:"cwd,omitempty"` // working directory
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`       // SSE transport
	Transport string            `json:"transport,omitempty"` // "sse" (default for URL)
}

func DefaultConfig() *Config {
	return &Config{
		Workspaces: make(map[string]WorkspaceConfig),
		Agents: AgentsConfig{
			Defaults: AgentDefaults{
				Name:                "picoclaw",
				Workspace:           "~/.picoclaw/workspace",
				RestrictToWorkspace: BoolPtr(true),
				Provider:            "",
				Model:               "glm-4.7",
				MaxTokens:           IntPtr(0),
				Temperature:         FloatPtr(0),
				MaxToolIterations:   IntPtr(0),
				Timeout:             IntPtr(0), // default 2 minutes

				Subagent: SubagentConfig{
					MaxIterations: IntPtr(10),
					MaxTokens:     IntPtr(8192),
					Temperature:   FloatPtr(0.7),
					MaxDepth:      IntPtr(5),
				},
			},
		},
		Channels: ChannelsConfig{
			WhatsApp: WhatsAppConfig{
				Enabled:   false,
				BridgeURL: "ws://localhost:3001",
				AllowFrom: FlexibleStringSlice{},
			},
			Telegram: TelegramConfig{
				Enabled:   false,
				Token:     "",
				AllowFrom: FlexibleStringSlice{},
			},
			Feishu: FeishuConfig{
				Enabled:           false,
				AppID:             "",
				AppSecret:         "",
				EncryptKey:        "",
				VerificationToken: "",
				AllowFrom:         FlexibleStringSlice{},
			},
			Discord: DiscordConfig{
				Enabled:   false,
				Token:     "",
				AllowFrom: FlexibleStringSlice{},
			},
			MaixCam: MaixCamConfig{
				Enabled:   false,
				Host:      "0.0.0.0",
				Port:      18790,
				AllowFrom: FlexibleStringSlice{},
			},
			QQ: QQConfig{
				Enabled:   false,
				AppID:     "",
				AppSecret: "",
				AllowFrom: FlexibleStringSlice{},
			},
			DingTalk: DingTalkConfig{
				Enabled:      false,
				ClientID:     "",
				ClientSecret: "",
				AllowFrom:    FlexibleStringSlice{},
			},
			Slack: SlackConfig{
				Enabled:   false,
				BotToken:  "",
				AppToken:  "",
				AllowFrom: FlexibleStringSlice{},
			},
			LINE: LINEConfig{
				Enabled:            false,
				ChannelSecret:      "",
				ChannelAccessToken: "",
				WebhookHost:        "0.0.0.0",
				WebhookPort:        18791,
				WebhookPath:        "/webhook/line",
				AllowFrom:          FlexibleStringSlice{},
			},
			OneBot: OneBotConfig{
				Enabled:            false,
				WSUrl:              "ws://127.0.0.1:3001",
				AccessToken:        "",
				ReconnectInterval:  5,
				GroupTriggerPrefix: []string{},
				AllowFrom:          FlexibleStringSlice{},
			},
		},
		Providers: ProvidersConfig{
			Anthropic:    ProviderEntries{"": {}},
			OpenAI:       ProviderEntries{"": {}},
			OpenRouter:   ProviderEntries{"": {}},
			Groq:         ProviderEntries{"": {}},
			Zhipu:        ProviderEntries{"": {}},
			VLLM:         ProviderEntries{"": {}},
			Gemini:       ProviderEntries{"": {}},
			Nvidia:       ProviderEntries{"": {}},
			Moonshot:     ProviderEntries{"": {}},
			ShengSuanYun: ProviderEntries{"": {}},
		},
		Gateway: GatewayConfig{
			Host: "0.0.0.0",
			Port: 18790,
		},
		Tools: ToolsConfig{
			Web: WebToolsConfig{
				Brave: BraveConfig{
					Enabled:    false,
					APIKey:     "",
					MaxResults: 5,
				},
				DuckDuckGo: DuckDuckGoConfig{
					Enabled:    true,
					MaxResults: 5,
				},
				SearXNG: SearXNGConfig{
					Enabled:    false,
					BaseURL:    "http://sherwood.local:8080/",
					MaxResults: 5,
				},
				Perplexity: PerplexityConfig{
					Enabled:    false,
					APIKey:     "",
					MaxResults: 5,
				},
			},
			Cron: CronToolsConfig{
				ExecTimeoutMinutes: 5, // default 5 minutes for LLM operations
			},
		},
		Heartbeat: HeartbeatConfig{
			Enabled:  true,
			Interval: 30, // default 30 minutes
		},
		Devices: DevicesConfig{
			Enabled:    false,
			MonitorUSB: true,
		},
	}
}

func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	// Strip comments and trailing commas so we can use JSONC
	data = jsonc.ToJSON(data)

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	cfg.mu.RLock()
	defer cfg.mu.RUnlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func (c *Config) WorkspacePath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return ExpandHome(c.Agents.Defaults.Workspace)
}

// ResolveWorkspace returns the workspace path for a sender ID, or the default path.
func (c *Config) ResolveWorkspace(senderID string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if senderID != "" {
		for _, ws := range c.Workspaces {
			for _, user := range ws.Users {
				if user == senderID {
					return ExpandHome(ws.Path)
				}
			}
		}
	}

	return ExpandHome(c.Agents.Defaults.Workspace)
}

// ResolveRestrictToWorkspace returns the restrict_to_workspace setting for a sender ID, or the global default.
func (c *Config) ResolveRestrictToWorkspace(senderID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if senderID != "" {
		for _, ws := range c.Workspaces {
			for _, user := range ws.Users {
				if user == senderID {
					if ws.RestrictToWorkspace != nil {
						return *ws.RestrictToWorkspace
					}
					break
				}
			}
		}
	}

	if c.Agents.Defaults.RestrictToWorkspace != nil {
		return *c.Agents.Defaults.RestrictToWorkspace
	}

	return true
}

func (c *Config) GetAPIKey() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Direct access changed to helper
	check := func(e ProviderEntries) string {
		for _, v := range e {
			if v.APIKey != "" {
				return v.APIKey
			}
		}
		return ""
	}

	if key := check(c.Providers.OpenRouter); key != "" {
		return key
	}
	if key := check(c.Providers.Anthropic); key != "" {
		return key
	}
	if key := check(c.Providers.OpenAI); key != "" {
		return key
	}
	if key := check(c.Providers.Gemini); key != "" {
		return key
	}
	if key := check(c.Providers.Zhipu); key != "" {
		return key
	}
	if key := check(c.Providers.Groq); key != "" {
		return key
	}
	if key := check(c.Providers.VLLM); key != "" {
		return key
	}
	if key := check(c.Providers.ShengSuanYun); key != "" {
		return key
	}
	return ""
}

func (c *Config) GetAPIBase() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check OpenRouter
	for _, v := range c.Providers.OpenRouter {
		if v.APIKey != "" {
			if v.APIBase != "" {
				return v.APIBase
			}
			return "https://openrouter.ai/api/v1"
		}
	}
	// Check Zhipu
	for _, v := range c.Providers.Zhipu {
		if v.APIKey != "" {
			return v.APIBase
		}
	}
	// Check VLLM
	for _, v := range c.Providers.VLLM {
		if v.APIKey != "" && v.APIBase != "" {
			return v.APIBase
		}
	}
	return ""
}

func ExpandHome(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		// BUG-10 FIX: Handle both forward and backward slashes (Windows)
		if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
			return filepath.Join(home, path[2:])
		}
		return home
	}
	return path
}
