package status

import (
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
)

// parse makes nodes from front matter. Each item is "ID kind" and then the other lines of the front matter. Each node has a Statement section with the text "S.".
func parse(t *testing.T, items ...string) []*node.Node {
	t.Helper()
	var out []*node.Node
	for _, it := range items {
		head, rest, _ := strings.Cut(it, "\n")
		id, kind, _ := strings.Cut(head, " ")
		src := "---\ntag: " + id + "\nkind: " + kind + "\nname: " + id + "\n" + rest + "\n---\n\n## Statement\n\nS.\n"
		n, err := node.Parse("nodes/"+id[:2]+"/"+id+".md", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

// theorem returns a record of a theorem with the axioms.
func theorem(decl string, axioms ...string) lean.Record {
	return lean.Record{Decl: decl, Kind: "theorem", Type: "T " + decl, TypeHash: lean.Hash("T " + decl), Axioms: axioms}
}

// review returns a review of decl by reviewer that counts for n and rec.
func review(id, reviewer, decl string, n *node.Node, rec lean.Record) Review {
	return Review{ID: id, Reviewer: reviewer, Date: "2026-10-25", Decl: decl, StatementHash: StatementHash(n), TypeHash: rec.TypeHash, Verdict: Faithful}
}

func find(t *testing.T, out []Status, tag string) Status {
	t.Helper()
	for _, s := range out {
		if s.Tag == tag {
			return s
		}
	}
	t.Fatalf("no status for %s", tag)
	return Status{}
}

func TestSources(t *testing.T) {
	tests := []struct {
		name  string
		nodes []string
		tag   string
		want  Level
	}{
		{"statement", []string{"1A0000 statement"}, "1A0000", Claimed},
		{"conjecture", []string{"1A0000 conjecture"}, "1A0000", Open},
		{"problem", []string{"1A0000 problem"}, "1A0000", Open},
		{"concept", []string{"1A0000 concept"}, "1A0000", None},
		{"one book", []string{"1A0000 statement\nsources:\n  - book: isbn:1\n    locator: p. 1"}, "1A0000", Claimed},
		{"two books", []string{"1A0000 statement\nsources:\n  - book: isbn:1\n    locator: p. 1\n  - book: isbn:2\n    locator: p. 2"}, "1A0000", Standard},
		{"the same book two times", []string{"1A0000 statement\nsources:\n  - book: isbn:1\n    locator: p. 1\n  - book: isbn:1\n    locator: p. 2"}, "1A0000", Claimed},
		{"a book without a locator", []string{"1A0000 statement\nsources:\n  - book: isbn:1\n    locator: p. 1\n  - book: isbn:2"}, "1A0000", Claimed},
		{"doi", []string{"1A0000 statement\nsources:\n  - doi: 10.1/x\n    locator: Theorem 2"}, "1A0000", Refereed},
		{"zbmath", []string{"1A0000 statement\nsources:\n  - zbmath: \"1234.56789\"\n    locator: Theorem 2"}, "1A0000", Refereed},
		{"doi without a locator", []string{"1A0000 statement\nsources:\n  - doi: 10.1/x"}, "1A0000", Claimed},
		{"arxiv", []string{"1A0000 statement\nsources:\n  - arxiv: \"2401.00001\"\n    locator: Theorem 2"}, "1A0000", Claimed},
		{"the books of a proof", []string{"1A0000 statement\nproofs: [1B0000]", "1B0000 proof\nproves: 1A0000\nsources:\n  - book: isbn:1\n    locator: p. 1\n  - book: isbn:2\n    locator: p. 2"}, "1A0000", Standard},
		{"the sources of a conjecture do not count", []string{"1A0000 conjecture\nsources:\n  - doi: 10.1/x\n    locator: Conjecture 1"}, "1A0000", Open},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := find(t, Compute(Input{Nodes: parse(t, tt.nodes...)}), tt.tag)
			if s.Evidence != tt.want {
				t.Errorf("evidence %s, want %s. Reasons: %q", s.Evidence, tt.want, s.Reasons)
			}
		})
	}
}

const twoBooks = "\nsources:\n  - book: isbn:1\n    locator: p. 1\n  - book: isbn:2\n    locator: p. 2"

func TestResolution(t *testing.T) {
	tests := []struct {
		name  string
		nodes []string
		want  Level
	}{
		{"resolved_by", []string{"1A0000 conjecture\nresolved_by: [1B0000]", "1B0000 statement" + twoBooks}, Standard},
		{"resolves proved", []string{"1A0000 conjecture", "1B0000 statement\nresolves:\n  - target: 1A0000\n    outcome: proved" + twoBooks}, Standard},
		{"resolves disproved", []string{"1A0000 conjecture", "1B0000 statement\nresolves:\n  - target: 1A0000\n    outcome: disproved" + twoBooks}, Refuted},
		{"disproved at E1", []string{"1A0000 conjecture", "1B0000 statement\nresolves:\n  - target: 1A0000\n    outcome: disproved"}, Open},
		{"refutes at E2", []string{"1A0000 statement" + twoBooks, "1B0000 counterexample\nrefutes:\n  - target: 1A0000" + twoBooks}, Refuted},
		{"refutes at E1", []string{"1A0000 statement", "1B0000 counterexample\nrefutes:\n  - target: 1A0000"}, Claimed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := find(t, Compute(Input{Nodes: parse(t, tt.nodes...)}), "1A0000")
			if s.Evidence != tt.want {
				t.Errorf("evidence %s, want %s. Reasons: %q", s.Evidence, tt.want, s.Reasons)
			}
		})
	}
}

// bound is a statement with one binding and a node that uses it, so that it is not an orphan.
func bound(match, adapter, reviews string) []string {
	f := "1A0000 statement\nformal:\n  - system: lean4\n    library: math2\n    decl: X.thm\n    match: " + match
	if adapter != "" {
		f += "\n    adapter: X.adapter"
	}
	if reviews != "" {
		f += "\n    fidelity_reviews: [" + reviews + "]"
	}
	return []string{f, "1B0000 statement\nuses: [1A0000]"}
}

func TestKernel(t *testing.T) {
	type tc struct {
		name     string
		match    string
		adapter  bool
		axioms   []string
		native   bool
		kind     string
		reviews  []string // the reviewers of the reviews that count
		stale    bool
		battery  bool
		replay   *Replay
		evidence Level
		fidelity Fidelity
		flags    []string
	}
	passed := &Replay{Passed: true}
	tests := []tc{
		{name: "exact", match: "exact", axioms: lean.StandardAxioms, reviews: []string{"github:a"}, replay: passed, evidence: Kernel, fidelity: F2, flags: []string{AC}},
		{name: "two reviewers", match: "exact", axioms: lean.StandardAxioms, reviews: []string{"github:a", "github:b"}, replay: passed, evidence: Kernel, fidelity: F3, flags: []string{AC}},
		{name: "one reviewer two times", match: "exact", axioms: lean.StandardAxioms, reviews: []string{"github:a", "github:a"}, replay: passed, evidence: Kernel, fidelity: F2, flags: []string{AC}},
		{name: "stronger with an adapter", match: "stronger", adapter: true, axioms: lean.StandardAxioms, reviews: []string{"github:a"}, replay: passed, evidence: Kernel, fidelity: F2, flags: []string{AC}},
		{name: "stronger without an adapter", match: "stronger", axioms: lean.StandardAxioms, reviews: []string{"github:a"}, replay: passed, evidence: Claimed, fidelity: F2, flags: []string{AC}},
		{name: "partial", match: "partial", axioms: lean.StandardAxioms, reviews: []string{"github:a", "github:b"}, replay: passed, evidence: Claimed, fidelity: F3, flags: []string{AC}},
		{name: "weaker", match: "weaker", axioms: lean.StandardAxioms, reviews: []string{"github:a"}, replay: passed, evidence: Claimed, fidelity: F2, flags: []string{AC}},
		{name: "no review", match: "exact", axioms: lean.StandardAxioms, replay: passed, evidence: Claimed, fidelity: F0, flags: []string{AC}},
		{name: "battery", match: "exact", axioms: lean.StandardAxioms, battery: true, replay: passed, evidence: Claimed, fidelity: F1, flags: []string{AC}},
		{name: "stale", match: "exact", axioms: lean.StandardAxioms, reviews: []string{"github:a"}, stale: true, replay: passed, evidence: Claimed, fidelity: F0, flags: []string{AC, StaleReview}},
		{name: "no replay", match: "exact", axioms: lean.StandardAxioms, reviews: []string{"github:a"}, evidence: Claimed, fidelity: F2, flags: []string{AC}},
		{name: "failed replay", match: "exact", axioms: lean.StandardAxioms, reviews: []string{"github:a"}, replay: &Replay{}, evidence: Claimed, fidelity: F2, flags: []string{AC}},
		{name: "external replay", match: "exact", axioms: lean.StandardAxioms, reviews: []string{"github:a"}, replay: &Replay{Passed: true, External: true}, evidence: Kernel, fidelity: F2, flags: []string{AC, ExternalReplay}},
		{name: "sorry", match: "exact", axioms: []string{"propext", "sorryAx"}, reviews: []string{"github:a"}, replay: passed, evidence: Claimed, fidelity: F2, flags: []string{NoAC}},
		{name: "other axiom", match: "exact", axioms: []string{"X.ax"}, reviews: []string{"github:a"}, replay: passed, evidence: Claimed, fidelity: F2, flags: []string{NoAC}},
		{name: "hypothesis", match: "exact", axioms: []string{"Classical.choice", HypothesisPrefix + "RH"}, reviews: []string{"github:a"}, replay: passed, evidence: Kernel, fidelity: F2, flags: []string{AC, Conditional}},
		{name: "no axioms", match: "exact", reviews: []string{"github:a"}, replay: passed, evidence: Kernel, fidelity: F2, flags: []string{NoAC}},
		{name: "native_decide", match: "exact", axioms: lean.StandardAxioms, native: true, reviews: []string{"github:a"}, replay: passed, evidence: Certified, fidelity: F2, flags: []string{AC, Computational}},
		{name: "a definition", match: "exact", kind: "def", reviews: []string{"github:a"}, replay: passed, evidence: Claimed, fidelity: F2, flags: []string{NoAC}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ids []string
			for i := range tt.reviews {
				ids = append(ids, "r-1A0000-0"+string(rune('1'+i)))
			}
			nodes := parse(t, bound(tt.match, map[bool]string{true: "X.adapter"}[tt.adapter], strings.Join(ids, ", "))...)
			checked := "X.thm"
			if tt.adapter {
				checked = "X.adapter"
			}
			rec := theorem(checked, tt.axioms...)
			if tt.kind != "" {
				rec.Kind = tt.kind
			}
			if tt.native {
				rec.NativeDecideAxioms = []string{"X.thm._native.1"}
			}
			in := Input{Nodes: nodes, Records: map[string]lean.Record{checked: rec}, Reviews: map[string]Review{}, Replays: map[string]Replay{}, Battery: map[string]bool{checked: tt.battery}}
			for i, id := range ids {
				in.Reviews[id] = review(id, tt.reviews[i], checked, nodes[0], rec)
			}
			if tt.stale {
				rec.TypeHash = lean.Hash("a new type")
				in.Records[checked] = rec
			}
			if tt.replay != nil {
				in.Replays[checked] = *tt.replay
			}
			s := find(t, Compute(in), "1A0000")
			if s.Evidence != tt.evidence || s.Fidelity != tt.fidelity || !slices.Equal(s.Flags, tt.flags) {
				t.Errorf("got %s %s %q, want %s %s %q. Reasons: %q", s.Evidence, s.Fidelity, s.Flags, tt.evidence, tt.fidelity, tt.flags, s.Reasons)
			}
		})
	}
}

// TestStale changes the type hash of a bound declaration after the review. The review stops to count, and the levels go down.
func TestStale(t *testing.T) {
	nodes := parse(t, bound("exact", "", "r-1A0000-01")...)
	rec := theorem("X.thm", lean.StandardAxioms...)
	in := Input{
		Nodes:   nodes,
		Records: map[string]lean.Record{"X.thm": rec},
		Reviews: map[string]Review{"r-1A0000-01": review("r-1A0000-01", "github:a", "X.thm", nodes[0], rec)},
		Replays: map[string]Replay{"X.thm": {Passed: true}},
	}
	if s := find(t, Compute(in), "1A0000"); s.Evidence != Kernel || s.Fidelity != F2 {
		t.Fatalf("before the change: %s %s, want E5 F2", s.Evidence, s.Fidelity)
	}
	rec.Type = "T X.thm with a new hypothesis"
	rec.TypeHash = lean.Hash(rec.Type)
	in.Records["X.thm"] = rec
	s := find(t, Compute(in), "1A0000")
	if s.Evidence != Claimed || s.Fidelity != F0 || !slices.Contains(s.Flags, StaleReview) {
		t.Fatalf("after the change: %s %s %q, want E1 F0 with %s", s.Evidence, s.Fidelity, s.Flags, StaleReview)
	}
	if got := s.Bindings[0].Reviews[0].State; got != Stale {
		t.Errorf("review state %q, want %q", got, Stale)
	}
}

// TestStatementStale changes the statement section after the review.
func TestStatementStale(t *testing.T) {
	nodes := parse(t, bound("exact", "", "r-1A0000-01")...)
	rec := theorem("X.thm", lean.StandardAxioms...)
	r := review("r-1A0000-01", "github:a", "X.thm", nodes[0], rec)
	r.StatementHash = node.HashText("An old statement.")
	in := Input{Nodes: nodes, Records: map[string]lean.Record{"X.thm": rec}, Reviews: map[string]Review{r.ID: r}}
	s := find(t, Compute(in), "1A0000")
	if s.Fidelity != F0 || s.Bindings[0].Reviews[0].State != Stale {
		t.Errorf("fidelity %s, state %q, want F0 and %q", s.Fidelity, s.Bindings[0].Reviews[0].State, Stale)
	}
}

func TestReviewStates(t *testing.T) {
	nodes := parse(t, bound("exact", "", "r-1A0000-01, r-1A0000-02, r-1A0000-03, r-1A0000-04")...)
	rec := theorem("X.thm", lean.StandardAxioms...)
	other := review("r-1A0000-02", "github:b", "Y.thm", nodes[0], rec)
	no := review("r-1A0000-03", "github:c", "X.thm", nodes[0], rec)
	no.Verdict = NotFaithful
	conv := review("r-1A0000-04", "github:d", "X.thm", nodes[0], rec)
	conv.Verdict = FaithfulWithConventions
	in := Input{Nodes: nodes, Records: map[string]lean.Record{"X.thm": rec}, Reviews: map[string]Review{other.ID: other, no.ID: no, conv.ID: conv}}
	s := find(t, Compute(in), "1A0000")
	var got []string
	for _, r := range s.Bindings[0].Reviews {
		got = append(got, r.State)
	}
	want := []string{Missing, OtherDecl, NotFaithfulRv, Counts}
	if !slices.Equal(got, want) || s.Fidelity != F2 {
		t.Errorf("states %q fidelity %s, want %q F2", got, s.Fidelity, want)
	}

	delete(in.Records, "X.thm")
	s = find(t, Compute(in), "1A0000")
	if st := s.Bindings[0].Reviews[3].State; st != Unchecked || s.Fidelity != F0 {
		t.Errorf("without a record: state %q fidelity %s, want %q F0", st, s.Fidelity, Unchecked)
	}
}

func TestSorryInClosure(t *testing.T) {
	nodes := parse(t,
		"1A0000 statement\nuses: [1C0000]\nformal:\n  - system: lean4\n    library: math2\n    decl: X.thm\n    match: exact\n    fidelity_reviews: [r-1A0000-01]",
		"1B0000 statement\nuses: [1A0000]",
		"1C0000 statement\nuses: [1D0000]",
		"1D0000 concept",
	)
	rec := theorem("X.thm", lean.StandardAxioms...)
	in := Input{
		Nodes:   nodes,
		Records: map[string]lean.Record{"X.thm": rec},
		Reviews: map[string]Review{"r-1A0000-01": review("r-1A0000-01", "github:a", "X.thm", nodes[0], rec)},
		Replays: map[string]Replay{"X.thm": {Passed: true}},
	}
	s := find(t, Compute(in), "1A0000")
	if s.Evidence != Kernel || !slices.Contains(s.Flags, SorryInClosure) {
		t.Errorf("got %s %q, want E5 with %s", s.Evidence, s.Flags, SorryInClosure)
	}
	if s := find(t, Compute(in), "1B0000"); slices.Contains(s.Flags, SorryInClosure) {
		t.Errorf("1B0000 has no binding, and it has the flag %s", SorryInClosure)
	}
}

func TestOrphan(t *testing.T) {
	out := Compute(Input{Nodes: parse(t, "1A0000 statement", "1B0000 statement\nuses: [1A0000]", "1C0000 topic")})
	for tag, want := range map[string]bool{"1A0000": false, "1B0000": true, "1C0000": false} {
		if got := slices.Contains(find(t, out, tag).Flags, Orphan); got != want {
			t.Errorf("%s orphan %v, want %v", tag, got, want)
		}
	}
}

func TestRetired(t *testing.T) {
	out := Compute(Input{Nodes: parse(t, "1A0000 statement\nretired:\n  reason: duplicate\n  successor: [1B0000]", "1B0000 statement")})
	if len(out) != 2 || !out[0].Retired || out[0].Evidence != None || out[1].Retired || out[1].Evidence != Claimed {
		t.Fatalf("got %+v", out)
	}
	if want := "The node is retired, so it has no status. Its successor is 1B0000."; out[0].Reasons[0] != want {
		t.Errorf("reason %q, want %q", out[0].Reasons[0], want)
	}
}

func TestLevelText(t *testing.T) {
	for l := None; l <= Kernel; l++ {
		b, _ := l.MarshalText()
		var m Level
		if err := m.UnmarshalText(b); err != nil || m != l {
			t.Errorf("%s: round trip gives %s, %v", l, m, err)
		}
	}
	for f := Unbound; f <= F3; f++ {
		b, _ := f.MarshalText()
		var m Fidelity
		if err := m.UnmarshalText(b); err != nil || m != f {
			t.Errorf("%s: round trip gives %s, %v", f, m, err)
		}
	}
	if Kernel.Word() != "kernel" || Refuted.String() != "E⊥" {
		t.Errorf("names %q %q", Kernel.Word(), Refuted.String())
	}
}
