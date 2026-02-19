package bus

import (
	"context"
	"log"
	"sync"
	"time"
)

type MessageBus struct {
	inbound  chan InboundMessage
	outbound chan OutboundMessage
	handlers map[string]MessageHandler
	closed   bool
	mu       sync.RWMutex
}

func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:  make(chan InboundMessage, 100),
		outbound: make(chan OutboundMessage, 100),
		handlers: make(map[string]MessageHandler),
	}
}

func (mb *MessageBus) PublishInbound(ctx context.Context, msg InboundMessage) {
	mb.publishWithRetry(ctx, mb.inbound, msg, "inbound")
}

func (mb *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	select {
	case msg := <-mb.inbound:
		return msg, true
	case <-ctx.Done():
		return InboundMessage{}, false
	}
}

func (mb *MessageBus) PublishOutbound(ctx context.Context, msg OutboundMessage) {
	mb.publishWithRetry(ctx, mb.outbound, msg, "outbound")
}

func (mb *MessageBus) publishWithRetry(ctx context.Context, ch interface{}, msg interface{}, label string) {
	mb.mu.RLock()
	closed := mb.closed
	mb.mu.RUnlock()

	if closed {
		return
	}

	// Exponential backoff: 10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 500ms
	backoff := 10 * time.Millisecond
	maxBackoff := 500 * time.Millisecond

	for {
		var ok bool
		switch channel := ch.(type) {
		case chan InboundMessage:
			select {
			case channel <- msg.(InboundMessage):
				ok = true
			default:
			}
		case chan OutboundMessage:
			select {
			case channel <- msg.(OutboundMessage):
				ok = true
			default:
			}
		}

		if ok {
			return
		}

		// Channel full, wait with backoff or context cancellation
		select {
		case <-ctx.Done():
			log.Printf("[WARN] bus: dropped %s message due to context cancellation", label)
			return
		case <-time.After(backoff):
			if backoff >= maxBackoff {
				log.Printf("[WARN] bus: dropped %s message after maximum backoff (bus full)", label)
				return
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

func (mb *MessageBus) SubscribeOutbound(ctx context.Context) (OutboundMessage, bool) {
	select {
	case msg := <-mb.outbound:
		return msg, true
	case <-ctx.Done():
		return OutboundMessage{}, false
	}
}

func (mb *MessageBus) RegisterHandler(channel string, handler MessageHandler) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.handlers[channel] = handler
}

func (mb *MessageBus) GetHandler(channel string) (MessageHandler, bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	handler, ok := mb.handlers[channel]
	return handler, ok
}

func (mb *MessageBus) Close() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.closed {
		return
	}
	mb.closed = true
	close(mb.inbound)
	close(mb.outbound)
}
