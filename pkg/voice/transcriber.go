package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
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

func NewTranscriber(sttCfg *config.STTConfig) Transcriber {
	if sttCfg == nil || !sttCfg.Enabled || sttCfg.APIBase == "" {
		logger.DebugCF("voice", "STT not configured or disabled", nil)
		return &STTTranscriber{
			httpClient: &http.Client{Timeout: 60 * time.Second},
		}
	}

	language := sttCfg.Language
	if language == "" {
		language = "auto"
	}

	model := sttCfg.Model
	if model == "" {
		model = "whisper-large-v3"
	}

	logger.DebugCF("voice", "Creating STT transcriber", map[string]interface{}{
		"model":       model,
		"api_base":    sttCfg.APIBase,
		"has_api_key": sttCfg.APIKey != "",
		"language":    language,
	})

	return &STTTranscriber{
		apiKey:     sttCfg.APIKey,
		apiBase:    sttCfg.APIBase,
		model:      model,
		language:   language,
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
		logger.ErrorCF("voice", "Failed to open audio file", map[string]any{"path": audioFilePath, "error": err})
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	fileInfo, err := audioFile.Stat()
	if err != nil {
		logger.ErrorCF("voice", "Failed to get file info", map[string]any{"path": audioFilePath, "error": err})
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	logger.DebugCF("voice", "Audio file details", map[string]any{
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

		logger.DebugCF("voice", "Copied audio file to multipart", map[string]interface{}{"bytes_copied": copied})

		if err := writer.WriteField("model", t.model); err != nil {
			logger.ErrorCF("voice", "Failed to write model field", map[string]interface{}{"error": err.Error()})
			return
		}

		if t.language != "" && t.language != "auto" {
			if err := writer.WriteField("language", t.language); err != nil {
				logger.ErrorCF("voice", "Failed to write language field", map[string]interface{}{"error": err.Error()})
				return
			}
		}

		if err := writer.Close(); err != nil {
			logger.ErrorCF("voice", "Failed to close writer", map[string]interface{}{"error": err.Error()})
			return
		}
	}()

	url := t.apiBase + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, pr)
	if err != nil {
		logger.ErrorCF("voice", "Failed to create request", map[string]interface{}{"error": err.Error()})
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	logger.DebugCF("voice", "Sending transcription request", map[string]interface{}{
		"url":       url,
		"file_size": fileInfo.Size(),
	})

	resp, err := t.httpClient.Do(req)
	if err != nil {
		logger.ErrorCF("voice", "Failed to send request", map[string]interface{}{"error": err.Error()})
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorCF("voice", "Failed to read response", map[string]interface{}{"error": err.Error()})
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.ErrorCF("voice", "Transcription API error", map[string]interface{}{
			"status_code": resp.StatusCode,
			"response":    string(body),
		})
		return nil, fmt.Errorf("transcription API error (status %d): %s", resp.StatusCode, string(body))
	}

	logger.DebugCF("voice", "Transcription API response", map[string]interface{}{
		"status_code": resp.StatusCode,
		"response":    string(body),
	})

	var result TranscriptionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		logger.ErrorCF("voice", "Failed to unmarshal transcription response", map[string]interface{}{
			"error":    err.Error(),
			"response": string(body),
		})
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	logger.InfoCF("voice", "Transcription completed", map[string]interface{}{
		"text":       utils.Truncate(result.Text, 50),
		"language":   result.Language,
		"duration":   result.Duration,
	})

	return &result, nil
}

func (t *STTTranscriber) IsAvailable() bool {
	available := t.apiBase != ""
	logger.DebugCF("voice", "Checking transcriber availability", map[string]interface{}{"available": available, "api_base": t.apiBase})
	return available
}

// GroqTranscriber uses Groq's Whisper API for speech-to-text
type GroqTranscriber struct {
	apiKey     string
	apiBase    string
	httpClient *http.Client
}

func NewGroqTranscriber(apiKey string) *GroqTranscriber {
	logger.DebugCF("voice", "Creating Groq transcriber", map[string]any{"has_api_key": apiKey != ""})

	apiBase := "https://api.groq.com/openai/v1"
	return &GroqTranscriber{
		apiKey:  apiKey,
		apiBase: apiBase,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (t *GroqTranscriber) Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error) {
	logger.InfoCF("voice", "Starting transcription", map[string]any{"audio_file": audioFilePath})

	audioFile, err := os.Open(audioFilePath)
	if err != nil {
		logger.ErrorCF("voice", "Failed to open audio file", map[string]any{"path": audioFilePath, "error": err})
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	fileInfo, err := audioFile.Stat()
	if err != nil {
		logger.ErrorCF("voice", "Failed to get file info", map[string]any{"path": audioFilePath, "error": err})
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	logger.DebugCF("voice", "Audio file details", map[string]any{
		"size_bytes": fileInfo.Size(),
		"file_name":  filepath.Base(audioFilePath),
	})

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	part, err := writer.CreateFormFile("file", filepath.Base(audioFilePath))
	if err != nil {
		logger.ErrorCF("voice", "Failed to create form file", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	copied, err := io.Copy(part, audioFile)
	if err != nil {
		logger.ErrorCF("voice", "Failed to copy file content", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}

	logger.DebugCF("voice", "File copied to request", map[string]any{"bytes_copied": copied})

	if err = writer.WriteField("model", "whisper-large-v3"); err != nil {
		logger.ErrorCF("voice", "Failed to write model field", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	if err = writer.WriteField("response_format", "json"); err != nil {
		logger.ErrorCF("voice", "Failed to write response_format field", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to write response_format field: %w", err)
	}

	if err = writer.Close(); err != nil {
		logger.ErrorCF("voice", "Failed to close multipart writer", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	url := t.apiBase + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, &requestBody)
	if err != nil {
		logger.ErrorCF("voice", "Failed to create request", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	logger.DebugCF("voice", "Sending transcription request to Groq API", map[string]any{
		"url":                url,
		"request_size_bytes": requestBody.Len(),
		"file_size_bytes":    fileInfo.Size(),
	})

	resp, err := t.httpClient.Do(req)
	if err != nil {
		logger.ErrorCF("voice", "Failed to send request", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorCF("voice", "Failed to read response", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.ErrorCF("voice", "API error", map[string]any{
			"status_code": resp.StatusCode,
			"response":    string(body),
		})
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	logger.DebugCF("voice", "Received response from Groq API", map[string]any{
		"status_code":         resp.StatusCode,
		"response_size_bytes": len(body),
	})

	var result TranscriptionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		logger.ErrorCF("voice", "Failed to unmarshal response", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	logger.InfoCF("voice", "Transcription completed successfully", map[string]any{
		"text_length":           len(result.Text),
		"language":              result.Language,
		"duration_seconds":      result.Duration,
		"transcription_preview": utils.Truncate(result.Text, 50),
	})

	return &result, nil
}

func (t *GroqTranscriber) IsAvailable() bool {
	available := t.apiKey != ""
	logger.DebugCF("voice", "Checking transcriber availability", map[string]any{"available": available})
	return available
}
