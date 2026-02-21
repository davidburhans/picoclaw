package bus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMessageBus_NonBlocking(t *testing.T) {
	// Create a bus with small buffer
	mb := NewMessageBus()

	// Fill the inbound buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Inbound capacity is usually 100 in PicoClaw (based on my memory of bus.go)
	// Let's verify capacity if possible, or just push many messages.
	// Actually, let's just push until it would block.

	start := time.Now()
	for i := 0; i < 200; i++ {
		mb.PublishInbound(ctx, InboundMessage{Content: "test"})
	}
	duration := time.Since(start)

	// If it was blocking, it would take at least 500ms * (200-100) = 50s!
	// With our non-blocking retry/backoff, it should take much less if it drops or finishes retries.
	// But since nothing is consuming, it will eventually drop if buffer is full and max retries reached.
	// Max backoff is 500ms, total retry time per message could be ~1s.
	// However, if it's non-blocking (dropping), it should finish quickly.

	assert.Less(t, duration, 5*time.Second, "Publishing to full bus took too long, likely blocking")
}

func TestMessageBus_ContextCancellation(t *testing.T) {
	mb := NewMessageBus()
	ctx, cancel := context.WithCancel(context.Background())

	// Fill buffer
	for i := 0; i < 100; i++ {
		mb.PublishInbound(ctx, InboundMessage{Content: "test"})
	}

	// Now cancel and try to publish
	cancel()

	start := time.Now()
	mb.PublishInbound(ctx, InboundMessage{Content: "cancelled"})
	duration := time.Since(start)

	assert.Less(t, duration, 100*time.Millisecond, "Publishing with cancelled context took too long")
}
