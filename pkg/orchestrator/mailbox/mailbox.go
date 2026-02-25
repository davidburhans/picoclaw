package mailbox

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Message represents an inter-instance message.
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Content   string    `json:"content"`
	Read      bool      `json:"read"`
	Timestamp time.Time `json:"timestamp"`
}

// MemoryStore is an in-memory implementation of the mailbox store.
type MemoryStore struct {
	mu       sync.RWMutex
	messages map[string]*Message
}

// NewMemoryStore creates a new in-memory mailbox.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		messages: make(map[string]*Message),
	}
}

// SendMessage sends a message from one user to another.
func (s *MemoryStore) SendMessage(ctx context.Context, from, to, content string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := uuid.New().String()
	msg := &Message{
		ID:        id,
		From:      from,
		To:        to,
		Content:   content,
		Read:      false,
		Timestamp: time.Now(),
	}
	s.messages[id] = msg
	return id, nil
}

// ListMessages returns all messages for a user (either sent or received).
// According to test logic, this returns messages directed to the user,
// or sent by the user (if we want sent messages?). But let's just do received messages for now 
// or maybe both, wait, let's check what test does: Kid receives, lists kid - sees 1 msg.
// ReadMessage test: Kid sends to Dad, Dad lists - sees 1 msg.
func (s *MemoryStore) ListMessages(ctx context.Context, user string) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Message
	for _, msg := range s.messages {
		if msg.To == user || msg.From == user {
			// Actually the test only lists mailbox. Let's return received messages only for inbox?
			// The second test sends from kid to dad, then dad lists and expects 1. Yes, To == user.
			// Let's just return To == user for a strict inbox, but what if they want to see sent? Let's just do To == user.
			if msg.To == user {
				// return a copy
				result = append(result, *msg)
			}
		}
	}
	return result, nil
}

// ReadMessage reads a specific message, marking it as read, provided the user is authorized.
func (s *MemoryStore) ReadMessage(ctx context.Context, user, msgID string) (*Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg, ok := s.messages[msgID]
	if !ok {
		return nil, fmt.Errorf("message not found")
	}

	// Only the recipient can read it? Or maybe the sender can read it too?
	// The test `ReadMessage unauthorized` checks that "mom" sends to "kid", and "dad" gets "unauthorized".
	if msg.To != user {
		return nil, fmt.Errorf("unauthorized")
	}

	msg.Read = true

	// return a copy
	msgCopy := *msg
	return &msgCopy, nil
}
