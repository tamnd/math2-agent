package autoform

import (
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
)

// report returns a battery report whose checks pass, with the check fail failing when it is not zero.
func report(fail int) *battery.Report {
	r := &battery.Report{Tag: "7K3QX2"}
	for i, name := range battery.Names() {
		c := battery.Check{Number: i + 1, Name: name, Result: battery.Pass}
		if i+1 == fail {
			c.Result, c.Class = battery.Fail, battery.Strong
		}
		r.Checks = append(r.Checks, c)
	}
	return r
}

func cand(hash, family string, fail int) Candidate {
	return Candidate{Run: "r-" + hash + family, Model: family + "-model", Family: family, Outcome: "success", Decl: "Math2.S." + hash, Hash: hash, Lean: "def Math2.S." + hash + " : Prop := True", Battery: report(fail)}
}

func TestKey(t *testing.T) {
	thm := lean.Record{Decl: "a", Kind: "theorem", Type: "x", TypeHash: lean.Hash("x")}
	prop := lean.Record{Decl: "b", Kind: "def", Type: "s 0\n", TypeHash: lean.Hash("s 0\n"), Value: "x", ValueHash: lean.Hash("x")}
	def := lean.Record{Decl: "c", Kind: "def", Type: "t", Value: "v"}
	switch {
	case Key(thm) != Key(prop):
		t.Errorf("a theorem and a definition of the same proposition have the keys %s and %s", Key(thm), Key(prop))
	case Key(def) != lean.Hash("tv"):
		t.Errorf("the key of a definition is %s", Key(def))
	case Key(lean.Record{Decl: "d", Error: "unknown declaration"}) != "":
		t.Error("a declaration that Lean does not know has a key")
	}
}

func TestSelect(t *testing.T) {
	for _, c := range []struct {
		name   string
		cands  []Candidate
		winner int
		groups []int // the size of each group
		split  bool
	}{
		{"agreement", []Candidate{cand("a", "claude", 0), cand("a", "claude", 0)}, 0, []int{2}, false},
		{"largest group", []Candidate{cand("b", "claude", 0), cand("a", "claude", 0), cand("a", "gpt", 0)}, 1, []int{2, 1}, true},
		{"more families", []Candidate{cand("b", "claude", 0), cand("b", "claude", 0), cand("a", "claude", 0), cand("a", "gpt", 0)}, 2, []int{2, 2}, true},
		{"first candidate", []Candidate{cand("b", "claude", 0), cand("a", "claude", 0)}, 0, []int{1, 1}, true},
		{"failing candidates", []Candidate{cand("a", "claude", 6), cand("b", "claude", 0), cand("a", "claude", 4)}, 1, []int{1}, false},
		{"no statement", []Candidate{{Run: "r-1", Outcome: "failure", Reason: "stuck"}}, -1, nil, false},
	} {
		r := Select("7K3QX2", c.cands)
		var sizes []int
		for _, g := range r.Groups {
			sizes = append(sizes, len(g.Candidates))
		}
		if r.Winner != c.winner || !slices.Equal(sizes, c.groups) || r.Split() != c.split {
			t.Errorf("%s: the winner is %d and the groups are %v, split %v", c.name, r.Winner, sizes, r.Split())
		}
	}
}

func TestUnbound(t *testing.T) {
	parse := func(text string) *node.Node {
		tag := text[len("---\ntag: "):][:6]
		n, err := node.Parse("nodes/"+tag[:3]+"/"+tag+".md", []byte(text))
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	nodes := []*node.Node{
		parse("---\ntag: 7K3QX2\nkind: statement\nname: S\nuses: [1H8D2C, 6PQ0WZ]\n---\n"),
		parse("---\ntag: 1H8D2C\nkind: concept\nname: C\nuses: [2B4F6H]\n---\n"),
		parse("---\ntag: 6PQ0WZ\nkind: definition\nname: D\nformal:\n  - system: lean4\n    library: mathlib\n    decl: D\n    match: exact\n---\n"),
		parse("---\ntag: 2B4F6H\nkind: concept\nname: E\nuses: [7K3QX2]\n---\n"),
	}
	if got := Unbound(nodes, "7K3QX2"); !slices.Equal(got, []string{"1H8D2C", "2B4F6H"}) {
		t.Errorf("the unbound nodes are %v", got)
	}
}

func TestMarkdown(t *testing.T) {
	r := Select("7K3QX2", []Candidate{cand("a", "claude", 0), cand("a", "claude", 0), cand("b", "claude", 0), cand("c", "claude", 6)})
	r.Unbound = []string{"1H8D2C"}
	md := r.Markdown()
	for _, want := range []string{
		"The formalize role ran 4 times on 7K3QX2. 4 runs gave a statement, and 3 of these pass the battery.",
		"| c1 | r-aclaude | claude-model | Math2.S.a | a | pass | 1 |",
		"| c4 | r-cclaude | claude-model | Math2.S.c | c | fail | - |",
		"The binding of c1 to `Math2.S.a` goes in at F0.",
		"The candidates split into 2 groups.",
		"All candidates come from the model family claude.",
		"| 6 negation not provable | pass | - | - |",
		"c3, from claude-model: it is in group 2.",
		"c4, from claude-model: it fails the battery.",
		"have no Lean binding: 1H8D2C.",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("the report has no %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "c2, from") {
		t.Errorf("the report shows a candidate of the winning group as dissenting:\n%s", md)
	}
	none := Select("7K3QX2", nil).Markdown()
	if !strings.Contains(none, "proposes no binding") || !strings.Contains(none, "Each concept and definition that the node uses has a Lean binding.") {
		t.Errorf("the report with no candidate is:\n%s", none)
	}
}
