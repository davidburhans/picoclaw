package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

type Synthesizer interface {
	Synthesize(ctx context.Context, text string) (string, error)
	IsAvailable() bool
}

type TTSRequest struct {
	Text              string  `json:"text"`
	Temperature       float64 `json:"temperature"`
	Exaggeration      float64 `json:"exaggeration"`
	CfgWeight         float64 `json:"cfg_weight"`
	SpeedFactor       float64 `json:"speed_factor"`
	Seed              int     `json:"seed"`
	Language          string  `json:"language"`
	VoiceMode         string  `json:"voice_mode"`
	SplitText         bool    `json:"split_text"`
	ChunkSize         int     `json:"chunk_size"`
	OutputFormat      string  `json:"output_format"`
	PredefinedVoiceID string  `json:"predefined_voice_id"`
}

type TTSSynthesizer struct {
	serverURL    string
	voiceID      string
	language     string
	temperature  float64
	exaggeration float64
	cfgWeight    float64
	speedFactor  float64
	seed         int
	chunkSize    int
	splitText    bool
	outputFormat string
	httpClient   *http.Client
}

func NewSynthesizer(ttsCfg *config.TTSConfig) Synthesizer {
	if ttsCfg == nil || !ttsCfg.Enabled || ttsCfg.ServerURL == "" {
		logger.DebugCF("voice", "TTS not configured or disabled", nil)
		return &TTSSynthesizer{
			serverURL:  "",
			httpClient: &http.Client{Timeout: 120 * time.Second},
		}
	}

	logger.DebugCF("voice", "Creating TTS synthesizer", map[string]interface{}{
		"server_url": ttsCfg.ServerURL,
		"voice_id":   ttsCfg.VoiceID,
		"language":   ttsCfg.Language,
		"auto_play":  ttsCfg.AutoPlay,
	})

	return &TTSSynthesizer{
		serverURL:    ttsCfg.ServerURL,
		voiceID:      ttsCfg.VoiceID,
		language:     ttsCfg.Language,
		temperature:  ttsCfg.Temperature,
		exaggeration: ttsCfg.Exaggeration,
		cfgWeight:    ttsCfg.CfgWeight,
		speedFactor:  ttsCfg.SpeedFactor,
		seed:         ttsCfg.Seed,
		chunkSize:    ttsCfg.ChunkSize,
		splitText:    ttsCfg.SplitText,
		outputFormat: ttsCfg.OutputFormat,
		httpClient:   &http.Client{Timeout: 120 * time.Second},
	}
}

// thinkTagRe matches <think>...</think> blocks (including multiline content).
var thinkTagRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

// stripThinkTags removes <think>...</think> blocks and trims surrounding whitespace.
func stripThinkTags(text string) string {
	return strings.TrimSpace(thinkTagRe.ReplaceAllString(text, ""))
}

func (s *TTSSynthesizer) Synthesize(ctx context.Context, text string) (string, error) {
	if !s.IsAvailable() {
		return "", fmt.Errorf("TTS synthesizer not available")
	}

	// Strip <think>...</think> blocks so internal reasoning is not spoken aloud.
	text = stripThinkTags(text)
	if text == "" {
		logger.DebugCF("voice", "Skipping TTS: text is empty after stripping think tags", nil)
		return "", nil
	}

	logger.InfoCF("voice", "Starting TTS synthesis", map[string]interface{}{
		"text_length": len(text),
		"server_url":  s.serverURL,
		"voice_id":    s.voiceID,
	})

	reqBody := TTSRequest{
		Text:              text,
		Temperature:       s.temperature,
		Exaggeration:      s.exaggeration,
		CfgWeight:         s.cfgWeight,
		SpeedFactor:       s.speedFactor,
		Seed:              s.seed,
		Language:          s.language,
		VoiceMode:         "predefined",
		SplitText:         s.splitText,
		ChunkSize:         s.chunkSize,
		OutputFormat:      s.outputFormat,
		PredefinedVoiceID: s.voiceID,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		logger.ErrorCF("voice", "Failed to marshal TTS request", map[string]interface{}{"error": err})
		return "", fmt.Errorf("failed to marshal TTS request: %w", err)
	}

	url := s.serverURL + "/tts"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.ErrorCF("voice", "Failed to create TTS request", map[string]interface{}{"error": err})
		return "", fmt.Errorf("failed to create TTS request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	logger.DebugCF("voice", "Sending TTS request", map[string]interface{}{
		"url":         url,
		"text_length": len(text),
	})

	resp, err := s.httpClient.Do(req)
	if err != nil {
		logger.ErrorCF("voice", "Failed to send TTS request", map[string]interface{}{"error": err})
		return "", fmt.Errorf("failed to send TTS request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.ErrorCF("voice", "TTS API error", map[string]interface{}{
			"status_code": resp.StatusCode,
			"response":    string(body),
		})
		return "", fmt.Errorf("TTS API error (status %d): %s", resp.StatusCode, string(body))
	}

	tmpDir := os.TempDir()
	tmpFile := filepath.Join(tmpDir, fmt.Sprintf("tts_output_%d.wav", time.Now().UnixNano()))

	outFile, err := os.Create(tmpFile)
	if err != nil {
		logger.ErrorCF("voice", "Failed to create temp file", map[string]interface{}{"error": err})
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		logger.ErrorCF("voice", "Failed to write audio to file", map[string]interface{}{"error": err})
		return "", fmt.Errorf("failed to write audio to file: %w", err)
	}

	fileInfo, err := outFile.Stat()
	if err != nil {
		logger.WarnCF("voice", "Failed to get file info", map[string]interface{}{"error": err})
	}

	var fileSize int64
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

	logger.InfoCF("voice", "TTS synthesis completed", map[string]interface{}{
		"output_file": tmpFile,
		"file_size":   fileSize,
	})

	return tmpFile, nil
}

func (s *TTSSynthesizer) IsAvailable() bool {
	available := s.serverURL != ""
	logger.DebugCF("voice", "Checking TTS availability", map[string]interface{}{"available": available, "server_url": s.serverURL})
	return available
}
