package channels

import (
	"reflect"
	"testing"
)

func TestSplitMessage(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		limit    int
		expected []string
	}{
		{
			name:     "Short message",
			content:  "hello",
			limit:    10,
			expected: []string{"hello"},
		},
		{
			name:     "Exactly at limit",
			content:  "1234567890",
			limit:    10,
			expected: []string{"1234567890"},
		},
		{
			name:     "Split at limit no newline",
			content:  "123456789012345",
			limit:    10,
			expected: []string{"1234567890", "12345"},
		},
		{
			name:     "Split at newline",
			content:  "12345\n67890",
			limit:    7,
			expected: []string{"12345", "67890"},
		},
		{
			name:     "Priority to newline",
			content:  "123\n4567890",
			limit:    8,
			expected: []string{"123", "4567890"},
		},
		{
			name:     "Multiple newlines",
			content:  "line1\nline2\nline3",
			limit:    12,
			expected: []string{"line1\nline2", "line3"},
		},
		{
			name:     "Exceeding limit with no newline",
			content:  "abcdefghij",
			limit:    5,
			expected: []string{"abcde", "fghij"},
		},
		{
			name:     "Unicode characters",
			content:  "你好世界！",
			limit:    2,
			expected: []string{"你好", "世界", "！"},
		},
		{
			name:     "Split with whitespace around newline",
			content:  "first part\nsecond part",
			limit:    12,
			expected: []string{"first part", "second part"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitMessage(tt.content, tt.limit)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("splitMessage() = %v, want %v", got, tt.expected)
			}
		})
	}
}
