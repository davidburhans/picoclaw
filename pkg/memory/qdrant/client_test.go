package qdrant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseQdrantAddress(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantHost string
		wantPort int
		wantTLS  bool
	}{
		{
			name:     "HTTP with port 6333",
			input:    "http://192.168.0.70:6333",
			wantHost: "192.168.0.70",
			wantPort: 6334,
			wantTLS:  false,
		},
		{
			name:     "HTTPS without port",
			input:    "https://qdrant.example.com",
			wantHost: "qdrant.example.com",
			wantPort: 6334,
			wantTLS:  true,
		},
		{
			name:     "Bare IP with port 6333",
			input:    "192.168.0.70:6333",
			wantHost: "192.168.0.70",
			wantPort: 6334,
			wantTLS:  false,
		},
		{
			name:     "HTTP without port",
			input:    "http://localhost",
			wantHost: "localhost",
			wantPort: 6334,
			wantTLS:  false,
		},
		{
			name:     "HTTPS with custom port",
			input:    "https://qdrant.example.com:8443",
			wantHost: "qdrant.example.com",
			wantPort: 8443,
			wantTLS:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort, gotTLS := ParseAddress(tt.input)
			assert.Equal(t, tt.wantHost, gotHost)
			assert.Equal(t, tt.wantPort, gotPort)
			assert.Equal(t, tt.wantTLS, gotTLS)
		})
	}
}
