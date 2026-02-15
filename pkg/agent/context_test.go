package agent

import (
	"strings"
	"testing"
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
