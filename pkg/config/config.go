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
	Bindings   []AgentBinding             `json:"bindings,omitempty"` // From Incoming
	Session    SessionConfig              `json:"session,omitempty"`  // From Incoming
	Channels   ChannelsConfig             `json:"channels"`
	Workspaces map[string]WorkspaceConfig `json:"workspaces"` // From HEAD
	Providers  ProvidersConfig            `json:"providers"`
	Gateway    GatewayConfig              `json:"gateway"`
	Tools      ToolsConfig                `json:"tools"`
	MCP        map[string]MCPServerConfig `json:"mcp"` // From HEAD
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
	List     []AgentConfig `json:"list,omitempty"`
}

// AgentModelConfig supports both string and structured model config.
// String format: "gpt-4" (just primary, no fallbacks)
// Object format: {"primary": "gpt-4", "fallbacks": ["claude-haiku"]}
type AgentModelConfig struct {
	Primary   string   `json:"primary,omitempty"`
	Fallbacks []string `json:"fallbacks,omitempty"`
}

func (m *AgentModelConfig) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		m.Primary = s
		m.Fallbacks = nil
		return nil
	}
	type raw struct {
		Primary   string   `json:"primary"`
		Fallbacks []string `json:"fallbacks"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	m.Primary = r.Primary
	m.Fallbacks = r.Fallbacks
	return nil
}

func (m AgentModelConfig) MarshalJSON() ([]byte, error) {
	if len(m.Fallbacks) == 0 && m.Primary != "" {
		return json.Marshal(m.Primary)
	}
	type raw struct {
		Primary   string   `json:"primary,omitempty"`
		Fallbacks []string `json:"fallbacks,omitempty"`
	}
	return json.Marshal(raw{Primary: m.Primary, Fallbacks: m.Fallbacks})
}

type AgentConfig struct {
	ID        string            `json:"id"`
	Default   bool              `json:"default,omitempty"`
	Name      string            `json:"name,omitempty"`
	Workspace string            `json:"workspace,omitempty"`
	Model     *AgentModelConfig `json:"model,omitempty"`
	Skills    []string          `json:"skills,omitempty"`
	Subagents *SubagentsConfig  `json:"subagents,omitempty"`
}

type SubagentsConfig struct {
	AllowAgents []string          `json:"allow_agents,omitempty"`
	Model       *AgentModelConfig `json:"model,omitempty"`
}

type PeerMatch struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type BindingMatch struct {
	Channel   string     `json:"channel"`
	AccountID string     `json:"account_id,omitempty"`
	Peer      *PeerMatch `json:"peer,omitempty"`
	GuildID   string     `json:"guild_id,omitempty"`
	TeamID    string     `json:"team_id,omitempty"`
}

type AgentBinding struct {
	AgentID string       `json:"agent_id"`
	Match   BindingMatch `json:"match"`
}

type SessionConfig struct {
	DMScope       string              `json:"dm_scope,omitempty"`
	IdentityLinks map[string][]string `json:"identity_links,omitempty"`
}

type AgentDefaults struct {
	Name                string         `json:"name" env:"PICOCLAW_AGENTS_DEFAULTS_NAME"`
	Workspace           string         `json:"workspace" env:"PICOCLAW_AGENTS_DEFAULTS_WORKSPACE"`
	RestrictToWorkspace *bool          `json:"restrict_to_workspace" env:"PICOCLAW_AGENTS_DEFAULTS_RESTRICT_TO_WORKSPACE"`
	Provider            string         `json:"provider" env:"PICOCLAW_AGENTS_DEFAULTS_PROVIDER"`
	Model               string         `json:"model" env:"PICOCLAW_AGENTS_DEFAULTS_MODEL"`
	ModelFallbacks      []string       `json:"model_fallbacks,omitempty"` // From Incoming
	ImageModel          string         `json:"image_model,omitempty" env:"PICOCLAW_AGENTS_DEFAULTS_IMAGE_MODEL"` // From Incoming
	ImageModelFallbacks []string       `json:"image_model_fallbacks,omitempty"` // From Incoming
	MaxTokens           *int           `json:"max_tokens" env:"PICOCLAW_AGENTS_DEFAULTS_MAX_TOKENS"`
	Temperature         *float64       `json:"temperature" env:"PICOCLAW_AGENTS_DEFAULTS_TEMPERATURE"`
	MaxToolIterations   *int           `json:"max_tool_iterations" env:"PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS"`
	Timeout             *int           `json:"timeout" env:"PICOCLAW_AGENTS_DEFAULTS_TIMEOUT"` // seconds
	Subagent            SubagentConfig `json:"subagent"` // From HEAD
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
	Overflow      OverflowEntries `json:"overflow,omitempty"`
}

type ScheduleEntries map[string]ScheduleConfig

type OverflowEntries map[string]OverflowConfig

type OverflowConfig struct {
	List []string `json:"list"`
}

func (o *OverflowEntries) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a map (new format)
	var m map[string]OverflowConfig
	if err := json.Unmarshal(data, &m); err == nil {
		// Heuristic: if it's a map, check if keys are named instances or OverflowConfig fields.
		isOldFormat := false
		for k := range m {
			switch strings.ToLower(k) {
			case "list":
				isOldFormat = true
			}
			if isOldFormat {
				break
			}
		}

		if !isOldFormat && len(m) > 0 {
			*o = m
			return nil
		}
	}

	// Try to unmarshal as a single config (old format/default)
	var single OverflowConfig
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	*o = OverflowEntries{"": single}
	return nil
}

func (o OverflowEntries) MarshalJSON() ([]byte, error) {
	// If it only contains the default entry, marshal as a single object
	if len(o) == 1 {
		if single, ok := o[""]; ok {
			return json.Marshal(single)
		}
	}
	// Otherwise marshal as a map
	return json.Marshal(map[string]OverflowConfig(o))
}

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
			if *p == nil {
				*p = make(ProviderEntries)
			}
			for k, v := range m {
				(*p)[k] = v
			}
			return nil
		}
	}

	// Try to unmarshal as a single config (old format)
	var single ProviderConfig
	if err := json.Unmarshal(data, &single); err != nil {
		return err
	}
	if *p == nil {
		*p = make(ProviderEntries)
	}
	// We need to decide how to merge 'single' into (*p)[""]
	// If (*p)[""] exists, we merge fields.
	existing := (*p)[""]
	if single.Model != "" { existing.Model = single.Model }
	if single.APIKey != "" { existing.APIKey = single.APIKey }
	if single.APIBase != "" { existing.APIBase = single.APIBase }
	if single.Proxy != "" { existing.Proxy = single.Proxy }
	if single.AuthMethod != "" { existing.AuthMethod = single.AuthMethod }
	if single.ConnectMode != "" { existing.ConnectMode = single.ConnectMode }
	if single.MaxTokens != nil { existing.MaxTokens = single.MaxTokens }
	if single.Temperature != nil { existing.Temperature = single.Temperature }
	if single.MaxToolIterations != nil { existing.MaxToolIterations = single.MaxToolIterations }
	if single.Timeout != nil { existing.Timeout = single.Timeout }
	if single.MaxConcurrentSessions != 0 { existing.MaxConcurrentSessions = single.MaxConcurrentSessions }
	if single.WebSearch != nil { existing.WebSearch = single.WebSearch }
	
	(*p)[""] = existing
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
	WebSearch         *bool    `json:"web_search,omitempty"` // Merged from OpenAIProviderConfig, generalized
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

// Removed OpenAIProviderConfig as valid field merged into ProviderConfig

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

type ExecConfig struct {
	EnableDenyPatterns bool     `json:"enable_deny_patterns" env:"PICOCLAW_TOOLS_EXEC_ENABLE_DENY_PATTERNS"`
	CustomDenyPatterns []string `json:"custom_deny_patterns" env:"PICOCLAW_TOOLS_EXEC_CUSTOM_DENY_PATTERNS"`
}

type ToolsConfig struct {
	Web  WebToolsConfig  `json:"web"`
	Cron CronToolsConfig `json:"cron"`
	Exec ExecConfig      `json:"exec"`
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
			OpenAI:       ProviderEntries{"": {WebSearch: BoolPtr(true)}},
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
			Exec: ExecConfig{
				EnableDenyPatterns: true,
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

	// Handle environment variables override
	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (c *Config) WorkspacePath() string {
	if c.Agents.Defaults.Workspace != "" {
		return ExpandHome(c.Agents.Defaults.Workspace)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "workspace")
}

func (c *Config) ResolveWorkspace(senderID string) string {
	// 1. Check for workspace override for this sender/user
	for name, ws := range c.Workspaces {
		for _, user := range ws.Users {
			if user == senderID {
				return ExpandHome(ws.Path)
			}
		}
		// Also check if the key matches senderID
		if name == senderID {
			return ExpandHome(ws.Path)
		}
	}
	// TODO: Implement proper user->workspace mapping
	return c.WorkspacePath()
}

func (c *Config) ResolveRestrictToWorkspace(senderID string) bool {
	// 1. Check for workspace override for this sender/user
	for name, ws := range c.Workspaces {
		for _, user := range ws.Users {
			if user == senderID {
				if ws.RestrictToWorkspace != nil {
					return *ws.RestrictToWorkspace
				}
				break
			}
		}
		if name == senderID {
			if ws.RestrictToWorkspace != nil {
				return *ws.RestrictToWorkspace
			}
		}
	}

	// Fallback to global default
	if c.Agents.Defaults.RestrictToWorkspace != nil {
		return *c.Agents.Defaults.RestrictToWorkspace
	}
	return true // Default to true for safety
}

func ExpandHome(path string) string {
	if path == "" {
		return path
	}
	if path[0] == '~' {
		home, _ := os.UserHomeDir()
		if len(path) > 1 && (path[1] == '/' || path[1] == '\\') {
			return filepath.Join(home, path[2:])
		}
		return home
	}
	return path
}
