# API Reference

## Core Interfaces

### LLMService

The central service orchestrating LLM interactions. Located in `internal/llm/service.go`.

```go
type LLMService struct { /* unexported fields */ }

func NewLLMService(ctx context.Context, cfg providers.LLM, toolMgr *tools.Manager) (*LLMService, error)
```

#### LLMChat

Runs the agentic tool loop: calls the LLM, executes tool calls, appends results, and repeats until done.

```go
func (s *LLMService) LLMChat(ctx context.Context, history []providers.Message, opts ...LLMOptions) (<-chan providers.StreamEvent, error)
```

**Parameters:**
- `history` — Conversation messages (user + assistant turns)
- `opts` — Optional `LLMOptions{SystemPrompt: "..."}` for system prompt injection

**Returns:** A channel of `StreamEvent` that emits text deltas, tool calls, tool results, errors, and a final done signal.

#### Summarize

Generates a conversation summary using structured completion.

```go
func (s *LLMService) Summarize(messages []providers.Message, current SummarizeResult) (SummarizeResult, error)
```

**Parameters:**
- `messages` — Messages to summarize
- `current` — Existing summary state to merge with

**Returns:**
```go
type SummarizeResult struct {
    Summary         string
    KeyFacts        []string
    SummarizedCount int64
}
```

---

## Provider Types

Defined in `internal/llm/providers/types.go`.

### Message

```go
type Message struct {
    Role     string     `json:"role"`      // "user" | "assistant" | "tool"
    Content  string     `json:"content"`
    Metadata []Metadata `json:"metadata,omitempty"`
}

type Metadata struct {
    Tool        []Tool       `json:"toolCalls"`
    Attachments []Attachment `json:"attachments"`
    Sources     []Source     `json:"sources"`
}
```

### StreamEvent

```go
type StreamEvent struct {
    Type    StreamEventType `json:"type"`
    Content string          `json:"content,omitempty"`
    Data    any             `json:"data,omitempty"`
}
```

Event types:

| Type | Content/Data | Description |
|------|-------------|-------------|
| `EventText` | `Content: string` | Text token delta |
| `EventThinking` | `Content: string` | Reasoning token (OpenRouter) |
| `EventToolCall` | `Data: Tool` | LLM requests tool execution |
| `EventToolResult` | `Data: Tool` | Tool execution result |
| `EventError` | `Data: string` | Error message |
| `EventDone` | — | Stream complete |

### Tool

```go
type Tool struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Arguments string `json:"arguments"`  // JSON string
    Output    string `json:"output"`
    IsError   string `json:"is_error"`   // "true" | "false"
}
```

### Attachment

```go
type Attachment struct {
    Name      string `json:"name"`
    MediaType string `json:"media_type"`  // e.g. "image/png"
    Path      string `json:"path"`
    Data      []byte `json:"-"`           // resolved at runtime
}
```

---

## Tool System

### Tool Definition

Located in `internal/llm/tools/tool.go`.

```go
type Tool struct {
    Name        string
    Description string
    Schema      map[string]any
    Execute     func(ctx context.Context, rawArgs json.RawMessage) (json.RawMessage, error)
}
```

### Creating Tools

Type-safe tool creation with automatic schema inference:

```go
func NewTool[In any](name, description string, handler func(ctx context.Context, input In) (any, error)) *Tool
```

### Tool Manager

```go
type Manager struct { /* unexported */ }

func NewManager(tools []*Tool) *Manager
func (m *Manager) All() []*Tool
func (m *Manager) Execute(ctx context.Context, name string, rawArgs string) (string, error)
```

### SchemaProvider Interface

For tools with complex schemas that can't be inferred from struct tags:

```go
type SchemaProvider interface {
    Schema() map[string]any
}
```

---

## Native Tools

### get_stock_price

Fetches OHLC price data from VietCap Trading API.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "symbol": { "type": "string", "description": "Stock symbol, e.g., HPG" },
    "time_frame": { "type": "string", "enum": ["ONE_DAY", "ONE_MINUTE", "ONE_HOUR"], "default": "ONE_DAY" },
    "count_back": { "type": "integer", "description": "Number of data points", "default": 10, "minimum": 1, "maximum": 100 }
  },
  "required": ["symbol"]
}
```

**Output:**
```json
{
  "symbol": "HPG",
  "prices": [
    { "time": "2025-05-28 00:00:00", "open": 25.5, "high": 26.0, "low": 25.2, "close": 25.8, "volume": 12345678 }
  ]
}
```

### get_report

Fetches quarterly or yearly financial ratios from VietCap GraphQL API.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "symbol": { "type": "string", "description": "Stock symbol, e.g., HPG" },
    "period": { "type": "string", "enum": ["Q", "Y"], "default": "Q" }
  },
  "required": ["symbol"]
}
```

**Output:** Full `CompanyFinancialRatio` object with revenue, ROE, ROA, PE, PB, EPS, margins, and 100+ financial metrics.

### piotroski_evaluation (internal)

Calculates the Piotroski F-Score (0-9) for fundamental analysis.

**Input:** `{ "symbol": "HPG" }`

**Output:**
```json
{
  "symbol": "HPG",
  "period": "Q1/2025",
  "score": 7,
  "details": {
    "net_income": true,
    "roa": true,
    "net_operating_cash_flow": true,
    "cash_flow_from_operations": true,
    "long_term_debt": false,
    "current_ratio": true,
    "news_issued": true,
    "gross_margin": true,
    "asset_turnover_ratio": false
  }
}
```

### altman_z-score (stub)

Calculates the Altman Z-Score for bankruptcy prediction. Currently returns placeholder values.

**Input:** `{ "symbol": "HPG" }`

---

## MCP Integration

### Client

Located in `internal/mcp/client.go`. Wraps the MCP SDK to manage stdio subprocess connections.

```go
func New(ctx context.Context, command string, args []string, extraEnv map[string]string) (*Client, error)
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error)
func (c *Client) ListTools(ctx context.Context) ([]*mcp.Tool, error)
func (c *Client) Close() error
```

### Manager

Located in `internal/mcp/manager.go`. Thread-safe registry of multiple MCP server connections.

```go
func NewManager(configs []ServerConfig) *Manager
func (m *Manager) GetOrStart(ctx context.Context, name string) (*Client, error)
func (m *Manager) CallTool(ctx context.Context, server, tool string, args map[string]any) (string, error)
func (m *Manager) ConfiguredServers() []string
func (m *Manager) CloseAll()
```

**ServerConfig:**
```go
type ServerConfig struct {
    Name    string            `json:"name"`
    Command string            `json:"command"`
    Args    []string          `json:"args"`
    Env     map[string]string `json:"env"`
}
```

### Tool Bridging

`tools.BridgeMCPTools` converts MCP tools into internal format:
- Tool names are prefixed: `<server>_<tool>` (e.g., `aws-docs_search_documentation`)
- Schemas are preserved from MCP `inputSchema`
- Execute closures route through `Manager.CallTool` (with retry)

---

## Prompt System

Located in `internal/llm/prompts/`.

### Template Loader

```go
func NewPromptLoader() *PromptLoader
func (p *PromptLoader) GetSystemPrompt(params SystemParams) (string, error)
func (p *PromptLoader) GetSummarizationPrompt(params SummarizationParams) (string, error)
func (p *PromptLoader) GetResearchPrompt(params ResearchParams) (string, error)
func (p *PromptLoader) GetMetricsPrompt(params MetricsParams) (string, error)
```

### Template Parameters

```go
type SystemParams struct {
    Date, Summary, KeyFacts string
}

type SummarizationParams struct {
    Summary, KeyFacts, Messages string
}

type ResearchParams struct {
    Ticker, Date string
}

type MetricsParams struct {
    Ticker, Content string
}
```

Templates are embedded via `//go:embed templates/*.txt` and parsed at package init time.

---

## Configuration

### Provider Selection

The active provider is determined by `LLM_PROVIDER` env var. The `LLM` config struct holds credentials for all providers; only the selected one is used.

```go
type LLM struct {
    OpenAI     OpenAI
    Anthropic  Anthropic
    OpenRouter OpenRouter
}

func (c *LLM) GetProviderName() ModelProvider  // from LLM_PROVIDER env
func (c *LLM) GetLLMModelName() string         // from LLM_MODEL env
func (c *LLM) GetEmbedModelName() string       // from EMBED_MODEL env
```

### AWS Bedrock (Anthropic)

When using Anthropic via AWS Bedrock, configure the `AWS` field:

```go
type AWSCredentialConfig struct {
    Type            string  // "default" or "assume_role"
    Region          string
    RoleARN         string  // required for assume_role
    Duration        int64   // seconds
    RoleSessionName string
}
```

### Shared HTTP Client

All non-Bedrock providers share a connection-pooled HTTP client:

```go
var SharedHTTPClient = &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 100,
        IdleConnTimeout:     90 * time.Second,
        ForceAttemptHTTP2:   true,
    },
}
```
