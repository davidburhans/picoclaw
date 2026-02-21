package tools

import (
	"context"
	"sync"
)

type SpeakCallback func(channel, chatID, content string) error

type SpeakTool struct {
	sendCallback   SpeakCallback
	defaultChannel string
	defaultChatID  string
	mu             sync.RWMutex
}

func NewSpeakTool() *SpeakTool {
	return &SpeakTool{}
}

func (t *SpeakTool) Name() string {
	return "speak"
}

func (t *SpeakTool) Description() string {
	return "Speak the message using text-to-speech. Use this when you want to talk to the user with your voice."
}

func (t *SpeakTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The text to speak",
			},
			"channel": map[string]interface{}{
				"type":        "string",
				"description": "Optional: target channel (telegram, discord, etc.)",
			},
			"chat_id": map[string]interface{}{
				"type":        "string",
				"description": "Optional: target chat/user ID",
			},
		},
		"required": []string{"content"},
	}
}

func (t *SpeakTool) SetContext(channel, chatID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.defaultChannel = channel
	t.defaultChatID = chatID
}

func (t *SpeakTool) SetCallback(callback SpeakCallback) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sendCallback = callback
}

func (t *SpeakTool) Execute(ctx context.Context, args map[string]interface{}) *ToolResult {
	t.mu.RLock()
	channel := t.defaultChannel
	chatID := t.defaultChatID
	callback := t.sendCallback
	t.mu.RUnlock()

	if callback == nil {
		return ErrorResult("speak tool not configured")
	}

	content, _ := args["content"].(string)
	if content == "" {
		return ErrorResult("content is required")
	}

	if ch, ok := args["channel"].(string); ok && ch != "" {
		channel = ch
	}
	if id, ok := args["chat_id"].(string); ok && id != "" {
		chatID = id
	}

	if channel == "" || chatID == "" {
		return ErrorResult("channel and chat_id are required")
	}

	if err := callback(channel, chatID, content); err != nil {
		return ErrorResult(err.Error())
	}

	return SilentResult("Spoke: " + content)
}
