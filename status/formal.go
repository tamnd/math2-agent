package status

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// The formal outcomes of a node (corpus spec K2.5, corpus arxiv AX10.4).
const (
	FormalBound      = "bound"        // the node has at least one formal binding, at any fidelity level
	FormalProved     = "proved"       // the node is at E5
	FormalPending    = "pending"      // a formalize or prove job is in the queue, or it ran and failed with a reason
	FormalBlocked    = "blocked"      // the formal statement needs a definition that no library has
	FormalOutOfReach = "out_of_reach" // a person decided that formalization is not useful now
	FormalNone       = "none"         // the node has no outcome yet
)

// FormalKinds are the kinds of node that get a formal outcome. K2.5 names the statements and the definitions. A conjecture and a problem state mathematics too, and a formal statement of them is the condition for a claim of a solution to be checkable (AX10.2), so they get an outcome as well.
var FormalKinds = []string{"statement", "definition", "conjecture", "problem"}

// FormalDecision is one record of registers/formal.reg. The register records the outcomes that the build cannot compute from the bindings and the levels: a job in the queue or a failed job, a missing definition, and the decision of a person. The last record of a tag is in effect.
type FormalDecision struct {
	Tag     string // the tag of the node
	Outcome string // pending, blocked, out_of_reach, or clear to end the decision before it
	Date    string // the date of the record, YYYY-MM-DD
	By      string // for blocked, the tag of the definition node that no library has; otherwise -
	Author  string // the person or the run that wrote the record, as orcid:, github: or m2/ and the role
	Reason  string // the reason, as words with a hyphen between them, for example needs-the-theory-of-perfectoid-spaces
}

// FormalClear ends the decision before it. The node then gets its outcome from the bindings and the levels, or none.
const FormalClear = "clear"

var (
	formalDate   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	formalReason = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// LoadFormal reads root/registers/formal.reg and returns the decision in effect for each tag. A tree without the register has no decisions.
func LoadFormal(root string) (map[string]FormalDecision, error) {
	out := map[string]FormalDecision{}
	err := register(root, "formal.reg", func(line int, f []string) error {
		if len(f) != 6 {
			return fmt.Errorf("line %d has %d fields, and a record has the fields tag, outcome, date, by, author and reason", line, len(f))
		}
		d := FormalDecision{Tag: f[0], Outcome: f[1], Date: f[2], By: f[3], Author: f[4], Reason: f[5]}
		switch d.Outcome {
		case FormalPending, FormalOutOfReach, FormalClear:
			if d.By != "-" {
				return fmt.Errorf("line %d: the field by is for blocked only, and it must be - for %s", line, d.Outcome)
			}
		case FormalBlocked:
			if d.By == "-" {
				return fmt.Errorf("line %d: blocked needs the tag of the definition in the field by", line)
			}
		default:
			return fmt.Errorf("line %d: the outcome %q is not pending, blocked, out_of_reach or clear", line, d.Outcome)
		}
		if !formalDate.MatchString(d.Date) {
			return fmt.Errorf("line %d: the date %q is not YYYY-MM-DD", line, d.Date)
		}
		if !formalReason.MatchString(d.Reason) {
			return fmt.Errorf("line %d: the reason %q is not words of lowercase letters and digits with a hyphen between them", line, d.Reason)
		}
		if old, ok := out[d.Tag]; ok && d.Date < old.Date {
			return fmt.Errorf("line %d: the record of %s has the date %s, which is before the date %s of the record before it", line, d.Tag, d.Date, old.Date)
		}
		if d.Outcome == FormalClear {
			delete(out, d.Tag)
			return nil
		}
		out[d.Tag] = d
		return nil
	})
	return out, err
}

// FormalOf is the formal outcome of one node.
type FormalOf struct {
	Tag     string `json:"tag"`
	Kind    string `json:"kind"`
	Outcome string `json:"outcome"`
	By      string `json:"by,omitempty"`     // for blocked, the definition node
	Reason  string `json:"reason,omitempty"` // the reason of the decision in the register
	Date    string `json:"date,omitempty"`   // the date of the decision in the register
}

// Formal is the formal outcomes of the nodes, as the build writes them to formal.json.
type Formal struct {
	Outcomes map[string]int `json:"outcomes"`           // the number of nodes with each outcome
	Nodes    []FormalOf     `json:"nodes"`              // the outcome of each node of a formal kind, in tag order
	Problems []string       `json:"problems,omitempty"` // the decisions that do not agree with the tree
}

// ComputeFormal gives the formal outcome of each node of a formal kind. The levels come first: a node at E5 is proved, and a node with a binding is bound, whatever the register says. Otherwise the decision in the register gives the outcome, and a node with neither has the outcome none. A blocked node names a definition, and K2.5 says that the definition has the outcome pending. A decision that does not agree with the tree is a problem.
func ComputeFormal(sts []Status, decisions map[string]FormalDecision) *Formal {
	f := &Formal{Outcomes: map[string]int{}, Nodes: []FormalOf{}}
	byTag := map[string]*Status{}
	for i := range sts {
		byTag[sts[i].Tag] = &sts[i]
	}
	outcome := map[string]string{}
	for i := range sts {
		s := &sts[i]
		if s.Retired || !slices.Contains(FormalKinds, s.Kind) {
			continue
		}
		r := FormalOf{Tag: s.Tag, Kind: s.Kind, Outcome: FormalNone}
		d, decided := decisions[s.Tag]
		switch {
		case s.Evidence == Kernel:
			r.Outcome = FormalProved
		case len(s.Bindings) > 0:
			r.Outcome = FormalBound
		case decided:
			r.Outcome, r.Reason, r.Date = d.Outcome, d.Reason, d.Date
			if d.Outcome == FormalBlocked {
				r.By = d.By
			}
		}
		outcome[s.Tag] = r.Outcome
		f.Outcomes[r.Outcome]++
		f.Nodes = append(f.Nodes, r)
	}
	for _, tag := range slices.Sorted(maps.Keys(decisions)) {
		d := decisions[tag]
		s, ok := byTag[tag]
		switch {
		case !ok || s.Retired:
			f.Problems = append(f.Problems, fmt.Sprintf("registers/formal.reg has the outcome %s for %s, which is not a node", d.Outcome, tag))
		case !slices.Contains(FormalKinds, s.Kind):
			f.Problems = append(f.Problems, fmt.Sprintf("registers/formal.reg has the outcome %s for %s, which is a %s and gets no formal outcome", d.Outcome, tag, s.Kind))
		case outcome[tag] != d.Outcome:
			f.Problems = append(f.Problems, fmt.Sprintf("registers/formal.reg has the outcome %s for %s, and the node is %s, so the record can be cleared", d.Outcome, tag, outcome[tag]))
		}
		if d.Outcome != FormalBlocked || outcome[tag] != FormalBlocked {
			continue
		}
		def, ok := byTag[d.By]
		switch {
		case !ok || def.Retired:
			f.Problems = append(f.Problems, fmt.Sprintf("%s is blocked by %s, which is not a node", tag, d.By))
		case def.Kind != "definition":
			f.Problems = append(f.Problems, fmt.Sprintf("%s is blocked by %s, which is a %s and not a definition", tag, d.By, def.Kind))
		case outcome[d.By] != FormalPending:
			f.Problems = append(f.Problems, fmt.Sprintf("%s is blocked by the definition %s, whose outcome is %s and not pending", tag, d.By, outcome[d.By]))
		}
	}
	slices.SortFunc(f.Nodes, func(a, b FormalOf) int { return strings.Compare(a.Tag, b.Tag) })
	return f
}
