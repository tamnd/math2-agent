package orchestrator_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/llm"
	"github.com/tamnd/math2-agent/orchestrator"
)

var now = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return now }

func role() *agent.Role {
	return &agent.Role{
		Name: "prove", Version: "0.1.1",
		Ladder: []agent.Rung{{Model: "claude-haiku-5-5"}, {Model: "claude-sonnet-5-5", Effort: "medium"}, {Model: "claude-opus-5-5", Effort: "high"}},
		Budget: agent.Budget{USD: 4},
	}
}

func job(id, target, behalf string) *agent.Job {
	return &agent.Job{ID: "j-2026-11-20-" + id, Role: "prove", Target: target, Goal: "the statement has a proof", OnBehalfOf: behalf}
}

// fake is a Dispatch. For each job and rung, it gives the outcome and the reason in plays, and a failure when plays has none. It spends the fraction spend of the budget of the run.
type fake struct {
	mu    sync.Mutex
	plays map[string]string // "TARGET RUNG" to "OUTCOME REASON"
	spend float64
	calls []string
	n     int
}

func (f *fake) dispatch(_ context.Context, job *agent.Job, rung int) (*agent.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	f.calls = append(f.calls, fmt.Sprintf("%s %d $%.2f", job.Target, rung, job.Budget.USD))
	play := f.plays[fmt.Sprintf("%s %d", job.Target, rung)]
	if play == "error" {
		return nil, errors.New("the work tree cannot be made")
	}
	outcome, reason, _ := strings.Cut(play, " ")
	if outcome == "" {
		outcome, reason = agent.Failure, "no progress in 6 turns"
	}
	return &agent.Record{
		ID: fmt.Sprintf("r-2026-11-20-%04x", f.n), Job: *job, Rung: role().Ladder[rung], Started: now,
		Outcome: outcome, Reason: reason, Spend: agent.Spend{Tokens: map[string]llm.Usage{}, USD: f.spend * job.Budget.USD},
	}, nil
}

func config() *orchestrator.Config {
	return &orchestrator.Config{Roles: map[string]float64{"prove": 100}}
}

func TestEscalation(t *testing.T) {
	f := &fake{plays: map[string]string{"7K3QX2 2": "success the goal holds"}, spend: 0.25}
	o := orchestrator.New(config(), []*agent.Role{role()}, f.dispatch, nil, clock)
	if _, err := o.Add(job("a1", "7K3QX2", "github:octocat")); err != nil {
		t.Fatal(err)
	}
	rep := o.Drain(context.Background())
	if got := strings.Join(f.calls, ", "); got != "7K3QX2 0 $4.00, 7K3QX2 1 $3.00, 7K3QX2 2 $2.25" {
		t.Errorf("the runs are %s", got)
	}
	if len(rep.Done) != 1 || rep.Done[0].Outcome != agent.Success || len(rep.Runs) != 3 || len(rep.Waiting) != 0 || len(rep.Over()) != 0 {
		t.Errorf("the report is %+v", rep)
	}
	if len(o.Queued()) != 0 {
		t.Errorf("the queues have %v", o.Queued())
	}
}

func TestLadderStart(t *testing.T) {
	past := []orchestrator.Attempt{{Run: "r-2026-11-01-0001", Role: "prove", Target: "2B8M4T", Rung: role().Ladder[1], Started: now.AddDate(0, 0, -19), Outcome: agent.Success}}
	f := &fake{}
	o := orchestrator.New(config(), []*agent.Role{role()}, f.dispatch, past, clock)
	_, _ = o.Add(job("a1", "7K3QX2", "github:octocat"))
	rep := o.Drain(context.Background())
	if got := strings.Join(f.calls, ", "); !strings.HasPrefix(got, "7K3QX2 1 ") || len(f.calls) != 2 {
		t.Errorf("the runs are %s", got)
	}
	if len(rep.Done) != 1 || rep.Done[0].Reason != "each rung of the ladder that the job allows failed" {
		t.Errorf("the report is %+v", rep.Done)
	}
}

func TestStuckAndErrors(t *testing.T) {
	f := &fake{plays: map[string]string{"7K3QX2 0": "failure stuck: false at n = 7", "2B8M4T 0": "error"}}
	o := orchestrator.New(config(), []*agent.Role{role()}, f.dispatch, nil, clock)
	_, _ = o.Add(job("a1", "7K3QX2", "github:octocat"))
	_, _ = o.Add(job("a2", "2B8M4T", "github:octocat"))
	rep := o.Drain(context.Background())
	if len(f.calls) != 2 || len(rep.Done) != 2 || len(rep.Runs) != 1 {
		t.Fatalf("the runs are %v, and the report is %+v", f.calls, rep)
	}
	for _, d := range rep.Done {
		want := map[string]string{"7K3QX2": "stuck: false at n = 7", "2B8M4T": "the run failed: the work tree cannot be made"}[d.Job.Target]
		if d.Outcome != agent.Failure || d.Reason != want {
			t.Errorf("the job of %s ends with %s, %q", d.Job.Target, d.Outcome, d.Reason)
		}
	}
}

func TestRetry(t *testing.T) {
	past := []orchestrator.Attempt{{Run: "r-2026-11-17-0001", Job: "j-2026-11-17-b1", Role: "prove", Target: "7K3QX2", Goal: "the statement has a proof", Rung: role().Ladder[0], Started: now.AddDate(0, 0, -3), Outcome: agent.Failure}}
	f := &fake{plays: map[string]string{"7K3QX2 1": "success the goal holds"}}
	o := orchestrator.New(config(), []*agent.Role{role()}, f.dispatch, past, clock)
	_, _ = o.Add(job("a1", "7K3QX2", "github:octocat"))
	o.Drain(context.Background())
	if got := strings.Join(f.calls, ", "); got != "7K3QX2 1 $4.00" {
		t.Errorf("the runs are %s", got)
	}

	r := role()
	r.Ladder = r.Ladder[:1]
	f = &fake{}
	o = orchestrator.New(config(), []*agent.Role{r}, f.dispatch, past, clock)
	_, _ = o.Add(job("a1", "7K3QX2", "github:octocat"))
	rep := o.Drain(context.Background())
	if len(f.calls) != 0 || len(rep.Waiting) != 1 || !strings.Contains(rep.Waiting[0].Why, "the run r-2026-11-17-0001 with claude-haiku-5-5 failed on 2026-11-17") {
		t.Errorf("the runs are %v, and the waiting jobs are %+v", f.calls, rep.Waiting)
	}
	now = now.AddDate(0, 0, 5)
	defer func() { now = now.AddDate(0, 0, -5) }()
	o.Drain(context.Background())
	if len(f.calls) != 1 {
		t.Errorf("after the retry time, the runs are %v", f.calls)
	}
}

func TestBudgets(t *testing.T) {
	c := config()
	c.Roles["prove"] = 5
	c.OnBehalfOf = map[string]float64{"project:ug-core": 3}
	c.Workers = 3
	f := &fake{spend: 1}
	past := []orchestrator.Attempt{
		{Run: "r-2026-10-30-0001", Role: "prove", OnBehalfOf: "project:ug-core", Started: now.AddDate(0, -1, 0), USD: 50},
		{Run: "r-2026-11-02-0001", Role: "prove", OnBehalfOf: "project:ug-core", Started: now.AddDate(0, 0, -18), USD: 1},
	}
	o := orchestrator.New(c, []*agent.Role{role()}, f.dispatch, past, clock)
	for i, target := range []string{"7K3QX2", "2B8M4T", "9ZQ4RW"} {
		j := job(fmt.Sprintf("a%d", i), target, "project:ug-core")
		j.Budget.USD = 1.5
		_, _ = o.Add(j)
	}
	rep := o.Drain(context.Background())
	spent := 0.0
	for _, a := range rep.Runs {
		spent += a.USD
	}
	// The project has $2 left in the month: $1.50 for the first run and $0.50 for the second.
	if len(rep.Runs) != 2 || spent != 2 || len(rep.Over()) != 0 {
		t.Errorf("the runs are %v", f.calls)
	}
	if len(rep.Waiting) != 2 || !strings.Contains(rep.Waiting[0].Why, "the money of prove or of project:ug-core for the month is used up") {
		t.Errorf("the waiting jobs are %+v", rep.Waiting)
	}
	if len(rep.Done) != 1 || rep.Done[0].Reason != "the budget of $1.50 of the job is used up" {
		t.Errorf("the jobs that ended are %+v", rep.Done)
	}
}

func TestFairShare(t *testing.T) {
	past := []orchestrator.Attempt{{Run: "r-2026-11-02-0001", Role: "prove", OnBehalfOf: "project:big", Started: now.AddDate(0, 0, -18), USD: 30}}
	f := &fake{plays: map[string]string{"7K3QX2 0": "success the goal holds", "2B8M4T 0": "success the goal holds", "9ZQ4RW 0": "success the goal holds"}}
	o := orchestrator.New(config(), []*agent.Role{role()}, f.dispatch, past, clock)
	big := job("a1", "7K3QX2", "project:big")
	big.Priority = 0.9
	low := job("a2", "2B8M4T", "project:small")
	low.Priority = 0.1
	high := job("a3", "9ZQ4RW", "project:small")
	high.Priority = 0.5
	for _, j := range []*agent.Job{big, low, high} {
		_, _ = o.Add(j)
	}
	o.Drain(context.Background())
	if got := strings.Join(f.calls, ", "); got != "9ZQ4RW 0 $4.00, 2B8M4T 0 $4.00, 7K3QX2 0 $4.00" {
		t.Errorf("the runs are %s", got)
	}
}

func TestAdd(t *testing.T) {
	o := orchestrator.New(config(), []*agent.Role{role()}, (&fake{}).dispatch, nil, clock)
	if id, err := o.Add(job("a1", "7K3QX2", "github:octocat")); err != nil || id != "j-2026-11-20-a1" {
		t.Fatalf("Add gives %s, %v", id, err)
	}
	if id, err := o.Add(job("a2", "7K3QX2", "schedule:standing-prove")); err != nil || id != "j-2026-11-20-a1" {
		t.Errorf("a job with the same target and goal gives %s, %v", id, err)
	}
	if _, err := o.Add(job("a1", "2B8M4T", "github:octocat")); err == nil {
		t.Error("Add accepts a job id that is in the queue")
	}
	if _, err := o.Add(job("a3", "2B8M4T", "")); err == nil {
		t.Error("Add accepts a job that names no person, project or schedule")
	}
	j := job("a4", "2B8M4T", "github:octocat")
	j.Role = "explain"
	if _, err := o.Add(j); err == nil || !strings.Contains(err.Error(), "cannot run the role explain") {
		t.Errorf("a job of a role with no runner gives %v", err)
	}
	if n := len(o.Queued()); n != 1 {
		t.Errorf("the queues have %d jobs", n)
	}
}

func TestNoBudget(t *testing.T) {
	f := &fake{}
	o := orchestrator.New(&orchestrator.Config{}, []*agent.Role{role()}, f.dispatch, nil, clock)
	_, _ = o.Add(job("a1", "7K3QX2", "github:octocat"))
	rep := o.Drain(context.Background())
	if len(f.calls) != 0 || len(rep.Waiting) != 1 || rep.Waiting[0].Why != "orchestrator.yaml gives no budget to the role prove" {
		t.Errorf("the report is %+v", rep)
	}
}

func TestReadAttempts(t *testing.T) {
	store := t.TempDir()
	for i, started := range []time.Time{now, now.Add(-time.Hour)} {
		rec, _ := (&fake{spend: 0.5}).dispatch(context.Background(), job(fmt.Sprintf("a%d", i), "7K3QX2", "github:octocat"), i)
		rec.ID = fmt.Sprintf("r-2026-11-20-%04d", i)
		rec.Started = started
		rec.Job.Budget.USD = 2
		rec.Spend.USD = 1
		if _, err := agent.WriteRecord(store, rec); err == nil {
			t.Error("WriteRecord writes a summary of a run with no model turn")
		}
	}
	as, err := orchestrator.ReadAttempts(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(as) != 2 || as[0].Run != "r-2026-11-20-0001" || as[0].Rung.Model != "claude-sonnet-5-5" || as[1].USD != 1 || as[1].Budget != 2 || as[1].OnBehalfOf != "github:octocat" {
		t.Errorf("the attempts are %+v", as)
	}
	if as, err := orchestrator.ReadAttempts(t.TempDir()); err != nil || len(as) != 0 {
		t.Errorf("an empty store gives %v, %v", as, err)
	}
}

func TestReadConfig(t *testing.T) {
	dir := t.TempDir()
	write := func(text string) string {
		p := filepath.Join(dir, "orchestrator.yaml")
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	c, err := orchestrator.ReadConfig(write("roles: {prove: 200, formalize: 100}\non_behalf_of: {project:ug-core: 50}\nretry: 72h\nworkers: 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Roles["prove"] != 200 || c.OnBehalfOf["project:ug-core"] != 50 || time.Duration(c.Retry) != 72*time.Hour || c.Workers != 2 {
		t.Errorf("the configuration is %+v", c)
	}
	for _, text := range []string{
		"roles: {golf: 1}\n",
		"roles: {prove: -1}\n",
		"on_behalf_of: {github:octocat: 5}\n",
		"workers: -1\n",
		"budget: 5\n",
	} {
		if _, err := orchestrator.ReadConfig(write(text)); err == nil {
			t.Errorf("ReadConfig accepts %q", text)
		}
	}
}
