// Package llm is the one interface of math2 to a language model (spec 12.3).
//
// A [Model] completes a [Request]: a system prompt, the messages of the conversation and the tools that the model can call. The [Response] has the content blocks of the model, the reason that it stopped and the [Usage] of tokens. The agent roles see only this interface, thus the model is a configuration value and not a dependency of the code. The package claude in this module is the client for the Claude models of Anthropic, and the package llmtest has a scripted model for tests.
//
// [Cost] gives the price of a usage in US dollars from a [Price] table. [Prices] is the table of spec 12.3.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// The types of a content block.
const (
	Text             = "text"
	Thinking         = "thinking"
	RedactedThinking = "redacted_thinking"
	ToolUse          = "tool_use"
	ToolResult       = "tool_result"
)

// The roles of a message.
const (
	User      = "user"
	Assistant = "assistant"
)

// The reasons that a model stops. A model can give other reasons, and a caller treats them as [EndTurn].
const (
	EndTurn   = "end_turn"
	MaxTokens = "max_tokens"
	ToolCall  = "tool_use"
	Refusal   = "refusal"
	PauseTurn = "pause_turn"
)

// Block is one content block of a message. The fields that a block uses depend on its type.
type Block struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`        // the text of a text block
	Thinking  string          `json:"thinking,omitempty"`    // the text of a thinking block, which can be empty
	Signature string          `json:"signature,omitempty"`   // the signature of a thinking block
	Data      string          `json:"data,omitempty"`        // the data of a redacted thinking block
	ID        string          `json:"id,omitempty"`          // the id of a tool call
	Name      string          `json:"name,omitempty"`        // the name of the tool of a tool call
	Input     json.RawMessage `json:"input,omitempty"`       // the input of a tool call, a JSON object
	ToolUseID string          `json:"tool_use_id,omitempty"` // the id of the tool call of a tool result
	Content   string          `json:"content,omitempty"`     // the text of a tool result
	IsError   bool            `json:"is_error,omitempty"`    // whether the tool call of a tool result failed
}

// TextBlock returns a text block.
func TextBlock(text string) Block { return Block{Type: Text, Text: text} }

// ResultBlock returns the result of the tool call with the id.
func ResultBlock(id, content string, isError bool) Block {
	return Block{Type: ToolResult, ToolUseID: id, Content: content, IsError: isError}
}

// Message is one turn of a conversation.
type Message struct {
	Role    string  `json:"role"` // User or Assistant
	Content []Block `json:"content"`
}

// Tool is a tool that the model can call. The schema is a JSON Schema of an object, the input of the tool.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"input_schema"`
}

// Request is one request to a model.
type Request struct {
	Model     string    `json:"model"`
	System    string    `json:"system,omitempty"` // the system prompt. It is the same for each run of a role, thus a client caches it.
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens"`       // the limit of the output tokens
	Effort    string    `json:"effort,omitempty"` // low, medium, high, xhigh or max. Empty means the default of the model.
}

// The levels of effort, from the lowest.
var efforts = []string{"low", "medium", "high", "xhigh", "max"}

// Validate reports the first problem of the request: no model, no message, a message with a role that is not known, no output tokens, an effort that is not known, or a tool with no name or a schema that is not a JSON object.
func (r *Request) Validate() error {
	switch {
	case r.Model == "":
		return errors.New("llm: the request names no model")
	case len(r.Messages) == 0:
		return errors.New("llm: the request has no message")
	case r.MaxTokens <= 0:
		return errors.New("llm: the request allows no output tokens")
	case r.Effort != "" && !slices.Contains(efforts, r.Effort):
		return fmt.Errorf("llm: the effort %q is not one of %v", r.Effort, efforts)
	}
	for i, m := range r.Messages {
		if m.Role != User && m.Role != Assistant {
			return fmt.Errorf("llm: message %d has the role %q", i, m.Role)
		}
	}
	for _, t := range r.Tools {
		var schema map[string]any
		if t.Name == "" || json.Unmarshal(t.Schema, &schema) != nil {
			return fmt.Errorf("llm: the tool %q has no name or a schema that is not a JSON object", t.Name)
		}
	}
	return nil
}

// Usage is the number of tokens of one or more requests.
type Usage struct {
	Input      int `json:"input"`       // the input tokens that the cache did not have
	Output     int `json:"output"`      // the output tokens, with the thinking tokens
	CacheRead  int `json:"cache_read"`  // the input tokens that came from the cache
	CacheWrite int `json:"cache_write"` // the input tokens that went into the cache
}

// Add returns the sum of u and v.
func (u Usage) Add(v Usage) Usage {
	return Usage{u.Input + v.Input, u.Output + v.Output, u.CacheRead + v.CacheRead, u.CacheWrite + v.CacheWrite}
}

// Response is the answer of a model to one request.
type Response struct {
	Model      string  `json:"model"` // the model that answered
	Content    []Block `json:"content"`
	StopReason string  `json:"stop_reason"`
	Usage      Usage   `json:"usage"`
}

// Calls returns the tool calls of the response, in order.
func (r *Response) Calls() []Block {
	var out []Block
	for _, b := range r.Content {
		if b.Type == ToolUse {
			out = append(out, b)
		}
	}
	return out
}

// Text returns the text blocks of the response, with a blank line between them.
func (r *Response) Text() string {
	s := ""
	for _, b := range r.Content {
		if b.Type == Text {
			if s != "" {
				s += "\n\n"
			}
			s += b.Text
		}
	}
	return s
}

// A Model completes requests. Complete returns an error when the request did not complete. A refusal or a stop at the token limit is a response, with its stop reason.
type Model interface {
	Complete(ctx context.Context, req *Request) (*Response, error)
}

// Price is the price of the tokens of a model, in US dollars for each million tokens. When Over is not nil and the input of a request, with the cached input, is more than Above tokens, the request has the price Over.
type Price struct {
	Input      float64 `json:"input" yaml:"input"`
	Output     float64 `json:"output" yaml:"output"`
	CacheRead  float64 `json:"cache_read" yaml:"cache_read"`
	CacheWrite float64 `json:"cache_write" yaml:"cache_write"`
	Above      int     `json:"above,omitempty" yaml:"above,omitempty"`
	Over       *Price  `json:"over,omitempty" yaml:"over,omitempty"`
}

// Prices are the prices of the models of spec 12.3 on the Claude API at the time of writing. A cache write has the price of the cache with a lifetime of 5 minutes, 1.25 times the input. Give a newer table to [Cost] when the prices change.
var Prices = map[string]Price{
	"claude-haiku-5-5":  {Input: 0.10, Output: 0.50, CacheRead: 0.01, CacheWrite: 0.125, Above: 100_000, Over: &Price{Input: 0.50, Output: 2.50, CacheRead: 0.05, CacheWrite: 0.625}},
	"claude-sonnet-5-5": {Input: 2, Output: 10, CacheRead: 0.20, CacheWrite: 2.50},
	"claude-opus-5-5":   {Input: 4, Output: 20, CacheRead: 0.20, CacheWrite: 5},
	"claude-fable-5-1":  {Input: 10, Output: 50, CacheRead: 0.25, CacheWrite: 12.50},
}

// Cost returns the price in US dollars of the usage of one request to the model, from the table. It reports false when the table has no price for the model.
func Cost(prices map[string]Price, model string, u Usage) (float64, bool) {
	p, ok := prices[model]
	if !ok {
		return 0, false
	}
	if p.Over != nil && u.Input+u.CacheRead+u.CacheWrite > p.Above {
		p = *p.Over
	}
	usd := float64(u.Input)*p.Input + float64(u.Output)*p.Output + float64(u.CacheRead)*p.CacheRead + float64(u.CacheWrite)*p.CacheWrite
	return usd / 1e6, true
}
