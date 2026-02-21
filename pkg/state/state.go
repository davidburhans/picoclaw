package state

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State represents the persistent state for a workspace.
// It includes information about the last active channel/chat.
type State struct {
	// LastChannel is the last channel used for communication
	LastChannel string `json:"last_channel,omitempty"`

	// LastChatID is the last chat ID used for communication
	LastChatID string `json:"last_chat_id,omitempty"`

	// Timestamp is the last time this state was updated
	Timestamp time.Time `json:"timestamp"`

	// SessionVersions tracks the current version number for each session base key
	SessionVersions map[string]int `json:"session_versions,omitempty"`

	// ActiveSessions tracks the currently active full session key for each base key
	ActiveSessions map[string]string `json:"active_sessions,omitempty"`
}

// Manager manages persistent state with atomic saves.
type Manager struct {
	workspace string
	state     *State
	mu        sync.RWMutex
	stateFile string
}

// NewManager creates a new state manager for the given workspace.
func NewManager(workspace string) *Manager {
	stateDir := filepath.Join(workspace, "state")
	stateFile := filepath.Join(stateDir, "state.json")
	oldStateFile := filepath.Join(workspace, "state.json")

	// Create state directory if it doesn't exist
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		log.Printf("ERROR: failed to create state directory: %v", err)
	}

	sm := &Manager{
		workspace: workspace,
		stateFile: stateFile,
		state: &State{
			SessionVersions: make(map[string]int),
			ActiveSessions:  make(map[string]string),
		},
	}

	// Try to load from new location first
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		// New file doesn't exist, try migrating from old location
		if data, err := os.ReadFile(oldStateFile); err == nil {
			if err := json.Unmarshal(data, sm.state); err == nil {
				// Initialize maps if missing after unmarshal
				if sm.state.SessionVersions == nil {
					sm.state.SessionVersions = make(map[string]int)
				}
				if sm.state.ActiveSessions == nil {
					sm.state.ActiveSessions = make(map[string]string)
				}
				// Migrate to new location
				sm.saveAtomic()
				log.Printf("[INFO] state: migrated state from %s to %s", oldStateFile, stateFile)
			}
		}
	} else {
		// Load from new location
		sm.load()
	}

	return sm
}

// SetLastChannel atomically updates the last channel and saves the state.
// This method uses a temp file + rename pattern for atomic writes,
// ensuring that the state file is never corrupted even if the process crashes.
func (sm *Manager) SetLastChannel(channel string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Update state
	sm.state.LastChannel = channel
	sm.state.Timestamp = time.Now()

	// Atomic save using temp file + rename
	if err := sm.saveAtomic(); err != nil {
		return fmt.Errorf("failed to save state atomically: %w", err)
	}

	return nil
}

// SetLastChatID atomically updates the last chat ID and saves the state.
func (sm *Manager) SetLastChatID(chatID string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Update state
	sm.state.LastChatID = chatID
	sm.state.Timestamp = time.Now()

	// Atomic save using temp file + rename
	if err := sm.saveAtomic(); err != nil {
		return fmt.Errorf("failed to save state atomically: %w", err)
	}

	return nil
}

// GetLastChannel returns the last channel from the state.
func (sm *Manager) GetLastChannel() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state.LastChannel
}

// GetLastChatID returns the last chat ID from the state.
func (sm *Manager) GetLastChatID() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state.LastChatID
}

// GetSessionVersion returns the current version for a session base key.
func (sm *Manager) GetSessionVersion(baseKey string) int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state.SessionVersions[baseKey]
}

// GetActiveSession returns the active full session key for a base key.
func (sm *Manager) GetActiveSession(baseKey string) string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state.ActiveSessions[baseKey]
}

// StartNewSession increments the version for a session and returns the new full key.
// The new key format is: baseKey_vN_name (with colons in baseKey replaced by underscores)
func (sm *Manager) StartNewSession(baseKey, name string) (string, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Initialize maps if nil (can happen if loaded from old state file without migration logic triggering)
	if sm.state.SessionVersions == nil {
		sm.state.SessionVersions = make(map[string]int)
	}
	if sm.state.ActiveSessions == nil {
		sm.state.ActiveSessions = make(map[string]string)
	}

	// Increment version
	newVersion := sm.state.SessionVersions[baseKey] + 1
	sm.state.SessionVersions[baseKey] = newVersion

	// Sanitize baseKey for filename (replace : with _)
	safeBaseKey := baseKey
	// Simple replacement for colon which is common in "channel:id" keys
	// We rely on the caller to provide a reasonably safe baseKey, but we strictly enforce no colons for the file part.
	// Actually, a better approach is to handle the replacement here or in the caller.
	// Let's assume the baseKey passed here is the logical key (e.g. "discord:123"),
	// and we want the file key to be "discord_123_v1_name".
	// But wait, the session manager expects the key to be the filename (without .json).
	// So we should construct a safe key here.

	// Replace all colons with underscores
	safeBaseKey = ""
	for _, c := range baseKey {
		if c == ':' {
			safeBaseKey += "_"
		} else {
			safeBaseKey += string(c)
		}
	}

	// Construct new full key
	newKey := fmt.Sprintf("%s_v%d", safeBaseKey, newVersion)
	if name != "" {
		newKey += "_" + name
	}

	// Update active session
	sm.state.ActiveSessions[baseKey] = newKey
	sm.state.Timestamp = time.Now()

	// Save state
	if err := sm.saveAtomic(); err != nil {
		return "", fmt.Errorf("failed to save state: %w", err)
	}

	return newKey, nil
}

// GetTimestamp returns the timestamp of the last state update.
func (sm *Manager) GetTimestamp() time.Time {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.state.Timestamp
}

// saveAtomic performs an atomic save using temp file + rename.
// This ensures that the state file is never corrupted:
// 1. Write to a temp file
// 2. Rename temp file to target (atomic on POSIX systems)
// 3. If rename fails, cleanup the temp file
//
// Must be called with the lock held.
func (sm *Manager) saveAtomic() error {
	// Create temp file in the same directory as the target
	tempFile := sm.stateFile + ".tmp"

	// Marshal state to JSON
	data, err := json.MarshalIndent(sm.state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temp file
	if err := os.WriteFile(tempFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename from temp to target
	if err := os.Rename(tempFile, sm.stateFile); err != nil {
		// Cleanup temp file if rename fails
		os.Remove(tempFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// load loads the state from disk.
func (sm *Manager) load() error {
	data, err := os.ReadFile(sm.stateFile)
	if err != nil {
		// File doesn't exist yet, that's OK
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}

	if err := json.Unmarshal(data, sm.state); err != nil {
		return fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return nil
}
