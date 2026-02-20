package mailbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Message struct {
	ID        string    `yaml:"id" json:"id"`
	From      string    `yaml:"from" json:"from"`
	To        string    `yaml:"to" json:"to"`
	Timestamp time.Time `yaml:"timestamp" json:"timestamp"`
	Read      bool      `yaml:"read" json:"read"`
	Content   string    `yaml:"-" json:"content"` // Body of markdown
	Filename  string    `yaml:"-" json:"-"`       // Name of the file on disk
}

type Client struct {
	basePath string
	mu       sync.RWMutex
}

func NewClient(basePath string) *Client {
	return &Client{basePath: basePath}
}

func (c *Client) getRecipientPath(recipient string) string {
	return filepath.Join(c.basePath, "to_"+recipient)
}

func (c *Client) Send(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if msg.ID == "" {
		msg.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	dir := c.getRecipientPath(msg.To)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	filename := fmt.Sprintf("%s_%s.md", msg.Timestamp.Format("20060102_150405"), msg.ID)
	path := filepath.Join(dir, filename)

	frontmatter, err := yaml.Marshal(msg)
	if err != nil {
		return err
	}

	content := fmt.Sprintf("---\n%s---\n\n%s", string(frontmatter), msg.Content)
	return os.WriteFile(path, []byte(content), 0644)
}

func (c *Client) List(recipient string) ([]Message, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	dir := c.getRecipientPath(recipient)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []Message{}, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var messages []Message
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || strings.HasPrefix(entry.Name(), "read_") {
			continue
		}

		msg, err := c.ReadMessage(recipient, entry.Name())
		if err != nil {
			continue // Skip corrupted messages
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

func (c *Client) ReadMessage(recipient, filename string) (Message, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	path := filepath.Join(c.getRecipientPath(recipient), filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return Message{}, err
	}

	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return Message{}, fmt.Errorf("invalid message format: missing frontmatter start")
	}

	parts := strings.SplitN(content, "---\n", 3)
	if len(parts) < 3 {
		return Message{}, fmt.Errorf("invalid message format: missing frontmatter end")
	}

	var msg Message
	if err := yaml.Unmarshal([]byte(parts[1]), &msg); err != nil {
		return Message{}, err
	}

	msg.Content = strings.TrimSpace(parts[2])
	msg.Filename = filename
	return msg, nil
}

func (c *Client) MarkRead(recipient, filename string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldPath := filepath.Join(c.getRecipientPath(recipient), filename)

	// Create 'read' subdirectory
	readDir := filepath.Join(c.getRecipientPath(recipient), "read")
	if err := os.MkdirAll(readDir, 0755); err != nil {
		return err
	}

	newPath := filepath.Join(readDir, filename)
	return os.Rename(oldPath, newPath)
}
