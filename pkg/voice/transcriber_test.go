package voice

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestNewTranscriber_WithModelConfig(t *testing.T) {
	modelCfg := &config.ModelConfig{
		ModelName: "whisper-local",
		Model:     "large-v3",
		Provider:  "stt",
		APIBase:   "http://localhost:8000/v1",
		APIKey:    "",
	}

	voiceCfg := &config.VoiceConfig{
		Model:    "whisper-local",
		Language: "auto",
	}

	transcriber := NewTranscriber(modelCfg, voiceCfg)

	if !transcriber.IsAvailable() {
		t.Error("Expected transcriber to be available with valid api_base")
	}
}

func TestNewTranscriber_NilConfig(t *testing.T) {
	voiceCfg := &config.VoiceConfig{
		Model:    "",
		Language: "auto",
	}

	transcriber := NewTranscriber(nil, voiceCfg)

	if transcriber.IsAvailable() {
		t.Error("Expected transcriber to NOT be available with nil config")
	}
}

func TestNewTranscriber_EmptyModel(t *testing.T) {
	modelCfg := &config.ModelConfig{
		ModelName: "",
		Model:     "",
		Provider:  "stt",
	}

	voiceCfg := &config.VoiceConfig{
		Model:    "",
		Language: "auto",
	}

	transcriber := NewTranscriber(modelCfg, voiceCfg)

	if transcriber.IsAvailable() {
		t.Error("Expected transcriber to NOT be available with empty model config")
	}
}

func TestNewTranscriber_LegacyGroq(t *testing.T) {
	transcriber := NewGroqTranscriber("test-api-key")

	if !transcriber.IsAvailable() {
		t.Error("Expected legacy Groq transcriber to be available with API key")
	}

	if transcriber.apiBase != "https://api.groq.com/openai/v1" {
		t.Errorf("Expected apiBase to be Groq URL, got %s", transcriber.apiBase)
	}
}

func TestSTTTranscriber_IsAvailable(t *testing.T) {
	tests := []struct {
		name     string
		apiBase  string
		expected bool
	}{
		{"empty apiBase", "", false},
		{"with apiBase", "http://localhost:8000/v1", true},
		{"with https", "https://api.groq.com/openai/v1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transcriber := &STTTranscriber{
				apiBase: tt.apiBase,
			}
			if got := transcriber.IsAvailable(); got != tt.expected {
				t.Errorf("IsAvailable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSTTTranscriber_Transcribe_NoApiBase(t *testing.T) {
	transcriber := &STTTranscriber{
		apiBase: "",
	}

	_, err := transcriber.Transcribe(context.Background(), "/nonexistent/audio.ogg")
	if err == nil {
		t.Error("Expected error when transcriber is not available")
	}
}
