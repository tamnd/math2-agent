package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing/fstest"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
)

func ExampleRunner_Run() {
	fsys := fstest.MapFS{
		"prove/role.yaml":    {Data: []byte("role: prove\nversion: 0.1.0\ntools: [lean_check]\nladder: [{model: claude-sonnet-5-5}]\nbudget: {usd: 0.50}\nmax_tokens: 4000\nturns: 10\nstuck: 3\n")},
		"prove/system.md":    {Data: []byte("You prove lemmas in Lean 4.")},
		"prove/CHANGELOG.md": {Data: []byte("# 0.1.0\n")},
	}
	role, err := agent.LoadRole(fsys, "prove")
	if err != nil {
		panic(err)
	}
	proved := false
	check := agent.Func{
		Def: llm.Tool{Name: "lean_check", Description: "Check Lean code.", Schema: json.RawMessage(`{"type":"object","properties":{"code":{"type":"string"}}}`)},
		Fn: func(context.Context, *agent.Job, json.RawMessage) (agent.Result, error) {
			proved = true
			return agent.Result{Text: "no errors"}, nil
		},
	}
	model := llmtest.New(llmtest.Call("lean_check", map[string]string{"code": "theorem t (n : Nat) : 0 + n = n := by simp"}, llm.Usage{Input: 3000, Output: 500}))
	r := agent.Runner{Role: role, Model: model, Tools: []agent.Tool{check}, Goal: func(context.Context) (bool, string, error) { return proved, "", nil }}
	job := &agent.Job{ID: "j-2026-11-02-1", Role: "prove", Target: "7K3QX2", Goal: "the binding has a proof", OnBehalfOf: "github:octocat"}
	rec, err := r.Run(context.Background(), job, 0, "Prove the statement of 7K3QX2.")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%s: %s, %d turn, $%.3f\n", rec.Outcome, rec.Reason, len(rec.Turns), rec.Spend.USD)
	// Output: success: the goal holds, 1 turn, $0.011
}
