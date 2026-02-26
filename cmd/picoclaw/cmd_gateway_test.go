package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sipeed/picoclaw/pkg/config"
)

type mockWebhookProcessor struct {
	called     bool
	content    string
	sessionKey string
	channel    string
	chatID     string
}

func (m *mockWebhookProcessor) ProcessDirectWithChannel(ctx context.Context, content string, sessionKey string, channel, chatID string) (string, error) {
	m.called = true
	m.content = content
	m.sessionKey = sessionKey
	m.channel = channel
	m.chatID = chatID
	return "ok", nil
}

func TestWebhookHandler(t *testing.T) {
	cfg := &config.Config{
		Gateway: config.GatewayConfig{
			Webhooks: map[string]config.WebhookConfig{
				"test-id": {
					Format: "json",
					Agent:  "default",
				},
				"github-id": {
					Format: "github",
					Secret: "my_secret",
					Agent:  "my_agent",
				},
			},
		},
	}

	tests := []struct {
		name           string
		method         string
		path           string
		body           string
		headers        map[string]string
		expectedStatus int
		expectCalled   bool
	}{
		{
			name:           "Method Not Allowed (GET)",
			method:         http.MethodGet,
			path:           "/webhook/test-id",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Not Found (Wrong Path)",
			method:         http.MethodPost,
			path:           "/wrong/test-id",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Webhook Not Found in Config",
			method:         http.MethodPost,
			path:           "/webhook/unknown-id",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Valid JSON Webhook",
			method:         http.MethodPost,
			path:           "/webhook/test-id",
			body:           `{"event": "push"}`,
			expectedStatus: http.StatusOK,
			expectCalled:   true,
		},
		{
			name:           "GitHub Webhook Missing Signature",
			method:         http.MethodPost,
			path:           "/webhook/github-id",
			body:           `{"event": "push"}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "GitHub Webhook Invalid Signature format",
			method:         http.MethodPost,
			path:           "/webhook/github-id",
			headers:        map[string]string{"X-Hub-Signature-256": "bad:format"},
			body:           `{"event": "push"}`,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "GitHub Webhook Invalid Signature value",
			method:         http.MethodPost,
			path:           "/webhook/github-id",
			headers:        map[string]string{"X-Hub-Signature-256": "sha256=abcdef"},
			body:           `{"event": "push"}`,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Valid GitHub Webhook",
			method:         http.MethodPost,
			path:           "/webhook/github-id",
			headers:        map[string]string{"X-GitHub-Event": "push"},
			body:           `{"event": "push"}`,
			expectedStatus: http.StatusOK,
			expectCalled:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProcessor := &mockWebhookProcessor{}
			handler := webhookHandler(mockProcessor, cfg)

			var reqBody *bytes.Buffer
			if tt.body != "" {
				reqBody = bytes.NewBufferString(tt.body)
			} else {
				reqBody = bytes.NewBufferString("")
			}

			req := httptest.NewRequest(tt.method, tt.path, reqBody)

			if tt.name == "Valid GitHub Webhook" {
				mac := hmac.New(sha256.New, []byte("my_secret"))
				mac.Write([]byte(tt.body))
				sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
				req.Header.Set("X-Hub-Signature-256", sig)
			}

			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v",
					status, tt.expectedStatus)
			}

			if mockProcessor.called != tt.expectCalled {
				t.Errorf("expected processor.ProcessDirectWithChannel called to be %v, got %v",
					tt.expectCalled, mockProcessor.called)
			}
		})
	}
}
