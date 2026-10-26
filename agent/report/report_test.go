package report_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/prove"
	"github.com/tamnd/math2-agent/agent/report"
	"github.com/tamnd/math2-agent/prov"
)

var update = flag.Bool("update", false, "write the golden files")

func TestWeek(t *testing.T) {
	for _, c := range []struct {
		in    string
		start string
	}{
		{"2026-W01", "2025-12-29"},
		{"2026-W44", "2026-10-26"},
		{"2026-W53", "2026-12-28"},
		{"2021-W01", "2021-01-04"},
	} {
		w, err := report.ParseWeek(c.in)
		if err != nil {
			t.Fatal(err)
		}
		if got := w.Start().Format(time.DateOnly); got != c.start || w.String() != c.in {
			t.Errorf("%s starts on %s, want %s", w, got, c.start)
		}
		if report.WeekOf(w.Start().Add(6*24*time.Hour+time.Hour)) != w {
			t.Errorf("the Sunday of %s is in another week", w)
		}
	}
	for _, bad := range []string{"2025-W53", "2026-W00", "2026-44", "2026-W4", "26-W44", "2026-Wxx"} {
		if _, err := report.ParseWeek(bad); err == nil {
			t.Errorf("ParseWeek(%q) gives no error", bad)
		}
	}
	if w := (report.Week{Year: 2026, Week: 1}).Prev(); w.String() != "2025-W52" {
		t.Errorf("the week before 2026-W01 is %s", w)
	}
}

// record returns a run record that started at the time.
func record(id, role, target, model, outcome, reason string, at time.Time, usd, budget float64, whys ...string) *agent.Record {
	r := &agent.Record{
		ID: id, Job: agent.Job{ID: "j-" + id[2:], Role: role, Target: target}, Rung: agent.Rung{Model: model},
		Started: at, Outcome: outcome, Reason: reason, Budget: agent.Budget{USD: budget}, Spend: agent.Spend{USD: usd},
	}
	for _, w := range whys {
		r.Turns = append(r.Turns, agent.Turn{Why: w})
	}
	return r
}

func input() report.Input {
	mon := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	shadow := record("r-2026-10-19-0008", "prove", "7K3QX2", "model-a", agent.Success, "the goal holds", mon.Add(30*time.Minute), 0.30, 1)
	shadow.Job.ID, shadow.Agent, shadow.Shadow = "j-2026-10-19-0001", "m2-agent/prove@1.0.0", true
	return report.Input{
		Records: []*agent.Record{
			record("r-2026-10-19-0001", "prove", "7K3QX2", "model-a", agent.Success, "the goal holds", mon, 0.40, 1),
			record("r-2026-10-19-0002", "prove", "7K3QX2", "model-b", agent.Success, "the goal holds", mon.Add(time.Hour), 0.80, 1),
			record("r-2026-10-20-0003", "prove", "4F2M9A", "model-a", agent.Failure, "stuck: false at n = 7", mon.Add(24*time.Hour), 0.20, 1),
			record("r-2026-10-21-0004", "prove", "4F2M9A", "model-a", agent.Failure, "the run used its turns", mon.Add(48*time.Hour), 1.30, 1, "the goal does not hold", prove.Tampered+": the run changed Math2.X, which the statement uses; put it back as it was"),
			record("r-2026-10-22-0005", "formalize", "9QW3E8", "model-a", agent.Partial, "the model stopped at the limit of the output tokens", mon.Add(72*time.Hour), 0.50, 2),
			shadow,
			// Not in the week.
			record("r-2026-10-12-0006", "prove", "7K3QX2", "model-a", agent.Success, "the goal holds", mon.Add(-7*24*time.Hour), 9, 10),
			record("r-2026-10-26-0007", "prove", "7K3QX2", "model-a", agent.Success, "the goal holds", mon.Add(7*24*time.Hour-9*time.Hour), 9, 10),
		},
		Logs: map[string][]prov.Line{
			"7K3QX2": {
				{Field: "formal[0].decl", Op: prov.Change, Agent: "m2-agent/prove@0.1.1", Run: "r-2026-10-19-0001", At: "2026-10-19T10:00:00Z"},
				{Field: "body.explanations.L1", Op: prov.Quarantine, Human: "orcid:0000-0002-1825-0097", At: "2026-10-23T12:00:00Z"},
				{Field: "body.explanations.L2", Op: prov.Quarantine, Human: "orcid:0000-0002-1825-0097", At: "2026-10-12T12:00:00Z"},
			},
			"9QW3E8": {
				{Field: "formal", Op: prov.Change, Agent: "m2-agent/formalize@0.1.0", Run: "r-2026-10-22-0005", At: "2026-10-23T10:00:00Z"},
			},
		},
		Areas: map[string]string{"7K3QX2": "11", "4F2M9A": "20"},
		Prev:  &report.Report{Roles: []report.Role{{Role: "prove", PerOutput: 2}}},
		Made:  time.Date(2026, 10, 26, 6, 0, 0, 0, time.UTC),
	}
}

func TestMake(t *testing.T) {
	w, _ := report.ParseWeek("2026-W43")
	rep := report.Make(w, input())
	if rep.From != "2026-10-19" || rep.To != "2026-10-25" || rep.Made != "2026-10-26" {
		t.Errorf("the dates are %s, %s and %s", rep.From, rep.To, rep.Made)
	}
	want := []report.Role{
		{Role: "formalize", Jobs: 1, Runs: 1, Partial: 1, Accepted: 1, USD: 0.5, PerOutput: 0.5},
		{Role: "prove", Jobs: 4, Runs: 4, Succeeded: 2, Failed: 2, Stuck: 1, Accepted: 1, USD: 2.7, PerOutput: 2.7, PrevPer: 2},
	}
	if len(rep.Roles) != len(want) {
		t.Fatalf("the roles are %+v", rep.Roles)
	}
	for i, r := range rep.Roles {
		r.USD, r.PerOutput = round(r.USD), round(r.PerOutput)
		if r != want[i] {
			t.Errorf("the role is %+v, want %+v", r, want[i])
		}
	}
	var tiers []string
	for _, x := range rep.Tiers {
		tiers = append(tiers, x.Role+" "+x.Model)
	}
	if !slices.Equal(tiers, []string{"formalize model-a", "prove model-a", "prove model-b"}) {
		t.Errorf("the tiers are %v", tiers)
	}
	if a := rep.Areas; len(a) != 2 || a[0].Area != "11" || a[0].Runs != 2 || a[0].Accepted != 1 || a[1].Area != "20" || a[1].Accepted != 0 {
		t.Errorf("the areas are %+v", a)
	}
	want2 := []report.Shadow{{Role: "prove", Candidate: "m2-agent/prove@1.0.0", Jobs: 1, Runs: 1, Succeeded: 1, USD: 0.3, MainRuns: 1, MainSucceeded: 1, MainUSD: 0.4}}
	if !slices.Equal(rep.Shadows, want2) {
		t.Errorf("the shadows are %+v", rep.Shadows)
	}
	in := rep.Incidents
	if len(in.Quarantines) != 1 || in.Quarantines[0].Field != "body.explanations.L1" {
		t.Errorf("the quarantines are %+v", in.Quarantines)
	}
	if !slices.Equal(in.Tampering, []string{"r-2026-10-21-0004"}) || !slices.Equal(in.OverBudget, []string{"r-2026-10-21-0004"}) {
		t.Errorf("the incidents are %+v", in)
	}
}

func round(v float64) float64 { return float64(int(v*100+0.5)) / 100 }

func TestGolden(t *testing.T) {
	w, _ := report.ParseWeek("2026-W43")
	rep := report.Make(w, input())
	js, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	empty := report.Make(w, report.Input{Made: input().Made})
	for name, got := range map[string][]byte{
		"2026-W43.md":   rep.Markdown(),
		"2026-W43.json": append(js, '\n'),
		"empty.md":      empty.Markdown(),
	} {
		p := filepath.Join("testdata", name)
		if *update {
			if err := os.WriteFile(p, got, 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(p) // #nosec G304 -- the golden files of the test.
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s is\n%s\nwant\n%s", name, got, want)
		}
	}
}
