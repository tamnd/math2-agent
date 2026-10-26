// Package llmtest has a scripted [llm.Model] for the tests of the agent roles. It calls no model service.
package llmtest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/tamnd/math2-agent/llm"
)

// Script is a model that gives its responses in order, one for each request. It records each request. A Script is safe for use by more than one goroutine.
type Script struct {
	mu        sync.Mutex
	responses []*llm.Response
	requests  []*llm.Request
	calls     int
}

// ErrEnd is the error of a request after the last response of a script.
var ErrEnd = errors.New("llmtest: the script has no more responses")

// New returns a script with the responses.
func New(responses ...*llm.Response) *Script { return &Script{responses: responses} }

// Complete records the request and returns the next response. A response with no model gets the model of the request, and a tool call with no id gets the id call-N, where N counts the calls of the script. It validates the request, as a model service does.
func (s *Script) Complete(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := *req
	c.Messages = append([]llm.Message(nil), req.Messages...)
	s.requests = append(s.requests, &c)
	if len(s.responses) == 0 {
		return nil, ErrEnd
	}
	r := *s.responses[0]
	s.responses = s.responses[1:]
	if r.Model == "" {
		r.Model = req.Model
	}
	r.Content = append([]llm.Block(nil), r.Content...)
	for i, b := range r.Content {
		if b.Type == llm.ToolUse && b.ID == "" {
			s.calls++
			r.Content[i].ID = fmt.Sprintf("call-%d", s.calls)
		}
	}
	return &r, nil
}

// Requests returns the requests that the script received, in order.
func (s *Script) Requests() []*llm.Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*llm.Request(nil), s.requests...)
}

// Say returns a response with the text that ends the turn.
func Say(text string, u llm.Usage) *llm.Response {
	return &llm.Response{Content: []llm.Block{llm.TextBlock(text)}, StopReason: llm.EndTurn, Usage: u}
}

// Call returns a response that calls the tool with the input. The input is marshalled to JSON. The script gives the call its id.
func Call(tool string, input any, u llm.Usage) *llm.Response {
	data, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	return &llm.Response{Content: []llm.Block{{Type: llm.ToolUse, Name: tool, Input: data}}, StopReason: llm.ToolCall, Usage: u}
}
