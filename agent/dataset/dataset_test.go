package dataset_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/dataset"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/novelty"
	"github.com/tamnd/math2-agent/prov"
)

func record(id, who string, started time.Time) *agent.Record {
	return &agent.Record{
		ID:      id,
		Job:     agent.Job{ID: "j-2026-10-26-1", Role: "prove", Target: "7K3QX2", Goal: "prove it", OnBehalfOf: who, RequestedBy: "github:octocat"},
		Agent:   "m2-agent/prove@0.1.0",
		Prompt:  "prove/1",
		Rung:    agent.Rung{Model: "claude-sonnet-5-5", Effort: "medium"},
		System:  "the system prompt",
		Context: "the brief",
		Turns: []agent.Turn{
			{Response: &llm.Response{Model: "claude-sonnet-5-5", Content: []llm.Block{{Type: llm.Text, Text: "a secret plan"}}}},
			{Response: &llm.Response{Model: "claude-opus-5-5"}},
		},
		Started:  started,
		Finished: started.Add(time.Minute),
		Outcome:  agent.Success,
		Reason:   "the goal holds",
		Budget:   agent.Budget{USD: 2},
		Spend:    agent.Spend{USD: 0.5, Seconds: 60},
	}
}

func TestMake(t *testing.T) {
	q := dataset.Quarter{Year: 2026, Quarter: 4}
	in := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	a := record("r-2026-11-02-aaaa0001", "github:octocat", in)
	b := record("r-2026-10-01-bbbb0002", "project:fermat", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	b.Outcome, b.Reason = agent.Failure, "the file /Users/someone/w/T.lean has an error, see me@example.com"
	b.Job.Role = "explore"
	b.Novelty = map[string]*novelty.Result{"~5e6f7a8b": {Verdict: novelty.Known}}
	before := record("r-2026-09-30-cccc0003", "project:fermat", time.Date(2026, 9, 30, 23, 59, 0, 0, time.UTC))
	after := record("r-2027-01-01-dddd0004", "project:fermat", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	logs := map[string][]prov.Line{"7K3QX2": {{Agent: "m2-agent/prove@0.1.0", Run: a.ID}}}

	rows := dataset.Make(q, []*agent.Record{a, before, b, after}, logs)
	if len(rows) != 2 || rows[0].Run != b.ID || rows[1].Run != a.ID {
		t.Fatalf("the rows are %+v", rows)
	}
	if r := rows[1]; !r.Merged || r.OnBehalfOf != dataset.Person || r.Turns != 2 || strings.Join(r.Models, " ") != "claude-sonnet-5-5 claude-opus-5-5" || r.Model != "claude-sonnet-5-5" || r.Spend.USD != 0.5 {
		t.Errorf("the row of the merged run is %+v", r)
	}
	if r := rows[0]; r.Merged || r.OnBehalfOf != "project:fermat" || r.Reason != "the file ~/w/T.lean has an error, see [email]" || r.Novelty["~5e6f7a8b"] != novelty.Known {
		t.Errorf("the row of the other run is %+v", r)
	}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"octocat", "a secret plan", "the system prompt", "the brief", "someone", "example.com"} {
		if strings.Contains(string(data), leak) {
			t.Errorf("the dataset has %q: %s", leak, data)
		}
	}
	if rows := dataset.Make(q, nil, nil); rows == nil || len(rows) != 0 {
		t.Errorf("a quarter with no runs gives %v", rows)
	}
}

func TestScrub(t *testing.T) {
	for in, want := range map[string]string{
		"the key sk-ant-api03-abc_DEF-123 failed":           "the key [key] failed",
		"push with ghp_" + strings.Repeat("a", 36) + " now": "push with [token] now",
		"github_pat_" + strings.Repeat("B", 30):             "[token]",
		"AKIA" + strings.Repeat("Z", 16) + " is set":        "[key] is set",
		"Authorization: Bearer abc.def.ghi-jkl":             "Authorization: Bearer [token]",
		"write to a.b@c.example.org":                        "write to [email]",
		"/home/ci/go/pkg and /Users/ann/x":                  "~/go/pkg and ~/x",
		`C:\Users\ann\x`:                                    `~\x`,
		"nothing to remove in 7K3QX2":                       "nothing to remove in 7K3QX2",
	} {
		if got := dataset.Scrub(in); got != want {
			t.Errorf("Scrub(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQuarter(t *testing.T) {
	q, err := dataset.ParseQuarter("2026-Q4")
	if err != nil || q != (dataset.Quarter{Year: 2026, Quarter: 4}) || q.String() != "2026-Q4" {
		t.Fatalf("ParseQuarter gives %v, %v", q, err)
	}
	if !q.Start().Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) || !q.End().Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("the quarter is from %v to %v", q.Start(), q.End())
	}
	if got := dataset.QuarterOf(time.Date(2026, 3, 31, 23, 0, 0, 0, time.UTC)); got.String() != "2026-Q1" {
		t.Errorf("QuarterOf gives %v", got)
	}
	for _, s := range []string{"2026-Q5", "2026-Q0", "26-Q1", "2026Q1", "2026-W43"} {
		if _, err := dataset.ParseQuarter(s); err == nil {
			t.Errorf("ParseQuarter(%q) gives no error", s)
		}
	}
}
