// Package autoform selects the formal statement of a node from the candidates of formalize runs (spec 10.2 and 10.3).
//
// m2 autoform runs the formalize role several times on one node. Each run has its own work tree and a model of the ladder of the role. A run that reaches its goal gives a [Candidate]: the Lean binding that the run appended, the hash of its statement and the report of the F1 battery on it. [Select] puts the candidates that pass the battery into groups by the hash of their statement, and the largest group wins. Agreement of independent candidates is evidence. A split into two or more groups usually points at an ambiguity of the informal statement, and the [Result] records it. [Unbound] lists the concepts and the definitions in the uses closure of the node that have no Lean binding. The statement cannot go higher than F1 before each of them has a binding at F2 (spec 10.3). [Result.Markdown] gives the body of the pull request.
package autoform

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
)

// Candidate is the output of one formalize run on the node.
type Candidate struct {
	Run     string          `json:"run"`               // the id of the formalize run
	Rung    int             `json:"rung"`              // the rung of the ladder of the run
	Model   string          `json:"model,omitempty"`   // the model of the run
	Family  string          `json:"family,omitempty"`  // the family of the model in the model register
	Outcome string          `json:"outcome"`           // the outcome of the run
	Reason  string          `json:"reason,omitempty"`  // why the run stopped, or why it gave no statement
	Decl    string          `json:"decl,omitempty"`    // the declaration of the new binding
	Match   string          `json:"match,omitempty"`   // the match of the new binding
	Hash    string          `json:"hash,omitempty"`    // the hash of the statement, from [Key]
	Lean    string          `json:"lean,omitempty"`    // the Lean files that the run changed
	Battery *battery.Report `json:"battery,omitempty"` // the report of the battery on the declaration, or nil when the run gave no statement
}

// Passes reports whether the candidate is a statement that the battery accepts: the declaration elaborates, and no check fails. A check that did not run does not stop a candidate, because m2 autoform runs the battery with no model and no prover.
func (c *Candidate) Passes() bool {
	if c.Battery == nil || c.Hash == "" || c.Battery.Check(1).Result != battery.Pass {
		return false
	}
	return !slices.ContainsFunc(c.Battery.Checks, func(k battery.Check) bool { return k.Result == battery.Fail })
}

// Key returns the hash of the statement of a record. The statement of a theorem is its type, and the statement of a definition of a proposition is its value, thus the two forms of one statement have the same key. Another definition is its type and its value together.
func Key(r lean.Record) string {
	switch {
	case r.Error != "":
		return ""
	case r.Kind == "theorem":
		return r.TypeHash
	case r.Statement():
		return r.ValueHash
	}
	return lean.Hash(r.Type + r.Value)
}

// Group is a group of candidates that pass the battery and have the same statement.
type Group struct {
	Hash       string   `json:"hash"`
	Candidates []int    `json:"candidates"` // the positions of the candidates in [Result.Candidates], in order
	Families   []string `json:"families"`   // the model families of the candidates, in order of name
}

// Result is the result of the selection.
type Result struct {
	Tag        string      `json:"tag"`
	Candidates []Candidate `json:"candidates"`
	Groups     []Group     `json:"groups"`  // the groups, the largest first
	Winner     int         `json:"winner"`  // the position of the winning candidate, or -1 when no candidate passes
	Unbound    []string    `json:"unbound"` // the concepts and the definitions in the uses closure of the node that have no Lean binding
}

// Select puts the candidates that pass the battery into groups by their hash. The largest group wins. Between two groups of the same size, the group with more model families wins, and then the group of the first candidate. The winner is the first candidate of the winning group.
func Select(tag string, cands []Candidate) *Result {
	r := &Result{Tag: tag, Candidates: cands, Groups: []Group{}, Winner: -1, Unbound: []string{}}
	at := map[string]int{}
	for i := range cands {
		c := &cands[i]
		if !c.Passes() {
			continue
		}
		g, ok := at[c.Hash]
		if !ok {
			g = len(r.Groups)
			at[c.Hash] = g
			r.Groups = append(r.Groups, Group{Hash: c.Hash, Families: []string{}})
		}
		r.Groups[g].Candidates = append(r.Groups[g].Candidates, i)
		if f := c.Family; f != "" && !slices.Contains(r.Groups[g].Families, f) {
			r.Groups[g].Families = append(r.Groups[g].Families, f)
			slices.Sort(r.Groups[g].Families)
		}
	}
	slices.SortStableFunc(r.Groups, func(a, b Group) int {
		return cmp.Or(cmp.Compare(len(b.Candidates), len(a.Candidates)), cmp.Compare(len(b.Families), len(a.Families)), cmp.Compare(a.Candidates[0], b.Candidates[0]))
	})
	if len(r.Groups) > 0 {
		r.Winner = r.Groups[0].Candidates[0]
	}
	return r
}

// Split reports whether the candidates that pass the battery have two or more statements.
func (r *Result) Split() bool { return len(r.Groups) > 1 }

// Families returns the model families of the candidates, in order of name.
func (r *Result) Families() []string {
	var out []string
	for _, c := range r.Candidates {
		if c.Family != "" && !slices.Contains(out, c.Family) {
			out = append(out, c.Family)
		}
	}
	slices.Sort(out)
	return out
}

// group returns the position of the group of the candidate i, or -1.
func (r *Result) group(i int) int {
	return slices.IndexFunc(r.Groups, func(g Group) bool { return slices.Contains(g.Candidates, i) })
}

// Unbound returns the concepts and the definitions in the uses closure of the node with the tag that have no Lean binding, in byte order. The node itself is not in the result. A tag of the closure that is not in the nodes is not in the result, because rule S07 reports it.
func Unbound(nodes []*node.Node, tag string) []string {
	byTag := map[string]*node.Node{}
	for _, n := range nodes {
		byTag[n.ID] = n
	}
	out := []string{}
	seen := map[string]bool{tag: true}
	queue := []string{tag}
	for len(queue) > 0 {
		n := byTag[queue[0]]
		queue = queue[1:]
		if n == nil {
			continue
		}
		for _, u := range n.List("uses") {
			if seen[u] {
				continue
			}
			seen[u] = true
			queue = append(queue, u)
			if m := byTag[u]; m != nil && (m.Kind() == "concept" || m.Kind() == "definition") && !bound(m) {
				out = append(out, u)
			}
		}
	}
	slices.Sort(out)
	return out
}

// bound reports whether the node has a Lean binding.
func bound(n *node.Node) bool {
	list, _ := n.Meta["formal"].([]any)
	for _, item := range list {
		if m, _ := item.(map[string]any); m["system"] == "lean4" && m["decl"] != nil {
			return true
		}
	}
	return false
}

// short returns the first 12 hex characters of a hash.
func short(h string) string {
	h = strings.TrimPrefix(h, node.HashPrefix)
	if len(h) > 12 {
		h = h[:12]
	}
	return h
}

// Markdown returns the report of the selection, for the body of the pull request: the candidates, the groups, the battery of the winner, the dissenting candidates and the definitions that need a binding first.
func (r *Result) Markdown() string {
	var b strings.Builder
	passed := 0
	stated := 0
	for _, c := range r.Candidates {
		if c.Hash != "" {
			stated++
		}
		if c.Passes() {
			passed++
		}
	}
	fmt.Fprintf(&b, "## m2 autoform %s\n\n", r.Tag)
	fmt.Fprintf(&b, "The formalize role ran %d times on %s. %d runs gave a statement, and %d of these pass the battery.\n\n", len(r.Candidates), r.Tag, stated, passed)
	b.WriteString("| Candidate | Run | Model | Declaration | Statement | Battery | Group |\n|---|---|---|---|---|---|---|\n")
	for i, c := range r.Candidates {
		res, grp := "no statement", "-"
		switch {
		case c.Passes():
			res = "pass"
		case c.Battery != nil:
			res = "fail"
		}
		if g := r.group(i); g >= 0 {
			grp = fmt.Sprint(g + 1)
		}
		fmt.Fprintf(&b, "| c%d | %s | %s | %s | %s | %s | %s |\n", i+1, c.Run, cmp.Or(c.Model, "-"), cmp.Or(c.Decl, "-"), cmp.Or(short(c.Hash), "-"), res, grp)
	}
	b.WriteString("\n### Selection\n\n")
	if r.Winner < 0 {
		b.WriteString("No candidate passes the battery, so this report proposes no binding.\n")
	} else {
		w := r.Candidates[r.Winner]
		g := r.Groups[0]
		fmt.Fprintf(&b, "The largest group has %d of the %d candidates that pass, from the model families %s. The binding of c%d to `%s` goes in at F0. A fidelity review takes it higher.\n", len(g.Candidates), passed, strings.Join(g.Families, ", "), r.Winner+1, w.Decl)
	}
	if r.Split() {
		fmt.Fprintf(&b, "\nThe candidates split into %d groups. A split usually points at an ambiguity of the informal statement, for example the convention for ℕ, whether a ring has a unit, or whether a sum is over a finite set. The reviewer must read the dissenting candidates.\n", len(r.Groups))
	}
	if fs := r.Families(); len(fs) < 2 {
		fmt.Fprintf(&b, "\nAll candidates come from the model family %s. Agreement in one family is weaker evidence than agreement across two families (spec 10.2).\n", cmp.Or(strings.Join(fs, ""), "that is not known"))
	}
	if r.Winner >= 0 {
		w := r.Candidates[r.Winner]
		b.WriteString("\n### Battery of the winner\n\n| Check | Result | Class | Detail |\n|---|---|---|---|\n")
		for _, k := range w.Battery.Checks {
			fmt.Fprintf(&b, "| %d %s | %s | %s | %s |\n", k.Number, k.Name, k.Result, cmp.Or(k.Class, "-"), cmp.Or(strings.ReplaceAll(k.Detail, "\n", " "), "-"))
		}
	}
	var dissent []int
	for i, c := range r.Candidates {
		if c.Lean != "" && (r.Winner < 0 || r.group(i) != 0) {
			dissent = append(dissent, i)
		}
	}
	if len(dissent) > 0 {
		b.WriteString("\n### Dissenting candidates\n")
		for _, i := range dissent {
			c := r.Candidates[i]
			why := "it fails the battery"
			if c.Passes() {
				why = fmt.Sprintf("it is in group %d", r.group(i)+1)
			}
			fmt.Fprintf(&b, "\nc%d, from %s: %s.\n\n```lean\n%s\n```\n", i+1, cmp.Or(c.Model, "a model that is not known"), why, strings.TrimSpace(c.Lean))
		}
	}
	b.WriteString("\n### Definitions\n\n")
	if len(r.Unbound) == 0 {
		b.WriteString("Each concept and definition that the node uses has a Lean binding.\n")
	} else {
		fmt.Fprintf(&b, "These concepts and definitions that the node uses have no Lean binding: %s. The statement cannot go higher than F1 before each of them has a binding at F2 (spec 10.3).\n", strings.Join(r.Unbound, ", "))
	}
	return b.String()
}
