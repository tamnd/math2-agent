package agent

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/zeebo/blake3"

	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/prov"
)

// StuckTool is the tool that each role has: the model calls it with a reason when it cannot reach the goal. A stuck report with a reason is an output of the run (spec 11.5).
const StuckTool = "report_stuck"

// The outcomes of a run (spec 11.6).
const (
	Success = "success"
	Failure = "failure"
	Partial = "partial"
)

// RecordsDir is the directory of the full run records in the artefact store. The summaries are in [prov.RunsDir].
const RecordsDir = "records"

// A Goal is the goal predicate of a run. It reports whether the goal holds. When the goal does not hold, why is the reason, and the run gives it to the model. An error stops the run.
type Goal func(ctx context.Context) (done bool, why string, err error)

// Runner runs jobs of one role.
type Runner struct {
	Role   *Role
	Model  llm.Model
	Tools  []Tool               // the tools of the role. The model sees only the tools in the profile of the role.
	Goal   Goal                 // the goal predicate. It runs after each turn. Nil means that no goal can hold, which is for tests.
	Prices map[string]llm.Price // the prices of the models. Nil means llm.Prices.
	Now    func() time.Time     // the clock. Nil means time.Now.
}

// Turn is one model turn of a run: the response and the results of its tool calls.
type Turn struct {
	Response *llm.Response `json:"response"`
	USD      float64       `json:"usd"`
	Results  []llm.Block   `json:"results,omitempty"`
	Why      string        `json:"why,omitempty"` // why the goal did not hold after the turn
}

// Spend is the spend of a run (spec 11.5).
type Spend struct {
	Tokens       map[string]llm.Usage `json:"tokens"` // the tokens of each model
	USD          float64              `json:"usd"`
	LeanSessionS float64              `json:"lean_session_s,omitempty"`
	ComputeCPUS  float64              `json:"compute_cpu_s,omitempty"`
	Seconds      float64              `json:"seconds"` // the wall time
}

// Record is the full record of one run (spec 11.6).
type Record struct {
	ID       string            `json:"id"`
	Job      Job               `json:"job"`
	Agent    string            `json:"agent"`  // the role at its version
	Prompt   string            `json:"prompt"` // the prompt version
	Files    map[string]string `json:"files"`  // the hash of each prompt file
	Rung     Rung              `json:"rung"`
	Tools    []string          `json:"tools"` // the tools that the model saw
	System   string            `json:"system"`
	Context  string            `json:"context"` // the first message of the run
	Turns    []Turn            `json:"turns"`
	Started  time.Time         `json:"started"`
	Finished time.Time         `json:"finished"`
	Outcome  string            `json:"outcome"`
	Reason   string            `json:"reason"`          // why the run stopped
	Claim    string            `json:"claim,omitempty"` // the last text of the model: its own assessment, which the run records and never uses
	Spend    Spend             `json:"spend"`
}

// Models returns the models that answered in the run, in the order of their first answer.
func (r *Record) Models() []string {
	var out []string
	for _, t := range r.Turns {
		if !slices.Contains(out, t.Response.Model) {
			out = append(out, t.Response.Model)
		}
	}
	return out
}

// newID returns a new run id for the time.
func newID(t time.Time) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "r-" + t.UTC().Format("2006-01-02") + "-" + hex.EncodeToString(b)
}

// Run runs the job on the rung of the ladder of the role, from 0, with the brief as the first message. The brief is the target, its neighbourhood and what the role needs (spec 12.2). An error means that the run could not start or that the model service failed. A run that stops for another reason gives a record with the outcome and the reason.
func (rn *Runner) Run(ctx context.Context, job *Job, rung int, brief string) (*Record, error) {
	if err := job.Validate(); err != nil {
		return nil, err
	}
	role := rn.Role
	if job.Role != role.Name {
		return nil, fmt.Errorf("agent: the job is for the role %s, and the runner is for %s", job.Role, role.Name)
	}
	if rung < 0 || rung >= len(role.Ladder) {
		return nil, fmt.Errorf("agent: the ladder of %s has no rung %d", role.Name, rung)
	}
	g := role.Ladder[rung]
	if len(job.Constraints.Models) > 0 && !slices.Contains(job.Constraints.Models, g.Model) {
		return nil, fmt.Errorf("agent: the job does not allow the model %s of rung %d", g.Model, rung)
	}
	now := rn.Now
	if now == nil {
		now = time.Now
	}
	prices := rn.Prices
	if prices == nil {
		prices = llm.Prices
	}
	price, ok := prices[g.Model]
	if !ok {
		return nil, fmt.Errorf("agent: no price for the model %s, thus the run cannot keep its budget", g.Model)
	}
	tools := map[string]Tool{StuckTool: stuck}
	for _, t := range rn.Tools {
		if slices.Contains(role.Tools, t.Spec().Name) {
			tools[t.Spec().Name] = t
		}
	}
	for _, name := range role.Tools {
		if tools[name] == nil {
			return nil, fmt.Errorf("agent: the runner has no tool %s of the profile of %s", name, role.Name)
		}
	}
	names := slices.Sorted(maps.Keys(tools))
	var defs []llm.Tool
	for _, n := range names {
		defs = append(defs, tools[n].Spec())
	}
	start := now()
	rec := &Record{
		ID: newID(start), Job: *job, Agent: role.Agent(), Prompt: role.Prompt(), Files: role.Files, Rung: g,
		Tools: names, System: role.System, Context: brief, Started: start,
		Spend: Spend{Tokens: map[string]llm.Usage{}},
	}
	b := job.budget(role)
	msgs := []llm.Message{{Role: llm.User, Content: []llm.Block{llm.TextBlock(brief)}}}
	cache := map[string]llm.Block{}
	idle := 0
	stop := func(outcome, reason string) (*Record, error) {
		rec.Outcome, rec.Reason = outcome, reason
		rec.Finished = now()
		rec.Spend.Seconds = rec.Finished.Sub(start).Seconds()
		return rec, nil
	}
	for turn := 1; ; turn++ {
		if turn > role.Turns {
			return stop(Failure, fmt.Sprintf("the run used its %d turns", role.Turns))
		}
		if reason := over(b, rec.Spend, now().Sub(start)); reason != "" {
			return stop(Failure, reason)
		}
		limit := role.MaxTokens
		if b.USD > 0 {
			p := price
			if p.Over != nil {
				p = *p.Over
			}
			left := int(math.Floor((b.USD - rec.Spend.USD) / p.Output * 1e6))
			limit = min(limit, left)
			if limit < 1 {
				return stop(Failure, fmt.Sprintf("the budget of $%.2f is used up", b.USD))
			}
		}
		resp, err := rn.Model.Complete(ctx, &llm.Request{Model: g.Model, System: role.System, Messages: msgs, Tools: defs, MaxTokens: limit, Effort: g.Effort})
		if err != nil {
			rec.Finished = now()
			return rec, fmt.Errorf("agent: turn %d: %w", turn, err)
		}
		usd, _ := llm.Cost(prices, g.Model, resp.Usage)
		model := cmp.Or(resp.Model, g.Model)
		resp.Model = model
		rec.Spend.Tokens[model] = rec.Spend.Tokens[model].Add(resp.Usage)
		rec.Spend.USD += usd
		t := Turn{Response: resp, USD: usd}
		if text := resp.Text(); text != "" {
			rec.Claim = text
		}
		msgs = append(msgs, llm.Message{Role: llm.Assistant, Content: resp.Content})
		calls := resp.Calls()
		switch {
		case resp.StopReason == llm.Refusal:
			rec.Turns = append(rec.Turns, t)
			return stop(Failure, "the model refused the request")
		case resp.StopReason == llm.MaxTokens:
			rec.Turns = append(rec.Turns, t)
			return stop(Partial, "the model stopped at the limit of the output tokens")
		case len(calls) == 0:
			done, why, err := rn.goal(ctx)
			t.Why = why
			rec.Turns = append(rec.Turns, t)
			if err != nil {
				return stop(Failure, "the goal check failed: "+err.Error())
			}
			if done {
				return stop(Success, "the goal holds")
			}
			idle++
			if idle >= role.Stuck {
				return stop(Failure, fmt.Sprintf("no progress in %d turns", idle))
			}
			msgs = append(msgs, llm.Message{Role: llm.User, Content: []llm.Block{llm.TextBlock(nudge(job, why))}})
			continue
		}
		progress := false
		for _, c := range calls {
			key := c.Name + "\x00" + string(c.Input)
			if old, ok := cache[key]; ok {
				t.Results = append(t.Results, llm.ResultBlock(c.ID, old.Content, old.IsError))
				continue
			}
			progress = true
			res := call(ctx, tools[c.Name], job, c)
			rec.Spend.LeanSessionS += res.LeanSessionS
			rec.Spend.ComputeCPUS += res.ComputeCPUS
			block := llm.ResultBlock(c.ID, res.Text, res.IsError)
			cache[key] = block
			t.Results = append(t.Results, block)
			if c.Name == StuckTool && !res.IsError {
				rec.Turns = append(rec.Turns, t)
				return stop(Failure, "stuck: "+res.Text)
			}
		}
		done, why, err := rn.goal(ctx)
		t.Why = why
		rec.Turns = append(rec.Turns, t)
		if err != nil {
			return stop(Failure, "the goal check failed: "+err.Error())
		}
		if done {
			return stop(Success, "the goal holds")
		}
		content := slices.Clone(t.Results)
		if why != "" {
			content = append(content, llm.TextBlock(nudge(job, why)))
		}
		msgs = append(msgs, llm.Message{Role: llm.User, Content: content})
		if progress {
			idle = 0
		} else if idle++; idle >= role.Stuck {
			return stop(Failure, fmt.Sprintf("no progress in %d turns", idle))
		}
	}
}

func (rn *Runner) goal(ctx context.Context) (bool, string, error) {
	if rn.Goal == nil {
		return false, "", nil
	}
	return rn.Goal(ctx)
}

// nudge returns the message to the model when the goal does not hold.
func nudge(job *Job, why string) string {
	return fmt.Sprintf("The goal does not hold yet: %s. Continue with the tools, or call %s with the reason.", strings.TrimSuffix(cmp.Or(why, job.Goal), "."), StuckTool)
}

// over returns why the spend is over the budget, or "".
func over(b Budget, s Spend, wall time.Duration) string {
	switch {
	case b.USD > 0 && s.USD >= b.USD:
		return fmt.Sprintf("the budget of $%.2f is used up", b.USD)
	case b.Wall > 0 && wall >= time.Duration(b.Wall):
		return fmt.Sprintf("the wall time of %s is used up", time.Duration(b.Wall))
	case b.LeanSessionS > 0 && s.LeanSessionS >= b.LeanSessionS:
		return fmt.Sprintf("the %g Lean session seconds are used up", b.LeanSessionS)
	case b.ComputeCPUS > 0 && s.ComputeCPUS >= b.ComputeCPUS:
		return fmt.Sprintf("the %g compute CPU seconds are used up", b.ComputeCPUS)
	}
	return ""
}

// call runs one tool call. A call of a tool that the model did not see fails.
func call(ctx context.Context, t Tool, job *Job, c llm.Block) Result {
	if t == nil {
		return Result{Text: fmt.Sprintf("The role has no tool %s.", c.Name), IsError: true}
	}
	res, err := t.Call(ctx, job, c.Input)
	if err != nil {
		res.Text, res.IsError = err.Error(), true
	}
	return res
}

var stuck = Func{
	Def: llm.Tool{
		Name:        StuckTool,
		Description: "Stop the run because the goal cannot be reached, and give the reason. A precise reason is a useful result, for example: the statement is false at n = 7, with the computation.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"reason":{"type":"string","description":"why the goal cannot be reached"}},"required":["reason"]}`),
	},
	Fn: func(_ context.Context, _ *Job, input json.RawMessage) (Result, error) {
		var in struct{ Reason string }
		if err := json.Unmarshal(input, &in); err != nil || in.Reason == "" {
			return Result{}, errors.New("give the reason")
		}
		return Result{Text: in.Reason}, nil
	},
}

// Summary returns the summary of the record for the provenance log (spec 11.6). The transcript is the hash of the full record.
func (r *Record) Summary(transcript string) *prov.Run {
	return &prov.Run{
		ID: r.ID, Agent: r.Agent, Prompt: r.Prompt, Models: r.Models(), OnBehalfOf: r.Job.OnBehalfOf,
		Started: r.Started.UTC().Format(time.RFC3339), Outcome: r.Outcome, Targets: []string{r.Job.Target},
		Seconds: r.Spend.Seconds, SpendUSD: r.Spend.USD, Transcript: transcript,
	}
}

// WriteRecord writes the full record to store/records/<id>.json and its summary to store/runs/<id>.json, and returns the summary. A run with no model turn gives no summary, because a summary names its models.
func WriteRecord(store string, r *Record) (*prov.Run, error) {
	if !prov.IsRun(r.ID) {
		return nil, fmt.Errorf("agent: the run id %q is not in its form", r.ID)
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	dir := filepath.Join(store, RecordsDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, r.ID+".json"), data, 0o600); err != nil {
		return nil, err
	}
	sum := blake3.Sum256(data)
	s := r.Summary("blake3:" + hex.EncodeToString(sum[:]))
	if len(s.Models) == 0 {
		return nil, errors.New("agent: the run has no model turn, thus it has no summary")
	}
	if err := prov.WriteRun(filepath.Join(store, prov.RunsDir), s); err != nil {
		return nil, err
	}
	return s, nil
}
