package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"telegram:123456", "telegram_123456"},
		{"discord:987654321", "discord_987654321"},
		{"slack:C01234", "slack_C01234"},
		{"no-colons-here", "no-colons-here"},
		{"multiple:colons:here", "multiple_colons_here"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSave_WithColonInKey(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	// Create a session with a key containing colon (typical channel session key).
	key := "telegram:123456"
	sm.GetOrCreate(key)
	sm.AddMessage(key, "user", "hello")

	// Save should succeed even though the key contains ':'
	if err := sm.Save(key); err != nil {
		t.Fatalf("Save(%q) failed: %v", key, err)
	}

	// The file on disk should use sanitized name.
	expectedFile := filepath.Join(tmpDir, "telegram_123456.json")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Fatalf("expected session file %s to exist", expectedFile)
	}

	// Load into a fresh manager and verify the session round-trips.
	sm2 := NewSessionManager(tmpDir)
	history := sm2.GetHistory(key)
	if len(history) != 1 {
		t.Fatalf("expected 1 message after reload, got %d", len(history))
	}
	if history[0].Content != "hello" {
		t.Errorf("expected message content %q, got %q", "hello", history[0].Content)
	}
}

func TestSave_RejectsPathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	sm := NewSessionManager(tmpDir)

	badKeys := []string{"", ".", "..", "foo/bar", "foo\\bar"}
	for _, key := range badKeys {
		sm.GetOrCreate(key)
		if err := sm.Save(key); err == nil {
			t.Errorf("Save(%q) should have failed but didn't", key)
		}
	}
}

func TestLazyLoading(t *testing.T) {
	tmpDir := t.TempDir()
	key := "test:lazy"

	// 1. Create a session and save it
	sm1 := NewSessionManager(tmpDir)
	sm1.GetOrCreate(key)
	sm1.AddMessage(key, "user", "message 1")
	sm1.AddMessage(key, "assistant", "response 1")
	if err := sm1.Save(key); err != nil {
		t.Fatalf("Failed to save: %v", err)
	}

	// 2. Load into sm2 - should be lazy initially
	sm2 := NewSessionManager(tmpDir)

	sm2.mu.RLock()
	session, ok := sm2.sessions[key]
	sm2.mu.RUnlock()

	if !ok {
		t.Fatalf("Session not found in sm2")
	}
	if session.Messages != nil {
		t.Errorf("Expected Messages to be nil (lazy) initially")
	}

	// 3. Trigger load via GetHistory
	history := sm2.GetHistory(key)
	if len(history) != 2 {
		t.Fatalf("Expected 2 messages after lazy load, got %d", len(history))
	}
	if history[0].Content != "message 1" {
		t.Errorf("Expected content 'message 1', got %q", history[0].Content)
	}

	// 4. Verify in-memory state is now loaded
	sm2.mu.RLock()
	if session.Messages == nil {
		t.Errorf("Expected Messages to be loaded (non-nil) after access")
	}
	sm2.mu.RUnlock()
}
