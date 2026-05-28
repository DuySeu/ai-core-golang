package prompts

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
	"time"
)

//go:embed templates/*.txt
var templatesFS embed.FS

// templates is parsed once at startup — any syntax error in .txt files
// will cause a panic immediately, rather than a silent runtime failure.
var templates = template.Must(
	template.ParseFS(templatesFS, "templates/*.txt"),
)

// ResearchParams holds the variables for the research prompt.
type ResearchParams struct {
	Ticker string
	Date   string
}

// MetricsParams holds the variables for the metrics prompt.
type MetricsParams struct {
	Ticker  string
	Content string
}

// SummarizationParams holds the variables for the summarization prompt.
type SummarizationParams struct {
	Summary  string
	KeyFacts string
	Messages string
}

// SystemParams holds the variables for the system prompt.
type SystemParams struct {
	Date     string
	Summary  string
	KeyFacts string
}

// PromptLoader renders prompts from embedded Go templates.
type PromptLoader struct{}

// NewPromptLoader creates a new prompt loader.
func NewPromptLoader() *PromptLoader {
	return &PromptLoader{}
}

// render executes a named template with the given data and returns the result.
func (p *PromptLoader) render(name string, data any) (string, error) {
	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("failed to render prompt %s: %w", name, err)
	}
	return buf.String(), nil
}

// GetResearchPrompt renders the research prompt with the given params.
func (p *PromptLoader) GetResearchPrompt(params ResearchParams) (string, error) {
	return p.render("research_prompt.txt", params)
}

// GetMetricsPrompt renders the metrics prompt with the given params.
func (p *PromptLoader) GetMetricsPrompt(params MetricsParams) (string, error) {
	return p.render("metrics_prompt.txt", params)
}

// GetSummarizationPrompt renders the summarization prompt with the given params.
func (p *PromptLoader) GetSummarizationPrompt(params SummarizationParams) (string, error) {
	return p.render("summarization_prompt.txt", params)
}

// GetSummaryPrompt renders the summary prompt with the given params.
func (p *PromptLoader) GetSystemPrompt(params SystemParams) (string, error) {
	params.Date = time.Now().Format("2006-01-02")
	return p.render("system_prompt.txt", params)
}
