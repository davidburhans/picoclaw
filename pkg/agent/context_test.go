package agent

import (
	"strings"
	"testing"

	"github.com/sipeed/picoclaw/pkg/providers"
)

func TestContextBuilder_AgentName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Default",
			input:    "",
			expected: "picoclaw",
		},
		{
			name:     "Custom",
			input:    "MyAgent",
			expected: "MyAgent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cb := NewContextBuilder("/tmp/workspace", tt.input)
			identity := cb.getIdentity()
			
			if !strings.Contains(identity, "# "+tt.expected) {
				t.Errorf("Identity header should contain %s, got: %s", tt.expected, identity)
			}
			if !strings.Contains(identity, "You are "+tt.expected) {
				t.Errorf("Identity body should contain %s, got: %s", tt.expected, identity)
			}
		})
	}
}

func TestContextBuilder_BuildMessages_RoleMerging(t *testing.T) {
	cb := NewContextBuilder("/tmp", "test-agent")
	
	history := []providers.Message{
		{Role: "user", Content: "Message 1"},
		// Missing assistant message here
	}
	
	current := "Message 2"
	
	messages := cb.BuildMessages(history, "", current, nil, "test", "chat")
	
	// Expected: [system, user (merged 1 & 2)]
	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(messages))
	}
	
	if messages[0].Role != "system" {
		t.Errorf("Expected first message to be system, got %s", messages[0].Role)
	}
	
	if messages[1].Role != "user" {
		t.Errorf("Expected second message to be user, got %s", messages[1].Role)
	}
	
	expectedContent := "Message 1\n\nMessage 2"
	if messages[1].Content != expectedContent {
		t.Errorf("Expected merged content '%s', got '%s'", expectedContent, messages[1].Content)
	}
}

func TestContextBuilder_BuildMessages_OrphanedToolRemoval(t *testing.T) {
	cb := NewContextBuilder("/tmp", "test-agent")
	
	history := []providers.Message{
		{Role: "tool", Content: "Tool Result", ToolCallID: "call_1"}, // Orphaned
		{Role: "user", Content: "Message 1"},
	}
	
	current := "Message 2"
	
	messages := cb.BuildMessages(history, "", current, nil, "test", "chat")
	
	// Expected: [system, user (merged 1 & 2)]
	// Orphaned tool should be removed.
	if len(messages) != 2 {
		t.Fatalf("Expected 2 messages, got %d", len(messages))
	}
	
	for _, m := range messages {
		if m.Role == "tool" {
			t.Errorf("Found orphaned tool message that should have been removed")
		}
	}
}
