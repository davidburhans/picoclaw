package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/caarlos0/env/v11"
	"github.com/sipeed/picoclaw/pkg/utils"
	"github.com/tidwall/jsonc"
)

// rrCounter is a global counter for round-robin load balancing across models.
var rrCounter atomic.Uint64

// FlexibleStringSlice is a []string that also accepts JSON numbers,
// so allow_from can contain both "123" and 123.
type FlexibleStringSlice []string

func BoolPtr(v bool) *bool        { return &v }
func IntPtr(v int) *int           { return &v }
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
	Bindings   []AgentBinding             `json:"bindings,omitempty"`
	Session    SessionConfig              `json:"session,omitempty"`
	Channels   ChannelsConfig             `json:"channels"`
	Workspaces map[string]WorkspaceConfig `json:"workspaces"`
	ModelList  []ModelConfig              `json:"model_list"`
	Gateway    GatewayConfig              `json:"gateway"`
	Tools      ToolsConfig                `json:"tools"`
	MCP        map[string]MCPServerConfig `json:"mcp"`
	Heartbeat  HeartbeatConfig            `json:"heartbeat"`
	Devices    DevicesConfig              `json:"devices"`
	Memory     MemoryConfig               `json:"memory"`
	Mailbox    MailboxConfig              `json:"mailbox"`
	Voice      VoiceConfig                `json:"voice"`
	Schedules  ScheduleEntries            `json:"schedules"`
	mu         sync.RWMutex
}

type MailboxConfig struct {
	Path string `json:"path" env:"PICOCLAW_MAILBOX_PATH"`
}

type VoiceConfig struct {
	TTS TTSConfig `json:"tts"`
	STT STTConfig `json:"stt"`
}

type STTConfig struct {
	Enabled  bool   `json:"enabled" env:"PICOCLAW_VOICE_STT_ENABLED"`
	APIBase  string `json:"api_base" env:"PICOCLAW_VOICE_STT_API_BASE"`
	APIKey   string `json:"api_key" env:"PICOCLAW_VOICE_STT_API_KEY"`
	Model    string `json:"model" env:"PICOCLAW_VOICE_STT_MODEL"`
	Language string `json:"language" env:"PICOCLAW_VOICE_STT_LANGUAGE"`
}

type TTSConfig struct {
	Enabled      bool    `json:"enabled" env:"PICOCLAW_VOICE_TTS_ENABLED"`
	ServerURL    string  `json:"server_url" env:"PICOCLAW_VOICE_TTS_SERVER_URL"`
	VoiceID      string  `json:"voice_id" env:"PICOCLAW_VOICE_TTS_VOICE_ID"`
	Language     string  `json:"language" env:"PICOCLAW_VOICE_TTS_LANGUAGE"`
	Temperature  float64 `json:"temperature" env:"PICOCLAW_VOICE_TTS_TEMPERATURE"`
	Exaggeration float64 `json:"exaggeration" env:"PICOCLAW_VOICE_TTS_EXAGGERATION"`
	CfgWeight    float64 `json:"cfg_weight" env:"PICOCLAW_VOICE_TTS_CFG_WEIGHT"`
	SpeedFactor  float64 `json:"speed_factor" env:"PICOCLAW_VOICE_TTS_SPEED_FACTOR"`
	Seed         int     `json:"seed" env:"PICOCLAW_VOICE_TTS_SEED"`
	ChunkSize    int     `json:"chunk_size" env:"PICOCLAW_VOICE_TTS_CHUNK_SIZE"`
	SplitText    bool    `json:"split_text" env:"PICOCLAW_VOICE_TTS_SPLIT_TEXT"`
	OutputFormat string  `json:"output_format" env:"PICOCLAW_VOICE_TTS_OUTPUT_FORMAT"`
	AutoPlay     bool    `json:"auto_play" env:"PICOCLAW_VOICE_TTS_AUTO_PLAY"`
	IncludeText  bool    `json:"include_text" env:"PICOCLAW_VOICE_TTS_INCLUDE_TEXT"`
}

type MemoryConfig struct {
	Enabled   bool            `json:"enabled" env:"PICOCLAW_MEMORY_ENABLED"`
	Provider  string          `json:"provider" env:"PICOCLAW_MEMORY_PROVIDER"`
	Qdrant    QdrantConfig    `json:"qdrant"`
	Embedding EmbeddingConfig `json:"embedding"`
}

type EmbeddingConfig struct {
	Provider  string `json:"provider" env:"PICOCLAW_MEMORY_EMBEDDING_PROVIDER"`
	Model     string `json:"model" env:"PICOCLAW_MEMORY_EMBEDDING_MODEL"`
	APIKey    string `json:"api_key" env:"PICOCLAW_MEMORY_EMBEDDING_API_KEY"`
	BaseURL   string `json:"base_url" env:"PICOCLAW_MEMORY_EMBEDDING_BASE_URL"`
	Timeout   int    `json:"timeout" env:"PICOCLAW_MEMORY_EMBEDDING_TIMEOUT"`
	ChunkSize int    `json:"chunk_size" env:"PICOCLAW_MEMORY_EMBEDDING_CHUNK_SIZE"`
	KeepAlive string `json:"keep_alive" env:"PICOCLAW_MEMORY_EMBEDDING_KEEP_ALIVE"`
	NumCtx    int    `json:"num_ctx" env:"PICOCLAW_MEMORY_EMBEDDING_NUM_CTX"`
}

type QdrantConfig struct {
	URL            string `json:"url" env:"PICOCLAW_MEMORY_QDRANT_URL"`
	CollectionName string `json:"collection_name" env:"PICOCLAW_MEMORY_QDRANT_COLLECTION_NAME"`
	APIKey         string `json:"api_key" env:"PICOCLAW_MEMORY_QDRANT_API_KEY"`
	ModelName      string `json:"model_name" env:"PICOCLAW_MEMORY_QDRANT_MODEL_NAME"`
}

type WorkspaceConfig struct {
	Path                 string              `json:"path" env:"PICOCLAW_WORKSPACES_{{.Name}}_PATH"`
	Users                FlexibleStringSlice `json:"users" env:"PICOCLAW_WORKSPACES_{{.Name}}_USERS"`
	RestrictToWorkspace  *bool               `json:"restrict_to_workspace"`
	AllowedExternalPaths []string            `json:"allowed_external_paths"`

	BirthYear       int      `json:"birth_year" env:"PICOCLAW_WORKSPACES_{{.Name}}_BIRTH_YEAR"`
	SafetyLevel     string   `json:"safety_level" env:"PICOCLAW_WORKSPACES_{{.Name}}_SAFETY_LEVEL"`
	AllowedTools    []string `json:"allowed_tools" env:"PICOCLAW_WORKSPACES_{{.Name}}_ALLOWED_TOOLS"`
	RestrictedTools []string `json:"restricted_tools" env:"PICOCLAW_WORKSPACES_{{.Name}}_RESTRICTED_TOOLS"`
	CanManageFamily bool     `json:"can_manage_family" env:"PICOCLAW_WORKSPACES_{{.Name}}_CAN_MANAGE_FAMILY"`
}

// MarshalJSON implements custom JSON marshaling for Config
// to omit providers section when empty and session when empty
func (c Config) MarshalJSON() ([]byte, error) {
	type Alias Config
	aux := &struct {
		Session *SessionConfig `json:"session,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(&c),
	}

	if c.Session.DMScope != "" || len(c.Session.IdentityLinks) > 0 {
		aux.Session = &c.Session
	}

	return json.Marshal(aux)
}

type AgentsConfig struct {
	Defaults AgentDefaults `json:"defaults"`
	List     []AgentConfig `json:"list,omitempty"`
}

// AgentModelConfig supports both string and structured model config.
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
	ModelFallbacks      []string       `json:"model_fallbacks,omitempty"`
	ImageModel          string         `json:"image_model,omitempty" env:"PICOCLAW_AGENTS_DEFAULTS_IMAGE_MODEL"`
	ImageModelFallbacks []string       `json:"image_model_fallbacks,omitempty"`
	MaxTokens           *int           `json:"max_tokens" env:"PICOCLAW_AGENTS_DEFAULTS_MAX_TOKENS"`
	Temperature         *float64       `json:"temperature" env:"PICOCLAW_AGENTS_DEFAULTS_TEMPERATURE"`
	MaxToolIterations   *int           `json:"max_tool_iterations" env:"PICOCLAW_AGENTS_DEFAULTS_MAX_TOOL_ITERATIONS"`
	Timeout             *int           `json:"timeout" env:"PICOCLAW_AGENTS_DEFAULTS_TIMEOUT"`
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
	WeCom    WeComConfig    `json:"wecom"`
	WeComApp WeComAppConfig `json:"wecom_app"`
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
	Enabled     bool                `json:"enabled" env:"PICOCLAW_CHANNELS_DISCORD_ENABLED"`
	Token       string              `json:"token" env:"PICOCLAW_CHANNELS_DISCORD_TOKEN"`
	AllowFrom   FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_DISCORD_ALLOW_FROM"`
	MentionOnly bool                `json:"mention_only" env:"PICOCLAW_CHANNELS_DISCORD_MENTION_ONLY"`
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

type WeComConfig struct {
	Enabled        bool                `json:"enabled" env:"PICOCLAW_CHANNELS_WECOM_ENABLED"`
	Token          string              `json:"token" env:"PICOCLAW_CHANNELS_WECOM_TOKEN"`
	EncodingAESKey string              `json:"encoding_aes_key" env:"PICOCLAW_CHANNELS_WECOM_ENCODING_AES_KEY"`
	WebhookURL     string              `json:"webhook_url" env:"PICOCLAW_CHANNELS_WECOM_WEBHOOK_URL"`
	WebhookHost    string              `json:"webhook_host" env:"PICOCLAW_CHANNELS_WECOM_WEBHOOK_HOST"`
	WebhookPort    int                 `json:"webhook_port" env:"PICOCLAW_CHANNELS_WECOM_WEBHOOK_PORT"`
	WebhookPath    string              `json:"webhook_path" env:"PICOCLAW_CHANNELS_WECOM_WEBHOOK_PATH"`
	AllowFrom      FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_WECOM_ALLOW_FROM"`
	ReplyTimeout   int                 `json:"reply_timeout" env:"PICOCLAW_CHANNELS_WECOM_REPLY_TIMEOUT"`
}

type WeComAppConfig struct {
	Enabled        bool                `json:"enabled" env:"PICOCLAW_CHANNELS_WECOM_APP_ENABLED"`
	CorpID         string              `json:"corp_id" env:"PICOCLAW_CHANNELS_WECOM_APP_CORP_ID"`
	CorpSecret     string              `json:"corp_secret" env:"PICOCLAW_CHANNELS_WECOM_APP_CORP_SECRET"`
	AgentID        int64               `json:"agent_id" env:"PICOCLAW_CHANNELS_WECOM_APP_AGENT_ID"`
	Token          string              `json:"token" env:"PICOCLAW_CHANNELS_WECOM_APP_TOKEN"`
	EncodingAESKey string              `json:"encoding_aes_key" env:"PICOCLAW_CHANNELS_WECOM_APP_ENCODING_AES_KEY"`
	WebhookHost    string              `json:"webhook_host" env:"PICOCLAW_CHANNELS_WECOM_APP_WEBHOOK_HOST"`
	WebhookPort    int                 `json:"webhook_port" env:"PICOCLAW_CHANNELS_WECOM_APP_WEBHOOK_PORT"`
	WebhookPath    string              `json:"webhook_path" env:"PICOCLAW_CHANNELS_WECOM_APP_WEBHOOK_PATH"`
	AllowFrom      FlexibleStringSlice `json:"allow_from" env:"PICOCLAW_CHANNELS_WECOM_APP_ALLOW_FROM"`
	ReplyTimeout   int                 `json:"reply_timeout" env:"PICOCLAW_CHANNELS_WECOM_APP_REPLY_TIMEOUT"`
}

type HeartbeatConfig struct {
	Enabled  bool `json:"enabled" env:"PICOCLAW_HEARTBEAT_ENABLED"`
	Interval int  `json:"interval" env:"PICOCLAW_HEARTBEAT_INTERVAL"`
}

type DevicesConfig struct {
	Enabled    bool `json:"enabled" env:"PICOCLAW_DEVICES_ENABLED"`
	MonitorUSB bool `json:"monitor_usb" env:"PICOCLAW_DEVICES_MONITOR_USB"`
}

type ScheduleConfig struct {
	Timezone string          `json:"timezone"`
	Default  ScheduleDefault `json:"default"`
	Rules    []ScheduleRule  `json:"rules"`
}

type ScheduleDefault struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type ScheduleRule struct {
	Days     []string       `json:"days"`
	Hours    *ScheduleHours `json:"hours"`
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
}

type ScheduleHours struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type ScheduleEntries map[string]ScheduleConfig

func (e *ScheduleEntries) UnmarshalJSON(data []byte) error {
	// Try map format first
	var m map[string]ScheduleConfig
	if err := json.Unmarshal(data, &m); err == nil {
		*e = m
		return nil
	}

	// Try single object format
	var s ScheduleConfig
	if err := json.Unmarshal(data, &s); err == nil {
		*e = map[string]ScheduleConfig{"": s}
		return nil
	}

	return fmt.Errorf("invalid schedule format: must be map or single object")
}

// ModelConfig represents a model-centric provider configuration.
type ModelConfig struct {
	ModelName string `json:"model_name"`
	Model     string `json:"model"`

	Provider string `json:"provider,omitempty"` // e.g. "ollama", "openai", "anthropic"
	APIBase  string `json:"api_base,omitempty"`
	BaseURL  string `json:"base_url,omitempty"` // alias for APIBase
	APIKey   string `json:"api_key"`
	Proxy    string `json:"proxy,omitempty"`

	AuthMethod  string `json:"auth_method,omitempty"`
	ConnectMode string `json:"connect_mode,omitempty"`
	Workspace   string `json:"workspace,omitempty"`

	RPM                   *int     `json:"rpm,omitempty"`
	MaxTokensField        string   `json:"max_tokens_field,omitempty"`
	MaxTokens             *int     `json:"max_tokens,omitempty"`
	Temperature           *float64 `json:"temperature,omitempty"`
	MaxConcurrentSessions *int     `json:"max_concurrent_sessions,omitempty"`
	Timeout               *int     `json:"timeout,omitempty"`
	Providers             []string `json:"providers,omitempty"` // For overflow provider
}

func (c *ModelConfig) Validate() error {
	if c.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}
	if c.Model == "" {
		return fmt.Errorf("model is required")
	}
	return nil
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
	ExecTimeoutMinutes int `json:"exec_timeout_minutes" env:"PICOCLAW_TOOLS_CRON_EXEC_TIMEOUT_MINUTES"`
}

type ExecConfig struct {
	EnableDenyPatterns bool     `json:"enable_deny_patterns" env:"PICOCLAW_TOOLS_EXEC_ENABLE_DENY_PATTERNS"`
	CustomDenyPatterns []string `json:"custom_deny_patterns" env:"PICOCLAW_TOOLS_EXEC_CUSTOM_DENY_PATTERNS"`
}

type ToolsConfig struct {
	Web    WebToolsConfig    `json:"web"`
	Cron   CronToolsConfig   `json:"cron"`
	Exec   ExecConfig        `json:"exec"`
	Skills SkillsToolsConfig `json:"skills"`
}

type SkillsToolsConfig struct {
	Registries            SkillsRegistriesConfig `json:"registries"`
	MaxConcurrentSearches int                    `json:"max_concurrent_searches" env:"PICOCLAW_SKILLS_MAX_CONCURRENT_SEARCHES"`
	SearchCache           SearchCacheConfig      `json:"search_cache"`
}

type SearchCacheConfig struct {
	MaxSize    int `json:"max_size" env:"PICOCLAW_SKILLS_SEARCH_CACHE_MAX_SIZE"`
	TTLSeconds int `json:"ttl_seconds" env:"PICOCLAW_SKILLS_SEARCH_CACHE_TTL_SECONDS"`
}

type SkillsRegistriesConfig struct {
	ClawHub ClawHubRegistryConfig `json:"clawhub"`
}

type ClawHubRegistryConfig struct {
	Enabled         bool   `json:"enabled" env:"PICOCLAW_SKILLS_REGISTRIES_CLAWHUB_ENABLED"`
	BaseURL         string `json:"base_url" env:"PICOCLAW_SKILLS_REGISTRIES_CLAWHUB_BASE_URL"`
	AuthToken       string `json:"auth_token" env:"PICOCLAW_SKILLS_REGISTRIES_CLAWHUB_AUTH_TOKEN"`
	SearchPath      string `json:"search_path" env:"PICOCLAW_SKILLS_REGISTRIES_CLAWHUB_SEARCH_PATH"`
	SkillsPath      string `json:"skills_path" env:"PICOCLAW_SKILLS_REGISTRIES_CLAWHUB_SKILLS_PATH"`
	DownloadPath    string `json:"download_path" env:"PICOCLAW_SKILLS_REGISTRIES_CLAWHUB_DOWNLOAD_PATH"`
	Timeout         int    `json:"timeout" env:"PICOCLAW_SKILLS_REGISTRIES_CLAWHUB_TIMEOUT"`
	MaxZipSize      int    `json:"max_zip_size" env:"PICOCLAW_SKILLS_REGISTRIES_CLAWHUB_MAX_ZIP_SIZE"`
	MaxResponseSize int    `json:"max_response_size" env:"PICOCLAW_SKILLS_REGISTRIES_CLAWHUB_MAX_RESPONSE_SIZE"`
}

type MCPServerConfig struct {
	Enabled   bool              `json:"enabled"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Transport string            `json:"transport,omitempty"`
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
				Temperature:         nil, // nil means use provider default
				MaxToolIterations:   IntPtr(0),

				Subagent: SubagentConfig{
					MaxIterations: IntPtr(10),
					MaxTokens:     IntPtr(8192),
					Temperature:   FloatPtr(0.7),
					MaxDepth:      IntPtr(5),
				},
			},
		},
		Bindings: []AgentBinding{},
		Session: SessionConfig{
			DMScope: "main",
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
		ModelList: []ModelConfig{
			{
				ModelName: "glm-4.7",
				Model:     "zhipu/glm-4.7",
				APIBase:   "https://open.bigmodel.cn/api/paas/v4",
				APIKey:    "",
			},
			{
				ModelName: "gpt-5.2",
				Model:     "openai/gpt-5.2",
				APIBase:   "https://api.openai.com/v1",
				APIKey:    "",
			},
			{
				ModelName: "claude-sonnet-4.6",
				Model:     "anthropic/claude-sonnet-4.6",
				APIBase:   "https://api.anthropic.com/v1",
				APIKey:    "",
			},
			{
				ModelName: "deepseek-chat",
				Model:     "deepseek/deepseek-chat",
				APIBase:   "https://api.deepseek.com/v1",
				APIKey:    "",
			},
			{
				ModelName: "gemini-2.0-flash",
				Model:     "gemini/gemini-2.0-flash-exp",
				APIBase:   "https://generativelanguage.googleapis.com/v1beta",
				APIKey:    "",
			},
			{
				ModelName: "qwen-plus",
				Model:     "qwen/qwen-plus",
				APIBase:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
				APIKey:    "",
			},
			{
				ModelName: "moonshot-v1-8k",
				Model:     "moonshot/moonshot-v1-8k",
				APIBase:   "https://api.moonshot.cn/v1",
				APIKey:    "",
			},
			{
				ModelName: "llama-3.3-70b",
				Model:     "groq/llama-3.3-70b-versatile",
				APIBase:   "https://api.groq.com/openai/v1",
				APIKey:    "",
			},
			{
				ModelName: "openrouter-auto",
				Model:     "openrouter/auto",
				APIBase:   "https://openrouter.ai/api/v1",
				APIKey:    "",
			},
			{
				ModelName: "nemotron-4-340b",
				Model:     "nvidia/nemotron-4-340b-instruct",
				APIBase:   "https://integrate.api.nvidia.com/v1",
				APIKey:    "",
			},
			{
				ModelName: "cerebras-llama-3.3-70b",
				Model:     "cerebras/llama-3.3-70b",
				APIBase:   "https://api.cerebras.ai/v1",
				APIKey:    "",
			},
			{
				ModelName: "doubao-pro",
				Model:     "volcengine/doubao-pro-32k",
				APIBase:   "https://ark.cn-beijing.volces.com/api/v3",
				APIKey:    "",
			},
			{
				ModelName: "deepseek-v3",
				Model:     "shengsuanyun/deepseek-v3",
				APIBase:   "https://api.shengsuanyun.com/v1",
				APIKey:    "",
			},
			{
				ModelName:  "gemini-flash",
				Model:      "antigravity/gemini-3-flash",
				AuthMethod: "oauth",
			},
			{
				ModelName:  "copilot-gpt-5.2",
				Model:      "github-copilot/gpt-5.2",
				APIBase:    "http://localhost:4321",
				AuthMethod: "oauth",
			},
			{
				ModelName: "llama3",
				Model:     "ollama/llama3",
				APIBase:   "http://localhost:11434/v1",
				APIKey:    "ollama",
			},
			{
				ModelName: "local-model",
				Model:     "vllm/custom-model",
				APIBase:   "http://localhost:8000/v1",
				APIKey:    "",
			},
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
				ExecTimeoutMinutes: 5,
			},
			Exec: ExecConfig{
				EnableDenyPatterns: true,
			},
		},
		Heartbeat: HeartbeatConfig{
			Enabled:  true,
			Interval: 30,
		},
		Devices: DevicesConfig{
			Enabled:    false,
			MonitorUSB: true,
		},
		Mailbox: MailboxConfig{
			Path: "~/.picoclaw/mailbox",
		},
		Memory: MemoryConfig{
			Enabled:  false,
			Provider: "qdrant",
			Qdrant: QdrantConfig{
				URL:            "http://localhost:6333",
				CollectionName: "picoclaw",
			},
		},
		Voice: VoiceConfig{
			STT: STTConfig{
				Enabled:  false,
				Language: "auto",
			},
			TTS: TTSConfig{
				Enabled:      false,
				ServerURL:    "http://localhost:8004",
				VoiceID:      "Emily.wav",
				Language:     "en",
				Temperature:  0.8,
				Exaggeration: 0.4,
				CfgWeight:    0.5,
				SpeedFactor:  1.0,
				Seed:         42,
				ChunkSize:    240,
				SplitText:    true,
				OutputFormat: "wav",
				AutoPlay:     false,
				IncludeText:  true,
			},
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

	data = jsonc.ToJSON(data)

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	if err := env.Parse(cfg); err != nil {
		return nil, err
	}

	if err := cfg.ValidateModelList(); err != nil {
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
		return utils.ExpandHome(c.Agents.Defaults.Workspace)
	}
	return filepath.Join(utils.ExpandHome("~/.picoclaw"), "workspace")
}

func (c *Config) ResolveWorkspace(senderID string) string {
	for name, ws := range c.Workspaces {
		for _, user := range ws.Users {
			if user == senderID {
				return utils.ExpandHome(ws.Path)
			}
		}
		if name == senderID {
			return utils.ExpandHome(ws.Path)
		}
	}
	return c.WorkspacePath()
}

func (c *Config) ResolveWorkspaceName(path string) string {
	path = utils.ExpandHome(path)
	for name, ws := range c.Workspaces {
		if utils.ExpandHome(ws.Path) == path {
			return name
		}
	}
	if path == utils.ExpandHome(c.Agents.Defaults.Workspace) {
		return "default"
	}
	return filepath.Base(path)
}

func (c *Config) GetWorkspaceNames() []string {
	names := []string{"default"}
	for name := range c.Workspaces {
		if name != "default" {
			names = append(names, name)
		}
	}
	return names
}

func (c *Config) ResolveMailboxPath() string {
	if c.Mailbox.Path != "" {
		return utils.ExpandHome(c.Mailbox.Path)
	}
	return filepath.Join(utils.ExpandHome("~/.picoclaw"), "mailbox")
}

// ResolveFamilyPath returns the path to the shared family data directory.
func (c *Config) ResolveFamilyPath() string {
	return filepath.Join(utils.ExpandHome("~/.picoclaw"), "family")
}

// GetWorkspaceConfig returns the WorkspaceConfig for a given workspace name.
func (c *Config) GetWorkspaceConfig(workspaceName string) *WorkspaceConfig {
	if ws, ok := c.Workspaces[workspaceName]; ok {
		return &ws
	}
	return nil
}

func (c *Config) ResolveRestrictToWorkspace(senderID string) bool {
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

	if c.Agents.Defaults.RestrictToWorkspace != nil {
		return *c.Agents.Defaults.RestrictToWorkspace
	}
	return true
}

// ExpandHome moved to pkg/utils/home.go

// GetModelConfig returns the ModelConfig for the given model name.
func (c *Config) GetModelConfig(modelName string) (*ModelConfig, error) {
	matches := c.findMatches(modelName)
	if len(matches) == 0 {
		return nil, fmt.Errorf("model %q not found in model_list or providers", modelName)
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}

	idx := rrCounter.Add(1) % uint64(len(matches))
	return &matches[idx], nil
}

func (c *Config) findMatches(modelName string) []ModelConfig {
	var matches []ModelConfig
	for i := range c.ModelList {
		if c.ModelList[i].ModelName == modelName {
			matches = append(matches, c.ModelList[i])
		}
	}
	return matches
}

// ValidateModelList validates all ModelConfig entries in the model_list.
func (c *Config) ValidateModelList() error {
	for i := range c.ModelList {
		if err := c.ModelList[i].Validate(); err != nil {
			return fmt.Errorf("model_list[%d]: %w", i, err)
		}
	}
	return nil
}
