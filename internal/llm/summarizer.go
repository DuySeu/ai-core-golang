package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/duyseu/llm-core/internal/llm/prompts"
	"github.com/duyseu/llm-core/internal/llm/providers"
)

const SummarizationThreshold int64 = 6

// SummarizeResult is the output of a summarization run.
type SummarizeResult struct {
	Summary         string
	KeyFacts        []string
	SummarizedCount int64
}

// summarizationResult is the structured output from the LLM.
type summarizationResult struct {
	Summary  string   `json:"summary"`
	KeyFacts []string `json:"key_facts"`
}

// Summarize generates an updated conversation summary from the given messages.
func (s *LLMService) Summarize(messages []providers.Message, current SummarizeResult) (SummarizeResult, error) {
	ctx := context.Background()

	var msgBuf strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&msgBuf, "%s: %s\n", m.Role, m.Content)
	}

	var factsStr string
	if len(current.KeyFacts) > 0 {
		factsStr = "- " + strings.Join(current.KeyFacts, "\n- ")
	}

	loader := prompts.NewPromptLoader()
	prompt, err := loader.GetSummarizationPrompt(prompts.SummarizationParams{
		Summary:  current.Summary,
		KeyFacts: factsStr,
		Messages: msgBuf.String(),
	})
	if err != nil {
		return SummarizeResult{}, fmt.Errorf("summarizer: build prompt: %w", err)
	}

	var result summarizationResult
	if err := s.structuredCompletion(ctx, prompt, &result); err != nil {
		return SummarizeResult{}, fmt.Errorf("summarizer: %w", err)
	}

	return SummarizeResult{
		Summary:         result.Summary,
		KeyFacts:        result.KeyFacts,
		SummarizedCount: current.SummarizedCount + int64(len(messages)),
	}, nil
}
