// PicoClaw - Ultra-lightweight personal AI agent
// Inspired by and based on nanobot: https://github.com/HKUDS/nanobot
// License: MIT
//
// Copyright (c) 2026 PicoClaw contributors

package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/auth"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

type HTTPProvider struct {
	id                string // Unique ID for concurrency tracking
	apiKey            string
	apiBase           string
	httpClient        *http.Client
	defaultModel      string
	maxTokens         int
	temperature       float64
	maxToolIterations int
	timeout           int
	maxConcurrentSessions int
}

func NewHTTPProvider(id, apiKey, apiBase, proxy string, timeoutSec int, model string, maxTokens int, temperature float64, maxToolIterations int, maxConcurrentSessions int) *HTTPProvider {
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	client := &http.Client{
		Timeout: timeout,
	}

	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err == nil {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		}
	}

	logger.InfoCF("http_provider", "Created HTTP provider", map[string]interface{}{
		"id":       id,
		"api_base": strings.TrimRight(apiBase, "/"),
		"timeout":  timeout.String(),
		"proxy":    proxy,
		"model":    model,
	})

	return &HTTPProvider{
		id:                id,
		apiKey:            apiKey,
		apiBase:           strings.TrimRight(apiBase, "/"),
		httpClient:        client,
		defaultModel:      model,
		maxTokens:         maxTokens,
		temperature:       temperature,
		maxToolIterations: maxToolIterations,
		timeout:           int(timeout.Seconds()),
		maxConcurrentSessions: maxConcurrentSessions,
	}
}

func (p *HTTPProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error) {
	// Acquire concurrency slot
	if p.id != "" && p.maxConcurrentSessions > 0 {
		if !GlobalConcurrencyTracker().Acquire(p.id, p.maxConcurrentSessions) {
			logger.WarnCF("http_provider", "Concurrency limit reached", map[string]interface{}{
				"id":  p.id,
				"max": p.maxConcurrentSessions,
			})
			return nil, fmt.Errorf("%w for provider %s", ErrConcurrencyLimit, p.id)
		}
		defer GlobalConcurrencyTracker().Release(p.id)
	}

	if p.apiBase == "" {
		return nil, fmt.Errorf("API base not configured")
	}

	// Strip provider prefix from model name (e.g., moonshot/kimi-k2.5 -> kimi-k2.5, groq/openai/gpt-oss-120b -> openai/gpt-oss-120b, ollama/qwen2.5:14b -> qwen2.5:14b)
	if idx := strings.Index(model, "/"); idx != -1 {
		prefix := model[:idx]
		if prefix == "moonshot" || prefix == "nvidia" || prefix == "groq" || prefix == "ollama" {
			model = model[idx+1:]
		}
	}

	requestBody := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}

	if len(tools) > 0 {
		requestBody["tools"] = tools
		requestBody["tool_choice"] = "auto"
	}

	if maxTokens, ok := options["max_tokens"].(int); ok {
		lowerModel := strings.ToLower(model)
		if strings.Contains(lowerModel, "glm") || strings.Contains(lowerModel, "o1") {
			requestBody["max_completion_tokens"] = maxTokens
		} else {
			requestBody["max_tokens"] = maxTokens
		}
	}

	if temperature, ok := options["temperature"].(float64); ok {
		lowerModel := strings.ToLower(model)
		// Kimi k2 models only support temperature=1
		if strings.Contains(lowerModel, "kimi") && strings.Contains(lowerModel, "k2") {
			requestBody["temperature"] = 1.0
		} else {
			requestBody["temperature"] = temperature
		}
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	logger.DebugCF("http_provider", "Sending LLM request", map[string]interface{}{
		"model":   model,
		"timeout": p.httpClient.Timeout.String(),
	})

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed:\n  Status: %d\n  Body:   %s", resp.StatusCode, string(body))
	}

	return p.parseResponse(body)
}

func (p *HTTPProvider) parseResponse(body []byte) (*LLMResponse, error) {
	var apiResponse struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function *struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *UsageInfo `json:"usage"`
	}

	if err := json.Unmarshal(body, &apiResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(apiResponse.Choices) == 0 {
		return &LLMResponse{
			Content:      "",
			FinishReason: "stop",
		}, nil
	}

	choice := apiResponse.Choices[0]

	toolCalls := make([]ToolCall, 0, len(choice.Message.ToolCalls))
	for _, tc := range choice.Message.ToolCalls {
		arguments := make(map[string]interface{})
		name := ""

		// Handle OpenAI format with nested function object
		if tc.Type == "function" && tc.Function != nil {
			name = tc.Function.Name
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &arguments); err != nil {
					arguments["raw"] = tc.Function.Arguments
				}
			}
		} else if tc.Function != nil {
			// Legacy format without type field
			name = tc.Function.Name
			if tc.Function.Arguments != "" {
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &arguments); err != nil {
					arguments["raw"] = tc.Function.Arguments
				}
			}
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:        tc.ID,
			Name:      name,
			Arguments: arguments,
		})
	}

	return &LLMResponse{
		Content:      choice.Message.Content,
		ToolCalls:    toolCalls,
		FinishReason: choice.FinishReason,
		Usage:        apiResponse.Usage,
	}, nil
}

func (p *HTTPProvider) GetDefaultModel() string {
	return p.defaultModel
}

func (p *HTTPProvider) GetMaxTokens() int {
	return p.maxTokens
}

func (p *HTTPProvider) GetTemperature() float64 {
	return p.temperature
}

func (p *HTTPProvider) GetMaxToolIterations() int {
	return p.maxToolIterations
}

func (p *HTTPProvider) GetTimeout() int {
	return p.timeout
}

func (p *HTTPProvider) GetMaxConcurrent() int {
	return p.maxConcurrentSessions
}

func createClaudeAuthProvider() (LLMProvider, error) {
	cred, err := auth.GetCredential("anthropic")
	if err != nil {
		return nil, fmt.Errorf("loading auth credentials: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf("no credentials for anthropic. Run: picoclaw auth login --provider anthropic")
	}
	return NewClaudeProviderWithTokenSource(cred.AccessToken, createClaudeTokenSource()), nil
}

func createCodexAuthProvider() (LLMProvider, error) {
	cred, err := auth.GetCredential("openai")
	if err != nil {
		return nil, fmt.Errorf("loading auth credentials: %w", err)
	}
	if cred == nil {
		return nil, fmt.Errorf("no credentials for openai. Run: picoclaw auth login --provider openai")
	}
	return NewCodexProviderWithTokenSource(cred.AccessToken, cred.AccountID, createCodexTokenSource()), nil
}

func CreateProvider(cfg *config.Config) (LLMProvider, error) {
	model := cfg.Agents.Defaults.Model
	providerStr := cfg.Agents.Defaults.Provider
	providerType, instanceName := config.ResolveProvider(providerStr)

	var apiKey, apiBase, proxy string
	var pConfig config.ProviderConfig
	var found bool

	var maxTokens int
	var temperature float64
	var maxToolIterations int
	var timeout int
	var maxConcurrentSessions int

	if providerType != "" || instanceName != "" {
		pConfig, found = cfg.Providers.Get(providerType, instanceName)
		if found {
			if model == "" {
				model = pConfig.Model
			}
			apiKey = pConfig.APIKey
			apiBase = pConfig.APIBase
			proxy = pConfig.Proxy
			if pConfig.MaxTokens != nil {
				maxTokens = *pConfig.MaxTokens
			}
			if pConfig.Temperature != nil {
				temperature = *pConfig.Temperature
			}
			if pConfig.MaxToolIterations != nil {
				maxToolIterations = *pConfig.MaxToolIterations
			}
			if pConfig.Timeout != nil {
				timeout = *pConfig.Timeout
			}
			maxConcurrentSessions = pConfig.MaxConcurrentSessions

			// Set dummy keys for providers that don't need them
			lowerType := strings.ToLower(providerType)
			if apiKey == "" {
				if lowerType == "ollama" {
					apiKey = "ollama"
				} else if lowerType == "vllm" {
					apiKey = "vllm"
				}
			}
		}
	}

	// Final Fallbacks for each individual field
	if model == "" {
		model = "glm-4.7"
	}
	if maxTokens == 0 {
		maxTokens = 8192
	}
	if temperature == 0 {
		temperature = 0.7
	}
	if maxToolIterations == 0 {
		maxToolIterations = 20
	}
	if timeout == 0 {
		timeout = 120
	}

	lowerModel := strings.ToLower(model)
	
	// Generate unique ID for this provider instance
	providerID := providerStr
	if providerID == "" {
		providerID = "default"
	}
	// Append model if handled by default fallbacks to differentiate
	if !found {
		providerID = fmt.Sprintf("%s:%s", providerID, model)
	}

	// Special provider types that don't just use HTTPProvider or have complex init
	if found {
		switch strings.ToLower(providerType) {
		case "openai", "gpt":
			if pConfig.AuthMethod == "codex-cli" {
				return NewCodexProviderWithTokenSource("", "", CreateCodexCliTokenSource()), nil
			}
			if pConfig.AuthMethod == "oauth" || pConfig.AuthMethod == "token" {
				return createCodexAuthProvider()
			}
			if apiBase == "" {
				apiBase = "https://api.openai.com/v1"
			}
		case "anthropic", "claude":
			if pConfig.AuthMethod == "oauth" || pConfig.AuthMethod == "token" {
				return createClaudeAuthProvider()
			}
			if apiBase == "" {
				apiBase = "https://api.anthropic.com/v1"
			}
		case "groq":
			if apiBase == "" {
				apiBase = "https://api.groq.com/openai/v1"
			}
		case "openrouter":
			if apiBase == "" {
				apiBase = "https://openrouter.ai/api/v1"
			}
		case "zhipu", "glm":
			if apiBase == "" {
				apiBase = "https://open.bigmodel.cn/api/paas/v4"
			}
		case "gemini", "google":
			if apiBase == "" {
				apiBase = "https://generativelanguage.googleapis.com/v1beta"
			}
		case "shengsuanyun":
			if apiBase == "" {
				apiBase = "https://router.shengsuanyun.com/api/v1"
			}
		case "deepseek":
			if apiBase == "" {
				apiBase = "https://api.deepseek.com/v1"
			}
			if model != "deepseek-chat" && model != "deepseek-reasoner" {
				model = "deepseek-chat"
			}
		case "github_copilot", "copilot":
			if apiBase == "" {
				apiBase = "localhost:4321"
			}
			return NewGitHubCopilotProvider(apiBase, pConfig.ConnectMode, model)
		}
	}

	// Handle standalone provider keywords that return specialized providers
	switch strings.ToLower(providerType) {
	case "overflow":
		var overflowConfig config.OverflowConfig
		var found bool

		if instanceName != "" {
			overflowConfig, found = cfg.Providers.Overflow[instanceName]
		}
		
		if !found {
			overflowConfig, found = cfg.Providers.Overflow[""]
			if !found && instanceName == "" && len(cfg.Providers.Overflow) > 0 {
				for _, v := range cfg.Providers.Overflow {
					overflowConfig = v
					found = true
					break
				}
			}
		}
		
		if !found {
			return nil, fmt.Errorf("overflow provider '%s' not configured", instanceName)
		}
		
		return NewOverflowProvider(cfg, overflowConfig.List), nil

	case "schedule":
		// instanceName is from ResolveProvider (e.g. "work" from "schedule/work")
		var scheduleConfig config.ScheduleConfig
		var found bool

		if instanceName != "" {
			scheduleConfig, found = cfg.Providers.Schedule[instanceName]
		}

		if !found {
			// Try empty key
			scheduleConfig, found = cfg.Providers.Schedule[""]
			if !found && instanceName == "" && len(cfg.Providers.Schedule) > 0 {
				// Fallback: pick first available
				for _, v := range cfg.Providers.Schedule {
					scheduleConfig = v
					found = true
					break
				}
			}
		}

		if !found {
			return nil, fmt.Errorf("schedule provider '%s' not configured", instanceName)
		}

		var location *time.Location
		var err error
		if scheduleConfig.Timezone != "" {
			location, err = time.LoadLocation(scheduleConfig.Timezone)
			if err != nil {
				return nil, fmt.Errorf("invalid schedule timezone %q: %w", scheduleConfig.Timezone, err)
			}
		} else {
			location = time.Local
		}
		return NewScheduleProvider(cfg, &scheduleConfig, location), nil
	case "claude-cli", "claudecode", "claude-code":
		workspace := cfg.WorkspacePath()
		if workspace == "" {
			workspace = "."
		}
		return NewClaudeCliProvider(workspace), nil
	case "codex-cli", "codex-code":
		workspace := cfg.WorkspacePath()
		if workspace == "" {
			workspace = "."
		}
		return NewCodexCliProvider(workspace), nil
	}

	// Fallback: detect provider from model name if no explicit provider config was found or used
	if apiKey == "" && apiBase == "" {
		switch {
		case (strings.Contains(lowerModel, "kimi") || strings.Contains(lowerModel, "moonshot") || strings.HasPrefix(model, "moonshot/")):
			p, ok := cfg.Providers.Get("moonshot", "")
			if ok && p.APIKey != "" {
				apiKey = p.APIKey
				apiBase = p.APIBase
				proxy = p.Proxy
				if apiBase == "" {
					apiBase = "https://api.moonshot.cn/v1"
				}
			}

		case strings.HasPrefix(model, "openrouter/") || strings.HasPrefix(model, "anthropic/") || strings.HasPrefix(model, "openai/") || strings.HasPrefix(model, "meta-llama/") || strings.HasPrefix(model, "deepseek/") || strings.HasPrefix(model, "google/"):
			p, ok := cfg.Providers.Get("openrouter", "")
			if ok && p.APIKey != "" {
				apiKey = p.APIKey
				proxy = p.Proxy
				apiBase = p.APIBase
				if apiBase == "" {
					apiBase = "https://openrouter.ai/api/v1"
				}
			}

		case (strings.Contains(lowerModel, "claude") || strings.HasPrefix(model, "anthropic/")):
			p, ok := cfg.Providers.Get("anthropic", "")
			if ok && (p.APIKey != "" || p.AuthMethod != "") {
				if p.AuthMethod == "oauth" || p.AuthMethod == "token" {
					return createClaudeAuthProvider()
				}
				apiKey = p.APIKey
				apiBase = p.APIBase
				proxy = p.Proxy
				if apiBase == "" {
					apiBase = "https://api.anthropic.com/v1"
				}
			}

		case (strings.Contains(lowerModel, "gpt") || strings.HasPrefix(model, "openai/")):
			p, ok := cfg.Providers.Get("openai", "")
			if ok && (p.APIKey != "" || p.AuthMethod != "") {
				if p.AuthMethod == "oauth" || p.AuthMethod == "token" {
					return createCodexAuthProvider()
				}
				apiKey = p.APIKey
				apiBase = p.APIBase
				proxy = p.Proxy
				if apiBase == "" {
					apiBase = "https://api.openai.com/v1"
				}
			}

		case (strings.Contains(lowerModel, "gemini") || strings.HasPrefix(model, "google/")):
			p, ok := cfg.Providers.Get("gemini", "")
			if ok && p.APIKey != "" {
				apiKey = p.APIKey
				apiBase = p.APIBase
				proxy = p.Proxy
				if apiBase == "" {
					apiBase = "https://generativelanguage.googleapis.com/v1beta"
				}
			}

		case (strings.Contains(lowerModel, "glm") || strings.Contains(lowerModel, "zhipu") || strings.Contains(lowerModel, "zai")):
			p, ok := cfg.Providers.Get("zhipu", "")
			if ok && p.APIKey != "" {
				apiKey = p.APIKey
				apiBase = p.APIBase
				proxy = p.Proxy
				if apiBase == "" {
					apiBase = "https://open.bigmodel.cn/api/paas/v4"
				}
			}

		case (strings.Contains(lowerModel, "groq") || strings.HasPrefix(model, "groq/")):
			p, ok := cfg.Providers.Get("groq", "")
			if ok && p.APIKey != "" {
				apiKey = p.APIKey
				apiBase = p.APIBase
				proxy = p.Proxy
				if apiBase == "" {
					apiBase = "https://api.groq.com/openai/v1"
				}
			}

		case (strings.Contains(lowerModel, "nvidia") || strings.HasPrefix(model, "nvidia/")):
			p, ok := cfg.Providers.Get("nvidia", "")
			if ok && p.APIKey != "" {
				apiKey = p.APIKey
				apiBase = p.APIBase
				proxy = p.Proxy
				if apiBase == "" {
					apiBase = "https://integrate.api.nvidia.com/v1"
				}
			}
		case (strings.Contains(lowerModel, "ollama") || strings.HasPrefix(model, "ollama/")):
			p, ok := cfg.Providers.Get("ollama", "")
			if ok && (p.APIKey != "" || p.APIBase != "") {
				logger.InfoCF("provider", "Ollama provider selected based on model name prefix", nil)
				apiKey = p.APIKey
				apiBase = p.APIBase
				proxy = p.Proxy
				if apiBase == "" {
					apiBase = "http://localhost:11434/v1"
				}
				if apiKey == "" {
					apiKey = "ollama"
				}
				logger.DebugCF("provider", "Ollama apiBase", map[string]interface{}{"api_base": apiBase})
			}
		case (cfg.Providers.VLLM != nil):
			p, ok := cfg.Providers.Get("vllm", "")
			if ok && p.APIBase != "" {
				apiKey = p.APIKey
				apiBase = p.APIBase
				proxy = p.Proxy
				if apiKey == "" {
					apiKey = "vllm"
				}
			}

		default:
			p, ok := cfg.Providers.Get("openrouter", "")
			if ok && p.APIKey != "" {
				apiKey = p.APIKey
				proxy = p.Proxy
				apiBase = p.APIBase
				if apiBase == "" {
					apiBase = "https://openrouter.ai/api/v1"
				}
			} else {
				return nil, fmt.Errorf("no API key configured for model: %s", model)
			}
		}
	}

	if apiKey == "" && !strings.HasPrefix(model, "bedrock/") {
		return nil, fmt.Errorf("no API key configured for provider (model: %s)", model)
	}

	if apiBase == "" {
		return nil, fmt.Errorf("no API base configured for provider (model: %s)", model)
	}

	return NewHTTPProvider(providerID, apiKey, apiBase, proxy, timeout, model, maxTokens, temperature, maxToolIterations, maxConcurrentSessions), nil
}
