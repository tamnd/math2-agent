package stage

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/status"
)

func TestOf(t *testing.T) {
	for _, c := range []struct {
		job  agent.Job
		want string
	}{
		{agent.Job{Role: "write", Goal: "node reaches W1"}, Stated},
		{agent.Job{Role: "write", Goal: "node reaches W3"}, Complete},
		{agent.Job{Role: "exercise", Goal: "node reaches W2"}, Usable},
		{agent.Job{Role: "curate", Goal: "row 9DK2RX7Q4H has an outcome"}, Decided},
		{agent.Job{Role: "write", Goal: "the node has a proof"}, Stated},
		{agent.Job{Role: "enumerate"}, Listed},
		{agent.Job{Role: "formalize"}, Bound},
		{agent.Job{Role: "bump"}, Other},
	} {
		if got := Of(c.job); got != c.want {
			t.Errorf("Of(%+v) is %s, not %s", c.job, got, c.want)
		}
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestMeasure(t *testing.T) {
	day := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	rec := func(id, job, role, goal string, at time.Time, usd float64) *agent.Record {
		return &agent.Record{ID: id, Job: agent.Job{ID: job, Role: role, Goal: goal}, Outcome: agent.Success, Started: at, Spend: agent.Spend{USD: usd, Seconds: 10}}
	}
	shadow := rec("r-2026-11-02-0009", "j-2026-11-02-1", "write", "node reaches W1", day, 9)
	shadow.Shadow = true
	in := Input{
		Records: []*agent.Record{
			rec("r-2026-11-02-0001", "j-2026-11-02-1", "write", "node reaches W1", day, 0.02),
			rec("r-2026-11-02-0002", "j-2026-11-02-1", "write", "node reaches W1", day.Add(time.Hour), 0.02),
			rec("r-2026-11-02-0003", "j-2026-11-02-2", "informalize", "node reaches W1", day, 0.04),
			rec("r-2026-10-02-0004", "j-2026-10-02-1", "write", "node reaches W1", day.AddDate(0, -1, 0), 1),
			shadow,
		},
		Logs: map[string][]prov.Line{
			"7K3QX2": {
				{Field: "body.statement", Op: prov.Add, Agent: "m2-agent/write@0.1.0", Run: "r-2026-11-02-0002"},
				{Field: "body.statement", Op: prov.Confirm, Human: "github:octocat", At: "2026-11-03T10:00:00Z"},
				{Field: "sources[0]", Op: prov.Confirm, Human: "github:octocat", At: "2026-11-03T10:00:00Z"},
				{Field: "body.proof", Op: prov.Confirm, Human: "github:octocat", At: "2026-10-03T10:00:00Z"},
			},
		},
		Reviews: map[string]status.Review{
			"r-7K3QX2-01": {Date: "2026-11-03", Minutes: 6},
			"r-7K3QX2-02": {Date: "2026-11-04"},
			"r-7K3QX2-03": {Date: "2026-10-04", Minutes: 60},
		},
		Rows: []inventory.Row{{ID: "9DK2RX7Q4H", History: []inventory.Event{
			{Op: "add", At: "2026-11-02", By: "r-2026-11-02-0003"},
			{Op: "merge", At: "2026-11-02", By: "github:octocat"},
			{Op: "outcome", At: "2026-11-05", By: "orcid:0000-0002-1825-0097"},
			{Op: "outcome", At: "2026-11-05", By: "r-2026-11-02-0003"},
		}}},
		From: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		Made: day.AddDate(0, 0, 7),
	}
	rep := Measure(in)
	if rep.From != "2026-11-01" || rep.To != "2026-11-30" {
		t.Errorf("the window is %s to %s", rep.From, rep.To)
	}
	got := map[string]Stage{}
	for _, s := range rep.Stages {
		got[s.Stage] = s
	}
	st := got[Stated]
	if st.Jobs != 2 || st.Runs != 3 || st.Accepted != 1 || !near(st.USD, 0.08) || !near(st.Seconds, 30) || strings.Join(st.Roles, ",") != "informalize,write" || st.Measured || st.Estimate == nil {
		t.Errorf("the stage stated is %+v", st)
	}
	if !near(st.PerJob(), 0.04) || !near(st.PerAccepted(), 0.08) || !near(st.AcceptanceRate(), 1.0/3) {
		t.Errorf("the stage stated has %v per job, %v per accepted run and the rate %v", st.PerJob(), st.PerAccepted(), st.AcceptanceRate())
	}
	if c := got[Confirmed]; c.Reviews != 1 || !near(c.ReviewLow, 1.0/60) || !near(c.ReviewHigh, 2.0/60) {
		t.Errorf("the stage confirmed is %+v", c)
	}
	if s := got[Sourced]; s.Reviews != 1 || !near(s.ReviewHigh, 2.0/60) {
		t.Errorf("the stage sourced is %+v", s)
	}
	if r := got[Reviewed]; r.Reviews != 2 || r.Timed != 1 || !near(r.TimedHours, 0.1) || !near(r.ReviewLow, 0.1+3.0/60) {
		t.Errorf("the stage reviewed is %+v", r)
	}
	if got[Inventoried].Reviews != 1 || got[Decided].Reviews != 1 {
		t.Errorf("the curator decisions are %+v and %+v", got[Inventoried], got[Decided])
	}
	if rep.Total.Runs != 3 || rep.Total.Reviews != 6 {
		t.Errorf("the total is %+v", rep.Total)
	}
	md := string(rep.Markdown())
	for _, want := range []string{"| stated | informalize, write | 2 | 3 | 1 | 33% | $0.08 | $0.040 | $0.080 | $0.020 | 2 of 1000 jobs |", "| reviewed | 2 | 1 | 0.10 | 0.15 | 3 |", "| confirmed | 1 | 0 | 0.00 | 0.02 to 0.03 | 1 to 2 |", "## Not measured"} {
		if !strings.Contains(md, want) {
			t.Errorf("the text has no %q:\n%s", want, md)
		}
	}
}

func TestMeasureEmpty(t *testing.T) {
	rep := Measure(Input{})
	if len(rep.Stages) != 0 || !strings.Contains(string(rep.Markdown()), "No run and no review") {
		t.Errorf("the empty report is %+v", rep)
	}
}
