package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type Session struct {
	Key      string              `json:"key"`
	Messages []providers.Message `json:"messages"`
	Summary  string              `json:"summary,omitempty"`
	Created  time.Time           `json:"created"`
	Updated  time.Time           `json:"updated"`
}

type SessionManager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
	storage  string
}

func NewSessionManager(storage string) *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
		storage:  storage,
	}

	if storage != "" {
		os.MkdirAll(storage, 0755)
		sm.loadSessions()
	}

	return sm
}

func (sm *SessionManager) GetOrCreate(key string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if ok {
		sm.ensureLoaded(key)
		return session
	}

	session = &Session{
		Key:      key,
		Messages: []providers.Message{},
		Created:  time.Now(),
		Updated:  time.Now(),
	}
	sm.sessions[key] = session

	return session
}

func (sm *SessionManager) AddMessage(sessionKey, role, content string) {
	sm.AddFullMessage(sessionKey, providers.Message{
		Role:    role,
		Content: content,
	})
}

// AddFullMessage adds a complete message with tool calls and tool call ID to the session.
// This is used to save the full conversation flow including tool calls and tool results.
func (sm *SessionManager) AddFullMessage(sessionKey string, msg providers.Message) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[sessionKey]
	if !ok {
		session = &Session{
			Key:      sessionKey,
			Messages: []providers.Message{},
			Created:  time.Now(),
		}
		sm.sessions[sessionKey] = session
	} else {
		sm.ensureLoaded(sessionKey)
	}

	session.Messages = append(session.Messages, msg)
	session.Updated = time.Now()
}

func (sm *SessionManager) GetHistory(key string) []providers.Message {
	sm.mu.Lock() // Upgraded to write lock because it might trigger Disk I/O via ensureLoaded
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if !ok {
		return []providers.Message{}
	}

	sm.ensureLoaded(key)
	history := make([]providers.Message, len(session.Messages))
	copy(history, session.Messages)
	return history
}

func (sm *SessionManager) GetSummary(key string) string {
	sm.mu.Lock() // Upgraded to write lock
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if !ok {
		return ""
	}
	sm.ensureLoaded(key)
	return session.Summary
}

func (sm *SessionManager) SetSummary(key string, summary string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if ok {
		sm.ensureLoaded(key)
		session.Summary = summary
		session.Updated = time.Now()
	}
}

func (sm *SessionManager) TruncateHistory(key string, keepLast int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if !ok {
		return
	}

	sm.ensureLoaded(key)
	if keepLast <= 0 {
		session.Messages = []providers.Message{}
		session.Updated = time.Now()
		return
	}

	if len(session.Messages) <= keepLast {
		return
	}

	session.Messages = session.Messages[len(session.Messages)-keepLast:]
	session.Updated = time.Now()
}

// DiscardFirst removes the first n messages from the session history.
// This is used after summarization to safely remove ONLY the messages
// that were actually included in the summary, avoiding race conditions
// with new messages that might have arrived during LLM generation.
func (sm *SessionManager) DiscardFirst(key string, n int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if !ok || n <= 0 {
		return
	}

	sm.ensureLoaded(key)
	if n >= len(session.Messages) {
		session.Messages = []providers.Message{}
	} else {
		session.Messages = session.Messages[n:]
	}
	session.Updated = time.Now()
}

// sanitizeFilename converts a session key into a cross-platform safe filename.
// Session keys use "channel:chatID" (e.g. "telegram:123456") but ':' is the
// volume separator on Windows, so filepath.Base would misinterpret the key.
// We replace it with '_'. The original key is preserved inside the JSON file,
// so loadSessions still maps back to the right in-memory key.
func sanitizeFilename(key string) string {
	return strings.ReplaceAll(key, ":", "_")
}

func (sm *SessionManager) ensureLoaded(key string) {
	session, ok := sm.sessions[key]
	if !ok || session.Messages != nil {
		return
	}

	// Session exists but messages are nil (lazy loading state)
	filename := sanitizeFilename(key)
	sessionPath := filepath.Join(sm.storage, filename+".json")
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		// If file is missing, initialize as empty
		session.Messages = []providers.Message{}
		return
	}

	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		session.Messages = []providers.Message{}
		return
	}

	// Update existing record with loaded data
	session.Messages = loaded.Messages
	session.Summary = loaded.Summary
	session.Created = loaded.Created
	session.Updated = loaded.Updated
}

func (sm *SessionManager) Save(key string) error {
	if sm.storage == "" {
		return nil
	}

	filename := sanitizeFilename(key)

	// filepath.IsLocal rejects empty names, "..", absolute paths, and
	// OS-reserved device names (NUL, COM1 … on Windows).
	// The extra checks reject "." and any directory separators so that
	// the session file is always written directly inside sm.storage.
	if filename == "." || !filepath.IsLocal(filename) || strings.ContainsAny(filename, `/\`) {
		return os.ErrInvalid
	}

	// Snapshot under read lock, then perform slow file I/O after unlock.
	sm.mu.RLock()
	stored, ok := sm.sessions[key]
	if !ok {
		sm.mu.RUnlock()
		return nil
	}

	// Ensure we have the data before saving!
	// This is a bit tricky since ensureLoaded needs a write lock or its own internal lock.
	// But Save is usually called after some modification which already loaded the data.
	// To be safe, we check here.
	if stored.Messages == nil {
		sm.mu.RUnlock()
		sm.mu.Lock()
		sm.ensureLoaded(key)
		stored = sm.sessions[key]
		sm.mu.Unlock()
		sm.mu.RLock()
	}

	snapshot := Session{
		Key:     stored.Key,
		Summary: stored.Summary,
		Created: stored.Created,
		Updated: stored.Updated,
	}
	if len(stored.Messages) > 0 {
		snapshot.Messages = make([]providers.Message, len(stored.Messages))
		copy(snapshot.Messages, stored.Messages)
	} else {
		snapshot.Messages = []providers.Message{}
	}
	sm.mu.RUnlock()

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	sessionPath := filepath.Join(sm.storage, filename+".json")
	tmpFile, err := os.CreateTemp(sm.storage, "session-*.tmp")
	if err != nil {
		return err
	}

	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(0644); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, sessionPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (sm *SessionManager) loadSessions() error {
	files, err := os.ReadDir(sm.storage)
	if err != nil {
		return err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		// Lazy loading: Record that the session exists.
		// We read the file once at startup to extract the canonical key.
		// Message history remains nil until accessed via GetHistory/GetOrCreate.
		sessionPath := filepath.Join(sm.storage, file.Name())
		data, err := os.ReadFile(sessionPath)
		if err != nil {
			continue
		}

		// Unmarshal only the key to build the index
		var meta struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}

		// Initialize with nil messages to trigger lazy load on property access
		sm.sessions[meta.Key] = &Session{
			Key:      meta.Key,
			Messages: nil,
		}
	}

	return nil
}

func (sm *SessionManager) RenameSession(oldKey, newKey string) error {
	logger.InfoCF("session", "RenameSession called", map[string]interface{}{"old": oldKey, "new": newKey})
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.storage == "" {
		return nil
	}

	// Validate keys - they shouldn't contain directory separators
	if newKey == "" || strings.ContainsAny(newKey, `/\`) {
		logger.WarnCF("session", "RenameSession: invalid newKey", map[string]interface{}{"key": newKey})
		return os.ErrInvalid
	}

	session, ok := sm.sessions[oldKey]
	if !ok {
		logger.WarnCF("session", "RenameSession: oldKey not found in memory", map[string]interface{}{"key": oldKey})
		return nil // Session not loaded or doesn't exist in memory
	}

	// Check if target already exists
	if _, exists := sm.sessions[newKey]; exists {
		logger.WarnCF("session", "RenameSession: target already exists", map[string]interface{}{"key": newKey})
		return os.ErrExist
	}

	// Rename file using sanitized paths
	oldFilename := sanitizeFilename(oldKey)
	newFilename := sanitizeFilename(newKey)

	oldPath := filepath.Join(sm.storage, oldFilename+".json")
	newPath := filepath.Join(sm.storage, newFilename+".json")

	logger.InfoCF("session", "RenameSession: physical rename", map[string]interface{}{"oldPath": oldPath, "newPath": newPath})
	if err := os.Rename(oldPath, newPath); err != nil {
		logger.WarnCF("session", "RenameSession: physical rename failed", map[string]interface{}{"error": err})
		return err
	}

	// Update memory
	session.Key = newKey
	sm.sessions[newKey] = session
	delete(sm.sessions, oldKey)

	return nil
}

// SetHistory updates the messages of a session.
func (sm *SessionManager) SetHistory(key string, history []providers.Message) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[key]
	if ok {
		sm.ensureLoaded(key)
		// Create a deep copy to strictly isolate internal state
		// from the caller's slice.
		msgs := make([]providers.Message, len(history))
		copy(msgs, history)
		session.Messages = msgs
		session.Updated = time.Now()
	}
}
