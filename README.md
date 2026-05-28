# LLM Core

A Go CLI toolkit for LLM-powered financial analysis of the Vietnamese stock market. Supports multiple LLM providers (OpenAI, Anthropic, OpenRouter, AWS Bedrock) with an agentic tool-calling loop, MCP integration, and conversation summarization.

## Features

- **Multi-provider LLM support** — OpenAI, Anthropic (direct + AWS Bedrock), OpenRouter via a unified provider abstraction
- **Agentic tool loop** — Iterative tool calling until the LLM completes its reasoning
- **Financial analysis tools** — Stock prices, financial reports, Piotroski F-Score, Altman Z-Score (Vietnamese market via VietCap API)
- **MCP integration** — Dynamically bridge tools from external MCP servers (e.g., AWS documentation)
- **Conversation summarization** — Structured completion to compress conversation history
- **Streaming output** — Real-time token streaming to console with UTF-8-safe chunking
- **Template-based prompts** — Embedded Go templates for system, research, metrics, and summarization prompts

## Quick Start

### Prerequisites

- Go 1.26+
- An LLM API key (OpenRouter recommended for multi-model access)
- PostgreSQL (for server mode)
- `uvx` (optional, for MCP tool bridging)

### Installation

```bash
git clone https://ai-core-golang.git
cd llm-core
go mod download
```

### Configuration

Create a `.env` file in the project root:

```env
# LLM Provider
LLM_PROVIDER=openrouter          # openai | anthropic | openrouter
LLM_MODEL=anthropic/claude-3-5-sonnet

# API Keys
OPENROUTER_API_KEY=sk-or-...

# Database (server mode)
POSTGRES_URL=postgres://user:pass@localhost:5432/llmcore?sslmode=disable
```

### Running

```bash
# Build
go build -o llm-core ./cmd/main.go

# Interactive chat with tool calling
./llm-core chat

# Summarize a conversation
./llm-core summarize

# Run API server
./llm-core server --port 8080
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `chat` | Interactive streaming chat with agentic tool loop and MCP tools |
| `summarize` | Demonstrate conversation summarization with structured output |
| `server` | Run the HTTP API server (requires PostgreSQL) |

## Project Structure

```
llm-core-golang/
├── cmd/
│   └── main.go                    # CLI entry point (urfave/cli)
├── internal/
│   ├── llm/
│   │   ├── service.go             # LLMService: agentic loop + summarization
│   │   ├── summarizer.go          # Conversation summarization logic
│   │   ├── providers/
│   │   │   ├── types.go           # Shared types (Message, StreamEvent, Tool)
│   │   │   ├── configs.go         # Configuration loading from env
│   │   │   ├── openai.go          # OpenAI provider implementation
│   │   │   ├── anthropic.go       # Anthropic provider (direct + Bedrock)
│   │   │   └── openrouter.go      # OpenRouter provider implementation
│   │   ├── tools/
│   │   │   ├── tool.go            # Tool definition + Manager
│   │   │   ├── bridge.go          # MCP → internal tool bridging
│   │   │   ├── schemas.go         # Tool registration
│   │   │   ├── const.go           # VietCap API constants & mappings
│   │   │   ├── get_stock_price.handler.go
│   │   │   ├── get_report.handler.go
│   │   │   ├── piotroski_evaluation.handler.go
│   │   │   └── altman_z-score.handler.go
│   │   └── prompts/
│   │       ├── loader.go          # Template rendering engine
│   │       └── templates/         # Embedded .txt templates
│   └── mcp/
│       ├── client.go              # MCP client (stdio subprocess)
│       └── manager.go             # Multi-server MCP manager
├── go.mod
├── go.sum
└── .env                           # Environment configuration
```

## Documentation

- [Architecture](./ARCHITECTURE.md) — System design, patterns, and data flow
- [Development Guide](./DEVELOPMENT.md) — Setup, building, testing, debugging
- [API Reference](./API.md) — Tool schemas, provider interfaces, MCP protocol
- [Contributing](./CONTRIBUTING.md) — Code style, workflow, and guidelines
