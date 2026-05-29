package tools

import (
	"ai-core-golang/internal/rag"
	"context"
	"fmt"
	"strings"
)

type SearchKnowledgeBaseInput struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

func (SearchKnowledgeBaseInput) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "The search query to find relevant documentation",
			},
			"top_k": map[string]any{
				"type":        "integer",
				"description": "Number of results to return (default 5)",
				"default":     5,
			},
		},
		"required": []string{"query"},
	}
}

// NewSearchKnowledgeBaseTool creates the RAG search tool with injected dependencies.
func NewSearchKnowledgeBaseTool(embedder *rag.Embedder, qdrant *rag.QdrantClient) *Tool {
	return NewTool("search_knowledge_base",
		"Search the internal knowledge base for relevant documentation. Use when the user asks about internal docs, guides, or reference material.",
		func(ctx context.Context, input SearchKnowledgeBaseInput) (any, error) {
			if input.Query == "" {
				return nil, fmt.Errorf("query is required")
			}
			topK := input.TopK
			if topK <= 0 {
				topK = 5
			}

			vecs, err := embedder.Embed(ctx, []string{input.Query})
			if err != nil {
				return "Knowledge base unavailable: " + err.Error(), nil
			}

			results, err := qdrant.Search(ctx, vecs[0], topK, 0.7)
			if err != nil {
				return "Knowledge base unavailable: " + err.Error(), nil
			}
			if len(results) == 0 {
				return "No relevant documents found.", nil
			}

			var sb strings.Builder
			for _, r := range results {
				sb.WriteString(fmt.Sprintf("[Source: %s > %s]\n", r.Payload["source_file"], r.Payload["heading_path"]))
				sb.WriteString(r.Payload["content"])
				sb.WriteString("\n\n")
			}
			return strings.TrimSpace(sb.String()), nil
		},
	)
}
