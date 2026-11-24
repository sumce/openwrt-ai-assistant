// Package llm is a thin wrapper around github.com/sashabaranov/go-openai —
// the library owns the wire protocol (SSE parsing, request marshalling,
// error shapes); this package adds what the library deliberately leaves
// out: incremental tool-call assembly across chunks and a non-streaming
// fallback for "OpenAI compatible" endpoints that ignore stream=true.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Client talks to one OpenAI-compatible endpoint.
type Client struct {
	baseURL string // API root WITHOUT /chat/completions, e.g. https://api.openai.com/v1
	apiKey  string
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey}
}

func (c *Client) oai() *openai.Client {
	cfg := openai.DefaultConfig(c.apiKey)
	cfg.BaseURL = c.baseURL
	return openai.NewClientWithConfig(cfg)
}

type StreamCallbacks struct {
	// OnText receives incremental assistant content as it arrives.
	OnText func(delta string)
}

type StreamResult struct {
	Content      string
	ToolCalls    []ToolCall // assembled across chunks
	FinishReason string
}

// StreamChat sends a streaming chat completion and assembles the result.
// When the endpoint does not really stream (plain JSON body or empty
// stream), it transparently falls back to a regular completion.
func (c *Client) StreamChat(ctx context.Context, model string, messages []Message, tools []ToolDef, cb StreamCallbacks) (*StreamResult, error) {
	req := openai.ChatCompletionRequest{
		Model:    model,
		Messages: toOpenAIMessages(messages),
		Stream:   true,
	}
	if len(tools) > 0 {
		req.Tools = toOpenAITools(tools)
	}

	res := &StreamResult{FinishReason: "stop"}
	stream, err := c.oai().CreateChatCompletionStream(ctx, req)
	if err == nil {
		for {
			chunk, rerr := stream.Recv()
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				stream.Close()
				return nil, wrapErr(rerr)
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]
			if choice.Delta.Content != "" {
				res.Content += choice.Delta.Content
				if cb.OnText != nil {
					cb.OnText(choice.Delta.Content)
				}
			}
			for _, tc := range choice.Delta.ToolCalls {
				idx := 0
				if tc.Index != nil {
					idx = *tc.Index
				}
				for len(res.ToolCalls) <= idx {
					res.ToolCalls = append(res.ToolCalls, ToolCall{Type: "function"})
				}
				t := &res.ToolCalls[idx]
				if tc.ID != "" {
					t.ID = tc.ID
				}
				if tc.Type != "" {
					t.Type = string(tc.Type)
				}
				if tc.Function.Name != "" {
					t.Function.Name = tc.Function.Name
				}
				t.Function.Arguments += tc.Function.Arguments
			}
			if choice.FinishReason != "" {
				res.FinishReason = string(choice.FinishReason)
			}
		}
		stream.Close()

		if res.Content != "" || len(res.ToolCalls) > 0 {
			return res, nil
		}
		// Nothing came back — the endpoint likely answered a plain JSON
		// body that is not SSE. Retry without streaming below.
	}

	req.Stream = false
	resp, ferr := c.oai().CreateChatCompletion(ctx, req)
	if ferr != nil {
		return nil, wrapErr(ferr)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("上游未返回任何选择")
	}
	msg := resp.Choices[0].Message
	res = &StreamResult{
		Content:      msg.Content,
		ToolCalls:    fromOpenAIToolCalls(msg.ToolCalls),
		FinishReason: string(resp.Choices[0].FinishReason),
	}
	if res.Content != "" && cb.OnText != nil {
		cb.OnText(res.Content)
	}
	return res, nil
}

// Test verifies connectivity: GET /models first (free); if the endpoint
// does not implement it, fall back to a 1-token chat completion.
// Returns the model list when available.
func (c *Client) Test(ctx context.Context) (models []string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	ms, err := c.oai().ListModels(ctx)
	if err == nil {
		for _, m := range ms.Models {
			models = append(models, m.ID)
		}
		return models, nil
	}

	_, cerr := c.oai().CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:     "ping",
		Messages:  []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, Content: "ping"}},
		MaxTokens: 1,
	})
	if cerr == nil {
		return nil, nil // reachable even though /models failed
	}
	return nil, wrapErr(cerr)
}

func wrapErr(err error) error {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) && apiErr.Message != "" {
		if apiErr.HTTPStatusCode != 0 {
			return fmt.Errorf("上游错误(%d): %s", apiErr.HTTPStatusCode, apiErr.Message)
		}
		return fmt.Errorf("上游错误: %s", apiErr.Message)
	}
	return err
}

// ---- type conversion between the internal shape and go-openai ----

func toOpenAIMessages(in []Message) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, len(in))
	for i, m := range in {
		om := openai.ChatCompletionMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			om.ToolCalls = make([]openai.ToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				om.ToolCalls[j] = openai.ToolCall{
					ID:       tc.ID,
					Type:     openai.ToolTypeFunction,
					Function: openai.FunctionCall{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
				}
			}
		}
		out[i] = om
	}
	return out
}

func toOpenAITools(in []ToolDef) []openai.Tool {
	out := make([]openai.Tool, len(in))
	for i, t := range in {
		out[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  json.RawMessage(t.Function.Parameters),
			},
		}
	}
	return out
}

func fromOpenAIToolCalls(in []openai.ToolCall) []ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]ToolCall, len(in))
	for i, tc := range in {
		out[i] = ToolCall{
			ID:       tc.ID,
			Type:     string(tc.Type),
			Function: FunctionCall{Name: tc.Function.Name, Arguments: tc.Function.Arguments},
		}
	}
	return out
}
