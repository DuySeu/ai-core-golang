package rag

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	knowledgeBaseDir = "knowledge_base"
	embedBatchSize   = 20
)

// ChunkKnowledgeBase reads all supported files from knowledge_base/ and chunks them.
// Pass nil chunker to use the default MarkdownChunker.
func ChunkKnowledgeBase(ctx context.Context, chunker Chunker) ([]Chunk, error) {
	if chunker == nil {
		chunker = &MarkdownChunker{}
	}
	var allChunks []Chunk
	err := filepath.WalkDir(knowledgeBaseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, _, err := readFile(path)
		if err != nil {
			log.Printf("Warning: skipping %s: %v", path, err)
			return nil
		}
		if content == "" {
			return nil
		}
		chunks, err := chunker.Chunk(ctx, path, content)
		if err != nil {
			log.Printf("Warning: chunking failed for %s: %v", path, err)
			return nil
		}
		// Override SourceFile to use basename for consistency
		base := filepath.Base(path)
		for i := range chunks {
			chunks[i].SourceFile = base
		}
		allChunks = append(allChunks, chunks...)
		return nil
	})
	return allChunks, err
}

// Ingest chunks all files and upserts vectors into Qdrant.
// Returns (filesProcessed, chunksStored, error).
func Ingest(ctx context.Context, embedder *Embedder, qdrant *QdrantClient) (int, int, error) {
	allChunks, err := ChunkKnowledgeBase(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("walk knowledge_base: %w", err)
	}
	if len(allChunks) == 0 {
		return 0, 0, nil
	}

	// Print per-file summary
	seen := map[string]int{}
	for _, c := range allChunks {
		seen[c.SourceFile]++
	}
	for f, n := range seen {
		fmt.Printf("Processing %s... %d chunks\n", f, n)
	}

	// Ensure collection using dimension from first batch
	firstBatch := allChunks[:min(embedBatchSize, len(allChunks))]
	texts := make([]string, len(firstBatch))
	for i, c := range firstBatch {
		texts[i] = c.Content
	}
	vecs, err := embedder.Embed(ctx, texts)
	if err != nil {
		return 0, 0, fmt.Errorf("embed first batch: %w", err)
	}
	if err := qdrant.EnsureCollection(ctx, len(vecs[0])); err != nil {
		return 0, 0, fmt.Errorf("ensure collection: %w", err)
	}

	stored := 0
	upsertBatch := func(batch []Chunk, vecs [][]float32) {
		pts := makePoints(batch, vecs)
		if err := qdrant.Upsert(ctx, pts); err != nil {
			log.Printf("Warning: upsert failed: %v", err)
			return
		}
		stored += len(pts)
	}

	upsertBatch(firstBatch, vecs)

	for i := len(firstBatch); i < len(allChunks); i += embedBatchSize {
		end := min(i+embedBatchSize, len(allChunks))
		batch := allChunks[i:end]
		batchTexts := make([]string, len(batch))
		for j, c := range batch {
			batchTexts[j] = c.Content
		}
		vecs, err := embedder.Embed(ctx, batchTexts)
		if err != nil {
			log.Printf("Warning: skipping batch %d-%d: %v", i, end, err)
			continue
		}
		upsertBatch(batch, vecs)
	}

	return len(seen), stored, nil
}

// readFile reads a file and returns its text content and extension.
// For binary formats (pdf, docx, xlsx), text is extracted via ExtractText.
func readFile(path string) (content, ext string, err error) {
	ext = strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".txt":
		b, e := os.ReadFile(path)
		if e != nil {
			return "", ext, e
		}
		return string(b), ext, nil
	case ".pdf", ".docx", ".xlsx":
		text, e := ExtractText(path, ext)
		if e != nil {
			return "", ext, e
		}
		return strings.TrimSpace(text), ext, nil
	default:
		return "", ext, nil // unsupported, skip silently
	}
}

func makePoints(chunks []Chunk, vecs [][]float32) []Point {
	pts := make([]Point, len(chunks))
	for i, c := range chunks {
		id := fmt.Sprintf("%x", sha256.Sum256([]byte(c.SourceFile+fmt.Sprint(c.ChunkIndex))))
		pts[i] = Point{
			ID:     id,
			Vector: vecs[i],
			Payload: map[string]string{
				"source_file":  c.SourceFile,
				"heading_path": c.HeadingPath,
				"content":      c.Content,
			},
		}
	}
	return pts
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
