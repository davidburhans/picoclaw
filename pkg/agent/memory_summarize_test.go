package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/memory"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type mockVectorDB struct {
	storedRecords []memory.VectorRecord
}

func (m *mockVectorDB) Store(ctx context.Context, collection string, record memory.VectorRecord) error {
	m.storedRecords = append(m.storedRecords, record)
	return nil
}

func (m *mockVectorDB) Search(ctx context.Context, collection string, vector []float32, limit, offset int, filters map[string]interface{}) ([]memory.SearchResult, error) {
	return nil, nil
}

func (m *mockVectorDB) EnsureCollection(ctx context.Context, name string, dimension int) error {
	return nil
}

func (m *mockVectorDB) Close() error {
	return nil
}

type mockEmbedder struct{}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{1.0, 2.0, 3.0}, nil
}

func (m *mockEmbedder) Dimension() int {
	return 3
}

type mockEmbedderWithHistory struct {
	embeddedTexts []string
}

func (m *mockEmbedderWithHistory) Embed(ctx context.Context, text string) ([]float32, error) {
	m.embeddedTexts = append(m.embeddedTexts, text)
	return []float32{1.0, 2.0, 3.0}, nil
}

func (m *mockEmbedderWithHistory) Dimension() int {
	return 3
}

func TestSummarizeSession_Archival(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-summarize-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: tmpDir,
				Name:      "test-agent",
			},
		},
		Memory: config.MemoryConfig{
			Enabled: true,
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &testMockProvider{} // From loop_test.go
	al := NewAgentLoop(cfg, msgBus, provider)

	// Setup mock memory manager
	mockDB := &mockVectorDB{}
	mockEmb := &mockEmbedder{}
	memMgr := memory.NewManager(cfg.Memory, mockDB, mockEmb)
	al.SetMemoryManager(memMgr)

	// Setup session history that exceeds threshold (simple test: manually trigger summarizeSession)
	sessionKey := "test-summarize-archive"
	wctx := al.getOrCreateWorkspaceContext("")

	// Create session first so SetHistory works
	wctx.sessions.GetOrCreate(sessionKey)

	history := []providers.Message{
		{Role: "user", Content: "Message 1"},
		{Role: "assistant", Content: "Response 1"},
		{Role: "user", Content: "Message 2"},
		{Role: "assistant", Content: "Response 2"},
		{Role: "user", Content: "Message 3"},
		{Role: "assistant", Content: "Response 3"},
	}
	wctx.sessions.SetHistory(sessionKey, history)

	// Trigger summarizeSession
	// It will take history[:len(history)-4], which is first 2 messages
	al.summarizeSession(wctx, sessionKey)

	// Verify archival was called
	if len(mockDB.storedRecords) == 0 {
		t.Fatal("Expected history segment to be archived to memory, but mock DB received no records")
	}

	record := mockDB.storedRecords[0]
	// Verify payload contains original session id
	if record.Payload["session_id"] != sessionKey {
		t.Errorf("Expected session_id '%s' in payload, got '%v'", sessionKey, record.Payload["session_id"])
	}

	// Verify ID is a valid UUID
	if _, err := uuid.Parse(record.ID); err != nil {
		t.Errorf("Expected record ID to be a valid UUID, got '%s': %v", record.ID, err)
	}

	// Verify content archived contains the truncated messages
	content, ok := record.Payload["content"].(string)
	if !ok {
		t.Fatal("Payload content is missing or not a string")
	}
	if !strings.Contains(content, "Message 1") || !strings.Contains(content, "Response 1") {
		t.Errorf("Archived content missing expected messages. Got: %s", content)
	}
	if strings.Contains(content, "Message 3") {
		t.Error("Archived content should NOT contain messages that were kept in history (the last 4)")
	}
}

func TestSummarizeSession_Chunking(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "agent-chunking-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Config with small chunk size to force split
	cfg := &config.Config{
		Agents: config.AgentsConfig{
			Defaults: config.AgentDefaults{
				Workspace: tmpDir,
				Name:      "test-agent",
			},
		},
		Memory: config.MemoryConfig{
			Enabled: true,
			Embedding: config.EmbeddingConfig{
				ChunkSize: 20, // Very small chunk size
			},
		},
	}

	msgBus := bus.NewMessageBus()
	provider := &testMockProvider{}
	al := NewAgentLoop(cfg, msgBus, provider)

	mockDB := &mockVectorDB{}
	mockEmb := &mockEmbedderWithHistory{}
	memMgr := memory.NewManager(cfg.Memory, mockDB, mockEmb)
	al.SetMemoryManager(memMgr)

	sessionKey := "test-chunking"
	wctx := al.getOrCreateWorkspaceContext("")
	wctx.sessions.GetOrCreate(sessionKey)

	// Create long message > 20 chars
	longContent := "This is a very long message that should definitely be chunked into multiple pieces by the archive session function."
	history := []providers.Message{
		{Role: "user", Content: longContent},
		// Add padding messages that won't be archived (last 4)
		{Role: "user", Content: "keep1"},
		{Role: "assistant", Content: "keep2"},
		{Role: "user", Content: "keep3"},
		{Role: "assistant", Content: "keep4"},
	}
	wctx.sessions.SetHistory(sessionKey, history)

	// Trigger summarize -> archive
	al.summarizeSession(wctx, sessionKey)

	// Verify we have multiple records
	if len(mockDB.storedRecords) <= 1 {
		t.Errorf("Expected multiple chunked records, got %d", len(mockDB.storedRecords))
	}

	// Verify chunks have correct indices
	for i, record := range mockDB.storedRecords {
		idx, ok := record.Payload["chunk_index"].(int)
		if !ok {
			t.Errorf("Record %d missing chunk_index", i)
		}
		if idx != i {
			t.Errorf("Expected chunk index %d, got %d", i, idx)
		}

		total, ok := record.Payload["total_chunks"].(int)
		if !ok {
			t.Errorf("Record %d missing total_chunks", i)
		}
		if total != len(mockDB.storedRecords) {
			t.Errorf("Expected total_chunks %d, got %d", len(mockDB.storedRecords), total)
		}

		// Verify ID is a valid UUID
		if _, err := uuid.Parse(record.ID); err != nil {
			t.Errorf("Record %d ID is not a valid UUID: %s", i, record.ID)
		}
	}
}
