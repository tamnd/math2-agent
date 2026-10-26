package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/llm/llmtest"
	"github.com/tamnd/math2-agent/prov"
)

const roleYAML = `role: prove
version: 0.1.0
tools: [lean_check]
ladder:
  - {model: claude-haiku-5-5, effort: medium}
  - {model: claude-opus-5-5, effort: high}
budget: {usd: 1, wall: 1h}
max_tokens: 1000
turns: 6
stuck: 2
`

func roleFS() fstest.MapFS {
	return fstest.MapFS{
		"prove/role.yaml":       {Data: []byte(roleYAML)},
		"prove/system.md":       {Data: []byte("You prove lemmas.")},
		"prove/CHANGELOG.md":    {Data: []byte("# 0.1.0\n")},
		"prove/style/naming.md": {Data: []byte("Name lemmas as Mathlib does.")},
		"prove/examples/r-1.md": {Data: []byte("An example.")},
		"explain/role.yaml":     {Data: []byte(strings.Replace(roleYAML, "role: prove", "role: prove", 1))},
		"explain/system.md":     {Data: []byte("x")},
		"explain/CHANGELOG.md":  {Data: []byte("x")},
	}
}

func TestLoadRole(t *testing.T) {
	r, err := agent.LoadRole(roleFS(), "prove")
	if err != nil {
		t.Fatal(err)
	}
	if r.Agent() != "m2-agent/prove@0.1.0" || r.Prompt() != "prove/0.1.0" || !prov.IsAgent(r.Agent()) {
		t.Errorf("the role is %s, %s", r.Agent(), r.Prompt())
	}
	if r.System != "You prove lemmas.\n\nName lemmas as Mathlib does." || len(r.Examples) != 1 || len(r.Files) != 5 {
		t.Errorf("the system prompt is %q, with %d examples and %d files", r.System, len(r.Examples), len(r.Files))
	}
	if time.Duration(r.Budget.Wall) != time.Hour || r.Ladder[1].Model != "claude-opus-5-5" {
		t.Errorf("the role is %+v", r)
	}
	if _, err := agent.LoadRole(roleFS(), "explain"); err == nil || !strings.Contains(err.Error(), `"prove" in the directory explain`) {
		t.Errorf("a role in the wrong directory gives %v", err)
	}
	if _, err := agent.LoadRole(roleFS(), "golf"); err == nil {
		t.Error("LoadRole accepts a role that is not in spec 12.1")
	}
	for name, text := range map[string]string{
		"a version":    strings.Replace(roleYAML, "0.1.0", "1.0", 1),
		"a field":      roleYAML + "temperature: 1\n",
		"an effort":    strings.Replace(roleYAML, "effort: high", "effort: huge", 1),
		"no turns":     strings.Replace(roleYAML, "turns: 6", "turns: 0", 1),
		"report_stuck": strings.Replace(roleYAML, "[lean_check]", "[lean_check, report_stuck]", 1),
	} {
		fsys := roleFS()
		fsys["prove/role.yaml"] = &fstest.MapFile{Data: []byte(text)}
		if _, err := agent.LoadRole(fsys, "prove"); err == nil {
			t.Errorf("LoadRole accepts a role with %s", name)
		}
	}
	fsys := roleFS()
	delete(fsys, "prove/system.md")
	if _, err := agent.LoadRole(fsys, "prove"); err == nil {
		t.Error("LoadRole accepts a role with no system prompt")
	}
}

func job() *agent.Job {
	return &agent.Job{ID: "j-2026-11-02-41a7", Role: "prove", Target: "7K3QX2", Goal: "the binding has a proof", OnBehalfOf: "project:ug-core"}
}

func TestJobValidate(t *testing.T) {
	if err := job().Validate(); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(j *agent.Job){
		"an id":       func(j *agent.Job) { j.ID = "job-1" },
		"a role":      func(j *agent.Job) { j.Role = "golf" },
		"no target":   func(j *agent.Job) { j.Target = "" },
		"an agent":    func(j *agent.Job) { j.OnBehalfOf = "m2-agent/prove@0.1.0" },
		"a requester": func(j *agent.Job) { j.RequestedBy = "project:x" },
	} {
		j := job()
		change(j)
		if j.Validate() == nil {
			t.Errorf("Validate accepts a job with %s", name)
		}
	}
}

// checker is a lean_check tool. The goal holds after a call with the code "done".
type checker struct{ done bool }

func (c *checker) tool() agent.Tool {
	return agent.Func{
		Def: llm.Tool{Name: "lean_check", Description: "Check Lean code.", Schema: json.RawMessage(`{"type":"object","properties":{"code":{"type":"string"}}}`)},
		Fn: func(_ context.Context, _ *agent.Job, input json.RawMessage) (agent.Result, error) {
			var in struct{ Code string }
			_ = json.Unmarshal(input, &in)
			if in.Code == "bad" {
				return agent.Result{}, errors.New("unknown identifier")
			}
			c.done = in.Code == "done"
			return agent.Result{Text: "no errors", LeanSessionS: 2}, nil
		},
	}
}

func runner(t *testing.T, model llm.Model) (*agent.Runner, *checker) {
	t.Helper()
	r, err := agent.LoadRole(roleFS(), "prove")
	if err != nil {
		t.Fatal(err)
	}
	c := &checker{}
	return &agent.Runner{Role: r, Model: model, Tools: []agent.Tool{c.tool()}, Goal: func(context.Context) (bool, string, error) { return c.done, "", nil }}, c
}

var u = llm.Usage{Input: 100, Output: 100}

func code(s string) map[string]string { return map[string]string{"code": s} }

func TestRun(t *testing.T) {
	for _, c := range []struct {
		name      string
		responses []*llm.Response
		outcome   string
		reason    string
		turns     int
	}{
		{"success", []*llm.Response{llmtest.Call("lean_check", code("bad"), u), llmtest.Call("lean_check", code("done"), u)}, agent.Success, "the goal holds", 2},
		{"stuck", []*llm.Response{llmtest.Call(agent.StuckTool, map[string]string{"reason": "false at n = 7"}, u)}, agent.Failure, "stuck: false at n = 7", 1},
		{"no progress", []*llm.Response{llmtest.Call("lean_check", code("a"), u), llmtest.Call("lean_check", code("a"), u), llmtest.Call("lean_check", code("a"), u)}, agent.Failure, "no progress in 2 turns", 3},
		{"a claim", []*llm.Response{llmtest.Say("I proved it.", u), llmtest.Say("Really.", u)}, agent.Failure, "no progress in 2 turns", 2},
		{"a refusal", []*llm.Response{{StopReason: llm.Refusal, Usage: u}}, agent.Failure, "the model refused the request", 1},
		{"the limit", []*llm.Response{{StopReason: llm.MaxTokens, Usage: u}}, agent.Partial, "the model stopped at the limit of the output tokens", 1},
		{"the budget", []*llm.Response{llmtest.Call("lean_check", code("a"), llm.Usage{Output: 2_000_000})}, agent.Failure, "the budget of $1.00 is used up", 1},
		{"the turns", []*llm.Response{llmtest.Call("lean_check", code("1"), u), llmtest.Call("lean_check", code("2"), u), llmtest.Call("lean_check", code("3"), u), llmtest.Call("lean_check", code("4"), u), llmtest.Call("lean_check", code("5"), u), llmtest.Call("lean_check", code("6"), u)}, agent.Failure, "the run used its 6 turns", 6},
		{"a tool outside the profile", []*llm.Response{llmtest.Call("bash", code("rm"), u), llmtest.Call("lean_check", code("done"), u)}, agent.Success, "the goal holds", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := llmtest.New(c.responses...)
			rn, _ := runner(t, s)
			rec, err := rn.Run(context.Background(), job(), 0, "Prove 7K3QX2.")
			if err != nil {
				t.Fatal(err)
			}
			if rec.Outcome != c.outcome || rec.Reason != c.reason || len(rec.Turns) != c.turns {
				t.Errorf("the run gives %s, %q after %d turns", rec.Outcome, rec.Reason, len(rec.Turns))
			}
			if m := rec.Models(); len(m) != 1 || m[0] != "claude-haiku-5-5" {
				t.Errorf("the models are %v", m)
			}
		})
	}
}

func TestRunTranscript(t *testing.T) {
	s := llmtest.New(llmtest.Call("bash", code("ls"), u), llmtest.Call("lean_check", code("bad"), u), llmtest.Call("lean_check", code("done"), u))
	rn, _ := runner(t, s)
	rec, err := rn.Run(context.Background(), job(), 1, "Prove 7K3QX2.")
	if err != nil {
		t.Fatal(err)
	}
	reqs := s.Requests()
	if len(reqs) != 3 || reqs[0].Model != "claude-opus-5-5" || reqs[0].Effort != "high" || reqs[0].System != rn.Role.System {
		t.Fatalf("the first request is %+v", reqs[0])
	}
	if n := len(reqs[0].Tools); n != 2 || reqs[0].Tools[0].Name != "lean_check" || reqs[0].Tools[1].Name != agent.StuckTool {
		t.Errorf("the tools are %+v", reqs[0].Tools)
	}
	// The budget of $1 at $20 for each million output tokens allows 50,000 tokens, and the role allows 1,000.
	if reqs[0].MaxTokens != 1000 {
		t.Errorf("the limit of the output tokens is %d", reqs[0].MaxTokens)
	}
	last := reqs[2].Messages
	if len(last) != 5 || last[2].Content[0].Content != "The role has no tool bash." || !last[2].Content[0].IsError || last[4].Content[0].Content != "unknown identifier" {
		t.Errorf("the messages are %+v", last)
	}
	if rec.Spend.LeanSessionS != 2 || rec.Spend.Tokens["claude-opus-5-5"] != (llm.Usage{Input: 300, Output: 300}) {
		t.Errorf("the spend is %+v", rec.Spend)
	}
	dir := t.TempDir()
	sum, err := agent.WriteRecord(dir, rec)
	if err != nil {
		t.Fatal(err)
	}
	back, err := prov.ReadRun(dir+"/"+prov.RunsDir, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if back.Agent != "m2-agent/prove@0.1.0" || back.Outcome != agent.Success || back.Targets[0] != "7K3QX2" || !strings.HasPrefix(back.Transcript, "blake3:") || back.Transcript != sum.Transcript {
		t.Errorf("the summary is %+v", back)
	}
}

func TestRunErrors(t *testing.T) {
	rn, _ := runner(t, llmtest.New())
	if _, err := rn.Run(context.Background(), job(), 2, "x"); err == nil {
		t.Error("Run accepts a rung that is not in the ladder")
	}
	j := job()
	j.Constraints.Models = []string{"claude-sonnet-5-5"}
	if _, err := rn.Run(context.Background(), j, 0, "x"); err == nil {
		t.Error("Run uses a model that the job does not allow")
	}
	j = job()
	j.Role = "explain"
	if _, err := rn.Run(context.Background(), j, 0, "x"); err == nil {
		t.Error("Run accepts a job of another role")
	}
	rn.Tools = nil
	if _, err := rn.Run(context.Background(), job(), 0, "x"); err == nil {
		t.Error("Run starts with no tool of the profile")
	}
	rn, _ = runner(t, llmtest.New())
	if _, err := rn.Run(context.Background(), job(), 0, "x"); !errors.Is(err, llmtest.ErrEnd) {
		t.Errorf("an error of the model gives %v", err)
	}
}

func TestRunWhy(t *testing.T) {
	model := llmtest.New(llmtest.Call("lean_check", code("try"), u), llmtest.Say("I think it is done.", u), llmtest.Call(agent.StuckTool, map[string]string{"reason": "the statement is false"}, u))
	r, _ := runner(t, model)
	r.Goal = func(context.Context) (bool, string, error) { return false, "the type hash of Lagrange changed.", nil }
	rec, err := r.Run(context.Background(), job(), 0, "Prove it.")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Outcome != agent.Failure || rec.Turns[0].Why != "the type hash of Lagrange changed." {
		t.Fatalf("record %+v", rec)
	}
	want := "The goal does not hold yet: the type hash of Lagrange changed. Continue with the tools, or call report_stuck with the reason."
	reqs := model.Requests()
	after := reqs[1].Messages[len(reqs[1].Messages)-1].Content
	if len(after) != 2 || after[0].Type != llm.ToolResult || after[1].Text != want {
		t.Errorf("the message after the tool call is %+v", after)
	}
	if last := reqs[2].Messages[len(reqs[2].Messages)-1].Content; len(last) != 1 || last[0].Text != want {
		t.Errorf("the message after the text turn is %+v", last)
	}
}
