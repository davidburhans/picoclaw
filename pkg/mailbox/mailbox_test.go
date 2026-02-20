package mailbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMailbox(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mailbox_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	client := NewClient(tempDir)

	msg := Message{
		ID:        "test-id",
		From:      "sender",
		To:        "recipient",
		Content:   "Hello World",
		Timestamp: time.Now(),
	}

	// Test Send
	err = client.Send(msg)
	assert.NoError(t, err)

	// Test List
	messages, err := client.List("recipient")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(messages))
	assert.Equal(t, "sender", messages[0].From)
	assert.Equal(t, "Hello World", messages[0].Content)

	// Test MarkRead
	filename := filepath.Base(filepath.Join(client.getRecipientPath("recipient"), messages[0].ID)) // This is wrong, filename includes timestamp
	// Let's get actual filename from dir
	entries, _ := os.ReadDir(client.getRecipientPath("recipient"))
	filename = entries[0].Name()

	err = client.MarkRead("recipient", filename)
	assert.NoError(t, err)

	// Test List after MarkRead (should be empty for unread)
	messages, err = client.List("recipient")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(messages))

	// Verify it moved to read/
	readEntries, err := os.ReadDir(filepath.Join(client.getRecipientPath("recipient"), "read"))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(readEntries))
}
