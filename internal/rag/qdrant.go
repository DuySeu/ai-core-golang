package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Point is a vector with metadata to store in Qdrant.
type Point struct {
	ID      string
	Vector  []float32
	Payload map[string]string
}

// SearchResult is a retrieved vector with its score and metadata.
type SearchResult struct {
	Score   float32
	Payload map[string]string
}

// QdrantConfig holds connection settings for Qdrant.
type QdrantConfig struct {
	Host       string
	Port       int
	APIKey     string
	Collection string
	UseHTTPS   bool
}

// QdrantClient talks to Qdrant's REST API.
type QdrantClient struct {
	baseURL    string
	apiKey     string
	collection string
	client     *http.Client
}

// NewQdrantClient creates a client from config.
func NewQdrantClient(cfg QdrantConfig) *QdrantClient {
	scheme := "http"
	if cfg.UseHTTPS {
		scheme = "https"
	}
	return &QdrantClient{
		baseURL:    fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, cfg.Port),
		apiKey:     cfg.APIKey,
		collection: cfg.Collection,
		client:     http.DefaultClient,
	}
}

// NewQdrantClientWithHTTP creates a client with a custom HTTP client.
func NewQdrantClientWithHTTP(cfg QdrantConfig, httpClient *http.Client) *QdrantClient {
	c := NewQdrantClient(cfg)
	c.client = httpClient
	return c
}

func (q *QdrantClient) do(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, q.baseURL+path, r)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

// EnsureCollection creates the collection if it doesn't exist.
func (q *QdrantClient) EnsureCollection(ctx context.Context, dimension int) error {
	_, status, err := q.do(ctx, "GET", "/collections/"+q.collection, nil)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return nil
	}
	_, status, err = q.do(ctx, "PUT", "/collections/"+q.collection, map[string]any{
		"vectors": map[string]any{
			"size":     dimension,
			"distance": "Cosine",
		},
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("create collection: status %d", status)
	}
	return nil
}

// Upsert stores a batch of points into Qdrant.
func (q *QdrantClient) Upsert(ctx context.Context, points []Point) error {
	type qdrantPoint struct {
		ID      string            `json:"id"`
		Vector  []float32         `json:"vector"`
		Payload map[string]string `json:"payload"`
	}
	qpts := make([]qdrantPoint, len(points))
	for i, p := range points {
		qpts[i] = qdrantPoint{ID: p.ID, Vector: p.Vector, Payload: p.Payload}
	}
	_, status, err := q.do(ctx, "PUT", "/collections/"+q.collection+"/points", map[string]any{
		"points": qpts,
	})
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("upsert points: status %d", status)
	}
	return nil
}

// Search performs a similarity search and returns results above the threshold.
func (q *QdrantClient) Search(ctx context.Context, vector []float32, topK int, threshold float64) ([]SearchResult, error) {
	b, status, err := q.do(ctx, "POST", "/collections/"+q.collection+"/points/search", map[string]any{
		"vector":          vector,
		"limit":           topK,
		"score_threshold": threshold,
		"with_payload":    true,
	})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search: status %d: %s", status, string(b))
	}

	var result struct {
		Result []struct {
			Score   float32           `json:"score"`
			Payload map[string]string `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("decode search results: %w", err)
	}

	out := make([]SearchResult, len(result.Result))
	for i, r := range result.Result {
		out[i] = SearchResult{Score: r.Score, Payload: r.Payload}
	}
	return out, nil
}
