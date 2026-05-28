package core

import (
	"context"
	"fmt"

	"github.com/duyseu/llm-core/internal/llm/providers"
	"github.com/duyseu/llm-core/internal/llm/tools"
)

type completionFunc func(context.Context, []providers.Message, []*tools.Tool, string) (<-chan providers.StreamEvent, error)
type structuredCompletionFunc func(ctx context.Context, prompt string, result any) error

// LLMService handles LLM interactions (agentic tool loop + summarization).
// It has no database dependency — all persistence is handled by the caller.
type LLMService struct {
	tools                *tools.Manager
	completion           completionFunc
	structuredCompletion structuredCompletionFunc
}

func NewLLMService(ctx context.Context, cfg providers.LLM, toolMgr *tools.Manager) (*LLMService, error) {
	var completion completionFunc
	var structuredCompletion structuredCompletionFunc
	switch cfg.GetProviderName() {
	case providers.ModelProviderOpenAI:
		client, err := providers.NewOpenAIClient(cfg.OpenAI)
		if err != nil {
			return nil, err
		}
		completion = func(ctx context.Context, history []providers.Message, tools []*tools.Tool, prompt string) (<-chan providers.StreamEvent, error) {
			return providers.OpenAICompletion(providers.OpenAICompletionParams{
				Context: ctx,
				Client:  client,
				Model:   cfg.GetLLMModelName(),
			}, history, tools)
		}
		structuredCompletion = func(ctx context.Context, prompt string, result any) error {
			return providers.OpenAIStructuredCompletion(providers.OpenAICompletionParams{
				Context: ctx,
				Client:  client,
				Model:   cfg.GetLLMModelName(),
				Prompt:  prompt,
			}, result)
		}
	case providers.ModelProviderAnthropic:
		client, err := providers.NewAnthropicClient(ctx, cfg.Anthropic)
		if err != nil {
			return nil, err
		}
		completion = func(ctx context.Context, history []providers.Message, tools []*tools.Tool, prompt string) (<-chan providers.StreamEvent, error) {
			return providers.AnthropicCompletion(providers.AnthropicCompletionParams{
				Context: ctx,
				Client:  client,
				Model:   cfg.GetLLMModelName(),
				Prompt:  prompt,
			}, history, tools)
		}
		structuredCompletion = func(ctx context.Context, prompt string, result any) error {
			return providers.AnthropicStructuredCompletion(providers.AnthropicCompletionParams{
				Context: ctx,
				Client:  client,
				Model:   cfg.GetLLMModelName(),
				Prompt:  prompt,
			}, result)
		}
	case providers.ModelProviderOpenRouter:
		client, err := providers.NewOpenRouterClient(cfg.OpenRouter)
		if err != nil {
			return nil, err
		}
		completion = func(ctx context.Context, history []providers.Message, tools []*tools.Tool, prompt string) (<-chan providers.StreamEvent, error) {
			return providers.OpenRouterCompletion(providers.OpenRouterCompletionParams{
				Context: ctx,
				Client:  client,
				Model:   cfg.GetLLMModelName(),
				Prompt:  prompt,
			}, history, tools)
		}
		structuredCompletion = func(ctx context.Context, prompt string, result any) error {
			return providers.OpenRouterStructuredCompletion(providers.OpenRouterCompletionParams{
				Context: ctx,
				Client:  client,
				Model:   cfg.GetLLMModelName(),
				Prompt:  prompt,
			}, result)
		}
	default:
		return nil, fmt.Errorf("unsupported provider: %q", cfg.GetProviderName())
	}

	return &LLMService{
		tools:                toolMgr,
		completion:           completion,
		structuredCompletion: structuredCompletion,
	}, nil
}

// ──────── Agentic Tool Loop ────────

// runToolRound runs each tool in order, streams ToolResult events, and appends
// one assistant history row containing every tool call/output pair.
func (s *LLMService) runToolRound(ctx context.Context, outputCh chan<- providers.StreamEvent, history []providers.Message, pending []providers.Tool) []providers.Message {
	assembled := make([]providers.Tool, 0, len(pending))
	for _, tc := range pending {
		result, execErr := s.tools.Execute(ctx, tc.Name, tc.Arguments)
		isError := "false"
		if execErr != nil {
			result = execErr.Error()
			isError = "true"
		}
		tr := providers.Tool{ID: tc.ID, Output: result, IsError: isError}
		outputCh <- providers.StreamEvent{Type: providers.EventToolResult, Data: tr}
		assembled = append(assembled, providers.Tool{
			ID:      tc.ID,
			Output:  tr.Output,
			IsError: tr.IsError,
		})
	}
	meta := []providers.Metadata{{Tool: assembled}}
	return append(history, providers.Message{Role: "assistant", Metadata: meta})
}

// LLMOptions holds optional parameters for LLMChat.
type LLMOptions struct {
	SystemPrompt string
}

// LLMChat runs the agentic tool loop: call LLM → execute tool calls → append results → repeat until done.
// It has no database interactions or session management.
func (s *LLMService) LLMChat(ctx context.Context, history []providers.Message, opts ...LLMOptions) (<-chan providers.StreamEvent, error) {
	outputCh := make(chan providers.StreamEvent, 4)

	go func() {
		defer close(outputCh)
		var opt LLMOptions
		if len(opts) > 0 {
			opt = opts[0]
		}
		var toolDefs []*tools.Tool
		if s.tools != nil {
			toolDefs = s.tools.All()
		}

	nextProviderRound:
		for {
			streamCh, err := s.completion(ctx, history, toolDefs, opt.SystemPrompt)
			if err != nil {
				outputCh <- providers.StreamEvent{Type: providers.EventError, Data: err.Error()}
				return
			}

			var pendingTools []providers.Tool

			for event := range streamCh {
				switch event.Type {
				case providers.EventText:
					outputCh <- event

				case providers.EventToolCall:
					tc := event.Data.(providers.Tool)
					outputCh <- event
					pendingTools = append(pendingTools, tc)

				case providers.EventDone:
					if len(pendingTools) > 0 {
						history = s.runToolRound(ctx, outputCh, history, pendingTools)
						pendingTools = pendingTools[:0]
						goto nextProviderRound
					}
					outputCh <- event
					return

				case providers.EventError:
					outputCh <- event
					return
				}
			}

			if len(pendingTools) > 0 {
				history = s.runToolRound(ctx, outputCh, history, pendingTools)
				continue
			}
			return
		}
	}()

	return outputCh, nil
}
