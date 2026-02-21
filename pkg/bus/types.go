package bus

type InboundMessage struct {
	Channel    string            `json:"channel"`
	SenderID   string            `json:"sender_id"`
	ChatID     string            `json:"chat_id"`
	Content    string            `json:"content"`
	Media      []string          `json:"media,omitempty"`
	SessionKey string            `json:"session_key"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

const (
	MessageTypeText     = "text"
	MessageTypeTyping   = "typing"
	MessageTypeReaction = "reaction"
	MessageTypeAudio    = "audio"
)

type OutboundMessage struct {
	Channel  string            `json:"channel"`
	ChatID   string            `json:"chat_id"`
	Content  string            `json:"content"`
	Type     string            `json:"type,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
	Audio    string            `json:"audio,omitempty"`
}

type MessageHandler func(InboundMessage) error
