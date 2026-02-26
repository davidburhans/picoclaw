package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

type Manager struct {
	db       VectorDB
	embedder Embedder
	config   config.MemoryConfig
}

func NewManager(cfg config.MemoryConfig, db VectorDB, embedder Embedder) *Manager {
	return &Manager{
		db:       db,
		embedder: embedder,
		config:   cfg,
	}
}

func (m *Manager) IsEnabled() bool {
	return m.config.Enabled && m.db != nil && m.embedder != nil
}

func (m *Manager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *Manager) ArchiveSession(ctx context.Context, workspaceID, sessionID string, messages []providers.Message) error {
	if !m.config.Enabled || m.db == nil || m.embedder == nil {
		return nil
	}

	// 1. Prepare text for embedding.
	// For now, let's just concatenate the last few messages or a summary.
	// A better approach might be to chunk it, but let's start simple.
	var sb strings.Builder
	for _, msg := range messages {
		if msg.Role == "system" {
			continue
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
	}

	text := sb.String()
	if text == "" {
		return nil
	}

	// 2. Chunk text using sliding window
	chunkSize := m.config.Embedding.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 4096 // Default
	}
	overlap := chunkSize / 10 // 10% overlap

	chunks := []string{}
	runes := []rune(text)

	if len(runes) <= chunkSize {
		chunks = append(chunks, text)
	} else {
		for i := 0; i < len(runes); i += (chunkSize - overlap) {
			end := i + chunkSize
			if end > len(runes) {
				end = len(runes)
			}
			chunks = append(chunks, string(runes[i:end]))
			if end == len(runes) {
				break
			}
		}
	}

	// 3. Process each chunk
	collection := m.config.Qdrant.CollectionName
	if collection == "" {
		collection = "picoclaw"
	}

	// We need to know the dimension for EnsureCollection.
	// We'll use the first chunk to determine it if needed.
	if len(chunks) > 0 {
		// Generate first embedding to get dimension
		vector, err := m.embedder.Embed(ctx, chunks[0])
		if err != nil {
			return fmt.Errorf("failed to generate embedding for first chunk: %w", err)
		}

		err = m.db.EnsureCollection(ctx, collection, len(vector))
		if err != nil {
			return fmt.Errorf("failed to ensure collection: %w", err)
		}

		// Store first chunk
		timestamp := time.Now().UnixNano()
		payload := map[string]interface{}{
			"workspace_id": workspaceID,
			"session_id":   sessionID,
			"content":      chunks[0],
			"timestamp":    timestamp / int64(time.Second),
			"chunk_index":  0,
			"total_chunks": len(chunks),
		}

		// Use UUID for point ID. Qdrant requires UUIDs or uint64.
		// We use MD5 hash of a stable string to generate a deterministic UUID.
		rawID0 := fmt.Sprintf("%s_%s_%d_%d", workspaceID, sessionID, timestamp, 0)
		pointID0 := uuid.NewMD5(uuid.NameSpaceURL, []byte(rawID0)).String()

		err = m.db.Store(ctx, collection, VectorRecord{
			ID:      pointID0,
			Vector:  vector,
			Payload: payload,
		})
		if err != nil {
			return fmt.Errorf("failed to store chunk 0 in vector db (ID: %s): %w", pointID0, err)
		}

		// Store remaining chunks
		for i := 1; i < len(chunks); i++ {
			vector, err := m.embedder.Embed(ctx, chunks[i])
			if err != nil {
				return fmt.Errorf("failed to generate embedding for chunk %d: %w", i, err)
			}

			payload := map[string]interface{}{
				"workspace_id": workspaceID,
				"session_id":   sessionID,
				"content":      chunks[i],
				"timestamp":    timestamp / int64(time.Second),
				"chunk_index":  i,
				"total_chunks": len(chunks),
			}

			rawIDi := fmt.Sprintf("%s_%s_%d_%d", workspaceID, sessionID, timestamp, i)
			pointIDi := uuid.NewMD5(uuid.NameSpaceURL, []byte(rawIDi)).String()

			err = m.db.Store(ctx, collection, VectorRecord{
				ID:      pointIDi,
				Vector:  vector,
				Payload: payload,
			})
			if err != nil {
				return fmt.Errorf("failed to store chunk %d in vector db (ID: %s): %w", i, pointIDi, err)
			}
		}
		logger.DebugCF("memory", "Archived session to vector DB", map[string]interface{}{
			"session": sessionID,
			"chunks":  len(chunks),
		})
	}

	return nil
}

func (m *Manager) Search(ctx context.Context, workspaceID, query string, limit, offset int) ([]SearchResult, error) {
	if !m.config.Enabled || m.db == nil || m.embedder == nil {
		return nil, nil
	}

	// 1. Generate embedding for query
	vector, err := m.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding for search: %w", err)
	}

	// 2. Search in DB
	collection := m.config.Qdrant.CollectionName
	if collection == "" {
		collection = "picoclaw"
	}

	// Prepare filters for workspace isolation
	filters := map[string]interface{}{
		"workspace_id": workspaceID,
	}

	results, err := m.db.Search(ctx, collection, vector, limit, offset, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to search in vector db: %w", err)
	}

	return results, nil
}

// SearchByDate finds semantically relevant chunks for the given query then
// returns them ordered by timestamp. It fetches a wider candidate set
// (candidateMultiplier * limit by similarity) and re-sorts client-side,
// because Qdrant cannot order_by and perform a vector search in the same query.
func (m *Manager) SearchByDate(ctx context.Context, workspaceID, query string, limit int, order string) ([]SearchResult, error) {
	if !m.config.Enabled || m.db == nil || m.embedder == nil {
		return nil, nil
	}

	const candidateMultiplier = 10
	candidates := limit * candidateMultiplier
	if candidates < 50 {
		candidates = 50
	}

	vector, err := m.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	collection := m.config.Qdrant.CollectionName
	if collection == "" {
		collection = "picoclaw"
	}

	filters := map[string]interface{}{
		"workspace_id": workspaceID,
	}

	results, err := m.db.Search(ctx, collection, vector, candidates, 0, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to search in vector db: %w", err)
	}

	sortResultsByDate(results, order)

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// sortResultsByDate sorts results in-place by the "timestamp" payload field.
func sortResultsByDate(results []SearchResult, order string) {
	getTS := func(r SearchResult) int64 {
		switch v := r.Payload["timestamp"].(type) {
		case int64:
			return v
		case float64:
			return int64(v)
		case int:
			return int64(v)
		}
		return 0
	}

	// Insertion sort — result sets are small.
	for i := 1; i < len(results); i++ {
		for j := i; j > 0; j-- {
			a, b := getTS(results[j-1]), getTS(results[j])
			if order == "asc" && a > b {
				results[j-1], results[j] = results[j], results[j-1]
			} else if order != "asc" && a < b {
				results[j-1], results[j] = results[j], results[j-1]
			} else {
				break
			}
		}
	}
}
