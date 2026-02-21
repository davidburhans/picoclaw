package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
)

type Client struct {
	provider  string
	model     string
	apiKey    string
	apiBase   string
	client    *http.Client
	chunkSize int
	keepAlive string
	numCtx    int
}

func NewClient(cfg config.EmbeddingConfig) *Client {
	provider := cfg.Provider
	model := cfg.Model
	apiKey := cfg.APIKey
	apiBase := cfg.BaseURL
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 // Default fallback
	}

	if apiBase == "" {
		if strings.EqualFold(provider, "openai") {
			apiBase = "https://api.openai.com/v1"
		} else if strings.EqualFold(provider, "ollama") {
			apiBase = "http://localhost:11434/v1"
		}
	}
	apiBase = strings.TrimRight(apiBase, "/")

	return &Client{
		provider:  provider,
		model:     model,
		apiKey:    apiKey,
		apiBase:   apiBase,
		chunkSize: cfg.ChunkSize,
		keepAlive: cfg.KeepAlive,
		numCtx:    cfg.NumCtx,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	reqBody := map[string]interface{}{
		"model": c.model,
		"input": text,
	}

	// Add Ollama-specific options if configured
	if strings.Contains(strings.ToLower(c.apiBase), "localhost:11434") || strings.EqualFold(c.provider, "ollama") {
		options := map[string]interface{}{}
		if c.numCtx > 0 {
			options["num_ctx"] = c.numCtx
		}
		if len(options) > 0 {
			reqBody["options"] = options
		}
		if c.keepAlive != "" {
			reqBody["keep_alive"] = c.keepAlive
		}
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiBase+"/embeddings", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API request failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("no embedding data returned")
	}

	return apiResp.Data[0].Embedding, nil
}

func (c *Client) Dimension() int {
	// Dimension often depends on the model.
	// For text-embedding-3-small it is 1536.
	// For text-embedding-3-large it can be up to 3072.
	// Ideally we fetch this once or configure it.
	// For now we'll rely on EnsureCollection being called with the dimension from the first embedding if needed,
	// or we hardcode some defaults.
	if strings.Contains(c.model, "small") {
		return 1536
	}
	if strings.Contains(c.model, "large") {
		return 3072
	}
	if strings.Contains(c.model, "ada-002") {
		return 1536
	}
	return 1536 // Default fallback
}
