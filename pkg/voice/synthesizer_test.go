package voice

import (
	"context"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestNewSynthesizer_Disabled(t *testing.T) {
	cfg := &config.TTSConfig{
		Enabled: false,
	}

	synth := NewSynthesizer(cfg)
	if synth.IsAvailable() {
		t.Error("Expected synthesizer to be unavailable when disabled")
	}
}

func TestNewSynthesizer_NoServerURL(t *testing.T) {
	cfg := &config.TTSConfig{
		Enabled:   true,
		ServerURL: "",
	}

	synth := NewSynthesizer(cfg)
	if synth.IsAvailable() {
		t.Error("Expected synthesizer to be unavailable with empty server URL")
	}
}

func TestNewSynthesizer_ValidConfig(t *testing.T) {
	cfg := &config.TTSConfig{
		Enabled:     true,
		ServerURL:   "http://localhost:8004",
		VoiceID:     "Emily.wav",
		Language:    "en",
		Temperature: 0.8,
	}

	synth := NewSynthesizer(cfg)
	if !synth.IsAvailable() {
		t.Error("Expected synthesizer to be available with valid config")
	}
}

func TestNewSynthesizer_NilConfig(t *testing.T) {
	synth := NewSynthesizer(nil)
	if synth.IsAvailable() {
		t.Error("Expected synthesizer to be unavailable with nil config")
	}
}

func TestTTSSynthesizer_Synthesize_NotAvailable(t *testing.T) {
	synth := &TTSSynthesizer{
		serverURL: "",
	}

	_, err := synth.Synthesize(context.Background(), "test")
	if err == nil {
		t.Error("Expected error when synthesizer is not available")
	}
}

func TestStripThinkTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple think block",
			input:    "<think>\nThe user is saying hello.\n</think>\n\nHello!",
			expected: "Hello!",
		},
		{
			name:     "multiline think block",
			input:    "<think>\nLine 1\nLine 2\nLine 3\n</think>\n\nResponse text here.",
			expected: "Response text here.",
		},
		{
			name:     "no think tags",
			input:    "Just a normal response.",
			expected: "Just a normal response.",
		},
		{
			name:     "only think tags",
			input:    "<think>\nInternal reasoning only.\n</think>",
			expected: "",
		},
		{
			name:     "multiple think blocks",
			input:    "<think>first</think> Hello <think>second</think> World",
			expected: "Hello  World",
		},
		{
			name:     "think tags inline",
			input:    "<think>reasoning</think>Hello!",
			expected: "Hello!",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripThinkTags(tt.input)
			if result != tt.expected {
				t.Errorf("stripThinkTags(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
