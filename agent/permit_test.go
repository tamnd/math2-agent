package agent_test

import (
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/workspace"
)

const target = `---
tag: 3C53TD
kind: statement
role: theorem
name: Lagrange's theorem
msc: [20D60]
formal:
  - system: lean4
    library: mathlib
    decl: Subgroup.card_subgroup_dvd_card
    match: stronger
---

## Statement

The order of a subgroup of a finite group divides the order of the group.
`

const root = "module\n\npublic import Math2.Cert.LRAT\npublic import Math2.Meta.Attr\n\n/-! The root module. -/\n"

func TestPermit(t *testing.T) {
	path := "nodes/3C/3C53TD.md"
	bind := strings.Replace(target, "---\n\n", "  - system: lean4\n    library: math2\n    decl: Math2.Algebra.lagrange\n    match: exact\n---\n\n", 1)
	rebind := strings.Replace(target, "decl: Subgroup.card_subgroup_dvd_card", "decl: Math2.Algebra.lagrange", 1)
	reviewed := strings.Replace(bind, "match: exact\n", "match: exact\n    fidelity_reviews: [r-3C53TD-01]\n", 1)
	lemma := "---\ntag: ~1a2b3c4d\nkind: statement\nrole: lemma\nname: A lemma\n---\n\n## Statement\n\nA lemma.\n"
	example := strings.Replace(lemma, "kind: statement", "kind: example", 1)
	explained := target + "\n## Explanations\n\n### L2 agent\n\nThe order of a [[subgroup|G5S1M9]] is a whole number that divides the order of the group.\n"
	human := target + "\n## Explanations\n\n### L2 human\n\nThe cosets of a subgroup have the same size.\n"
	imported := strings.Replace(root, "public import Math2.Meta.Attr\n", "public import Math2.Meta.Attr\npublic import Math2.Algebra.Lagrange\n", 1)
	for _, tc := range []struct {
		name, role string
		c          workspace.Change
		err        string
	}{
		{"formalize binds", "formalize", workspace.Change{Path: path, Old: []byte(target), New: []byte(bind)}, ""},
		{"formalize adds a statement", "formalize", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", New: []byte("module\n")}, ""},
		{"formalize adds a conjecture", "formalize", workspace.Change{Path: "lean/Math2/Conjectures/Collatz.lean", New: []byte("module\n")}, ""},
		{"formalize imports", "formalize", workspace.Change{Path: "lean/Math2.lean", Old: []byte(root), New: []byte(imported)}, ""},
		{"formalize changes Lean", "formalize", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", Old: []byte("a"), New: []byte("b")}, "does not change them"},
		{"formalize rebinds", "formalize", workspace.Change{Path: path, Old: []byte(target), New: []byte(rebind)}, "appends bindings"},
		{"formalize reviews", "formalize", workspace.Change{Path: path, Old: []byte(target), New: []byte(reviewed)}, "no fidelity reviews"},
		{"formalize adds a node", "formalize", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(lemma)}, "does not add nodes"},
		{"prove proves", "prove", workspace.Change{Path: path, Old: []byte(target), New: []byte(rebind)}, ""},
		{"prove adds a lemma", "prove", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(lemma)}, ""},
		{"prove changes a proof", "prove", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", Old: []byte("a"), New: []byte("b")}, ""},
		{"prove adds an example", "prove", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(example)}, "is a example"},
		{"prove binds", "prove", workspace.Change{Path: path, Old: []byte(target), New: []byte(bind)}, "does not add or remove"},
		{"prove changes the match", "prove", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "match: stronger", "match: exact", 1))}, "only formal[0].decl"},
		{"prove changes the name", "prove", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "name: Lagrange's", "name: The", 1))}, "the field name"},
		{"prove changes the body", "prove", workspace.Change{Path: path, Old: []byte(target), New: []byte(target + "\nMore.\n")}, "the body"},
		{"prove changes a conjecture", "prove", workspace.Change{Path: "lean/Math2/Conjectures/Collatz.lean", Old: []byte("a"), New: []byte("b")}, "conjectures"},
		{"another node", "prove", workspace.Change{Path: "nodes/AA/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(rebind, "3C53TD", "AA0001", 1))}, "not the target"},
		{"an axiom", "prove", workspace.Change{Path: "lean/Math2/Hypotheses/GRH.lean", New: []byte("axiom grh : True\n")}, "no role writes"},
		{"the attribute", "formalize", workspace.Change{Path: "lean/Math2/Meta/Attr.lean", Old: []byte("a"), New: []byte("b")}, "no role writes"},
		{"a delete", "prove", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", Old: []byte("a")}, "does not delete"},
		{"a review", "prove", workspace.Change{Path: "reviews/3C/r-3C53TD-01.md", New: []byte("x")}, "does not write this file"},
		{"a register", "formalize", workspace.Change{Path: "registers/bindings.reg", Old: []byte("a"), New: []byte("b")}, "does not write this file"},
		{"the lakefile", "prove", workspace.Change{Path: "lean/lakefile.toml", Old: []byte("a"), New: []byte("b")}, "does not write this file"},
		{"a removed import", "prove", workspace.Change{Path: "lean/Math2.lean", Old: []byte(root), New: []byte(strings.Replace(root, "public import Math2.Cert.LRAT\n", "", 1))}, "does more than add imports"},
		{"an import of Mathlib", "prove", workspace.Change{Path: "lean/Math2.lean", Old: []byte(root), New: []byte(root + "public import Mathlib\n")}, "does more than add imports"},
		{"a removed last line", "prove", workspace.Change{Path: "lean/Math2.lean", Old: []byte(root), New: []byte(strings.TrimSuffix(root, "/-! The root module. -/\n"))}, "removes lines"},
		{"explain explains", "explain", workspace.Change{Path: path, Old: []byte(target), New: []byte(explained)}, ""},
		{"explain changes its explanation", "explain", workspace.Change{Path: path, Old: []byte(explained), New: []byte(strings.Replace(explained, "a whole number", "an integer", 1))}, ""},
		{"explain removes its explanation", "explain", workspace.Change{Path: path, Old: []byte(explained), New: []byte(target)}, ""},
		{"explain adds a second explanation", "explain", workspace.Change{Path: path, Old: []byte(human), New: []byte(human + "\n### L1 agent\n\nA group splits into cosets of the same size.\n")}, ""},
		{"explain changes a human explanation", "explain", workspace.Change{Path: path, Old: []byte(human), New: []byte(strings.Replace(human, "same size", "equal size", 1))}, "not marked agent"},
		{"explain marks itself human", "explain", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(explained, "L2 agent", "L2 human", 1))}, "is not marked"},
		{"explain adds a human heading", "explain", workspace.Change{Path: path, Old: []byte(explained), New: []byte(explained + "\n### L2 human\n\nMore.\n")}, "is not marked"},
		{"explain changes the statement", "explain", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(explained, "finite group divides", "group divides", 1))}, "the field body.statement"},
		{"explain changes the front matter", "explain", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(explained, "msc: [20D60]", "msc: [20D99]", 1))}, "the field msc"},
		{"explain writes text before the explanations", "explain", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(explained, "## Explanations\n", "## Explanations\n\nSome text.\n", 1))}, "the field body.explanations"},
		{"explain adds an example", "explain", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(example)}, ""},
		{"explain adds a lemma", "explain", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(lemma)}, "of the kind statement"},
		{"explain binds an example", "explain", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(strings.Replace(example, "---\n\n", "formal: []\n---\n\n", 1))}, "no formal field"},
		{"explain explains another node", "explain", workspace.Change{Path: "nodes/AA/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(explained, "3C53TD", "AA0001", 1))}, "not the target"},
		{"explain writes Lean", "explain", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", New: []byte("module\n")}, "writes no Lean"},
		{"explain imports", "explain", workspace.Change{Path: "lean/Math2.lean", Old: []byte(root), New: []byte(imported)}, "writes no Lean"},
		{"an option", "prove", workspace.Change{Path: "lean/Math2.lean", Old: []byte(root), New: []byte(root + "set_option maxHeartbeats 0\n")}, "does more than add imports"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := agent.Permit(tc.role, "3C53TD", []workspace.Change{tc.c})
			switch {
			case tc.err == "" && err != nil:
				t.Errorf("error %v", err)
			case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
				t.Errorf("error %v, want %q", err, tc.err)
			}
		})
	}
	if err := agent.Permit("curate", "3C53TD", nil); err == nil {
		t.Error("curate has no permission set yet, and Permit gave no error")
	}
}
