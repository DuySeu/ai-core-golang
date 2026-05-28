package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/duyseu/llm-core/internal/llm/tools"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type OpenAICompletionParams struct {
	Context context.Context
	Client  *openai.Client
	Model   string
	Prompt  string
}

// NewOpenAIClient builds an OpenAI-compatible client from the given config.
func NewOpenAIClient(config OpenAI) (*openai.Client, error) {
	if config.APIKey == "" {
		return nil, fmt.Errorf("OpenAI API key is not found")
	}

	opts := []option.RequestOption{
		option.WithAPIKey(config.APIKey),
		option.WithHTTPClient(SharedHTTPClient),
	}

	if strings.Contains(config.BaseURL, "openrouter.ai") {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	} else if config.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(config.BaseURL))
	}

	client := openai.NewClient(opts...)
	return &client, nil
}

// emitTextDeltas forwards visible assistant text from a stream delta directly onto ch.
func emitTextDeltas(ch chan<- StreamEvent, delta openai.ChatCompletionChunkChoiceDelta) {
	if delta.Content != "" {
		ch <- StreamEvent{Type: EventText, Content: delta.Content}
	}
	if delta.Refusal != "" {
		ch <- StreamEvent{Type: EventText, Content: delta.Refusal}
	}
}

// OpenAICompletion sends messages to an OpenAI-compatible endpoint and returns a streaming event channel.
func OpenAICompletion(params OpenAICompletionParams, messages []Message, tools []*tools.Tool) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 256)
	msgs := []openai.ChatCompletionMessageParamUnion{}
	// -- Map messages --
	openaiMsgs := mapToOpenAIMessages(messages)

	req := openai.ChatCompletionNewParams{
		Model:    params.Model,
		Messages: append(msgs, openaiMsgs...),
	}
	if params.Prompt != "" {
		msgs = append(msgs, openai.SystemMessage(params.Prompt))
	}

	if len(tools) > 0 {
		reqTools := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
		for _, t := range tools {
			reqTools = append(reqTools, openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  t.Schema,
			}))
		}
		req.Tools = reqTools
	}

	stream := params.Client.Chat.Completions.NewStreaming(params.Context, req)

	go func() {
		defer close(ch)

		// Accumulate parallel tool calls by stream index.
		type toolCallAccum struct {
			ID        string
			Name      string
			Arguments string
		}
		pendingToolCalls := make(map[int]*toolCallAccum)

		for stream.Next() {
			chunk := stream.Current()

			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			delta := choice.Delta

			emitTextDeltas(ch, delta)

			for _, tc := range delta.ToolCalls {
				idx := int(tc.Index)
				existing, ok := pendingToolCalls[idx]
				if !ok {
					existing = &toolCallAccum{}
					pendingToolCalls[idx] = existing
				}
				if tc.ID != "" {
					existing.ID = tc.ID
				}
				if tc.Function.Name != "" {
					existing.Name = tc.Function.Name
				}
				existing.Arguments += tc.Function.Arguments
			}

			if choice.FinishReason == "tool_calls" {
				indices := make([]int, 0, len(pendingToolCalls))
				for k := range pendingToolCalls {
					indices = append(indices, k)
				}
				sort.Ints(indices)
				for _, k := range indices {
					full := pendingToolCalls[k]
					if full == nil || full.Name == "" {
						continue
					}
					ch <- StreamEvent{
						Type: EventToolCall,
						Data: Tool{
							ID:        full.ID,
							Name:      full.Name,
							Arguments: full.Arguments,
						},
					}
				}
				pendingToolCalls = make(map[int]*toolCallAccum)
			}
		}

		if err := stream.Err(); err != nil {
			ch <- StreamEvent{Type: EventError, Data: err.Error()}
			return
		}

		ch <- StreamEvent{Type: EventDone}
	}()

	return ch, nil
}

func mapToOpenAIMessages(messages []Message) []openai.ChatCompletionMessageParamUnion {
	var result []openai.ChatCompletionMessageParamUnion

	for _, m := range messages {
		content := strings.TrimSpace(m.Content)

		switch m.Role {
		case "user":
			var attachments []Attachment
			if len(m.Metadata) > 0 {
				attachments = m.Metadata[0].Attachments
			}
			if len(attachments) > 0 {
				parts := []openai.ChatCompletionContentPartUnionParam{
					openai.TextContentPart(content),
				}
				for _, a := range attachments {
					if strings.HasPrefix(a.MediaType, "image/") {
						dataURL := "data:" + a.MediaType + ";base64," + base64.StdEncoding.EncodeToString(a.Data)
						parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
							URL: dataURL,
						}))
					}
				}
				result = append(result, openai.UserMessage(parts))
			} else {
				result = append(result, openai.UserMessage(content))
			}
		case "assistant":
			msg := openai.ChatCompletionAssistantMessageParam{
				Content: openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(content),
				},
			}

			var toolResults []openai.ChatCompletionMessageParamUnion

			if len(m.Metadata) > 0 {
				meta := m.Metadata[0]
				for _, t := range meta.Tool {
					if t.ID != "" {
						msg.ToolCalls = append(msg.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
							OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
								ID: t.ID,
								Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
									Name:      t.Name,
									Arguments: t.Arguments,
								},
							},
						})

						toolResults = append(toolResults, openai.ToolMessage(t.Output, t.ID))
					}
				}
			}

			result = append(result, openai.ChatCompletionMessageParamUnion{
				OfAssistant: &msg,
			})
			result = append(result, toolResults...)
		default:
			result = append(result, openai.UserMessage(content))
		}
	}

	return result
}

// OpenAIStructuredCompletion calls the OpenAI API without streaming using
// response_format json_object. The prompt must describe the desired JSON structure.
// Result must be a pointer; the JSON response is unmarshalled into it.
func OpenAIStructuredCompletion(params OpenAICompletionParams, result any) error {
	req := openai.ChatCompletionNewParams{
		Model: params.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(params.Prompt),
		},
		ResponseFormat: openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
		},
	}

	res, err := params.Client.Chat.Completions.New(params.Context, req)
	if err != nil {
		return fmt.Errorf("openai structured: %w", err)
	}
	if len(res.Choices) == 0 {
		return fmt.Errorf("openai structured: empty response")
	}
	return json.Unmarshal([]byte(res.Choices[0].Message.Content), result)
}
