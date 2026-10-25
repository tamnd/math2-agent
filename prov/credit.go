package prov

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// The CRediT roles that [Credit] gives (https://credit.niso.org).
const (
	Curation      = "Data curation"
	FormalWork    = "Formal analysis"
	Software      = "Software"
	Validation    = "Validation"
	OriginalDraft = "Writing – original draft"
	Editing       = "Writing – review & editing"
)

// Contribution is one change of a person to one field, or one change of an agent run that the person started.
type Contribution struct {
	Tag   string // the node
	Field string // the field
	Op    string // the operation of the line
	Role  string // the CRediT role
	At    string // the time of the change
	Run   string // the agent run, when an agent made the change on behalf of the person, or ""
}

// Credit returns the contributions of the person in the logs from the time from up to the time to, without to, in the order of role, tag and time. A zero time has no bound. A change of an agent counts for the person when the run of the line acted on behalf of the person. The runs are the run summaries by id, and nil means that only the lines of the person count.
func Credit(logs map[string][]Line, runs map[string]*Run, person string, from, to time.Time) []Contribution {
	var out []Contribution
	for t, lines := range logs {
		for _, l := range lines {
			c := Contribution{Tag: t, Field: l.Field, Op: l.Op, Role: role(l), At: l.At}
			switch {
			case l.Human == person:
			case l.Agent != "" && runs[l.Run] != nil && runs[l.Run].OnBehalfOf == person:
				c.Run = l.Run
			default:
				continue
			}
			at, err := time.Parse(time.RFC3339, l.At)
			if err != nil || !from.IsZero() && at.Before(from) || !to.IsZero() && !at.Before(to) {
				continue
			}
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Contribution) int {
		return cmp.Or(cmp.Compare(a.Role, b.Role), cmp.Compare(a.Tag, b.Tag), cmp.Compare(a.At, b.At), cmp.Compare(a.Field, b.Field))
	})
	return out
}

// role returns the CRediT role of the change in the line.
func role(l Line) string {
	f := l.Field
	has := func(prefixes ...string) bool {
		return slices.ContainsFunc(prefixes, func(p string) bool { return within(f, p) })
	}
	switch {
	case l.Op == Confirm || l.Op == Quarantine || strings.Contains(f, "fidelity_reviews"):
		return Validation
	case has("formal"):
		return FormalWork
	case has("same_as", "close_to", "aligned", "msc", "sources", "aliases", "names"):
		return Curation
	case has("engine", "output", "certificate"):
		return Software
	case l.Op == Add && (f == Whole || has("body")):
		return OriginalDraft
	}
	return Editing
}
