package llm_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/tamnd/math2-agent/llm"
)

func TestValidate(t *testing.T) {
	msg := []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock("hi")}}}
	tool := llm.Tool{Name: "get", Schema: json.RawMessage(`{"type":"object"}`)}
	good := llm.Request{Model: "m", Messages: msg, MaxTokens: 10, Effort: "max", Tools: []llm.Tool{tool}}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(r *llm.Request){
		"no model":    func(r *llm.Request) { r.Model = "" },
		"no message":  func(r *llm.Request) { r.Messages = nil },
		"no tokens":   func(r *llm.Request) { r.MaxTokens = 0 },
		"an effort":   func(r *llm.Request) { r.Effort = "huge" },
		"a role":      func(r *llm.Request) { r.Messages = []llm.Message{{Role: "system"}} },
		"a tool":      func(r *llm.Request) { r.Tools = []llm.Tool{{Name: "get", Schema: json.RawMessage(`[]`)}} },
		"a tool name": func(r *llm.Request) { r.Tools = []llm.Tool{{Schema: json.RawMessage(`{}`)}} },
	} {
		r := good
		change(&r)
		if r.Validate() == nil {
			t.Errorf("Validate accepts a request with %s", name)
		}
	}
}

func TestCost(t *testing.T) {
	u := llm.Usage{Input: 1000, Output: 2000, CacheRead: 10000, CacheWrite: 4000}
	for _, c := range []struct {
		model string
		u     llm.Usage
		usd   float64
	}{
		{"claude-opus-5-5", u, (1000*4 + 2000*20 + 10000*0.2 + 4000*5) / 1e6},
		{"claude-haiku-5-5", u, (1000*0.1 + 2000*0.5 + 10000*0.01 + 4000*0.125) / 1e6},
		{"claude-haiku-5-5", llm.Usage{Input: 100_001, Output: 10}, (100_001*0.5 + 10*2.5) / 1e6},
	} {
		usd, ok := llm.Cost(llm.Prices, c.model, c.u)
		if !ok || math.Abs(usd-c.usd) > 1e-12 {
			t.Errorf("%s %+v costs %v, and the test wants %v", c.model, c.u, usd, c.usd)
		}
	}
	if _, ok := llm.Cost(llm.Prices, "gpt", u); ok {
		t.Error("Cost knows a model that is not in the table")
	}
	if got := u.Add(u); got != (llm.Usage{2000, 4000, 20000, 8000}) {
		t.Errorf("the sum is %+v", got)
	}
}

func TestResponse(t *testing.T) {
	r := llm.Response{Content: []llm.Block{llm.TextBlock("a"), {Type: llm.ToolUse, Name: "get"}, {Type: llm.Thinking}, llm.TextBlock("b")}}
	if r.Text() != "a\n\nb" || len(r.Calls()) != 1 || r.Calls()[0].Name != "get" {
		t.Errorf("the text is %q and the calls are %+v", r.Text(), r.Calls())
	}
}
