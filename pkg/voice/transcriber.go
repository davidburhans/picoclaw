package voice

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/utils"
)

type Transcriber interface {
	Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error)
	IsAvailable() bool
}

type TranscriptionResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

type STTTranscriber struct {
	apiKey     string
	apiBase    string
	model      string
	language   string
	httpClient *http.Client
}

func NewTranscriber(modelCfg *config.ModelConfig, voiceCfg *config.VoiceConfig) Transcriber {
	if modelCfg == nil || modelCfg.ModelName == "" {
		logger.DebugCF("voice", "No voice model configured, transcriber unavailable", nil)
		return &STTTranscriber{
			apiKey:     "",
			apiBase:    "",
			model:      "",
			language:   "",
			httpClient: &http.Client{Timeout: 60 * time.Second},
		}
	}

	apiBase := modelCfg.BaseURL
	if apiBase == "" {
		apiBase = modelCfg.APIBase
	}

	language := voiceCfg.Language
	if language == "" {
		language = "auto"
	}

	logger.DebugCF("voice", "Creating STT transcriber", map[string]interface{}{
		"model_name":  modelCfg.ModelName,
		"model":       modelCfg.Model,
		"provider":    modelCfg.Provider,
		"api_base":    apiBase,
		"has_api_key": modelCfg.APIKey != "",
		"language":    language,
	})

	return &STTTranscriber{
		apiKey:     modelCfg.APIKey,
		apiBase:    apiBase,
		model:      modelCfg.Model,
		language:   language,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func NewGroqTranscriber(apiKey string) *STTTranscriber {
	logger.DebugCF("voice", "Creating Groq transcriber (legacy)", map[string]interface{}{"has_api_key": apiKey != ""})

	return &STTTranscriber{
		apiKey:     apiKey,
		apiBase:    "https://api.groq.com/openai/v1",
		model:      "whisper-large-v3",
		language:   "",
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (t *STTTranscriber) Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error) {
	if !t.IsAvailable() {
		return nil, fmt.Errorf("transcriber not available")
	}

	logger.InfoCF("voice", "Starting transcription", map[string]interface{}{
		"audio_file": audioFilePath,
		"model":      t.model,
		"api_base":   t.apiBase,
	})

	audioFile, err := os.Open(audioFilePath)
	if err != nil {
		logger.ErrorCF("voice", "Failed to open audio file", map[string]interface{}{"path": audioFilePath, "error": err})
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	fileInfo, err := audioFile.Stat()
	if err != nil {
		logger.ErrorCF("voice", "Failed to get file info", map[string]interface{}{"path": audioFilePath, "error": err})
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	logger.DebugCF("voice", "Audio file details", map[string]interface{}{
		"size_bytes": fileInfo.Size(),
		"file_name":  filepath.Base(audioFilePath),
	})

	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		defer writer.Close()

		part, err := writer.CreateFormFile("file", filepath.Base(audioFilePath))
		if err != nil {
			logger.ErrorCF("voice", "Failed to create form file", map[string]interface{}{"error": err.Error()})
			return
		}

		copied, err := io.Copy(part, audioFile)
		if err != nil {
			logger.ErrorCF("voice", "Failed to copy file content", map[string]interface{}{"error": err.Error()})
			return
		}

		logger.DebugCF("voice", "File streamed to request", map[string]interface{}{"bytes_copied": copied})

		modelName := t.model
		if modelName == "" {
			modelName = "whisper-large-v3"
		}
		if err := writer.WriteField("model", modelName); err != nil {
			logger.ErrorCF("voice", "Failed to write model field", map[string]interface{}{"error": err.Error()})
			return
		}

		if err := writer.WriteField("response_format", "json"); err != nil {
			logger.ErrorCF("voice", "Failed to write response_format field", map[string]interface{}{"error": err.Error()})
			return
		}

		if t.language != "" && t.language != "auto" {
			if err := writer.WriteField("language", t.language); err != nil {
				logger.ErrorCF("voice", "Failed to write language field", map[string]interface{}{"error": err.Error()})
				return
			}
		}
	}()

	url := t.apiBase + "/audio/transcriptions"
	if !strings.HasSuffix(url, "/") && !strings.Contains(url, "/audio/") {
		url = strings.TrimSuffix(url, "/v1") + "/v1/audio/transcriptions"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, pr)
	if err != nil {
		logger.ErrorCF("voice", "Failed to create request", map[string]interface{}{"error": err})
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	logger.DebugCF("voice", "Sending transcription request", map[string]interface{}{
		"url":             url,
		"file_size_bytes": fileInfo.Size(),
	})

	resp, err := t.httpClient.Do(req)
	if err != nil {
		logger.ErrorCF("voice", "Failed to send request", map[string]interface{}{"error": err})
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorCF("voice", "Failed to read response", map[string]interface{}{"error": err})
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.ErrorCF("voice", "API error", map[string]interface{}{
			"status_code": resp.StatusCode,
			"response":    string(body),
		})
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	logger.DebugCF("voice", "Received response from STT API", map[string]interface{}{
		"status_code":         resp.StatusCode,
		"response_size_bytes": len(body),
	})

	var result TranscriptionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		logger.ErrorCF("voice", "Failed to unmarshal response", map[string]interface{}{"error": err})
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	logger.InfoCF("voice", "Transcription completed successfully", map[string]interface{}{
		"text_length":           len(result.Text),
		"language":              result.Language,
		"duration_seconds":      result.Duration,
		"transcription_preview": utils.Truncate(result.Text, 50),
	})

	return &result, nil
}

func (t *STTTranscriber) IsAvailable() bool {
	available := t.apiBase != ""
	logger.DebugCF("voice", "Checking transcriber availability", map[string]interface{}{"available": available, "api_base": t.apiBase})
	return available
}
