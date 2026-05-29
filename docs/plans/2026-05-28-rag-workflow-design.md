# RAG Workflow Design

## Overview

Extend llm-core with a Retrieval-Augmented Generation (RAG) workflow for internal knowledge base documents. The LLM agent automatically retrieves relevant context from indexed markdown/text files during chat via a tool call.

## Architecture

Two new capabilities:

1. **`ingest` CLI command** — reads all `.md` and `.txt` files from `knowledge_base/`, splits them into chunks, generates embeddings via OpenRouter, and upserts vectors into Qdrant.

2. **`search_knowledge_base` tool** — registered alongside existing financial tools. During chat, the LLM calls this tool with a query. The tool embeds the query, performs similarity search against Qdrant, and returns top-k relevant chunks.

### Data Flow

```
Ingest:  knowledge_base/*.md → chunker → OpenRouter embed → Qdrant upsert

Chat:    User question → LLM decides to call search_knowledge_base
         → embed query → Qdrant search → return chunks to LLM
         → LLM synthesizes answer
```

### New Packages

```
internal/
├── rag/
│   ├── chunker.go      # Markdown-aware text splitting
│   ├── embedder.go     # OpenRouter embedding client
│   ├── qdrant.go       # Qdrant client (upsert + search)
│   └── ingest.go       # Orchestrates: read files → chunk → embed → store
```

Tool handler: `internal/llm/tools/search_knowledge_base.handler.go`

## Chunking & Ingestion

### Chunker (`internal/rag/chunker.go`)

- Splits markdown files on heading boundaries (`#`, `##`, `###`)
- Each chunk retains its heading hierarchy as prefix (e.g., `# Doc Title > ## Section`) for context
- If a section exceeds ~1000 characters, further splits on paragraph boundaries (`\n\n`) with ~200 char overlap
- Each chunk carries metadata: `source_file`, `heading_path`, `chunk_index`
- For `.txt` files, falls back to paragraph-based splitting (no headings)

### Ingest Flow (`internal/rag/ingest.go`)

- Walks `knowledge_base/` for `.md` and `.txt` files
- Batches chunks (20 at a time) for embedding API calls
- Upserts to Qdrant with deterministic ID derived from `file_path + chunk_index` — re-running ingest overwrites stale vectors
- Prints progress: files processed, chunks generated, vectors stored
- Qdrant collection auto-created on first ingest using dimension size from embedding model response

## Retrieval Tool

### Tool Definition (`internal/llm/tools/search_knowledge_base.handler.go`)

- Name: `search_knowledge_base`
- Description: "Search the internal knowledge base for relevant documentation. Use when the user asks about internal docs, guides, or reference material."
- Parameters: `{ "query": string, "top_k": int (optional, default 5) }`

### Handler Logic

1. Embed the query via OpenRouter (same embedder used in ingest)
2. Search Qdrant with query vector, retrieve top-k results above similarity threshold (0.7)
3. Format results with source attribution:

```
[Source: architecture.md > ## Data Flow]
The system uses event-driven...

[Source: setup-guide.md > ## Prerequisites]
You need Docker installed...
```

### Integration

- Registered in `schemas.go` alongside existing tools
- RAG tool initialized in `runChatCmd` with Qdrant and embedder dependencies
- System prompt addition: "You have access to an internal knowledge base via the search_knowledge_base tool. Use it when questions relate to internal documentation."
- No forced retrieval — the LLM decides when to call the tool

## Embedder & Qdrant Client

### Embedder (`internal/rag/embedder.go`)

- Uses OpenRouter's `/api/v1/embeddings` endpoint (OpenAI-compatible format)
- Reuses `providers.SharedHTTPClient` for connection pooling
- Method: `Embed(ctx, texts []string) ([][]float32, error)`
- Model from `EMBED_MODEL` env var

### Qdrant Client (`internal/rag/qdrant.go`)

- HTTP client talking to Qdrant's REST API (no gRPC dependency)
- Methods:
  - `EnsureCollection(ctx, dimension int)` — creates collection if not exists
  - `Upsert(ctx, points []Point)` — batch upsert with payload metadata
  - `Search(ctx, vector []float32, topK int, threshold float64) ([]SearchResult, error)`
- Config from existing `providers.Qdrant` struct

### Types

```go
type Point struct {
    ID      string
    Vector  []float32
    Payload map[string]string  // source_file, heading_path, content
}

type SearchResult struct {
    Score   float32
    Payload map[string]string
}
```

## Configuration

Additions to `.env`:

```env
# Embeddings
EMBED_MODEL=nvidia/llama-nemotron-embed-vl-1b-v2:free

# Qdrant
QDRANT_HOST=localhost
QDRANT_PORT=6333
QDRANT_API_KEY=
QDRANT_COLLECTION=knowledge_base
QDRANT_USE_HTTPS=false
```

No new config structs — uses existing `providers.Config`.

## Error Handling

- **Ingest:** logs per-file errors, continues processing. Returns summary with success/failure counts.
- **Embedding failures:** retries once with backoff, then skips batch and logs.
- **Qdrant unavailable:** tool returns "Knowledge base unavailable" so the LLM informs the user.
- **No results above threshold:** tool returns "No relevant documents found" — LLM proceeds without RAG context.

## No New Dependencies

All functionality uses `net/http` calls to Qdrant REST API and OpenRouter. Reuses existing shared HTTP client and config patterns.

---

## Testing the RAG Workflow

Once implementation is complete, here's how to test end-to-end:

### 1. Ingest Documents

Place your markdown/text files in the `knowledge_base/` folder, then run:

```bash
./llm-core ingest
```

Expected output:

```
Processing knowledge_base/architecture.md... 12 chunks
Processing knowledge_base/setup-guide.md... 8 chunks
---
Ingested 2 files, 20 chunks, 20 vectors stored in Qdrant.
```

### 2. Test Retrieval via Chat

```bash
./llm-core chat
```

Ask a question that relates to your knowledge base content. The LLM should call `search_knowledge_base` and you'll see:

```
[🔧 Calling Tool: search_knowledge_base]
    args: {"query":"how to set up the project","top_k":5}

[✅ Tool Result] [Source: setup-guide.md > ## Prerequisites]
You need Docker installed...
```

### 3. Test Chunking Strategy

To verify chunks are generated correctly before embedding, use the dedicated chunking command:

```bash
./llm-core chunking
```

This runs the chunker, writes all chunks with metadata to `chunking.json`, and skips embedding/Qdrant entirely — useful for tuning chunk sizes without API costs.

### 4. Test Retrieval Quality Directly

Add a `search` subcommand for direct vector search without the LLM:

```bash
./llm-core search "how to configure Lambda function URL"
```

Expected output:

```
[0.92] architecture.md > ## Lambda Configuration
  Lambda function URLs provide a dedicated endpoint...

[0.87] setup-guide.md > ## Deployment
  Deploy each service as a Lambda function...

[0.74] troubleshooting.md > ## Common Issues
  If the function URL returns 403...
```

This lets you test retrieval quality, threshold tuning, and embedding accuracy independently of the LLM's decision to call the tool.

### 5. Test Chunking in Isolation

Run the chunker standalone and inspect the output as JSON:

```bash
./llm-core chunking
```

This reads all files from `knowledge_base/`, applies the markdown-aware splitting strategy, and writes the result to `chunking.json` in the current directory.

Output file (`chunking.json`):

```json
[
  {
    "source_file": "architecture.md",
    "heading_path": "# Architecture > ## Data Flow",
    "chunk_index": 0,
    "content": "The system uses event-driven communication between services...",
    "char_count": 847
  },
  {
    "source_file": "architecture.md",
    "heading_path": "# Architecture > ## Data Flow",
    "chunk_index": 1,
    "content": "Each event is published to an SNS topic...",
    "char_count": 623
  }
]
```

No embedding, no Qdrant — purely tests the chunking logic. Useful for tuning chunk sizes and verifying heading extraction before running a full ingest.

### Summary of New Commands

| Command | Purpose |
|---------|---------|
| `./llm-core ingest` | Index `knowledge_base/` into Qdrant |
| `./llm-core chunking` | Chunk `knowledge_base/` and write results to `chunking.json` |
| `./llm-core search "query"` | Test retrieval directly (no LLM involved) |
| `./llm-core chat` | Chat with RAG tool available (existing command, enhanced) |
