package auth

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/utils"
)

type AuthCredential struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	AccountID    string    `json:"account_id,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Provider     string    `json:"provider"`
	AuthMethod   string    `json:"auth_method"`
	Email        string    `json:"email,omitempty"`
	ProjectID    string    `json:"project_id,omitempty"`
}

type AuthStore struct {
	Credentials map[string]*AuthCredential `json:"credentials"`
}

var (
	storeMu sync.RWMutex
	// authFile can be overridden by tests
	authFile = ""
)

func (c *AuthCredential) IsExpired() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(c.ExpiresAt)
}

func (c *AuthCredential) NeedsRefresh() bool {
	if c.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(5 * time.Minute).After(c.ExpiresAt)
}

func authFilePath() string {
	if authFile != "" {
		return authFile
	}
	home := utils.ExpandHome("~")
	return filepath.Join(home, ".picoclaw", "auth.json")
}

func LoadStore() (*AuthStore, error) {
	path := authFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AuthStore{Credentials: make(map[string]*AuthCredential)}, nil
		}
		return nil, err
	}

	var store AuthStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	if store.Credentials == nil {
		store.Credentials = make(map[string]*AuthCredential)
	}
	return &store, nil
}

func SaveStore(store *AuthStore) error {
	path := authFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func authLockPath() string {
	return authFilePath() + ".lock"
}

// withLock executes a function with a file-level advisory lock.
// It uses retries and jitter to wait for the lock if held by another process.
func withLock(fn func() error) error {
	lockPath := authLockPath()
	maxAttempts := 10
	start := time.Now()
	timeout := 5 * time.Second

	for i := 0; i < maxAttempts; i++ {
		// Attempt to create the lock file exclusively
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			// Lock acquired
			f.Close()
			defer os.Remove(lockPath)
			return fn()
		}

		if !os.IsExist(err) {
			return fmt.Errorf("failed to create lock file: %w", err)
		}

		// Check if we timed out
		if time.Since(start) > timeout {
			return fmt.Errorf("timeout waiting for auth lock")
		}

		// Wait with jittered backoff: 100ms - 300ms
		time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
	}

	return fmt.Errorf("failed to acquire auth lock after %d attempts", maxAttempts)
}

func GetCredential(provider string) (*AuthCredential, error) {
	storeMu.RLock()
	defer storeMu.RUnlock()

	var cred *AuthCredential
	err := withLock(func() error {
		store, err := LoadStore()
		if err != nil {
			return err
		}
		cred = store.Credentials[provider]
		return nil
	})

	return cred, err
}

func SetCredential(provider string, cred *AuthCredential) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	return withLock(func() error {
		store, err := LoadStore()
		if err != nil {
			return err
		}
		store.Credentials[provider] = cred
		return SaveStore(store)
	})
}

func DeleteCredential(provider string) error {
	storeMu.Lock()
	defer storeMu.Unlock()

	return withLock(func() error {
		store, err := LoadStore()
		if err != nil {
			return err
		}
		delete(store.Credentials, provider)
		return SaveStore(store)
	})
}

func DeleteAllCredentials() error {
	storeMu.Lock()
	defer storeMu.Unlock()

	return withLock(func() error {
		path := authFilePath()
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	})
}
