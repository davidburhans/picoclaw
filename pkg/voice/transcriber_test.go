package voice

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestNewTranscriber_WithSTTConfig(t *testing.T) {
	sttCfg := &config.STTConfig{
		Enabled:  true,
		APIBase:  "http://localhost:8000/v1",
		APIKey:   "test-key",
		Model:    "whisper-large-v3",
		Language: "en",
	}

	transcriber := NewTranscriber(sttCfg)

	if !transcriber.IsAvailable() {
		t.Error("Expected transcriber to be available with valid STT config")
	}
}

func TestNewTranscriber_NilConfig(t *testing.T) {
	transcriber := NewTranscriber(nil)

	if transcriber.IsAvailable() {
		t.Error("Expected transcriber to NOT be available with nil config")
	}
}

func TestNewTranscriber_Disabled(t *testing.T) {
	sttCfg := &config.STTConfig{
		Enabled: false,
		APIBase: "http://localhost:8000/v1",
		APIKey:  "test-key",
		Model:   "whisper-large-v3",
	}

	transcriber := NewTranscriber(sttCfg)

	if transcriber.IsAvailable() {
		t.Error("Expected transcriber to NOT be available when disabled")
	}
}

func TestNewTranscriber_EmptyAPIBase(t *testing.T) {
	sttCfg := &config.STTConfig{
		Enabled: true,
		APIBase: "",
		APIKey:  "test-key",
		Model:   "whisper-large-v3",
	}

	transcriber := NewTranscriber(sttCfg)

	if transcriber.IsAvailable() {
		t.Error("Expected transcriber to NOT be available with empty api_base")
	}
}

func TestNewTranscriber_DefaultModel(t *testing.T) {
	sttCfg := &config.STTConfig{
		Enabled: true,
		APIBase: "https://api.groq.com/openai/v1",
		APIKey:  "test-key",
		Model:   "", // should default to whisper-large-v3
	}

	transcriber := NewTranscriber(sttCfg)

	if !transcriber.IsAvailable() {
		t.Error("Expected transcriber to be available")
	}

	stt := transcriber.(*STTTranscriber)
	if stt.model != "whisper-large-v3" {
		t.Errorf("Expected default model whisper-large-v3, got %s", stt.model)
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
