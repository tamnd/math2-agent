// Package claude is the client of [llm.Model] for the Claude models of Anthropic, through the official Go SDK.
//
// The client streams each request, thus a request with a large output does not time out. It caches the system prompt and the tool definitions, which are the same for each run of a role, and the prefix of the conversation. It never forces a tool choice and never sets a thinking budget: the current models think by default, and the effort of the request controls the depth (spec 12.3). It keeps the thinking blocks as the service returned them, thus the agent can send them back in the next request.
package claude

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/tamnd/math2-agent/llm"
)

// Client is a [llm.Model] on the Claude API.
type Client struct {
	c anthropic.Client
}

// New returns a client with the options of the SDK. With no options, the SDK reads the key from the environment variable ANTHROPIC_API_KEY.
func New(opts ...option.RequestOption) *Client {
	return &Client{c: anthropic.NewClient(opts...)}
}

// Complete sends the request and returns the response of the model.
func (c *Client) Complete(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	params, err := Params(req)
	if err != nil {
		return nil, err
	}
	stream := c.c.Messages.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()
	var msg anthropic.Message
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return nil, fmt.Errorf("claude: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	return response(&msg), nil
}

// Params returns the parameters of the SDK for the request.
func Params(req *llm.Request) (anthropic.MessageNewParams, error) {
	p := anthropic.MessageNewParams{
		Model:        req.Model,
		MaxTokens:    int64(req.MaxTokens),
		CacheControl: anthropic.NewCacheControlEphemeralParam(),
	}
	if req.System != "" {
		p.System = []anthropic.TextBlockParam{{Text: req.System, CacheControl: anthropic.NewCacheControlEphemeralParam()}}
	}
	if req.Effort != "" {
		p.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(req.Effort)}
	}
	for _, t := range req.Tools {
		var schema struct {
			Properties any      `json:"properties"`
			Required   []string `json:"required"`
		}
		if err := json.Unmarshal(t.Schema, &schema); err != nil {
			return p, fmt.Errorf("claude: the schema of the tool %s: %w", t.Name, err)
		}
		tool := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: schema.Properties, Required: schema.Required},
		}
		p.Tools = append(p.Tools, anthropic.ToolUnionParam{OfTool: &tool})
	}
	for i, m := range req.Messages {
		var blocks []anthropic.ContentBlockParamUnion
		for _, b := range m.Content {
			switch b.Type {
			case llm.Text:
				blocks = append(blocks, anthropic.NewTextBlock(b.Text))
			case llm.Thinking:
				blocks = append(blocks, anthropic.NewThinkingBlock(b.Signature, b.Thinking))
			case llm.RedactedThinking:
				blocks = append(blocks, anthropic.NewRedactedThinkingBlock(b.Data))
			case llm.ToolUse:
				blocks = append(blocks, anthropic.NewToolUseBlock(b.ID, b.Input, b.Name))
			case llm.ToolResult:
				blocks = append(blocks, anthropic.NewToolResultBlock(b.ToolUseID, b.Content, b.IsError))
			default:
				return p, fmt.Errorf("claude: message %d has a block of the type %q", i, b.Type)
			}
		}
		if m.Role == llm.Assistant {
			p.Messages = append(p.Messages, anthropic.NewAssistantMessage(blocks...))
		} else {
			p.Messages = append(p.Messages, anthropic.NewUserMessage(blocks...))
		}
	}
	return p, nil
}

// response converts a message of the SDK. It keeps the blocks that the agent loop uses, and drops the blocks of server tools, which a request of math2 does not enable.
func response(m *anthropic.Message) *llm.Response {
	r := &llm.Response{
		Model:      m.Model,
		StopReason: string(m.StopReason),
		Usage: llm.Usage{
			Input:      int(m.Usage.InputTokens),
			Output:     int(m.Usage.OutputTokens),
			CacheRead:  int(m.Usage.CacheReadInputTokens),
			CacheWrite: int(m.Usage.CacheCreationInputTokens),
		},
	}
	for _, b := range m.Content {
		switch b.Type {
		case llm.Text:
			r.Content = append(r.Content, llm.TextBlock(b.Text))
		case llm.Thinking:
			r.Content = append(r.Content, llm.Block{Type: llm.Thinking, Thinking: b.Thinking, Signature: b.Signature})
		case llm.RedactedThinking:
			r.Content = append(r.Content, llm.Block{Type: llm.RedactedThinking, Data: b.Data})
		case llm.ToolUse:
			r.Content = append(r.Content, llm.Block{Type: llm.ToolUse, ID: b.ID, Name: b.Name, Input: b.Input})
		}
	}
	return r
}
