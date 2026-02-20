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

	"github.com/sipeed/picoclaw/pkg/logger"
)

type HTTPProvider struct {
	id                    string // Unique ID for concurrency tracking
	apiKey                string
	apiBase               string
	httpClient            *http.Client
	defaultModel          string
	maxTokens             int
	temperature           float64
	maxToolIterations     int
	timeout               int
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
		id:                    id,
		apiKey:                apiKey,
		apiBase:               strings.TrimRight(apiBase, "/"),
		httpClient:            client,
		defaultModel:          model,
		maxTokens:             maxTokens,
		temperature:           temperature,
		maxToolIterations:     maxToolIterations,
		timeout:               int(timeout.Seconds()),
		maxConcurrentSessions: maxConcurrentSessions,
	}
}

func NewHTTPProviderWithMaxTokensField(apiKey, apiBase, proxy, maxTokensField string) *HTTPProvider {
	timeout := 120 * time.Second
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

	return &HTTPProvider{
		apiKey:       apiKey,
		apiBase:      strings.TrimRight(apiBase, "/"),
		httpClient:   client,
		defaultModel: "",
		maxTokens:    8192,
		temperature:  0.7,
		timeout:      120,
	}
}

func (p *HTTPProvider) Chat(ctx context.Context, messages []Message, tools []ToolDefinition, model string, options map[string]interface{}) (*LLMResponse, error) {
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

func (p *HTTPProvider) GetID() string {
	return p.id
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
