package providers

type ModelProvider string
type StreamEventType string

const (
	EventThinking           StreamEventType = "thinking"
	EventText               StreamEventType = "text"
	EventToolCall           StreamEventType = "tool_call"
	EventToolResult         StreamEventType = "tool_result"
	EventError              StreamEventType = "error"
	EventDone               StreamEventType = "done"
	ModelProviderAWS        ModelProvider   = "aws"
	ModelProviderAnthropic  ModelProvider   = "anthropic"
	ModelProviderOpenAI     ModelProvider   = "openai"
	ModelProviderOpenRouter ModelProvider   = "openrouter"
)

type StreamEvent struct {
	Type    StreamEventType `json:"type"`
	Content string          `json:"content,omitempty"` // For text delta
	Data    any             `json:"data,omitempty"`    // For Error or ToolCall/Result details
}

type Message struct {
	Role     string     `json:"role"`
	Content  string     `json:"content"`
	Metadata []Metadata `json:"metadata,omitempty"`
}

type Metadata struct {
	Tool        []Tool       `json:"toolCalls"`
	Attachments []Attachment `json:"attachments"`
	Sources     []Source     `json:"sources"`
}

type Tool struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
	Output    string `json:"output"`
	IsError   string `json:"is_error"`
}

type Attachment struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Path      string `json:"path"`
	Data      []byte `json:"-"` // transient: resolved at runtime, not persisted
}

// Source is a reference URL used in the research.
type Source struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}
