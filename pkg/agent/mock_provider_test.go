package agent

import (
	"context"

	"github.com/sipeed/picoclaw/pkg/providers"
)

type mockProvider struct{}

func (m *mockProvider) Chat(ctx context.Context, messages []providers.Message, tools []providers.ToolDefinition, model string, opts map[string]interface{}) (*providers.LLMResponse, error) {
	return &providers.LLMResponse{
		Content:   "Mock response",
		ToolCalls: []providers.ToolCall{},
	}, nil
}

func (m *mockProvider) GetID() string {
	return "mock"
}

func (m *mockProvider) GetDefaultModel() string {
	return "mock-model"
}

func (m *mockProvider) GetMaxTokens() int {
	return 8192
}

func (m *mockProvider) GetTemperature() float64 {
	return 0.7
}

func (m *mockProvider) GetMaxToolIterations() int {
	return 20
}

func (m *mockProvider) GetTimeout() int {
	return 300
}

func (m *mockProvider) GetMaxConcurrent() int {
	return 1
}
