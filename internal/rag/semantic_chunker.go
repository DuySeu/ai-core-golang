package rag

import (
	"context"
	"math"
	"strings"
)

const semanticChunkThreshold = 0.5

// SemanticChunker splits text by detecting semantic boundaries via embeddings.
type SemanticChunker struct {
	embedder *Embedder
}

func (s *SemanticChunker) Chunk(ctx context.Context, filename, content string) ([]Chunk, error) {
	sentences := splitSentences(content)
	if len(sentences) == 0 {
		return nil, nil
	}

	vectors, err := embedBatched(ctx, s.embedder, sentences, 20)
	if err != nil {
		return nil, err
	}

	var chunks []Chunk
	var buf strings.Builder
	buf.WriteString(sentences[0])
	idx := 0

	flush := func() {
		if text := strings.TrimSpace(buf.String()); text != "" {
			chunks = append(chunks, Chunk{
				SourceFile:  filename,
				HeadingPath: "semantic",
				ChunkIndex:  idx,
				Content:     text,
				CharCount:   len(text),
			})
			idx++
			buf.Reset()
		}
	}

	for i := 1; i < len(sentences); i++ {
		if cosineSimilarity(vectors[i-1], vectors[i]) < semanticChunkThreshold || buf.Len() > maxChunkChars {
			flush()
		} else if buf.Len() > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(sentences[i])
	}
	flush()

	return chunks, nil
}

func splitSentences(text string) []string {
	var sentences []string
	var buf strings.Builder
	runes := []rune(text)
	for i, r := range runes {
		buf.WriteRune(r)
		if (r == '.' || r == '!' || r == '?') &&
			(i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n') {
			if s := strings.TrimSpace(buf.String()); s != "" {
				sentences = append(sentences, s)
			}
			buf.Reset()
		}
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		sentences = append(sentences, s)
	}
	return sentences
}

func embedBatched(ctx context.Context, embedder *Embedder, texts []string, batchSize int) ([][]float32, error) {
	var all [][]float32
	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		vecs, err := embedder.Embed(ctx, texts[i:end])
		if err != nil {
			return nil, err
		}
		all = append(all, vecs...)
	}
	return all, nil
}

func cosineSimilarity(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if denom := math.Sqrt(normA) * math.Sqrt(normB); denom != 0 {
		return dot / denom
	}
	return 0
}
