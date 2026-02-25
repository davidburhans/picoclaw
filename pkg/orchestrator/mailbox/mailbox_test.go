package mailbox

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailboxStore(t *testing.T) {
	// Red phase: We expect MemoryStore to not exist yet or not implement the methods.
	store := NewMemoryStore()

	ctx := context.Background()

	t.Run("SendMessage and ListMessages", func(t *testing.T) {
		msgID, err := store.SendMessage(ctx, "dad", "kid", "Clean your room")
		require.NoError(t, err)
		assert.NotEmpty(t, msgID)

		messages, err := store.ListMessages(ctx, "kid")
		require.NoError(t, err)
		require.Len(t, messages, 1)

		assert.Equal(t, "dad", messages[0].From)
		assert.Equal(t, "kid", messages[0].To)
		assert.Equal(t, "Clean your room", messages[0].Content)
		assert.False(t, messages[0].Read)
		assert.WithinDuration(t, time.Now(), messages[0].Timestamp, 5*time.Second)
	})

	t.Run("ReadMessage", func(t *testing.T) {
		msgID, _ := store.SendMessage(ctx, "kid", "dad", "Done!")

		// Reading it should mark it as read
		msg, err := store.ReadMessage(ctx, "dad", msgID)
		require.NoError(t, err)
		assert.Equal(t, "Done!", msg.Content)
		assert.True(t, msg.Read)

		// Listing should show it as read
		messages, _ := store.ListMessages(ctx, "dad")
		require.Len(t, messages, 1)
		assert.True(t, messages[0].Read)
	})

	t.Run("ReadMessage unauthorized", func(t *testing.T) {
		msgID, _ := store.SendMessage(ctx, "mom", "kid", "Hi")

		// Dad tries to read kid's message
		_, err := store.ReadMessage(ctx, "dad", msgID)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unauthorized")
	})
}
