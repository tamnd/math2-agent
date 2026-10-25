// Package status computes the status of each node from its evidence (spec 04.7). No node file states a status. The build computes it again at each commit.
//
// The status has two axes. The evidence level, E⊥ and E0 to E5, says how strong the evidence for the truth of a node is. The fidelity level, F0 to F3, says how strongly we believe that a formal statement says what the informal statement says. [Compute] gives both levels, the derived flags, the items of evidence and one reason for each decision, so that m2 status can show why a node has its levels.
//
// The rules for the sources are the rules of M1. A book source with a locator is a textbook source. A doi, zbmath or mr source with a locator is a refereed source. Each other source is a claim. The record plane of M2 makes these rules more exact with the type of each venue.
//
// A review counts for a binding when the binding lists its id in fidelity_reviews, the review read the declaration that the kernel checks, the verdict is faithful or faithful-with-conventions, and the two hashes of the review are the hashes of the build: the hash of the statement section and the elaborated-type hash. When one of the hashes changed, the review is stale and does not count (rule F02).
package status

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
)

// The derived flags of spec 04.7.
const (
	AC             = "ac"               // each bound proof uses Classical.choice
	NoAC           = "noac"             // a bound proof does not use Classical.choice
	Computational  = "computational"    // a bound proof uses native_decide
	SorryInClosure = "sorry_in_closure" // a node that the node depends on is not at E5
	StaleReview    = "stale_review"     // a review stopped counting because a hash changed
	Orphan         = "orphan"           // no edge from another node ends at the node, and it is not a topic
	Conditional    = "conditional"      // a bound proof uses a hypothesis from Math2/Hypotheses/
	ExternalReplay = "external_replay"  // E5 rests on the replay of an external library
)

// HypothesisPrefix starts the name of each named hypothesis (spec 07.5). An axiom with this prefix makes a result conditional, and it does not stop E5.
const HypothesisPrefix = "Math2.Hypotheses."

// The states of a review of a binding.
const (
	Counts        = "counts"            // the review counts for the fidelity level
	Stale         = "stale"             // a hash changed after the review
	Missing       = "missing"           // the binding lists the id, and no review has it
	OtherDecl     = "other_declaration" // the review read a different declaration
	NotFaithfulRv = "not_faithful"      // the verdict is not-faithful
	Unchecked     = "unchecked"         // the build has no record of the declaration, so the hashes cannot be compared
)

// Input is the evidence of a build.
type Input struct {
	Nodes   []*node.Node
	Records map[string]lean.Record // the extraction record of each declaration
	Reviews map[string]Review      // the reviews by id
	Replays map[string]Replay      // the replay result of each declaration. It is empty until the checker stack runs.
	Battery map[string]bool        // the result of the machine checks of each declaration (spec 10.4). It is empty until M4.
}

// Replay is the result of the independent replay and the comparator check of one declaration (spec 07.7).
type Replay struct {
	Passed   bool              `json:"passed"`
	External bool              `json:"external,omitempty"` // the result comes from the replay of an external library at its pinned commit
	Checkers map[string]string `json:"checkers,omitempty"` // the version of each checker
	Reason   string            `json:"reason,omitempty"`   // why the replay failed
}

// Status is the computed status of one node.
type Status struct {
	Tag      string    `json:"tag"`
	Kind     string    `json:"kind"`
	Name     string    `json:"name"`
	Retired  bool      `json:"retired,omitempty"` // the node is retired, and it has no levels
	Evidence Level     `json:"evidence"`
	Fidelity Fidelity  `json:"fidelity"`
	Flags    []string  `json:"flags"`
	Items    []Item    `json:"items"`
	Bindings []Binding `json:"bindings"`
	Reasons  []string  `json:"reasons"` // one sentence for each decision, in order
}

// Item is one item of evidence.
type Item struct {
	Kind   string `json:"kind"`   // source, proof, formal, resolution or refutation
	Ref    string `json:"ref"`    // the source id, the tag or the declaration
	Detail string `json:"detail"` // the locator, the system or the match
	Level  Level  `json:"level"`  // the level that the item gives
}

// Binding is the status of one Lean binding of a node.
type Binding struct {
	Decl     string        `json:"decl"`
	Adapter  string        `json:"adapter,omitempty"`
	Match    string        `json:"match"`
	Checked  string        `json:"checked"` // the declaration that the kernel checks and that a review reads: the adapter, or the declaration
	TypeHash string        `json:"type_hash"`
	Axioms   []string      `json:"axioms"`
	Fidelity Fidelity      `json:"fidelity"`
	Reviews  []ReviewState `json:"reviews"`
	Replay   *Replay       `json:"replay,omitempty"`
	Level    Level         `json:"level"`  // E5, E4 or None
	Reason   string        `json:"reason"` // why the binding has its level
	native   bool
	needs    bool
}

// NeedsReplay reports whether the binding has no replay result and passes each other check of E5. The build replays only such a binding. The JSON form of a Binding does not keep the value.
func (b Binding) NeedsReplay() bool { return b.needs }

// ReviewState is a review of a binding and the state of the review.
type ReviewState struct {
	ID       string `json:"id"`
	Reviewer string `json:"reviewer,omitempty"`
	State    string `json:"state"`
}

// evidenceKinds are the kinds that have an evidence level.
var evidenceKinds = []string{"statement", "conjecture", "problem", "example", "counterexample", "proof"}

// openKinds are the kinds that start at E0.
var openKinds = []string{"conjecture", "problem"}

// Compute returns the status of each node, in tag order. A retired node has no levels, and the other nodes do not see its edges.
func Compute(in Input) []Status {
	g := graph.Build(in.Nodes)
	byTag := map[string]*node.Node{}
	for _, n := range in.Nodes {
		if _, retired := n.Meta["retired"]; !retired {
			byTag[n.ID] = n
		}
	}
	out := []Status{}
	for _, n := range byTag {
		out = append(out, base(in, g, byTag, n))
	}
	slices.SortFunc(out, func(a, b Status) int { return strings.Compare(a.Tag, b.Tag) })
	index := map[string]int{}
	for i, s := range out {
		index[s.Tag] = i
	}
	level := func(levels []Level, tag string) (Level, bool) {
		i, ok := index[tag]
		if !ok {
			return None, false
		}
		return levels[i], true
	}
	snapshot := func() []Level {
		l := make([]Level, len(out))
		for i, s := range out {
			l[i] = s.Evidence
		}
		return l
	}

	// A resolved conjecture or problem takes the level of the statement that resolves it: a node in resolved_by, or a node that resolves it with the outcome proved or settled.
	first := snapshot()
	for i := range out {
		s := &out[i]
		if !slices.Contains(openKinds, s.Kind) {
			continue
		}
		resolvers := byTag[s.Tag].List("resolved_by")
		for _, e := range g.In(graph.Resolves, s.Tag) {
			if e.Label != "disproved" {
				resolvers = append(resolvers, e.From)
			}
		}
		slices.Sort(resolvers)
		for _, t := range slices.Compact(resolvers) {
			l, ok := level(first, t)
			if !ok {
				continue
			}
			s.Items = append(s.Items, Item{Kind: "resolution", Ref: t, Level: l})
			if l > s.Evidence {
				s.Evidence = l
				s.Reasons = append(s.Reasons, fmt.Sprintf("%s resolves it at %s.", t, l))
			}
		}
	}

	// A refutation at E2 or above makes the node E⊥. A resolution with the outcome disproved is a refutation.
	second := snapshot()
	for i := range out {
		s := &out[i]
		if s.Evidence == None {
			continue
		}
		refuters := g.In(graph.Refutes, s.Tag)
		for _, e := range g.In(graph.Resolves, s.Tag) {
			if e.Label == "disproved" {
				refuters = append(refuters, e)
			}
		}
		for _, e := range refuters {
			l, ok := level(second, e.From)
			if !ok {
				continue
			}
			s.Items = append(s.Items, Item{Kind: "refutation", Ref: e.From, Detail: e.Via, Level: l})
			if l >= Standard && s.Evidence != Refuted {
				s.Evidence = Refuted
				s.Reasons = append(s.Reasons, fmt.Sprintf("%s refutes it at %s, so the level is E⊥.", e.From, l))
			} else if l < Standard {
				s.Reasons = append(s.Reasons, fmt.Sprintf("%s refutes it only at %s. A refutation needs E2 or above.", e.From, l))
			}
		}
	}

	// sorry_in_closure: a node with a binding depends on a node below E5.
	final := snapshot()
	for i := range out {
		s := &out[i]
		if len(s.Bindings) == 0 || s.Evidence == None {
			continue
		}
		if t, l, ok := below(g, byTag, final, index, s.Tag); ok {
			s.Flags = append(s.Flags, SorryInClosure)
			s.Reasons = append(s.Reasons, fmt.Sprintf("%s in its closure is at %s, so the flag %s is set.", t, l, SorryInClosure))
		}
	}
	for i := range out {
		slices.Sort(out[i].Flags)
		out[i].Flags = slices.Compact(out[i].Flags)
	}
	for _, n := range in.Nodes {
		if byTag[n.ID] == nil {
			out = append(out, retired(n))
		}
	}
	slices.SortFunc(out, func(a, b Status) int { return strings.Compare(a.Tag, b.Tag) })
	return out
}

// retired returns the status of a retired node. It has no levels and no flags.
func retired(n *node.Node) Status {
	s := Status{Tag: n.ID, Kind: n.Kind(), Name: n.Name(), Retired: true, Fidelity: Unbound, Flags: []string{}, Items: []Item{}, Bindings: []Binding{}}
	r, _ := n.Meta["retired"].(map[string]any)
	var next []string
	switch v := r["successor"].(type) {
	case string:
		next = []string{v}
	case []any:
		for _, x := range v {
			next = append(next, str(x))
		}
	}
	switch len(next) {
	case 0:
		s.Reasons = []string{"The node is retired, so it has no status."}
	case 1:
		s.Reasons = []string{fmt.Sprintf("The node is retired, so it has no status. Its successor is %s.", next[0])}
	default:
		s.Reasons = []string{fmt.Sprintf("The node is retired, so it has no status. Its successors are %s.", strings.Join(next, ", "))}
	}
	return s
}

// base computes the status of n from its own evidence and from the evidence of its proofs.
func base(in Input, g *graph.Graph, byTag map[string]*node.Node, n *node.Node) Status {
	s := Status{Tag: n.ID, Kind: n.Kind(), Name: n.Name(), Fidelity: Unbound, Flags: []string{}, Items: []Item{}, Bindings: []Binding{}, Reasons: []string{}}
	hasLevel := slices.Contains(evidenceKinds, s.Kind)
	open := slices.Contains(openKinds, s.Kind)
	reason := func(format string, args ...any) { s.Reasons = append(s.Reasons, fmt.Sprintf(format, args...)) }

	// The bindings and the fidelity level.
	stmt := statementHash(n)
	for _, f := range formals(n) {
		b := binding(in, f, stmt)
		if b.Fidelity > s.Fidelity {
			s.Fidelity = b.Fidelity
		}
		for _, r := range b.Reviews {
			if r.State == Stale {
				s.Flags = append(s.Flags, StaleReview)
				reason("The review %s of %s is stale, because a hash changed after the review.", r.ID, b.Checked)
			}
		}
		s.Bindings = append(s.Bindings, b)
	}
	if s.Fidelity != Unbound {
		reason("The best binding has the fidelity %s.", s.Fidelity)
	}
	flags(&s, in)

	if !hasLevel {
		reason("A %s has no evidence level.", s.Kind)
		if !orphanFree(g, byTag, n) {
			s.Flags = append(s.Flags, Orphan)
		}
		return s
	}
	if open {
		s.Evidence = Open
		reason("A %s starts at E0.", s.Kind)
	} else {
		s.Evidence = Claimed
		reason("The node states it, so it is at least E1.")
	}

	// The sources of the node and of its proofs.
	if !open {
		holders := []*node.Node{n}
		for _, e := range g.In(graph.Proves, n.ID) {
			if p := byTag[e.From]; p != nil {
				holders = append(holders, p)
				s.Items = append(s.Items, Item{Kind: "proof", Ref: p.ID, Level: Claimed})
			}
		}
		textbooks := map[string]bool{}
		var refereed []string
		for _, h := range holders {
			for _, src := range sources(h) {
				l := Claimed
				switch {
				case src.locator == "":
				case src.system == "book":
					textbooks[src.id] = true
					l = Standard
				case src.system == "doi" || src.system == "zbmath" || src.system == "mr":
					refereed = append(refereed, src.system+":"+src.id)
					l = Refereed
				}
				s.Items = append(s.Items, Item{Kind: "source", Ref: src.system + ":" + src.id, Detail: src.locator, Level: l})
			}
		}
		switch {
		case len(refereed) > 0:
			s.Evidence = Refereed
			reason("The source %s is in a refereed venue and has a locator, so the level is E3.", refereed[0])
		case len(textbooks) >= 2:
			s.Evidence = Standard
			reason("%d textbook sources have locators, so the level is E2.", len(textbooks))
		case len(textbooks) == 1:
			reason("One textbook source has a locator. E2 needs two independent textbook sources.")
		}
	}

	// The formal bindings.
	if !open {
		for _, b := range s.Bindings {
			s.Items = append(s.Items, Item{Kind: "formal", Ref: b.Checked, Detail: b.Match, Level: b.Level})
			reason("%s", b.Reason)
			if b.Level > s.Evidence {
				s.Evidence = b.Level
			}
		}
		for _, b := range s.Bindings {
			if b.Level == Kernel && b.Replay != nil && b.Replay.External {
				s.Flags = append(s.Flags, ExternalReplay)
			}
		}
	}
	if !orphanFree(g, byTag, n) {
		s.Flags = append(s.Flags, Orphan)
	}
	return s
}

// formal is one formal entry of a node.
type formal struct {
	system, decl, adapter, match string
	reviews                      []string
}

func formals(n *node.Node) []formal {
	list, _ := n.Meta["formal"].([]any)
	var out []formal
	for _, item := range list {
		m, _ := item.(map[string]any)
		f := formal{system: str(m["system"]), decl: str(m["decl"]), adapter: str(m["adapter"]), match: str(m["match"])}
		if f.system != "lean4" || f.decl == "" {
			continue
		}
		rs, _ := m["fidelity_reviews"].([]any)
		for _, r := range rs {
			f.reviews = append(f.reviews, str(r))
		}
		out = append(out, f)
	}
	return out
}

// binding computes the fidelity level and the kernel level of one binding.
func binding(in Input, f formal, stmt string) Binding {
	b := Binding{Decl: f.decl, Adapter: f.adapter, Match: f.match, Checked: cmp.Or(f.adapter, f.decl), Fidelity: F0, Axioms: []string{}, Reviews: []ReviewState{}}
	rec, ok := in.Records[b.Checked]
	known := ok && rec.Error == ""
	if known {
		b.TypeHash = rec.TypeHash
		b.Axioms = slices.Clone(rec.Axioms)
		b.native = len(rec.NativeDecideAxioms) > 0
	}
	reviewers := map[string]bool{}
	for _, id := range f.reviews {
		r, ok := in.Reviews[id]
		st := ReviewState{ID: id, Reviewer: r.Reviewer}
		switch {
		case !ok:
			st.State = Missing
		case r.Decl != b.Checked:
			st.State = OtherDecl
		case r.Verdict == NotFaithful:
			st.State = NotFaithfulRv
		case !known:
			st.State = Unchecked
		case r.StatementHash != stmt || r.TypeHash != rec.TypeHash:
			st.State = Stale
		default:
			st.State = Counts
			reviewers[r.Reviewer] = true
		}
		b.Reviews = append(b.Reviews, st)
	}
	switch {
	case len(reviewers) >= 2:
		b.Fidelity = F3
	case len(reviewers) == 1:
		b.Fidelity = F2
	case in.Battery[b.Checked]:
		b.Fidelity = F1
	}
	if r, ok := in.Replays[b.Checked]; ok {
		b.Replay = &r
	}
	b.Level, b.Reason = kernel(b, rec, known)
	if b.Replay == nil {
		passed := b
		passed.Replay = &Replay{Passed: true}
		level, _ := kernel(passed, rec, known)
		b.needs = level != None
	}
	return b
}

// kernel decides whether a binding gives E5 or E4, and gives the reason.
func kernel(b Binding, rec lean.Record, known bool) (Level, string) {
	no := func(format string, args ...any) (Level, string) {
		return None, fmt.Sprintf("The binding %s is not E5: ", b.Checked) + fmt.Sprintf(format, args...) + "."
	}
	switch {
	case b.Match == "partial":
		return no("a statement-only binding (match: partial) is never E5 (rule F07)")
	case b.Match == "weaker":
		return no("a weaker formal statement does not prove the node")
	case (b.Match == "stronger" || b.Match == "variant") && b.Adapter == "":
		return no("a %s match needs an adapter", b.Match)
	case b.Match != "exact" && b.Match != "stronger" && b.Match != "variant":
		return no("the match %q is not exact, stronger or variant", b.Match)
	case !known && rec.Error != "":
		return no("M2Extract gives the error %q", rec.Error)
	case !known:
		return no("the build has no extraction record of the declaration")
	case rec.Kind != "theorem":
		return no("the declaration is a %s, not a theorem", rec.Kind)
	}
	for _, ax := range rec.Axioms {
		if ax == "sorryAx" {
			return no("the proof uses sorry")
		}
		if !slices.Contains(lean.StandardAxioms, ax) && !strings.HasPrefix(ax, HypothesisPrefix) {
			return no("the proof uses the axiom %s", ax)
		}
	}
	switch {
	case b.Fidelity < F2:
		return no("the fidelity is %s, and E5 needs F2", b.Fidelity)
	case b.Replay == nil:
		return no("the declaration has no replay result")
	case !b.Replay.Passed && b.Replay.Reason != "":
		return no("the replay failed: %s", b.Replay.Reason)
	case !b.Replay.Passed:
		return no("the replay or the comparator check failed")
	case b.native:
		return Certified, fmt.Sprintf("The binding %s passes each check of E5, but the proof uses native_decide, so the level is E4.", b.Checked)
	}
	return Kernel, fmt.Sprintf("The kernel checks %s with the standard axioms, the fidelity is %s and the replay passed, so the level is E5.", b.Checked, b.Fidelity)
}

// flags sets the flags that come from the axioms of the bindings.
func flags(s *Status, in Input) {
	var withRecord, choice int
	for _, b := range s.Bindings {
		rec, ok := in.Records[b.Checked]
		if !ok || rec.Error != "" {
			continue
		}
		withRecord++
		if slices.Contains(rec.Axioms, "Classical.choice") {
			choice++
		}
		if len(rec.NativeDecideAxioms) > 0 {
			s.Flags = append(s.Flags, Computational)
		}
		if slices.ContainsFunc(rec.Axioms, func(ax string) bool { return strings.HasPrefix(ax, HypothesisPrefix) }) {
			s.Flags = append(s.Flags, Conditional)
		}
	}
	switch {
	case withRecord == 0:
	case choice < withRecord:
		s.Flags = append(s.Flags, NoAC)
	default:
		s.Flags = append(s.Flags, AC)
	}
}

// orphanFree reports whether an edge from another node ends at n, or n is a topic. An edge from a formal declaration or from a record does not count.
func orphanFree(g *graph.Graph, byTag map[string]*node.Node, n *node.Node) bool {
	if n.Kind() == "topic" {
		return true
	}
	for _, t := range graph.Types() {
		for _, e := range g.In(t, n.ID) {
			if e.From != n.ID && byTag[e.From] != nil {
				return true
			}
		}
	}
	return false
}

// below finds a node with an evidence level below E5 in the uses closure of tag and of its proofs.
func below(g *graph.Graph, byTag map[string]*node.Node, levels []Level, index map[string]int, tag string) (string, Level, bool) {
	seen := map[string]bool{tag: true}
	stack := []string{tag}
	for _, e := range g.In(graph.Proves, tag) {
		stack = append(stack, e.From)
		seen[e.From] = true
	}
	for len(stack) > 0 {
		t := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, e := range g.Out(graph.Uses, t) {
			if seen[e.To] {
				continue
			}
			seen[e.To] = true
			stack = append(stack, e.To)
			i, ok := index[e.To]
			if ok && levels[i] != None && levels[i] < Kernel && byTag[e.To] != nil {
				return e.To, levels[i], true
			}
		}
	}
	return "", None, false
}

// source is one source of a node.
type source struct{ system, id, locator string }

// sourceKeys are the identifier keys of a source in the node schema.
var sourceKeys = []string{"zbmath", "arxiv", "doi", "book", "mr", "stacks", "kerodon", "mathoverflow", "mathse", "erdos", "formal_conjectures", "palomar", "hexagon", "url"}

func sources(n *node.Node) []source {
	list, _ := n.Meta["sources"].([]any)
	var out []source
	for _, item := range list {
		m, _ := item.(map[string]any)
		for _, k := range sourceKeys {
			if id := str(m[k]); id != "" {
				out = append(out, source{system: k, id: id, locator: str(m["locator"])})
				break
			}
		}
	}
	return out
}

// statementHash returns the hash of the first statement section of n, or "" when n has none.
func statementHash(n *node.Node) string {
	names := check.StatementSections()
	for _, sec := range check.Sections(n) {
		if slices.Contains(names, sec.Name) {
			return node.HashText(sec.Text)
		}
	}
	return ""
}

// StatementHash returns the hash that a review of n records as statement_hash: the hash of the first section of n that states the mathematics, as node.HashText gives it. It is "" when n has no such section.
func StatementHash(n *node.Node) string { return statementHash(n) }

func str(v any) string {
	s, _ := v.(string)
	return s
}
