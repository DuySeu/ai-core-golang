package rag

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	maxChunkChars = 1000
	overlapChars  = 200
)

// Chunk represents a single piece of a document ready for embedding.
type Chunk struct {
	SourceFile  string `json:"source_file"`
	HeadingPath string `json:"heading_path"`
	ChunkIndex  int    `json:"chunk_index"`
	Content     string `json:"content"`
	CharCount   int    `json:"char_count"`
}

// Chunker is the interface all chunking strategies must implement.
type Chunker interface {
	Chunk(ctx context.Context, filename, content string) ([]Chunk, error)
}

// ──────────────────────────────────────────────────────────────────────────────
// Registry
// ──────────────────────────────────────────────────────────────────────────────

var registry = map[string]func(*Embedder) Chunker{
	"markdown": func(_ *Embedder) Chunker { return &MarkdownChunker{} },
	"semantic": func(e *Embedder) Chunker { return &SemanticChunker{embedder: e} },
}

// GetChunker returns a Chunker by name. embedder is only used for semantic.
func GetChunker(name string, embedder *Embedder) (Chunker, error) {
	fn, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown chunking strategy %q (available: %s)", name, AvailableStrategies())
	}
	return fn(embedder), nil
}

// RegisterChunker adds a new strategy to the registry.
func RegisterChunker(name string, factory func(*Embedder) Chunker) {
	registry[name] = factory
}

// AvailableStrategies returns a comma-separated list of registered strategy names.
func AvailableStrategies() string {
	names := make([]string, 0, len(registry))
	for k := range registry {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}

// ──────────────────────────────────────────────────────────────────────────────
// MarkdownChunker — splits on headings, then paragraphs with overlap
// ──────────────────────────────────────────────────────────────────────────────

type MarkdownChunker struct{}

func (m *MarkdownChunker) Chunk(_ context.Context, filename, content string) ([]Chunk, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	base := filepath.Base(filename)
	switch ext {
	case ".md":
		return chunkMarkdown(base, content), nil
	default:
		return chunkByParagraph(base, "", content), nil
	}
}

func chunkMarkdown(filename, content string) []Chunk {
	lines := strings.Split(content, "\n")
	type section struct {
		heading string
		body    strings.Builder
	}

	var sections []section
	var current section

	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			if strings.TrimSpace(current.body.String()) != "" {
				sections = append(sections, current)
			}
			current = section{heading: strings.TrimSpace(line)}
		} else {
			current.body.WriteString(line)
			current.body.WriteByte('\n')
		}
	}
	if strings.TrimSpace(current.body.String()) != "" {
		sections = append(sections, current)
	}

	var chunks []Chunk
	idx := 0
	for _, sec := range sections {
		body := strings.TrimSpace(sec.body.String())
		if body == "" {
			continue
		}
		chunks = append(chunks, splitLarge(filename, sec.heading, body, &idx)...)
	}
	return chunks
}

func chunkByParagraph(filename, heading, content string) []Chunk {
	idx := 0
	return splitLarge(filename, heading, content, &idx)
}

func splitLarge(filename, heading, text string, idx *int) []Chunk {
	if len(text) <= maxChunkChars {
		c := Chunk{
			SourceFile:  filename,
			HeadingPath: heading,
			ChunkIndex:  *idx,
			Content:     text,
			CharCount:   len(text),
		}
		*idx++
		return []Chunk{c}
	}

	paragraphs := strings.Split(text, "\n\n")
	var chunks []Chunk
	var buf strings.Builder

	flush := func() {
		s := strings.TrimSpace(buf.String())
		if s == "" {
			return
		}
		chunks = append(chunks, Chunk{
			SourceFile:  filename,
			HeadingPath: heading,
			ChunkIndex:  *idx,
			Content:     s,
			CharCount:   len(s),
		})
		*idx++
		if len(s) > overlapChars {
			buf.Reset()
			buf.WriteString(s[len(s)-overlapChars:])
		} else {
			buf.Reset()
		}
	}

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		if buf.Len()+len(para)+2 > maxChunkChars {
			flush()
		}
		if buf.Len() > 0 {
			buf.WriteString("\n\n")
		}
		buf.WriteString(para)
	}
	flush()

	if len(chunks) == 0 {
		for i := 0; i < len(text); i += maxChunkChars - overlapChars {
			end := i + maxChunkChars
			if end > len(text) {
				end = len(text)
			}
			s := text[i:end]
			chunks = append(chunks, Chunk{
				SourceFile:  filename,
				HeadingPath: fmt.Sprintf("%s [part %d]", heading, *idx),
				ChunkIndex:  *idx,
				Content:     s,
				CharCount:   len(s),
			})
			*idx++
		}
	}
	return chunks
}
