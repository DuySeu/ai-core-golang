package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"ai-core-golang/internal/llm/tools"

	openrouter "github.com/OpenRouterTeam/go-sdk"
	"github.com/OpenRouterTeam/go-sdk/models/components"
	"github.com/OpenRouterTeam/go-sdk/optionalnullable"
)

type OpenRouterCompletionParams struct {
	Context context.Context
	Client  *openrouter.OpenRouter
	Model   string
	Prompt  string
}

// NewOpenRouterClient builds an OpenRouter client from the given config.
func NewOpenRouterClient(config OpenRouter) (*openrouter.OpenRouter, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is not found")
	}
	client := openrouter.New(
		openrouter.WithSecurity(config.APIKey),
		openrouter.WithClient(SharedHTTPClient),
	)
	return client, nil
}

// OpenRouterCompletion calls the OpenRouter API using the official SDK.
func OpenRouterCompletion(params OpenRouterCompletionParams, messages []Message, tools []*tools.Tool) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 256)
	msgs := []components.ChatMessages{}
	// -- Map messages --
	openrouterMsgs := mapToOpenRouterMessages(messages)

	req := components.ChatRequest{
		Model:    openrouter.Pointer(params.Model),
		Messages: append(msgs, openrouterMsgs...),
		Stream:   openrouter.Pointer(true),
	}
	if params.Prompt != "" {
		msgs = append(msgs, components.CreateChatMessagesSystem(
			components.ChatSystemMessage{
				Content: components.CreateChatSystemMessageContentStr(params.Prompt),
				Role:    components.ChatSystemMessageRoleSystem,
			},
		))
	}
	if len(tools) > 0 {
		req.Tools = make([]components.ChatFunctionTool, 0, len(tools))
		for _, t := range tools {
			req.Tools = append(req.Tools, components.CreateChatFunctionToolChatFunctionToolFunction(components.ChatFunctionToolFunction{
				Type: components.ChatFunctionToolTypeFunction,
				Function: components.ChatFunctionToolFunctionFunction{
					Name:        t.Name,
					Description: openrouter.String(t.Description),
					Parameters:  t.Schema,
				},
			}))
		}
	}

	res, err := params.Client.Chat.Send(params.Context, req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: send request: %w", err)
	}

	if res.EventStream == nil {
		return nil, fmt.Errorf("openrouter: expected event stream but got none")
	}

	go func() {
		defer close(ch)
		defer res.EventStream.Close()

		// Accumulate tool call arguments across delta chunks
		pendingToolCalls := map[int]*Tool{}

		for res.EventStream.Next() {
			event := res.EventStream.Value()
			chunk := event.Data

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]

			// ── Text delta ───────────────────────────────────────────────────
			// OptionalNullable is map[bool]*T keyed by true (see go-sdk/optionalnullable).
			if ptr, set := choice.Delta.Content.Get(); set && ptr != nil && *ptr != "" {
				ch <- StreamEvent{Type: EventText, Content: *ptr}
			}
			// Reasoning (a.k.a. "thinking") deltas are streamed separately so
			// clients can render them as a distinct thought stream.
			if ptr, set := choice.Delta.Reasoning.Get(); set && ptr != nil && *ptr != "" {
				ch <- StreamEvent{Type: EventThinking, Content: *ptr}
			}

			// ── Tool call deltas (accumulate by index) ───────────────────────
			for _, tc := range choice.Delta.ToolCalls {
				idx := int(tc.Index)
				if existing, ok := pendingToolCalls[idx]; ok {
					// Append argument fragment
					if tc.Function != nil && tc.Function.Arguments != nil {
						existing.Arguments += *tc.Function.Arguments
					}
				} else {
					// First delta for this tool call
					tool := &Tool{}
					if tc.ID != nil {
						tool.ID = *tc.ID
					}
					if tc.Function != nil {
						if tc.Function.Name != nil {
							tool.Name = *tc.Function.Name
						}
						if tc.Function.Arguments != nil {
							tool.Arguments = *tc.Function.Arguments
						}
					}
					pendingToolCalls[idx] = tool
				}
			}

			// ── Finish reasons ───────────────────────────────────────────────
			if choice.FinishReason != nil {
				reason := string(*choice.FinishReason)
				switch reason {
				case "tool_calls":
					// Emit each fully-assembled tool call
					for i := 0; i < len(pendingToolCalls); i++ {
						if tc, ok := pendingToolCalls[i]; ok {
							ch <- StreamEvent{
								Type: EventToolCall,
								Data: *tc,
							}
						}
					}
					// Reset for a potential second tool-call round
					pendingToolCalls = map[int]*Tool{}

				case "stop":
					ch <- StreamEvent{Type: EventDone}
					return
				}
			}
		}

		if err := res.EventStream.Err(); err != nil {
			ch <- StreamEvent{Type: EventError, Data: err.Error()}
			return
		}
		ch <- StreamEvent{Type: EventDone}
	}()

	return ch, nil
}

// mapToOpenRouterMessages maps conversation history and tool definitions into OpenRouter API
func mapToOpenRouterMessages(messages []Message) []components.ChatMessages {
	msgs := make([]components.ChatMessages, 0, len(messages))

	appendToolResultMessages := func(meta []Metadata) {
		if len(meta) == 0 {
			return
		}
		for _, t := range meta[0].Tool {
			if t.ID == "" {
				continue
			}
			msgs = append(msgs, components.CreateChatMessagesTool(components.ChatToolMessage{
				Role:       components.ChatToolMessageRoleTool,
				ToolCallID: t.ID,
				Content:    components.CreateChatToolMessageContentStr(t.Output),
			}))
		}
	}

	for _, m := range messages {
		var msg components.ChatMessages
		role := strings.ToLower(m.Role)
		content := strings.TrimSpace(m.Content)

		switch role {
		case "user":
			var attachments []Attachment
			if len(m.Metadata) > 0 {
				attachments = m.Metadata[0].Attachments
			}
			if len(attachments) > 0 {
				parts := []components.ChatContentItems{
					components.CreateChatContentItemsText(components.ChatContentText{Text: content}),
				}
				for _, a := range attachments {
					if strings.HasPrefix(a.MediaType, "image/") {
						dataURL := "data:" + a.MediaType + ";base64," + base64.StdEncoding.EncodeToString(a.Data)
						parts = append(parts, components.CreateChatContentItemsImageURL(components.ChatContentImage{
							ImageURL: components.ChatContentImageImageURL{URL: dataURL},
						}))
					}
				}
				msg = components.CreateChatMessagesUser(components.ChatUserMessage{
					Role:    components.ChatUserMessageRoleUser,
					Content: components.CreateChatUserMessageContentArrayOfChatContentItems(parts),
				})
			} else {
				msg = components.CreateChatMessagesUser(components.ChatUserMessage{
					Role:    components.ChatUserMessageRoleUser,
					Content: components.CreateChatUserMessageContentStr(content),
				})
			}
		case "assistant":
			var toolCalls []components.ChatToolCall
			if len(m.Metadata) > 0 {
				for _, t := range m.Metadata[0].Tool {
					if t.ID != "" {
						toolCalls = append(toolCalls, components.ChatToolCall{
							ID:   t.ID,
							Type: components.ChatToolCallTypeFunction,
							Function: components.ChatToolCallFunction{
								Name:      t.Name,
								Arguments: t.Arguments,
							},
						})
					}
				}
			}
			assistantMsg := components.ChatAssistantMessage{
				Role:      components.ChatAssistantMessageRoleAssistant,
				ToolCalls: toolCalls,
			}
			if content != "" {
				assistantMsg.Content = optionalnullable.From(openrouter.Pointer(components.CreateChatAssistantMessageContentStr(content)))
			}
			msgs = append(msgs, components.CreateChatMessagesAssistant(assistantMsg))
			appendToolResultMessages(m.Metadata)
			continue
		case "tool":
			appendToolResultMessages(m.Metadata)
			continue
		}

		msgs = append(msgs, msg)
	}

	return msgs
}

// OpenRouterStructuredCompletion calls the OpenRouter API without streaming using
// response_format json_object. The prompt must describe the desired JSON structure.
// Result must be a pointer; the JSON response is unmarshalled into it.
func OpenRouterStructuredCompletion(params OpenRouterCompletionParams, result any) error {
	rf := components.CreateResponseFormatJSONObject(components.FormatJSONObjectConfig{})

	req := components.ChatRequest{
		Model: openrouter.Pointer(params.Model),
		Messages: []components.ChatMessages{
			components.CreateChatMessagesUser(components.ChatUserMessage{
				Content: components.CreateChatUserMessageContentStr(params.Prompt),
				Role:    components.ChatUserMessageRoleUser,
			}),
		},
		Stream:         openrouter.Pointer(false),
		ResponseFormat: &rf,
	}

	res, err := params.Client.Chat.Send(params.Context, req)
	if err != nil {
		return fmt.Errorf("openrouter structured: %w", err)
	}
	if res.ChatResult == nil || len(res.ChatResult.Choices) == 0 {
		return fmt.Errorf("openrouter structured: empty response")
	}

	ptr, set := res.ChatResult.Choices[0].Message.Content.Get()
	if !set || ptr == nil || ptr.Str == nil {
		return fmt.Errorf("openrouter structured: no content")
	}
	return json.Unmarshal([]byte(*ptr.Str), result)
}
