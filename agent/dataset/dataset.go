// Package dataset makes the quarterly dataset of the agent runs (spec 17.4).
//
// The dataset of a quarter has one [Row] for each run that started in the quarter, merged or not: the job, the role at its version, the models, the outcome, the budget and the spend. A run is merged when a provenance line on the branch names it, as for rule X03. Thus a later dataset of the same quarter can have more merged runs.
//
// The dataset has no transcripts. The transcripts of the merged runs are public through their run records (spec 11.6), and the row of a run has the id of its record. [Make] also removes the personal data and the secrets. A job on behalf of a person gives only the word person, and the person who requested the job is not in the row. [Scrub] removes the email addresses, the keys and the tokens, and the home directories from each text of the row.
package dataset

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/prov"
)

// Quarter is a quarter of a year.
type Quarter struct {
	Year    int
	Quarter int // from 1 to 4
}

// QuarterOf returns the quarter of the time in UTC.
func QuarterOf(t time.Time) Quarter {
	t = t.UTC()
	return Quarter{t.Year(), (int(t.Month())-1)/3 + 1}
}

// ParseQuarter parses a quarter in the form YYYY-Qn, for example 2026-Q4.
func ParseQuarter(s string) (Quarter, error) {
	y, q, ok := strings.Cut(s, "-Q")
	year, err1 := strconv.Atoi(y)
	n, err2 := strconv.Atoi(q)
	if !ok || len(y) != 4 || err1 != nil || err2 != nil || n < 1 || n > 4 {
		return Quarter{}, fmt.Errorf("dataset: the quarter %q is not in the form YYYY-Qn", s)
	}
	return Quarter{year, n}, nil
}

// String returns the quarter in the form YYYY-Qn.
func (q Quarter) String() string { return fmt.Sprintf("%04d-Q%d", q.Year, q.Quarter) }

// Start returns the first instant of the quarter in UTC.
func (q Quarter) Start() time.Time {
	return time.Date(q.Year, time.Month(3*q.Quarter-2), 1, 0, 0, 0, 0, time.UTC)
}

// End returns the first instant after the quarter.
func (q Quarter) End() time.Time { return q.Start().AddDate(0, 3, 0) }

// Person is the value of [Row.OnBehalfOf] for a job on behalf of a person.
const Person = "person"

// A Row is one run of the dataset.
type Row struct {
	Run        string            `json:"run"` // the id of the run record
	Job        string            `json:"job"`
	Role       string            `json:"role"`
	Agent      string            `json:"agent"`  // the role at its version
	Prompt     string            `json:"prompt"` // the prompt version
	Model      string            `json:"model"`  // the model of the rung of the run
	Effort     string            `json:"effort,omitempty"`
	Models     []string          `json:"models"` // the models that answered, in the order of their first answer
	Target     string            `json:"target"`
	Goal       string            `json:"goal"`
	OnBehalfOf string            `json:"on_behalf_of"` // a project, a schedule, or [Person]
	Started    time.Time         `json:"started"`
	Finished   time.Time         `json:"finished"`
	Outcome    string            `json:"outcome"`
	Reason     string            `json:"reason"`
	Turns      int               `json:"turns"`
	Budget     agent.Budget      `json:"budget"`
	Spend      agent.Spend       `json:"spend"`
	Shadow     bool              `json:"shadow,omitempty"`
	Merged     bool              `json:"merged"`
	Novelty    map[string]string `json:"novelty,omitempty"` // the verdict of the novelty check of each output, by node id
}

// Make returns the rows of the runs that started in the quarter, in the order of their start. The logs are the provenance logs of the branch by node, and they give the merged runs.
func Make(q Quarter, recs []*agent.Record, logs map[string][]prov.Line) []Row {
	merged := map[string]bool{}
	for _, ls := range logs {
		for _, l := range ls {
			if l.Agent != "" && l.Run != "" {
				merged[l.Run] = true
			}
		}
	}
	start, end := q.Start(), q.End()
	rows := []Row{}
	for _, r := range recs {
		if r.Started.Before(start) || !r.Started.Before(end) {
			continue
		}
		who := r.Job.OnBehalfOf
		if prov.IsPerson(who) {
			who = Person
		}
		row := Row{
			Run: r.ID, Job: r.Job.ID, Role: r.Job.Role, Agent: r.Agent, Prompt: r.Prompt,
			Model: r.Rung.Model, Effort: r.Rung.Effort, Models: r.Models(),
			Target: r.Job.Target, Goal: Scrub(r.Job.Goal), OnBehalfOf: who,
			Started: r.Started.UTC(), Finished: r.Finished.UTC(), Outcome: r.Outcome, Reason: Scrub(r.Reason), Turns: len(r.Turns),
			Budget: r.Budget, Spend: r.Spend, Shadow: r.Shadow, Merged: merged[r.ID],
		}
		if row.Models == nil {
			row.Models = []string{}
		}
		if len(r.Novelty) > 0 {
			row.Novelty = map[string]string{}
			for _, id := range slices.Sorted(maps.Keys(r.Novelty)) {
				if res := r.Novelty[id]; res != nil {
					row.Novelty[id] = res.Verdict
				}
			}
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b Row) int { return cmp.Or(a.Started.Compare(b.Started), cmp.Compare(a.Run, b.Run)) })
	return rows
}

// The texts that Scrub removes, with the text that takes their place.
var secrets = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]+`), "[key]"},
	{regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})`), "[token]"},
	{regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`), "[key]"},
	{regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]{8,}`), "Bearer [token]"},
	{regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`), "[email]"},
	{regexp.MustCompile(`(?:/Users|/home)/[^/\s"']+`), "~"},
	{regexp.MustCompile(`(?i)\b[A-Z]:\\Users\\[^\\\s"']+`), "~"},
}

// Scrub returns the text with no email addresses, no keys of the Anthropic API or of AWS, no GitHub tokens or bearer tokens, and no home directories. The home directory of a path becomes ~.
func Scrub(s string) string {
	for _, x := range secrets {
		s = x.re.ReplaceAllString(s, x.with)
	}
	return s
}
