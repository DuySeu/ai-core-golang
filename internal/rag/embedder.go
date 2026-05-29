package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Embedder generates vector embeddings via OpenRouter's embeddings endpoint.
type Embedder struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewEmbedder creates an Embedder using env config and a shared HTTP client.
func NewEmbedder() *Embedder {
	return &Embedder{
		apiKey:  os.Getenv("OPENROUTER_API_KEY"),
		model:   os.Getenv("EMBED_MODEL"),
		baseURL: "https://openrouter.ai/api/v1/embeddings",
		client:  http.DefaultClient,
	}
}

// NewEmbedderWithClient creates an Embedder with a custom HTTP client.
func NewEmbedderWithClient(client *http.Client) *Embedder {
	e := NewEmbedder()
	e.client = client
	return e
}

// Embed generates embeddings for a batch of texts.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{
		"model": e.model,
		"input": texts,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	var resp *http.Response
	for attempt := range 2 {
		resp, err = e.client.Do(req)
		if err == nil {
			break
		}
		if attempt == 0 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embeddings: %w", err)
	}

	out := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		out[i] = d.Embedding
	}
	return out, nil
}
