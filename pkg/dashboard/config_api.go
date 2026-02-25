package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// ConfigAPI handles configuration management endpoints.
type ConfigAPI struct {
	configPath string
	appConfig  *config.Config
}

// NewConfigAPI creates a new ConfigAPI.
func NewConfigAPI(configPath string, cfg *config.Config) *ConfigAPI {
	return &ConfigAPI{
		configPath: configPath,
		appConfig:  cfg,
	}
}

// RegisterRoutes registers configuration API routes.
func (api *ConfigAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/config", api.handleConfig)
	mux.HandleFunc("/api/config/schema", api.handleSchema)
	mux.HandleFunc("/api/config/backups", api.handleBackups)
	mux.HandleFunc("/api/config/rollback", api.handleRollback)
	mux.HandleFunc("/api/restart", api.handleRestart)
}

func (api *ConfigAPI) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := os.ReadFile(api.configPath)
		if err != nil {
			http.Error(w, "Failed to read config", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)

	case http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		// 1. Validate JSON
		var testCfg config.Config
		if err := json.Unmarshal(body, &testCfg); err != nil {
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}

		// 2. Backup existing config
		if err := api.createBackup(); err != nil {
			logger.ErrorCF("dashboard", "Failed to create backup", map[string]interface{}{"error": err})
		}

		// 3. Save new config
		if err := os.WriteFile(api.configPath, body, 0644); err != nil {
			http.Error(w, "Failed to save config", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "saved"})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (api *ConfigAPI) handleSchema(w http.ResponseWriter, r *http.Request) {
	schema := GenerateSchema()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(schema)
}

func (api *ConfigAPI) handleBackups(w http.ResponseWriter, r *http.Request) {
	backups, err := api.listBackups()
	if err != nil {
		http.Error(w, "Failed to list backups", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(backups)
}

func (api *ConfigAPI) handleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	backupPath := filepath.Join(filepath.Dir(api.configPath), "backups", req.Filename)
	data, err := os.ReadFile(backupPath)
	if err != nil {
		http.Error(w, "Backup not found", http.StatusNotFound)
		return
	}

	if err := os.WriteFile(api.configPath, data, 0644); err != nil {
		http.Error(w, "Failed to rollback", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "rolled back"})
}

func (api *ConfigAPI) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "restarting"})

	// Trigger graceful restart
	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(0) // Rely on Docker/Systemd restart policy
	}()
}

func (api *ConfigAPI) createBackup() error {
	backupDir := filepath.Join(filepath.Dir(api.configPath), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	timestamp := time.Now().Format("20060102150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("config_%s.json", timestamp))

	data, err := os.ReadFile(api.configPath)
	if err != nil {
		return err
	}

	return os.WriteFile(backupPath, data, 0644)
}

func (api *ConfigAPI) listBackups() ([]string, error) {
	backupDir := filepath.Join(filepath.Dir(api.configPath), "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	return names, nil
}
