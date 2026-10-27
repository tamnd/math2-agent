// Package stage measures the stages of the corpus pipeline (corpus spec K12.1, K12.5 to K12.7).
//
// The pilot replaces the planning numbers of K12.6 with measured numbers. [Measure] reads the run records, the provenance logs, the fidelity reviews and the history of the inventory rows, and gives for each stage the runs, the acceptance rate, the model cost and the review hours. A run is accepted when a provenance line names it, as in the weekly agent report. The stage of a run comes from the goal of its job, for example "node reaches W2", and else from its role.
//
// A review act is a fidelity review, a confirmation of a field by a person, or a decision of a person on an inventory row. Only a fidelity review records its time, in minutes. For each other act, the review hours of its stage use the minutes of K12.5, and the report says that they are estimates. A stage has measured numbers when it has [Settled] jobs or more. Before that, the planning numbers of package queue stay the numbers to plan with, and the report gives both.
package stage

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/prov"
	"github.com/tamnd/math2-agent/queue"
	"github.com/tamnd/math2-agent/status"
)

// Settled is the number of jobs after which the measured numbers of a stage replace the planning numbers (corpus K12.6).
const Settled = 1000

// The stages of an item (corpus K12.1), with the formal track and the review queues of K12.2.
const (
	Listed      = "listed"      // enumerate: the item is a mention
	Inventoried = "inventoried" // the mention belongs to a row: the dedup judge and the close cases
	Decided     = "decided"     // the row has an outcome
	Stated      = "stated"      // W1
	Usable      = "usable"      // W2
	Complete    = "complete"    // W3
	Confirmed   = "confirmed"   // a person confirmed a section that an agent wrote (rule P04)
	Sourced     = "sourced"     // a person confirmed a source (rule P06)
	Sanity      = "sanity"      // the sanity checks of K9.3
	Bound       = "bound"       // a binding at F0
	Reviewed    = "reviewed"    // a fidelity review: F2 or F3
	Proved      = "proved"      // E5
	Other       = "other"       // a run of a role that no stage names
)

// Stages are the stages in the order of the report.
var Stages = []string{Listed, Inventoried, Decided, Stated, Usable, Complete, Confirmed, Sourced, Sanity, Bound, Reviewed, Proved, Other}

// queues names the standing queue of each stage, for the planning numbers of package queue.
var queues = map[string]string{
	Listed: queue.Enumerate, Inventoried: queue.Cluster, Decided: queue.Decide, Stated: queue.State,
	Usable: queue.Complete, Complete: queue.Complete, Confirmed: queue.Confirm, Sourced: queue.Source, Sanity: queue.Sanity,
}

// reviewMinutes are the minutes of one review act of a stage with no queue estimate (corpus K12.5): 3 minutes for each fidelity review of an undergraduate statement.
var reviewMinutes = map[string][2]float64{Reviewed: {3, 3}}

// byRole is the stage of a run whose goal names no stage.
var byRole = map[string]string{
	"enumerate": Listed, "curate": Decided, "write": Stated, "informalize": Stated, "explain": Usable, "exercise": Usable,
	"review-assist": Confirmed, "explore": Sanity, "formalize": Bound, "align": Bound, "prove": Proved,
}

var levelRe = regexp.MustCompile(`\bW([1-3])\b`)

// Of returns the stage of a job: the writing level that its goal names, an outcome of a row, or else the stage of its role.
func Of(j agent.Job) string {
	if m := levelRe.FindStringSubmatch(j.Goal); m != nil {
		return [...]string{Stated, Usable, Complete}[m[1][0]-'1']
	}
	if strings.Contains(j.Goal, "outcome") {
		return Decided
	}
	return cmp.Or(byRole[j.Role], Other)
}

// Input is what Measure reads.
type Input struct {
	Records []*agent.Record          // the run records
	Logs    map[string][]prov.Line   // the provenance logs, by node
	Reviews map[string]status.Review // the fidelity reviews, by id
	Rows    []inventory.Row          // the inventory rows, for the decisions of the curators
	From    time.Time                // the first day of the report. The zero time means no first day.
	To      time.Time                // the day after the last day. The zero time means no last day.
	Made    time.Time                // the time of the report
}

// Report is the measured cost of each stage.
type Report struct {
	From    string   `json:"from,omitempty"` // the first day, YYYY-MM-DD, or "" with no first day
	To      string   `json:"to,omitempty"`   // the last day, or ""
	Made    string   `json:"made"`           // the date of the report, which is the date of the counts of acceptance
	Stages  []Stage  `json:"stages"`
	Total   Stage    `json:"total"` // the sum of the stages, with the stage "total"
	Missing []string `json:"not_measured"`
}

// Stage is the measured numbers of one stage.
type Stage struct {
	Stage     string   `json:"stage"`
	Queue     string   `json:"queue,omitempty"` // the standing queue with the planning numbers, or ""
	Roles     []string `json:"roles,omitempty"` // the roles of the runs
	Jobs      int      `json:"jobs"`
	Runs      int      `json:"runs"`
	Succeeded int      `json:"succeeded"`
	Accepted  int      `json:"accepted"` // the runs that a provenance line names
	TokensIn  int      `json:"tokens_in"`
	TokensOut int      `json:"tokens_out"`
	USD       float64  `json:"usd"`
	Seconds   float64  `json:"seconds"` // the wall time of the runs

	// Measured reports whether the stage has Settled jobs or more, so that its numbers replace the planning numbers.
	Measured bool            `json:"measured"`
	Estimate *queue.Estimate `json:"estimate,omitempty"` // the planning numbers of each job, or nil

	Reviews     int     `json:"reviews"`             // the review acts
	Timed       int     `json:"reviews_timed"`       // the review acts that record their time
	TimedHours  float64 `json:"review_hours_timed"`  // the hours that the timed acts record
	ReviewLow   float64 `json:"review_hours_low"`    // the timed hours, and the low estimate for each act with no time
	ReviewHigh  float64 `json:"review_hours_high"`   // the timed hours, and the high estimate for each act with no time
	Unestimated int     `json:"reviews_no_estimate"` // the acts with no time and no estimate
}

// AcceptanceRate returns the share of the runs that are accepted, or 0 with no run.
func (s Stage) AcceptanceRate() float64 {
	if s.Runs == 0 {
		return 0
	}
	return float64(s.Accepted) / float64(s.Runs)
}

// PerAccepted returns the dollars of the stage for each accepted run, or 0 with no accepted run.
func (s Stage) PerAccepted() float64 {
	if s.Accepted == 0 {
		return 0
	}
	return s.USD / float64(s.Accepted)
}

// PerJob returns the dollars of the stage for each job, or 0 with no job.
func (s Stage) PerJob() float64 {
	if s.Jobs == 0 {
		return 0
	}
	return s.USD / float64(s.Jobs)
}

var missing = []string{
	"the time of a confirmation, of a source check and of a decision on an inventory row: these acts record no time, thus their hours are the estimates of corpus K12.5",
	"the sample audit of K9.7 and the reader reports of K9.8: they have no records yet",
}

// in reports whether the time is in the window of the input.
func (in *Input) in(t time.Time) bool {
	return (in.From.IsZero() || !t.Before(in.From)) && (in.To.IsZero() || t.Before(in.To))
}

// inDay reports whether the day, YYYY-MM-DD or RFC 3339, is in the window. A day that does not parse is not.
func (in *Input) inDay(s string) bool {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		if t, err = time.Parse(time.DateOnly, s); err != nil {
			return false
		}
	}
	return in.in(t)
}

// Measure measures each stage in the window of the input. A shadow run is not counted, because the system uses none of its output.
func Measure(in Input) *Report {
	rep := &Report{Made: in.Made.UTC().Format(time.DateOnly), Missing: missing}
	if !in.From.IsZero() {
		rep.From = in.From.UTC().Format(time.DateOnly)
	}
	if !in.To.IsZero() {
		rep.To = in.To.UTC().AddDate(0, 0, -1).Format(time.DateOnly)
	}
	stages := map[string]*Stage{}
	get := func(name string) *Stage {
		s := stages[name]
		if s == nil {
			s = &Stage{Stage: name, Queue: queues[name]}
			stages[name] = s
		}
		return s
	}

	accepted := map[string]bool{}
	for _, node := range slices.Sorted(maps.Keys(in.Logs)) {
		for _, l := range in.Logs[node] {
			if l.Agent != "" && l.Run != "" {
				accepted[l.Run] = true
			}
			if l.Op == prov.Confirm && prov.IsPerson(l.Human) && in.inDay(l.At) {
				name := Confirmed
				if l.Field == "sources" || strings.HasPrefix(l.Field, "sources[") || strings.HasPrefix(l.Field, "sources.") {
					name = Sourced
				}
				get(name).Reviews++
			}
		}
	}

	jobs := map[string]map[string]bool{}
	for _, r := range in.Records {
		if r.Shadow || !in.in(r.Started) {
			continue
		}
		name := Of(r.Job)
		s := get(name)
		if jobs[name] == nil {
			jobs[name] = map[string]bool{}
		}
		jobs[name][r.Job.ID] = true
		if !slices.Contains(s.Roles, r.Job.Role) {
			s.Roles = append(s.Roles, r.Job.Role)
		}
		s.Runs++
		if r.Outcome == agent.Success {
			s.Succeeded++
		}
		if accepted[r.ID] {
			s.Accepted++
		}
		for _, u := range r.Spend.Tokens {
			s.TokensIn += u.Input + u.CacheRead + u.CacheWrite
			s.TokensOut += u.Output
		}
		s.USD += r.Spend.USD
		s.Seconds += r.Spend.Seconds
	}
	for name, js := range jobs {
		stages[name].Jobs = len(js)
	}

	for _, id := range slices.Sorted(maps.Keys(in.Reviews)) {
		r := in.Reviews[id]
		if !in.inDay(r.Date) {
			continue
		}
		s := get(Reviewed)
		s.Reviews++
		if r.Minutes > 0 {
			s.Timed++
			s.TimedHours += r.Minutes / 60
		}
	}

	for _, row := range in.Rows {
		for _, e := range row.History {
			if !prov.IsPerson(e.By) || !in.inDay(e.At) {
				continue
			}
			switch e.Op {
			case "merge", "split":
				get(Inventoried).Reviews++
			case "outcome":
				get(Decided).Reviews++
			}
		}
	}

	rep.Total = Stage{Stage: "total"}
	for _, name := range Stages {
		s := stages[name]
		if s == nil {
			continue
		}
		slices.Sort(s.Roles)
		s.Measured = s.Jobs >= Settled
		low, high, known := 0.0, 0.0, false
		if e, ok := queue.Estimates[s.Queue]; ok && s.Queue != "" {
			s.Estimate = &e
			low, high, known = e.ReviewLow, e.ReviewHigh, e.ReviewKnown
		}
		if m, ok := reviewMinutes[name]; ok {
			low, high, known = m[0], m[1], true
		}
		untimed := s.Reviews - s.Timed
		s.ReviewLow, s.ReviewHigh = s.TimedHours, s.TimedHours
		if known {
			s.ReviewLow += float64(untimed) * low / 60
			s.ReviewHigh += float64(untimed) * high / 60
		} else {
			s.Unestimated = untimed
		}
		rep.Stages = append(rep.Stages, *s)
		t := &rep.Total
		t.Jobs += s.Jobs
		t.Runs += s.Runs
		t.Succeeded += s.Succeeded
		t.Accepted += s.Accepted
		t.TokensIn += s.TokensIn
		t.TokensOut += s.TokensOut
		t.USD += s.USD
		t.Seconds += s.Seconds
		t.Reviews += s.Reviews
		t.Timed += s.Timed
		t.TimedHours += s.TimedHours
		t.ReviewLow += s.ReviewLow
		t.ReviewHigh += s.ReviewHigh
		t.Unestimated += s.Unestimated
	}
	return rep
}

// Markdown returns the report as text.
func (r *Report) Markdown() []byte {
	var b strings.Builder
	b.WriteString("# The cost of each stage of the corpus\n\n")
	switch {
	case r.From != "" && r.To != "":
		fmt.Fprintf(&b, "The report counts the runs and the reviews from %s to %s. ", r.From, r.To)
	case r.From != "":
		fmt.Fprintf(&b, "The report counts the runs and the reviews from %s. ", r.From)
	case r.To != "":
		fmt.Fprintf(&b, "The report counts the runs and the reviews to %s. ", r.To)
	default:
		b.WriteString("The report counts each run and each review. ")
	}
	fmt.Fprintf(&b, "A run is accepted when a provenance line names it, and the counts of acceptance are as of %s. A stage has measured numbers after %d jobs. Before that, the planning numbers of corpus K12.6 stay the numbers to plan with.\n\n", r.Made, Settled)
	if len(r.Stages) == 0 {
		b.WriteString("No run and no review is in the window.\n\n")
	} else {
		b.WriteString("## Runs and cost\n\nThe table has the stages with a run.\n\n| Stage | Roles | Jobs | Runs | Accepted | Rate | Spend | Per job | Per accepted run | Planned per job | Basis |\n|---|---|---|---|---|---|---|---|---|---|---|\n")
		for _, s := range append(slices.Clone(r.Stages), r.Total) {
			if s.Runs == 0 && s.Stage != "total" {
				continue
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %s | $%.2f | %s | %s | %s | %s |\n", s.Stage, strings.Join(s.Roles, ", "), s.Jobs, s.Runs, s.Accepted, rate(s), s.USD, dollars(s.PerJob()), dollars(s.PerAccepted()), planned(s.Estimate), basis(s))
		}
		b.WriteString("\n## Review\n\nThe table has the stages with a review act. A review act is a fidelity review, a confirmation by a person or a decision of a person on an inventory row.\n\n| Stage | Review acts | With a time | Recorded hours | Hours, with the estimates | Planned minutes per act |\n|---|---|---|---|---|---|\n")
		for _, s := range append(slices.Clone(r.Stages), r.Total) {
			if s.Reviews == 0 && s.Stage != "total" {
				continue
			}
			hours := fmt.Sprintf("%.2f", s.ReviewLow)
			if high := fmt.Sprintf("%.2f", s.ReviewHigh); high != hours {
				hours += " to " + high
			}
			if s.Unestimated > 0 {
				hours += fmt.Sprintf(", and %d acts with no estimate", s.Unestimated)
			}
			fmt.Fprintf(&b, "| %s | %d | %d | %.2f | %s | %s |\n", s.Stage, s.Reviews, s.Timed, s.TimedHours, hours, plannedReview(s))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Not measured\n\n")
	for _, m := range r.Missing {
		fmt.Fprintf(&b, "- %s\n", m)
	}
	return []byte(b.String())
}

func rate(s Stage) string {
	if s.Runs == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", 100*s.AcceptanceRate())
}

func dollars(x float64) string {
	if x == 0 {
		return "-"
	}
	return fmt.Sprintf("$%.3f", x)
}

func planned(e *queue.Estimate) string {
	switch {
	case e == nil || !e.CostKnown:
		return "-"
	case e.CostLow == e.CostHigh:
		return fmt.Sprintf("$%.3f", e.CostHigh)
	}
	return fmt.Sprintf("$%.3f to $%.3f", e.CostLow, e.CostHigh)
}

func plannedReview(s Stage) string {
	if m, ok := reviewMinutes[s.Stage]; ok {
		return fmt.Sprintf("%g", m[0])
	}
	if s.Estimate == nil || !s.Estimate.ReviewKnown {
		return "-"
	}
	if s.Estimate.ReviewLow == s.Estimate.ReviewHigh {
		return fmt.Sprintf("%g", s.Estimate.ReviewLow)
	}
	return fmt.Sprintf("%g to %g", s.Estimate.ReviewLow, s.Estimate.ReviewHigh)
}

func basis(s Stage) string {
	if s.Stage == "total" || s.Runs == 0 {
		return "-"
	}
	if s.Measured {
		return "measured"
	}
	return fmt.Sprintf("%d of %d jobs", s.Jobs, Settled)
}
