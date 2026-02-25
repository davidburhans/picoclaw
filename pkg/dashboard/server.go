package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
)

//go:embed static/*
var staticFS embed.FS

// Server extends the basic health server with dashboard capabilities.
type Server struct {
	host     string
	port     int
	server   *http.Server
	activity *ActivityBuffer
	config   *ConfigAPI
}

// NewServer creates a new dashboard server.
func NewServer(host string, port int, msgBus *bus.MessageBus, configPath string, cfg *config.Config) *Server {
	s := &Server{
		host:     host,
		port:     port,
		activity: NewActivityBuffer(100),
		config:   NewConfigAPI(configPath, cfg),
	}

	if msgBus != nil {
		s.activity.Subscribe(msgBus)
	}

	return s
}

// Start starts the dashboard server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Health endpoints (legacy)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)

	// Metrics endpoint
	mux.Handle("/metrics", promhttp.Handler())

	// Dashboard API
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/activity", s.handleActivity)

	// Config API
	s.config.RegisterRoutes(mux)

	// Static files (SPA)
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	// Note: We need a sub-filesystem or to handle the prefix if embedded folder is not root
	// For "embed static/*", the path is static/index.html
	// So we use a strip prefix or a sub filesystem

	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("/dashboard/", http.StripPrefix("/dashboard/", fileServer))
	// Redirect root to dashboard
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/dashboard/static/", http.StatusFound)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.host, s.port),
		Handler: mux,
	}

	return s.server.ListenAndServe()
}

// Stop stops the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "OK")
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "READY")
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]interface{}{
		"uptime":    time.Since(time.Now()).String(), // Placeholder
		"version":   "1.0.0",
		"timestamp": time.Now().UnixMilli(),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	events := s.activity.GetEvents()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(events)
}

// ActivityBuffer stores a ring buffer of recent events.
type ActivityBuffer struct {
	mu     sync.RWMutex
	events []map[string]interface{}
	size   int
}

// NewActivityBuffer creates a new ring buffer for activity.
func NewActivityBuffer(size int) *ActivityBuffer {
	return &ActivityBuffer{
		events: make([]map[string]interface{}, 0, size),
		size:   size,
	}
}

// Subscribe listens to the message bus and adds events to the buffer.
func (ab *ActivityBuffer) Subscribe(msgBus *bus.MessageBus) {
	ch := msgBus.Monitor()

	go func() {
		for msg := range ch {
			switch m := msg.(type) {
			case bus.InboundMessage:
				ab.Add(map[string]interface{}{
					"time":      time.Now().UnixMilli(),
					"type":      "inbound",
					"channel":   m.Channel,
					"sender":    m.SenderID,
					"content":   m.Content,
					"direction": "in",
				})
			case bus.OutboundMessage:
				ab.Add(map[string]interface{}{
					"time":      time.Now().UnixMilli(),
					"type":      "outbound",
					"channel":   m.Channel,
					"chatID":    m.ChatID,
					"content":   m.Content,
					"direction": "out",
				})
			}
		}
	}() // Note: We should handle close/context if needed, but for dashboard background it's fine.
}

// Add adds an event to the buffer.
func (ab *ActivityBuffer) Add(event map[string]interface{}) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if len(ab.events) >= ab.size {
		ab.events = ab.events[1:]
	}
	ab.events = append(ab.events, event)
}

// GetEvents returns a copy of the recorded events.
func (ab *ActivityBuffer) GetEvents() []map[string]interface{} {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	res := make([]map[string]interface{}, len(ab.events))
	copy(res, ab.events)
	return res
}
