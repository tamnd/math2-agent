package llmtest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
)

func TestScript(t *testing.T) {
	s := llmtest.New(llmtest.Call("get", map[string]string{"tag": "1A0000"}, llm.Usage{Input: 5}), llmtest.Say("done", llm.Usage{Output: 3}))
	req := &llm.Request{Model: "m", MaxTokens: 10, Messages: []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock("go")}}}}
	r, err := s.Complete(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if c := r.Calls(); len(c) != 1 || c[0].ID != "call-1" || string(c[0].Input) != `{"tag":"1A0000"}` || r.Model != "m" {
		t.Errorf("the first response is %+v", r)
	}
	if r, _ := s.Complete(context.Background(), req); r.Text() != "done" {
		t.Errorf("the second response is %+v", r)
	}
	if _, err := s.Complete(context.Background(), req); !errors.Is(err, llmtest.ErrEnd) {
		t.Errorf("the third request gives %v", err)
	}
	if _, err := s.Complete(context.Background(), &llm.Request{}); err == nil {
		t.Error("the script accepts a request that is not valid")
	}
	if n := len(s.Requests()); n != 3 {
		t.Errorf("the script recorded %d requests", n)
	}
}
