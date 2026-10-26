package agent_test

import (
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/align"
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
	concept := "---\ntag: ~1a2b3c4d\nkind: concept\nname: Coset\n---\n\n## Summary\n\nA coset of a subgroup.\n"
	queue := `{"id":"aaaaaaaaaaaa","node":"3C53TD","edge":"duplicate","system":"math2","target":"G5S1M9","source":"oeis","score":0.9,"centrality":3,"proposed":"2026-10-01","by":"m2/0.2.0 align","records":"-"}` + "\n"
	same := func(a, b, by string) string {
		p := align.NewDuplicate(a, b)
		p.By, p.Proposed, p.Why = by, "2026-10-26", "The two nodes state the same theorem."
		data, err := align.FormatQueue([]align.Proposal{p})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	edge := func(n, e, system, to, by string) string {
		p := align.NewProposal(n, e, system, to)
		p.Source, p.By, p.Proposed, p.Why = system, by, "2026-10-26", "The two state the same theorem."
		data, err := align.FormatQueue([]align.Proposal{p})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	wd := edge("3C53TD", "same_as", "wikidata", "Q1", "m2-agent/align@0.1.0")
	other := `{"id":"bbbbbbbbbbbb","node":"3C53TD","edge":"close_to","system":"wikidata","target":"Q2","source":"wikidata","score":0.8,"centrality":3,"proposed":"2026-10-01","by":"m2/0.2.0 align","records":"-"}` + "\n"
	mathlib := strings.Replace(target, "---\n\n", "  - system: lean4\n    library: mathlib\n    decl: Subgroup.card_dvd_of_le\n    match: exact\n---\n\n", 1)
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
		{"curate adds a statement", "curate", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(lemma)}, ""},
		{"curate adds a concept", "curate", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(concept)}, ""},
		{"curate adds an example", "curate", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(example)}, "of the kind example"},
		{"curate binds a new node", "curate", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(strings.Replace(lemma, "---\n\n", "formal: []\n---\n\n", 1))}, "no field formal"},
		{"curate aligns a new node", "curate", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(strings.Replace(lemma, "---\n\n", "same_as: {wikidata: Q2}\n---\n\n", 1))}, "no field same_as"},
		{"curate explains a new node", "curate", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(lemma + "\n## Explanations\n\n### L2 agent\n\nMore.\n")}, "writes no explanations"},
		{"curate renames", "curate", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "name: Lagrange's", "name: The", 1))}, ""},
		{"curate changes the msc", "curate", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "msc: [20D60]", "msc: [20D60, 20E07]\nuses: [G5S1M9]\naliases: [lagrange]", 1))}, ""},
		{"curate curates another node", "curate", workspace.Change{Path: "nodes/AA/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(target, "3C53TD\n", "AA0001\naliases: [lagrange]\n", 1))}, ""},
		{"curate changes the body", "curate", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "finite group divides", "group divides", 1))}, "does not change the body"},
		{"curate changes a binding", "curate", workspace.Change{Path: path, Old: []byte(target), New: []byte(rebind)}, "the field formal"},
		{"curate writes Lean", "curate", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", New: []byte("module\n")}, "writes no Lean"},
		{"curate proposes", "curate", workspace.Change{Path: "queues/dedup.jsonl", New: []byte(same("G5S1M9", "3C53TD", "m2-agent/curate@0.1.0"))}, ""},
		{"curate adds a proposal", "curate", workspace.Change{Path: "queues/dedup.jsonl", Old: []byte(queue), New: []byte(queue + same("AA0001", "3C53TD", "m2-agent/curate@0.1.0"))}, ""},
		{"curate removes a proposal", "curate", workspace.Change{Path: "queues/dedup.jsonl", Old: []byte(queue), New: []byte(same("AA0001", "3C53TD", "m2-agent/curate@0.1.0"))}, "does not change or remove the proposal aaaaaaaaaaaa"},
		{"curate proposes as align", "curate", workspace.Change{Path: "queues/dedup.jsonl", New: []byte(same("G5S1M9", "3C53TD", "m2/0.2.0 align"))}, "adds only its own proposals"},
		{"curate proposes a node with itself", "curate", workspace.Change{Path: "queues/dedup.jsonl", New: []byte(same("G5S1M9", "G5S1M9", "m2-agent/curate@0.1.0"))}, "does not name two nodes"},
		{"curate forges an id", "curate", workspace.Change{Path: "queues/dedup.jsonl", New: []byte(strings.Replace(same("G5S1M9", "3C53TD", "m2-agent/curate@0.1.0"), `"edge":"duplicate"`, `"edge":"same_as"`, 1))}, "is not a duplicate proposal"},
		{"curate writes the align queue", "curate", workspace.Change{Path: "queues/align/oeis.jsonl", New: []byte(queue)}, "does not write this file"},
		{"align proposes", "align", workspace.Change{Path: "queues/align/wikidata.jsonl", New: []byte(wd)}, ""},
		{"align adds a proposal", "align", workspace.Change{Path: "queues/align/wikidata.jsonl", Old: []byte(other), New: []byte(other + wd)}, ""},
		{"align changes its proposal", "align", workspace.Change{Path: "queues/align/wikidata.jsonl", Old: []byte(wd), New: []byte(strings.Replace(wd, "same theorem", "same result", 1))}, ""},
		{"align removes its proposal", "align", workspace.Change{Path: "queues/align/wikidata.jsonl", Old: []byte(other + wd), New: []byte(other)}, ""},
		{"align removes a proposal of the aligner", "align", workspace.Change{Path: "queues/align/wikidata.jsonl", Old: []byte(other + wd), New: []byte(wd)}, "changes or removes the proposal bbbbbbbbbbbb"},
		{"align proposes from another node", "align", workspace.Change{Path: "queues/align/wikidata.jsonl", New: []byte(edge("AA0001", "same_as", "wikidata", "Q1", "m2-agent/align@0.1.0"))}, "only from the target"},
		{"align proposes as the aligner", "align", workspace.Change{Path: "queues/align/wikidata.jsonl", New: []byte(edge("3C53TD", "same_as", "wikidata", "Q1", "m2/0.2.0 align"))}, "adds only its own proposals"},
		{"align proposes a duplicate", "align", workspace.Change{Path: "queues/align/wikidata.jsonl", New: []byte(edge("3C53TD", "duplicate", "wikidata", "Q1", "m2-agent/align@0.1.0"))}, "align proposes only"},
		{"align writes the wrong file", "align", workspace.Change{Path: "queues/align/oeis.jsonl", New: []byte(wd)}, "the queue file is of oeis"},
		{"align forges an id", "align", workspace.Change{Path: "queues/align/wikidata.jsonl", New: []byte(strings.Replace(wd, `"target":"Q1"`, `"target":"Q3"`, 1))}, "the wrong id"},
		{"align writes the dedup queue", "align", workspace.Change{Path: "queues/dedup.jsonl", New: []byte(same("G5S1M9", "3C53TD", "m2-agent/align@0.1.0"))}, "does not write this file"},
		{"align binds Mathlib", "align", workspace.Change{Path: path, Old: []byte(target), New: []byte(mathlib)}, ""},
		{"align binds Math2", "align", workspace.Change{Path: path, Old: []byte(target), New: []byte(bind)}, "library mathlib"},
		{"align reviews", "align", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(mathlib, "match: exact\n", "match: exact\n    fidelity_reviews: [r-3C53TD-01]\n", 1))}, "no fidelity reviews"},
		{"align rebinds", "align", workspace.Change{Path: path, Old: []byte(target), New: []byte(rebind)}, "does not change the bindings"},
		{"align writes same_as", "align", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "msc: [20D60]", "msc: [20D60]\nsame_as: {wikidata: Q1}", 1))}, "the field same_as"},
		{"align binds another node", "align", workspace.Change{Path: "nodes/AA/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(mathlib, "3C53TD", "AA0001", 1))}, "not the target"},
		{"align writes Lean", "align", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", New: []byte("module\n")}, "writes no Lean"},
		{"bump rebinds", "bump", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "Subgroup.card_subgroup_dvd_card", "Subgroup.card_dvd_card", 1))}, ""},
		{"bump changes the match", "bump", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "match: stronger", "match: exact", 1))}, "bump changes only formal[0].decl"},
		{"bump binds", "bump", workspace.Change{Path: path, Old: []byte(target), New: []byte(bind)}, "does not add or remove bindings"},
		{"bump changes the msc", "bump", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "msc: [20D60]", "msc: [20D60, 20E07]", 1))}, "the field msc"},
		{"bump rebinds another node", "bump", workspace.Change{Path: "nodes/AA/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(rebind, "3C53TD", "AA0001", 1))}, "not the target"},
		{"bump adds a node", "bump", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(lemma)}, "bump does not add nodes"},
		{"bump writes Lean", "bump", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", Old: []byte("a"), New: []byte("b")}, "writes no Lean"},
		{"bump writes the register", "bump", workspace.Change{Path: "registers/bindings.reg", Old: []byte("# tag\n"), New: []byte("# tag\n3C53TD lean4 a b rename c -\n")}, "does not write this file"},
		{"review-assist writes the packet", "review-assist", workspace.Change{Path: "reports/review/3C53TD.md", New: []byte("# Review packet of 3C53TD\n")}, ""},
		{"review-assist changes the packet", "review-assist", workspace.Change{Path: "reports/review/3C53TD.md", Old: []byte("a"), New: []byte("b")}, ""},
		{"review-assist writes another packet", "review-assist", workspace.Change{Path: "reports/review/AA0001.md", New: []byte("a")}, "only the review packet of the target"},
		{"review-assist writes a review", "review-assist", workspace.Change{Path: "reviews/3C/r-3C53TD-01.md", New: []byte("a")}, "only the review packet of the target"},
		{"review-assist changes the node", "review-assist", workspace.Change{Path: path, Old: []byte(target), New: []byte(rebind)}, "only the review packet of the target"},
		{"review-assist removes the packet", "review-assist", workspace.Change{Path: "reports/review/3C53TD.md", Old: []byte("a")}, "does not delete a file"},
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
	if err := agent.Permit("explore", "3C53TD", nil); err == nil {
		t.Error("explore has no permission set yet, and Permit gave no error")
	}
}
