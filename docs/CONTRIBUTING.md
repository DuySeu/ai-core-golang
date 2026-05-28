# Contributing

## Development Workflow

1. Create a feature branch from `main`
2. Implement changes following the guidelines below
3. Write/update tests for any behavioral changes
4. Run `go test ./...` and ensure all tests pass
5. Run `go vet ./...` for static analysis
6. Submit a pull request with a clear description

## Code Style

### Go Conventions

- Follow standard Go formatting (`gofmt`)
- Use meaningful names — no `temp`, `data`, `result` without context
- Keep functions focused and short; extract when a function does more than one thing
- Prefer returning errors over panicking
- Use `context.Context` as the first parameter for functions that do I/O
- Group imports: stdlib, external, internal (separated by blank lines)

### Project-Specific Patterns

- **Provider functions** — Use function types (`completionFunc`, `structuredCompletionFunc`) not interfaces
- **Tool handlers** — Use `NewTool[In]` generic constructor with typed handler functions
- **Streaming** — Return `<-chan StreamEvent`; close the channel when done
- **Error events** — Send errors through the event channel, don't return them from the goroutine
- **MCP tools** — Prefix bridged tool names with server name: `<server>_<tool>`
- **Config** — Read from environment variables via `os.Getenv`; use `LoadConfig()` for structured access

### File Naming

- Tool handlers: `<tool_name>.handler.go`
- Provider implementations: `<provider_name>.go`
- Prompt templates: `templates/<name>.txt`
- Constants/mappings: `const.go`

## Code Review Criteria

All changes are reviewed across five axes:

1. **Correctness** — Does it work? Edge cases handled? Error paths covered?
2. **Readability** — Can someone understand it without explanation? Could it be simpler?
3. **Architecture** — Does it follow existing patterns? Clean module boundaries?
4. **Security** — Input validation? No secrets in code? Safe HTTP handling?
5. **Performance** — Appropriate for the use case? No unnecessary allocations in hot paths?

Approval standard: approve when the change definitely improves overall code health, even if it isn't perfect.

## Testing Requirements

### When to Write Tests

- Any new tool handler
- Any new provider implementation
- Bug fixes (reproduce the bug with a test first)
- Changes to the agentic loop or summarization logic

### Test Structure

```go
func TestToolName_HappyPath(t *testing.T) {
    // Arrange
    input := MyInput{Symbol: "HPG"}
    
    // Act
    result, err := HandleMyTool(context.Background(), input)
    
    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    // verify result...
}

func TestToolName_InvalidInput(t *testing.T) {
    input := MyInput{Symbol: ""}
    _, err := HandleMyTool(context.Background(), input)
    if err == nil {
        t.Fatal("expected error for empty symbol")
    }
}
```

### Running Tests

```bash
go test ./...              # All tests
go test -race ./...        # With race detection
go test -v ./internal/llm/tools/...  # Specific package
```

## Adding New Features

### New Tool Checklist

- [ ] Create `internal/llm/tools/<name>.handler.go`
- [ ] Define input struct with `json` tags and `jsonschema` descriptions
- [ ] Implement `SchemaProvider` if schema is complex
- [ ] Register in `schemas.go` → `RegisterTools()`
- [ ] Add tests for happy path and error cases
- [ ] Update `docs/API.md` with tool schema and output format

### New Provider Checklist

- [ ] Create `internal/llm/providers/<name>.go`
- [ ] Implement streaming completion function
- [ ] Implement structured completion function
- [ ] Add config struct and env var loading
- [ ] Add case to `NewLLMService` switch
- [ ] Add provider constant to `types.go`
- [ ] Test with `./llm-core chat`

### New Prompt Template Checklist

- [ ] Create `internal/llm/prompts/templates/<name>.txt`
- [ ] Define params struct in `loader.go`
- [ ] Add render method to `PromptLoader`
- [ ] Verify template renders correctly (no missing variables)

## Commit Messages

Use clear, descriptive commit messages:

```
feat: add get_stock_price tool for VietCap API
fix: handle empty response in Anthropic structured completion
refactor: extract message mapping into separate functions
docs: update API reference with new tool schemas
```

## Documentation Updates

When making code changes, update the relevant docs:

- New tools → `docs/API.md`
- Architecture changes → `docs/ARCHITECTURE.md`
- New env vars or setup steps → `docs/DEVELOPMENT.md`
- New patterns or conventions → this file

## Questions?

Open an issue for discussion before starting large changes. For small fixes and improvements, submit a PR directly.
