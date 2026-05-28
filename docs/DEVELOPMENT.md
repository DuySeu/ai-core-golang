# Development Guide

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.26+ | Language runtime |
| `uvx` | latest | MCP server subprocess launcher (optional) |

## Setup

### 1. Clone and install dependencies

```bash
git clone https://github.com/duyseu/llm-core.git
cd llm-core
go mod download
```

### 2. Configure environment

```bash
cp .env.example .env
# Edit .env with your API keys
```

Required environment variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `LLM_PROVIDER` | Provider to use | `openrouter`, `openai`, `anthropic` |
| `LLM_MODEL` | Model identifier | `anthropic/claude-3-5-sonnet` |
| `OPENROUTER_API_KEY` | API key for OpenRouter | `sk-or-...` |

Optional variables:

| Variable | Description |
|----------|-------------|
| `EMBED_MODEL` | Embedding model name |
| `DB_HOST`, `DB_PORT`, `DB_USERNAME`, `DB_PASSWORD`, `DB_DATABASE`, `DB_SSLMODE` | Individual DB config |
| `QDRANT_HOST`, `QDRANT_PORT`, `QDRANT_API_KEY`, `QDRANT_COLLECTION` | Vector DB config |
| `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET` | Object storage |

### 3. Build

```bash
go build -o llm-core ./cmd/main.go
```

### 4. Run

```bash
# Chat mode (interactive, with tool calling)
./llm-core chat

# Summarization demo
./llm-core summarize

# Server mode (requires POSTGRES_URL)
./llm-core server --port 8080
```

## Project Layout

```
cmd/main.go              → CLI commands and orchestration
internal/llm/            → Core LLM service layer
internal/llm/providers/  → Provider implementations (OpenAI, Anthropic, OpenRouter)
internal/llm/tools/      → Tool definitions and handlers
internal/llm/prompts/    → Template-based prompt system
internal/mcp/            → MCP client and server manager
```

## Adding a New LLM Provider

1. Create `internal/llm/providers/<name>.go`
2. Implement the completion function matching:
   ```go
   func(ctx context.Context, messages []Message, tools []*tools.Tool, systemPrompt string) (<-chan StreamEvent, error)
   ```
3. Implement structured completion:
   ```go
   func(ctx context.Context, prompt string, result any) error
   ```
4. Add config struct to `configs.go`
5. Add case to the switch in `service.go` → `NewLLMService`
6. Add provider constant to `types.go`

## Adding a New Tool

1. Create `internal/llm/tools/<name>.handler.go`
2. Define input struct with JSON tags and optional `Schema()` method:
   ```go
   type MyInput struct {
       Param string `json:"param" jsonschema:"Description of param"`
   }
   ```
3. Implement handler:
   ```go
   func HandleMyTool(ctx context.Context, input MyInput) (any, error) {
       // implementation
       return result, nil
   }
   ```
4. Register in `schemas.go` → `RegisterTools()`:
   ```go
   NewTool("my_tool", "Description for LLM", HandleMyTool)
   ```

Schema inference:
- Simple structs: schema auto-generated from `json` tags + `jsonschema` tag for descriptions
- Complex schemas: implement `SchemaProvider` interface with custom `Schema() map[string]any`

## Adding a New MCP Server

Add configuration in `cmd/main.go` → `runChatCmd`:

```go
mcpConfigs = append(mcpConfigs, mcp.ServerConfig{
    Name:    "my-server",
    Command: "uvx",
    Args:    []string{"my-mcp-server@latest"},
    Env:     map[string]string{"KEY": "value"},
})
```

Tools from the server are automatically bridged with the prefix `<server-name>_<tool-name>`.

## Adding a New Prompt Template

1. Create `internal/llm/prompts/templates/<name>.txt` with Go template syntax
2. Define params struct in `loader.go`
3. Add render method:
   ```go
   func (p *PromptLoader) GetMyPrompt(params MyParams) (string, error) {
       return p.render("my_template.txt", params)
   }
   ```

Templates are embedded at compile time via `//go:embed` — no runtime file access needed.

## Debugging

### Verbose tool output

The chat command prints tool calls and results inline:
```
[🔧 Calling Tool: get_stock_price]
    args: {"symbol":"HPG","time_frame":"ONE_DAY","count_back":10}
[✅ Tool Result] {"symbol":"HPG","prices":[...]}
```

### MCP debugging

MCP manager logs connection events:
```
[MCP] CallTool: server=aws-docs tool=search_documentation args=...
```

Failed MCP connections are logged as warnings and skipped (non-fatal).

### Common issues

| Issue | Cause | Fix |
|-------|-------|-----|
| `environment variable X is not set` | Missing .env entry | Add to `.env` |
| `unsupported provider` | Invalid `LLM_PROVIDER` | Use `openai`, `anthropic`, or `openrouter` |
| `uvx not found` | MCP launcher missing | Install `uv` or remove MCP configs |
| `failed to connect to database` | Bad `POSTGRES_URL` | Check PostgreSQL is running |
| `tool not found: X` | Tool not registered | Add to `RegisterTools()` |

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific package
go test ./internal/llm/tools/...

# Race detection
go test -race ./...
```

## Building for Production

```bash
# Optimized binary
go build -ldflags="-s -w" -o llm-core ./cmd/main.go

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o llm-core ./cmd/main.go
```
