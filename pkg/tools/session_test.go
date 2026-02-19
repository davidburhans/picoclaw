package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
)

func TestReadSessionTool(t *testing.T) {
	// Setup
	tmpDir, err := os.MkdirTemp("", "session-tool-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sessionsDir := filepath.Join(tmpDir, "sessions")
	os.MkdirAll(sessionsDir, 0755)

	// Create a dummy session file
	sess := session.Session{
		Key:     "test_session_v1",
		Created: time.Now(),
		Summary: "Test summary",
		Messages: []providers.Message{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi there"},
		},
	}
	data, _ := json.Marshal(sess)
	os.WriteFile(filepath.Join(sessionsDir, "test_session_v1.json"), data, 0644)

	// Test tool
	tool := NewReadSessionTool(tmpDir)

	// Case 1: Valid session
	result := tool.Execute(context.Background(), map[string]interface{}{
		"session_key": "test_session_v1",
	})
	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.ForLLM)
	}
	if result.ForLLM == "" {
		t.Error("Expected content, got empty string")
	}

	// Verify content format
	if !strings.Contains(result.ForLLM, "test_session_v1") {
		t.Error("Output should contain session key")
	}
	if !strings.Contains(result.ForLLM, "Hello") {
		t.Error("Output should contain message content")
	}

	// Case 2: Invalid session
	result = tool.Execute(context.Background(), map[string]interface{}{
		"session_key": "nonexistent",
	})
	if !result.IsError {
		t.Error("Expected error for non-existent session")
	}
}
