package claude_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/claude"
)

// events is the stream of a response with a thinking block and a tool call.
var events = []string{
	`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":null,"usage":{"input_tokens":10,"output_tokens":1,"cache_read_input_tokens":500,"cache_creation_input_tokens":30}}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Check the case n = 0."}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-1"}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"lean_check","input":{}}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"code\": "}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"#check Nat\"}"}}`,
	`{"type":"content_block_stop","index":1}`,
	`{"type":"message_delta","delta":{"stop_reason":"tool_use","stop_sequence":null},"usage":{"output_tokens":42}}`,
	`{"type":"message_stop"}`,
}

func TestComplete(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(data, &body); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			var head struct{ Type string }
			_ = json.Unmarshal([]byte(e), &head)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", head.Type, e)
		}
	}))
	defer srv.Close()
	c := claude.New(option.WithoutEnvironmentDefaults(), option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	req := &llm.Request{
		Model:     "claude-opus-5-5",
		System:    "You prove lemmas.",
		MaxTokens: 1000,
		Effort:    "high",
		Tools:     []llm.Tool{{Name: "lean_check", Description: "Check Lean code.", Schema: json.RawMessage(`{"type":"object","properties":{"code":{"type":"string"}},"required":["code"]}`)}},
		Messages: []llm.Message{
			{Role: llm.User, Content: []llm.Block{llm.TextBlock("Prove 0 + n = n.")}},
			{Role: llm.Assistant, Content: []llm.Block{{Type: llm.Thinking, Thinking: "Use induction.", Signature: "sig-0"}, {Type: llm.ToolUse, ID: "toolu_0", Name: "lean_check", Input: json.RawMessage(`{"code":"example : 0 + 0 = 0 := rfl"}`)}}},
			{Role: llm.User, Content: []llm.Block{llm.ResultBlock("toolu_0", "no errors", false)}},
		},
	}
	resp, err := c.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	want := &llm.Response{
		Model:      "claude-opus-5-5",
		StopReason: llm.ToolCall,
		Usage:      llm.Usage{Input: 10, Output: 42, CacheRead: 500, CacheWrite: 30},
		Content: []llm.Block{
			{Type: llm.Thinking, Thinking: "Check the case n = 0.", Signature: "sig-1"},
			{Type: llm.ToolUse, ID: "toolu_1", Name: "lean_check", Input: json.RawMessage(`{"code": "#check Nat"}`)},
		},
	}
	if !reflect.DeepEqual(resp, want) {
		t.Errorf("the response is %+v", resp)
	}
	sent, _ := json.Marshal(body)
	for _, s := range []string{
		`"stream":true`,
		`"effort":"high"`,
		`"system":[{"cache_control":{"type":"ephemeral"},"text":"You prove lemmas.","type":"text"}]`,
		`{"signature":"sig-0","thinking":"Use induction.","type":"thinking"}`,
		`"input":{"code":"example : 0 + 0 = 0 := rfl"}`,
		`"tool_use_id":"toolu_0"`,
		`"required":["code"]`,
	} {
		if !strings.Contains(string(sent), s) {
			t.Errorf("the request has no %s:\n%s", s, sent)
		}
	}
	for _, s := range []string{"tool_choice", "budget_tokens"} {
		if strings.Contains(string(sent), s) {
			t.Errorf("the request has %s", s)
		}
	}
}

func TestCompleteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`)
	}))
	defer srv.Close()
	c := claude.New(option.WithoutEnvironmentDefaults(), option.WithAPIKey("test"), option.WithBaseURL(srv.URL), option.WithMaxRetries(0))
	_, err := c.Complete(context.Background(), &llm.Request{Model: "claude-opus-5-5", MaxTokens: 10, Messages: []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock("hi")}}}})
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("the error is %v", err)
	}
	if _, err := c.Complete(context.Background(), &llm.Request{Model: "claude-opus-5-5"}); err == nil {
		t.Error("Complete sends a request with no message")
	}
}
