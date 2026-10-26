package review_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/review"
	"github.com/tamnd/math2-agent/status"
)

const groupNode = `---
tag: 1A0000
kind: definition
name: Group
formal:
  - {system: lean4, library: mathlib, decl: Group, match: exact}
---

## Definition

A group is a set with an associative operation, an identity and inverses.
`

const lagrangeNode = `---
tag: 1B0000
kind: statement
name: Lagrange's theorem
uses: [1A0000]
formal:
  - system: lean4
    library: mathlib
    decl: Subgroup.card_subgroup_dvd_card
    match: stronger
    adapter: Math2.Match.lagrange
---

## Statement

For each finite group G and each subgroup H of G, the order of H divides the order of G.
`

// tree writes a tree with the two nodes and the registers, and returns its root.
func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"nodes/1A0/1A0000.md":     groupNode,
		"nodes/1B0/1B0000.md":     lagrangeNode,
		"registers/reviewers.reg": "# reviewer first areas affiliations\ngithub:reviewer 2026-01-01 20 -\n",
		"registers/bindings.reg":  "# tag system old new cause mathlib author\n1B0000 lean4 - Subgroup.card_subgroup_dvd_card bind abc github:author\n",
	}
	for name, text := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

var records = map[string]lean.Record{
	"Group": {Decl: "Group", Kind: "inductive", TypeHash: "blake3:group", StatementPP: "Type u → Type u"},
	"Math2.Match.lagrange": {
		Decl:                     "Math2.Match.lagrange",
		Kind:                     "theorem",
		TypeHash:                 "blake3:lagrange",
		StatementPP:              "∀ {G : Type u_1} [inst : Group G] [inst_1 : Finite G] (H : Subgroup G), Nat.card ↥H ∣ Nat.card G",
		StatementPPAll:           "∀ {G : Type u_1} [inst : Group.{u_1} G] [inst_1 : Finite.{u_1 + 1} G] (H : Subgroup.{u_1} G inst), @Dvd.dvd.{0} Nat Nat.instDvdNat (Nat.card.{u_1} (Subgroup.toSet H)) (Nat.card.{u_1} G)",
		UsesConstantsInStatement: []string{"Group", "Finite", "Subgroup", "Nat.card", "Dvd.dvd", "Nat.instDvdNat"},
	},
}

// input reads the tree in root and computes the status, as a build does.
func input(t *testing.T, root string) review.Input {
	t.Helper()
	nodes, err := node.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	reviews, err := status.LoadReviews(root)
	if err != nil {
		t.Fatal(err)
	}
	reviewers, err := status.LoadReviewers(root)
	if err != nil {
		t.Fatal(err)
	}
	authors, err := status.LoadAuthors(root)
	if err != nil {
		t.Fatal(err)
	}
	st := status.Compute(status.Input{Nodes: nodes, Records: records, Reviews: reviews, Reviewers: reviewers, Authors: authors})
	report := battery.Report{Tag: "1B0000", Decl: "Math2.Match.lagrange", Checks: make([]battery.Check, 10)}
	for i, name := range battery.Names() {
		report.Checks[i] = battery.Check{Number: i + 1, Name: name, Result: battery.Pass}
	}
	report.Checks[2] = battery.Check{Number: 3, Name: "constant audit", Result: battery.Open, Detail: "No node binds these constants: Finite, Subgroup, Nat.card."}
	report.Checks[8] = battery.Check{Number: 9, Name: "critic", Result: battery.Flag, Detail: "The statement uses Nat.card, which is 0 for an infinite group."}
	return review.Input{Nodes: nodes, Status: st, Records: records, Battery: []battery.Report{report}, Authors: authors}
}

func TestMake(t *testing.T) {
	root := tree(t)
	p, err := review.Make(input(t, root), "1B0000")
	if err != nil {
		t.Fatal(err)
	}
	if p.Section != "Statement" || !strings.HasPrefix(p.Statement, "For each finite group") || p.StatementHash == "" || p.Stale {
		t.Errorf("the packet is %+v", p)
	}
	b, err := p.Binding("")
	if err != nil {
		t.Fatal(err)
	}
	if b.Checked != "Math2.Match.lagrange" || b.TypeHash != "blake3:lagrange" || b.Author != "github:author" || b.Battery == nil || len(b.Constants) != 6 {
		t.Fatalf("the binding is %+v", b)
	}
	if c := b.Constants[0]; c.Name != "Group" || c.Type != "Type u → Type u" || len(c.Nodes) != 1 || c.Nodes[0].Tag != "1A0000" || c.Nodes[0].Fidelity != status.F0 {
		t.Errorf("the constant Group is %+v", c)
	}
	if c := b.Constants[1]; c.Name != "Finite" || len(c.Nodes) != 0 || c.Structural || !b.Constants[4].Structural {
		t.Errorf("the constant Finite is %+v", c)
	}
	if _, err := p.Binding("Math2.Match.lagrange"); err != nil {
		t.Error(err)
	}
	if _, err := p.Binding("Other"); err == nil {
		t.Error("Binding finds a declaration that the node does not bind")
	}
	if _, err := review.Make(input(t, root), "1C0000"); err == nil {
		t.Error("Make finds a node that is not in the tree")
	}

	var buf bytes.Buffer
	if err := p.WriteText(&buf, 100); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	for _, want := range []string{
		"1B0000 statement: Lagrange's theorem",
		"you review      Math2.Match.lagrange",
		"author          github:author",
		"Statement                                        | Lean\n",
		"For each finite group G and each subgroup H of   | ∀ {G : Type u_1} [inst : Group G] [inst_1 :\n",
		"                                                 | Nat.card G\n",
		"binding         Subgroup.card_subgroup_dvd_card\nmatch           stronger\nadapter         Math2.Match.lagrange\n",
		"Lean without notation\n  ∀ {G : Type u_1} [inst : Group.{u_1} G]",
		"  Group: bound to 1A0000 Group (F0)\n      Type u → Type u\n",
		"  Finite: no node binds it\n",
		"  Dvd.dvd: logic or notation\n  Nat.instDvdNat: logic or notation\n",
		"Battery         F1 no\n",
		"   3 constant audit                 open\n      No node binds these constants: Finite, Subgroup, Nat.card.\n",
		"   9 critic                         flag\n      The statement uses Nat.card",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the text has no %q:\n%s", want, text)
		}
	}
	for l := range strings.Lines(text) {
		if n := len([]rune(strings.TrimRight(l, "\n"))); n > 100 {
			t.Errorf("the line is %d runes wide: %q", n, l)
		}
	}
}

func TestWrite(t *testing.T) {
	root := tree(t)
	p, err := review.Make(input(t, root), "1B0000")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := p.Binding("")
	r, paths, err := review.Write(root, p, b, status.Review{Reviewer: "github:reviewer", Date: "2026-11-01", Verdict: status.FaithfulWithConventions, Conventions: []string{"the order of a finite group is Nat.card"}, Notes: " The adapter reads the theorem for a finite group. "})
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "r-1B0000-01" || r.Decl != "Math2.Match.lagrange" || r.StatementHash != p.StatementHash || r.TypeHash != "blake3:lagrange" || r.Notes != "The adapter reads the theorem for a finite group." {
		t.Errorf("the review is %+v", r)
	}
	if len(paths) != 2 || paths[0] != "reviews/1B0/1B0000.yaml" || paths[1] != "nodes/1B0/1B0000.md" {
		t.Errorf("Write changed %v", paths)
	}
	data, err := os.ReadFile(filepath.Join(root, "nodes", "1B0", "1B0000.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "    fidelity_reviews: [r-1B0000-01]\n") {
		t.Errorf("the node file has no review id:\n%s", data)
	}

	// The review counts, and the binding is at F2.
	p, err = review.Make(input(t, root), "1B0000")
	if err != nil {
		t.Fatal(err)
	}
	if b := p.Bindings[0]; b.Fidelity != status.F2 || len(b.Reviews) != 1 || b.Reviews[0].State != status.Counts {
		t.Errorf("after the review, the binding is %+v", b)
	}

	// A second review gets the next id, and the file keeps the first review as it was.
	before, err := os.ReadFile(filepath.Join(root, "reviews", "1B0", "1B0000.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	r, _, err = review.Write(root, p, p.Bindings[0], status.Review{Reviewer: "orcid:0000-0002-1825-0097", Date: "2026-11-02", Verdict: status.NotFaithful, Class: battery.Junk, Notes: "Nat.card is 0 for an infinite group."})
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(root, "reviews", "1B0", "1B0000.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "r-1B0000-02" || !bytes.HasPrefix(after, before) {
		t.Errorf("the second review is %s, and the file is:\n%s", r.ID, after)
	}
	reviews, err := status.LoadReviews(root)
	if err != nil || len(reviews) != 2 || reviews["r-1B0000-02"].Class != battery.Junk {
		t.Errorf("the tree has the reviews %+v, %v", reviews, err)
	}
	data, _ = os.ReadFile(filepath.Join(root, "nodes", "1B0", "1B0000.md"))
	if !strings.Contains(string(data), "fidelity_reviews: [r-1B0000-01, r-1B0000-02]") {
		t.Errorf("the node file does not have the two ids:\n%s", data)
	}
}

func TestFill(t *testing.T) {
	root := tree(t)
	p, err := review.Make(input(t, root), "1B0000")
	if err != nil {
		t.Fatal(err)
	}
	b := p.Bindings[0]
	ok := status.Review{Reviewer: "github:reviewer", Verdict: status.Faithful}
	if r, err := review.Fill(p, b, ok); err != nil || len(r.Date) != 10 {
		t.Errorf("Fill gives %+v, %v", r, err)
	}
	tests := []struct {
		name string
		edit func(*status.Review)
		want string
	}{
		{"agent", func(r *status.Review) { r.Reviewer = "agent:m2-agent/review@1.0.0" }, "not a person"},
		{"bot", func(r *status.Review) { r.Reviewer = "github:dependabot[bot]" }, "not a person"},
		{"author", func(r *status.Review) { r.Reviewer = "github:author" }, "the author of a binding cannot review it"},
		{"bad date", func(r *status.Review) { r.Date = "1 November" }, "is not YYYY-MM-DD"},
		{"no verdict", func(r *status.Review) { r.Verdict = "" }, "is not faithful"},
		{"conventions without the verdict", func(r *status.Review) { r.Conventions = []string{"x"} }, "has no conventions"},
		{"no conventions", func(r *status.Review) { r.Verdict = status.FaithfulWithConventions }, "needs the conventions"},
		{"no class", func(r *status.Review) { r.Verdict = status.NotFaithful }, "needs one class"},
		{"unknown class", func(r *status.Review) { r.Verdict, r.Class = status.NotFaithful, "M-other" }, "needs one class"},
	}
	for _, tt := range tests {
		r := ok
		tt.edit(&r)
		if _, err := review.Fill(p, b, r); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: Fill gives %v, and the error must have %q", tt.name, err, tt.want)
		}
	}
	stale := p
	stale.Stale = true
	if _, err := review.Fill(stale, b, ok); err == nil || !strings.Contains(err.Error(), "Run m2 build") {
		t.Errorf("a stale packet gives %v", err)
	}
	b.TypeHash = ""
	if _, err := review.Fill(p, b, ok); err == nil {
		t.Error("a binding without a type hash gives a review")
	}
}

func TestPerson(t *testing.T) {
	for id, want := range map[string]bool{
		"github:octocat":            true,
		"github:a-b":                true,
		"orcid:0000-0002-1825-0097": true,
		"orcid:0000-0002-1694-233X": true,
		"github:-x":                 false,
		"github:a--b":               false,
		"github:renovate[bot]":      false,
		"orcid:1234":                false,
		"m2-agent/review@1.0.0":     false,
		"":                          false,
	} {
		if got := review.Person(id); got != want {
			t.Errorf("Person(%q) = %v", id, got)
		}
	}
}
