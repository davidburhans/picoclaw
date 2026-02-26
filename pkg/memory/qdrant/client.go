package qdrant

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/qdrant/go-client/qdrant"
	"github.com/sipeed/picoclaw/pkg/memory"
)

type Client struct {
	client *qdrant.Client
}

func NewClient(rawURL, apiKey string) (*Client, error) {
	host, port, useTLS := ParseAddress(rawURL)

	client, err := qdrant.NewClient(&qdrant.Config{
		Host:   host,
		Port:   port,
		UseTLS: useTLS,
		APIKey: apiKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create qdrant client: %w", err)
	}

	return &Client{client: client}, nil
}

func ParseAddress(rawURL string) (string, int, bool) {
	host := rawURL
	port := 6334 // Default gRPC port
	useTLS := false

	// Try to parse as URL
	if strings.Contains(rawURL, "://") {
		u, err := url.Parse(rawURL)
		if err == nil {
			host = u.Hostname()
			if p := u.Port(); p != "" {
				if p == "6333" {
					port = 6334
				} else {
					fmt.Sscanf(p, "%d", &port)
				}
			}
			if u.Scheme == "https" {
				useTLS = true
			}
		}
	} else {
		// Bare host:port or just host
		parts := strings.Split(rawURL, ":")
		if len(parts) == 2 {
			host = parts[0]
			if parts[1] == "6333" {
				port = 6334
			} else {
				fmt.Sscanf(parts[1], "%d", &port)
			}
		}
	}

	return host, port, useTLS
}

func (c *Client) Store(ctx context.Context, collection string, record memory.VectorRecord) error {
	upsertPoints := &qdrant.UpsertPoints{
		CollectionName: collection,
		Points: []*qdrant.PointStruct{
			{
				Id:      qdrant.NewID(record.ID),
				Vectors: qdrant.NewVectors(record.Vector...),
				Payload: qdrant.NewValueMap(record.Payload),
			},
		},
	}

	_, err := c.client.Upsert(ctx, upsertPoints)
	if err != nil {
		return fmt.Errorf("failed to upsert point: %w", err)
	}

	return nil
}

func (c *Client) Search(ctx context.Context, collection string, vector []float32, limit, offset int, filters map[string]interface{}) ([]memory.SearchResult, error) {
	queryPoints := &qdrant.QueryPoints{
		CollectionName: collection,
		Limit:          qdrant.PtrOf(uint64(limit)),
		Offset:         qdrant.PtrOf(uint64(offset)),
		WithPayload:    qdrant.NewWithPayload(true),
	}

	// 1. Handle Filters
	if len(filters) > 0 {
		var must []*qdrant.Condition
		for k, v := range filters {
			if s, ok := v.(string); ok {
				must = append(must, qdrant.NewMatch(k, s))
			}
		}
		if len(must) > 0 {
			queryPoints.Filter = &qdrant.Filter{
				Must: must,
			}
		}
	}

	// 2. Vector search
	queryPoints.Query = qdrant.NewQueryNearest(qdrant.NewVectorInput(vector...))

	resp, err := c.client.Query(ctx, queryPoints)
	if err != nil {
		return nil, fmt.Errorf("failed to query points: %w", err)
	}

	results := make([]memory.SearchResult, len(resp))
	for i, r := range resp {
		results[i] = memory.SearchResult{
			ID:      r.Id.String(),
			Score:   r.Score,
			Payload: convertPayload(r.Payload),
		}
	}

	return results, nil
}

func convertPayload(p map[string]*qdrant.Value) map[string]interface{} {
	if p == nil {
		return nil
	}
	res := make(map[string]interface{})
	for k, v := range p {
		switch kind := v.GetKind().(type) {
		case *qdrant.Value_StringValue:
			res[k] = kind.StringValue
		case *qdrant.Value_DoubleValue:
			res[k] = kind.DoubleValue
		case *qdrant.Value_IntegerValue:
			res[k] = kind.IntegerValue
		case *qdrant.Value_BoolValue:
			res[k] = kind.BoolValue
		case *qdrant.Value_NullValue:
			res[k] = nil
		default:
			res[k] = v.String()
		}
	}
	return res
}

func (c *Client) EnsureCollection(ctx context.Context, name string, dimension int) error {
	collections, err := c.client.ListCollections(ctx)
	if err != nil {
		return fmt.Errorf("failed to list collections: %w", err)
	}

	exists := false
	for _, col := range collections {
		if col == name {
			exists = true
			break
		}
	}

	if !exists {
		err = c.client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: name,
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     uint64(dimension),
				Distance: qdrant.Distance_Cosine,
			}),
		})
		if err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}
	}

	// Ensure a range payload index exists on `timestamp` so that order_by queries work.
	// This is idempotent — Qdrant silently succeeds if the index already exists.
	ftInt := qdrant.FieldType_FieldTypeInteger
	_, err = c.client.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: name,
		FieldName:      "timestamp",
		FieldType:      &ftInt,
	})
	if err != nil {
		return fmt.Errorf("failed to create timestamp index: %w", err)
	}

	return nil
}

func (c *Client) Close() error {
	return c.client.Close()
}
