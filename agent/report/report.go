// Package report makes the weekly agent report (spec 12.9).
//
// The report of an ISO week counts the runs that started in the week: the jobs and the outcomes of each role, the acceptance and the cost of each role and model, the cost of each role in each area of the MSC, and the incidents. A run is accepted when a provenance line on the branch names it, as for rule X03. Thus the counts of acceptance are as of the day that the report is made, and a later report of the same week can count more. [Make] makes the report from the run records and the provenance logs, and [Report.Markdown] writes its text. The JSON form of a [Report] is its twin.
package report

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/agent/prove"
	"github.com/tamnd/math2-agent/prov"
)

// Week is an ISO 8601 week.
type Week struct {
	Year int
	Week int
}

// WeekOf returns the ISO week of the time.
func WeekOf(t time.Time) Week {
	y, w := t.ISOWeek()
	return Week{y, w}
}

// ParseWeek parses a week in the form 2026-W44.
func ParseWeek(s string) (Week, error) {
	y, w, ok := strings.Cut(s, "-W")
	year, err1 := strconv.Atoi(y)
	week, err2 := strconv.Atoi(w)
	if !ok || err1 != nil || err2 != nil || len(y) != 4 || len(w) != 2 {
		return Week{}, fmt.Errorf("report: the week %q is not in the form 2026-W44", s)
	}
	wk := Week{year, week}
	if week < 1 || WeekOf(wk.Start()) != wk {
		return Week{}, fmt.Errorf("report: the year %d has no week %d", year, week)
	}
	return wk, nil
}

// String returns the week in the form 2026-W44.
func (w Week) String() string { return fmt.Sprintf("%04d-W%02d", w.Year, w.Week) }

// Start returns the start of the Monday of the week, in UTC.
func (w Week) Start() time.Time {
	jan4 := time.Date(w.Year, time.January, 4, 0, 0, 0, 0, time.UTC)
	monday := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
	return monday.AddDate(0, 0, 7*(w.Week-1))
}

// Prev returns the week before.
func (w Week) Prev() Week { return WeekOf(w.Start().AddDate(0, 0, -7)) }

// Input is what a report reads.
type Input struct {
	Records []*agent.Record        // the run records of the store
	Logs    map[string][]prov.Line // the provenance logs of the branch, by node
	Areas   map[string]string      // the area of each node, the first two digits of its first MSC code. A node with no area is not in the table of areas.
	Prev    *Report                // the report of the week before, for the trend, or nil
	Made    time.Time              // the time of the report
}

// Report is the weekly agent report.
type Report struct {
	Week      string    `json:"week"`
	From      string    `json:"from"` // the Monday of the week
	To        string    `json:"to"`   // the Sunday of the week
	Made      string    `json:"made"` // the date of the report, which is the date of the counts of acceptance
	Roles     []Role    `json:"roles"`
	Tiers     []Tier    `json:"tiers"`
	Areas     []Area    `json:"areas"`
	Incidents Incidents `json:"incidents"`
	Missing   []string  `json:"not_measured"` // the parts of spec 12.9 that the report does not measure yet
}

// Role is the volume and the cost of one role in the week.
type Role struct {
	Role      string  `json:"role"`
	Jobs      int     `json:"jobs"` // the jobs with a run in the week
	Runs      int     `json:"runs"` // the runs that started in the week
	Succeeded int     `json:"succeeded"`
	Failed    int     `json:"failed"` // the failed runs, with the stuck runs
	Stuck     int     `json:"stuck"`  // the runs that ended with a stuck report
	Partial   int     `json:"partial"`
	Accepted  int     `json:"accepted"` // the runs that a provenance line on the branch names
	USD       float64 `json:"usd"`
	PerOutput float64 `json:"usd_per_accepted,omitempty"`      // the dollars of the role for each accepted run, or 0 with no accepted run
	PrevPer   float64 `json:"prev_usd_per_accepted,omitempty"` // the same number in the report of the week before, or 0
}

// Tier is the acceptance and the cost of one role on one model in the week.
type Tier struct {
	Role      string  `json:"role"`
	Model     string  `json:"model"`
	Runs      int     `json:"runs"`
	Succeeded int     `json:"succeeded"`
	Accepted  int     `json:"accepted"`
	USD       float64 `json:"usd"`
	PerOutput float64 `json:"usd_per_accepted,omitempty"`
}

// Area is the cost of one role in one area of the MSC in the week.
type Area struct {
	Role      string  `json:"role"`
	Area      string  `json:"area"` // two digits, for example 11
	Runs      int     `json:"runs"`
	Accepted  int     `json:"accepted"`
	USD       float64 `json:"usd"`
	PerOutput float64 `json:"usd_per_accepted,omitempty"`
}

// Incidents are the events of the week that a person must see.
type Incidents struct {
	Quarantines []Quarantine `json:"quarantines"`
	Tampering   []string     `json:"tampering"`   // the runs in which the pin of the statement caught a change to its closure
	OverBudget  []string     `json:"over_budget"` // the runs that spent more than their budget
}

// Quarantine is a field that a person quarantined in the week.
type Quarantine struct {
	Node  string `json:"node"`
	Field string `json:"field"`
	Human string `json:"human"`
	At    string `json:"at"`
}

// missing lists the parts of spec 12.9 that need data that the tree and the store do not have yet.
var missing = []string{
	"pull requests opened, merged, closed and changed before merge: the runs do not open pull requests yet, thus a run counts as accepted when a provenance line on the branch names it",
	"the audit sample of auto-merged pull requests: no pull request merges with no review yet",
	"injection detections and stale reviews from bumps",
	"ladder changes: the ladder of each role changes only with a new role version",
}

// Make makes the report of the week.
func Make(w Week, in Input) *Report {
	start := w.Start()
	end := start.AddDate(0, 0, 7)
	rep := &Report{
		Week: w.String(), From: start.Format(time.DateOnly), To: end.AddDate(0, 0, -1).Format(time.DateOnly), Made: in.Made.UTC().Format(time.DateOnly),
		Incidents: Incidents{Quarantines: []Quarantine{}, Tampering: []string{}, OverBudget: []string{}},
		Missing:   missing,
	}
	accepted := map[string]bool{}
	for _, node := range slices.Sorted(maps.Keys(in.Logs)) {
		for _, l := range in.Logs[node] {
			if l.Agent != "" && l.Run != "" {
				accepted[l.Run] = true
			}
			if at, err := time.Parse(time.RFC3339, l.At); l.Op == prov.Quarantine && err == nil && !at.Before(start) && at.Before(end) {
				rep.Incidents.Quarantines = append(rep.Incidents.Quarantines, Quarantine{Node: node, Field: l.Field, Human: l.Human, At: l.At})
			}
		}
	}
	roles := map[string]*Role{}
	jobs := map[string]map[string]bool{}
	tiers := map[[2]string]*Tier{}
	areas := map[[2]string]*Area{}
	for _, r := range in.Records {
		if r.Started.Before(start) || !r.Started.Before(end) {
			continue
		}
		name := r.Job.Role
		ro := roles[name]
		if ro == nil {
			ro = &Role{Role: name}
			roles[name] = ro
			jobs[name] = map[string]bool{}
		}
		jobs[name][r.Job.ID] = true
		ok := accepted[r.ID]
		count := func(runs, succeeded, acc *int, usd *float64) {
			*runs++
			if r.Outcome == agent.Success {
				*succeeded++
			}
			if ok {
				*acc++
			}
			*usd += r.Spend.USD
		}
		count(&ro.Runs, &ro.Succeeded, &ro.Accepted, &ro.USD)
		switch {
		case r.Outcome == agent.Partial:
			ro.Partial++
		case r.Outcome != agent.Success:
			ro.Failed++
			if strings.HasPrefix(r.Reason, "stuck: ") {
				ro.Stuck++
			}
		}
		k := [2]string{name, cmp.Or(r.Rung.Model, "unknown")}
		t := tiers[k]
		if t == nil {
			t = &Tier{Role: k[0], Model: k[1]}
			tiers[k] = t
		}
		count(&t.Runs, &t.Succeeded, &t.Accepted, &t.USD)
		if area := in.Areas[r.Job.Target]; area != "" {
			k := [2]string{name, area}
			a := areas[k]
			if a == nil {
				a = &Area{Role: name, Area: area}
				areas[k] = a
			}
			var none int
			count(&a.Runs, &none, &a.Accepted, &a.USD)
		}
		if slices.ContainsFunc(r.Turns, func(t agent.Turn) bool { return strings.Contains(t.Why, prove.Tampered) }) {
			rep.Incidents.Tampering = append(rep.Incidents.Tampering, r.ID)
		}
		if b := r.Budget.USD; b > 0 && r.Spend.USD > b {
			rep.Incidents.OverBudget = append(rep.Incidents.OverBudget, r.ID)
		}
	}
	prev := map[string]float64{}
	if in.Prev != nil {
		for _, r := range in.Prev.Roles {
			prev[r.Role] = r.PerOutput
		}
	}
	for _, name := range slices.Sorted(maps.Keys(roles)) {
		r := roles[name]
		r.Jobs = len(jobs[name])
		r.PerOutput = per(r.USD, r.Accepted)
		r.PrevPer = prev[name]
		rep.Roles = append(rep.Roles, *r)
	}
	for _, k := range slices.SortedFunc(maps.Keys(tiers), cmpKey) {
		t := tiers[k]
		t.PerOutput = per(t.USD, t.Accepted)
		rep.Tiers = append(rep.Tiers, *t)
	}
	for _, k := range slices.SortedFunc(maps.Keys(areas), cmpKey) {
		a := areas[k]
		a.PerOutput = per(a.USD, a.Accepted)
		rep.Areas = append(rep.Areas, *a)
	}
	return rep
}

func cmpKey(a, b [2]string) int { return cmp.Or(cmp.Compare(a[0], b[0]), cmp.Compare(a[1], b[1])) }

// per returns the dollars for each accepted run, or 0 when no run is accepted.
func per(usd float64, accepted int) float64 {
	if accepted == 0 {
		return 0
	}
	return usd / float64(accepted)
}

// dollars writes an amount, or a dash for 0.
func dollars(v float64) string {
	if v == 0 {
		return "-"
	}
	return fmt.Sprintf("$%.2f", v)
}

// Markdown returns the text of the report.
func (r *Report) Markdown() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "# The agent report of %s\n\n", r.Week)
	fmt.Fprintf(&b, "The week is from %s to %s. The report counts the runs that started in the week. A run is accepted when a provenance line on the branch names it, and the counts of acceptance are as of %s.\n\n", r.From, r.To, r.Made)

	b.WriteString("## Volume\n\n")
	if len(r.Roles) == 0 {
		b.WriteString("No run started in the week.\n\n")
	} else {
		b.WriteString("| Role | Jobs | Runs | Succeeded | Failed | Stuck | Partial |\n|---|---|---|---|---|---|---|\n")
		for _, x := range r.Roles {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %d | %d |\n", x.Role, x.Jobs, x.Runs, x.Succeeded, x.Failed, x.Stuck, x.Partial)
		}
		b.WriteString("\nThe failed runs include the stuck runs.\n\n")
	}

	b.WriteString("## Acceptance\n\n")
	if len(r.Tiers) == 0 {
		b.WriteString("No run started in the week.\n\n")
	} else {
		b.WriteString("| Role | Model | Runs | Succeeded | Accepted | Rate |\n|---|---|---|---|---|---|\n")
		for _, t := range r.Tiers {
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %.0f%% |\n", t.Role, t.Model, t.Runs, t.Succeeded, t.Accepted, 100*float64(t.Accepted)/float64(t.Runs))
		}
		b.WriteString("\n")
	}

	b.WriteString("## Cost\n\n")
	if len(r.Roles) == 0 {
		b.WriteString("No run started in the week.\n\n")
	} else {
		b.WriteString("| Role | Spend | Accepted | Per accepted run | The week before |\n|---|---|---|---|---|\n")
		for _, x := range r.Roles {
			fmt.Fprintf(&b, "| %s | $%.2f | %d | %s | %s |\n", x.Role, x.USD, x.Accepted, dollars(x.PerOutput), dollars(x.PrevPer))
		}
		b.WriteString("\n| Role | Model | Spend | Per accepted run |\n|---|---|---|---|\n")
		for _, t := range r.Tiers {
			fmt.Fprintf(&b, "| %s | %s | $%.2f | %s |\n", t.Role, t.Model, t.USD, dollars(t.PerOutput))
		}
		if len(r.Areas) > 0 {
			b.WriteString("\n| Role | MSC area | Runs | Accepted | Spend | Per accepted run |\n|---|---|---|---|---|---|\n")
			for _, a := range r.Areas {
				fmt.Fprintf(&b, "| %s | %s | %d | %d | $%.2f | %s |\n", a.Role, a.Area, a.Runs, a.Accepted, a.USD, dollars(a.PerOutput))
			}
		}
		b.WriteString("\nA dash means that no run is accepted.\n\n")
	}

	b.WriteString("## Incidents\n\n")
	in := r.Incidents
	if len(in.Quarantines)+len(in.Tampering)+len(in.OverBudget) == 0 {
		b.WriteString("None.\n\n")
	} else {
		for _, q := range in.Quarantines {
			fmt.Fprintf(&b, "- %s quarantined the field %s of %s at %s.\n", q.Human, q.Field, q.Node, q.At)
		}
		for _, id := range in.Tampering {
			fmt.Fprintf(&b, "- The pin of the statement caught a change to its closure in the run %s.\n", id)
		}
		for _, id := range in.OverBudget {
			fmt.Fprintf(&b, "- The run %s spent more than its budget.\n", id)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Not measured yet\n\n")
	for _, m := range r.Missing {
		fmt.Fprintf(&b, "- %s.\n", strings.ToUpper(m[:1])+m[1:])
	}
	return []byte(b.String())
}
