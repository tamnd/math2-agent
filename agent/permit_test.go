package agent_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/agent"
	"github.com/tamnd/math2-agent/align"
	"github.com/tamnd/math2-agent/inventory"
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
	path := "nodes/3C5/3C53TD.md"
	bind := strings.Replace(target, "---\n\n", "  - system: lean4\n    library: math2\n    decl: Math2.Algebra.lagrange\n    match: exact\n---\n\n", 1)
	rebind := strings.Replace(target, "decl: Subgroup.card_subgroup_dvd_card", "decl: Math2.Algebra.lagrange", 1)
	reviewed := strings.Replace(bind, "match: exact\n", "match: exact\n    fidelity_reviews: [r-3C53TD-01]\n", 1)
	lemma := "---\ntag: ~1a2b3c4d\nkind: statement\nrole: lemma\nname: A lemma\n---\n\n## Statement\n\nA lemma.\n"
	example := strings.Replace(lemma, "kind: statement", "kind: example", 1)
	explained := target + "\n## Explanations\n\n### L2 agent\n\nThe order of a [[subgroup|G5S1M9]] is a whole number that divides the order of the group.\n"
	human := target + "\n## Explanations\n\n### L2 human\n\nThe cosets of a subgroup have the same size.\n"
	conjecture := "---\ntag: ~1a2b3c4d\nkind: conjecture\nname: A pattern\n---\n\n## Statement\n\nA pattern.\n"
	computation := "---\ntag: ~1a2b3c4d\nkind: computation\nname: A search\nclaim: x\nengine: {image: m2-python@sha256:00, entry: compute/~/~1a2b3c4d/main.py}\n---\n\n## Description\n\nA search.\n"
	proof := "---\ntag: ~1a2b3c4d\nkind: proof\nname: Proof of Lagrange's theorem\nproves: 3C53TD\n---\n\n## Proof\n\n1. The cosets of $H$ partition $G$.\n"
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
	mention := func(source, native, by string) string {
		m := inventory.Mention{ID: inventory.MentionID(source, native, 1), Source: source, Snapshot: "-", Kind: "statement", Name: "Heine-Borel theorem", Class: "open", By: by}
		data, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return string(data) + "\n"
	}
	enumerated := "agent:m2-agent/enumerate@0.1.0"
	first := mention("model-claude", "54D30.heine-borel-theorem", enumerated)
	shard := inventory.MentionShard(inventory.MentionID("model-claude", "54D30.heine-borel-theorem", 1))
	mentions := inventory.MentionsDir + "/model-claude/" + shard + ".jsonl"
	// second is a mention of the same shard, whose id sorts after the id of first.
	var second string
	for i := 0; second == ""; i++ {
		native := fmt.Sprintf("54D30.z%d", i)
		if inventory.MentionShard(inventory.MentionID("model-claude", native, 1)) == shard {
			second = mention("model-claude", native, enumerated)
		}
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
		{"another node", "prove", workspace.Change{Path: "nodes/AA0/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(rebind, "3C53TD", "AA0001", 1))}, "not the target"},
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
		{"explain explains another node", "explain", workspace.Change{Path: "nodes/AA0/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(explained, "3C53TD", "AA0001", 1))}, "not the target"},
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
		{"curate curates another node", "curate", workspace.Change{Path: "nodes/AA0/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(target, "3C53TD\n", "AA0001\naliases: [lagrange]\n", 1))}, ""},
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
		{"align binds another node", "align", workspace.Change{Path: "nodes/AA0/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(mathlib, "3C53TD", "AA0001", 1))}, "not the target"},
		{"align writes Lean", "align", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", New: []byte("module\n")}, "writes no Lean"},
		{"bump rebinds", "bump", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "Subgroup.card_subgroup_dvd_card", "Subgroup.card_dvd_card", 1))}, ""},
		{"bump changes the match", "bump", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "match: stronger", "match: exact", 1))}, "bump changes only formal[0].decl"},
		{"bump binds", "bump", workspace.Change{Path: path, Old: []byte(target), New: []byte(bind)}, "does not add or remove bindings"},
		{"bump changes the msc", "bump", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "msc: [20D60]", "msc: [20D60, 20E07]", 1))}, "the field msc"},
		{"bump rebinds another node", "bump", workspace.Change{Path: "nodes/AA0/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(rebind, "3C53TD", "AA0001", 1))}, "not the target"},
		{"bump adds a node", "bump", workspace.Change{Path: "nodes/~1/~1a2b3c4d.md", New: []byte(lemma)}, "bump does not add nodes"},
		{"bump writes Lean", "bump", workspace.Change{Path: "lean/Math2/Algebra/Lagrange.lean", Old: []byte("a"), New: []byte("b")}, "writes no Lean"},
		{"bump writes the register", "bump", workspace.Change{Path: "registers/bindings.reg", Old: []byte("# tag\n"), New: []byte("# tag\n3C53TD lean4 a b rename c -\n")}, "does not write this file"},
		{"review-assist writes the packet", "review-assist", workspace.Change{Path: "reports/review/3C53TD.md", New: []byte("# Review packet of 3C53TD\n")}, ""},
		{"review-assist changes the packet", "review-assist", workspace.Change{Path: "reports/review/3C53TD.md", Old: []byte("a"), New: []byte("b")}, ""},
		{"review-assist writes another packet", "review-assist", workspace.Change{Path: "reports/review/AA0001.md", New: []byte("a")}, "only the review packet of the target"},
		{"review-assist writes a review", "review-assist", workspace.Change{Path: "reviews/3C/r-3C53TD-01.md", New: []byte("a")}, "only the review packet of the target"},
		{"review-assist changes the node", "review-assist", workspace.Change{Path: path, Old: []byte(target), New: []byte(rebind)}, "only the review packet of the target"},
		{"review-assist removes the packet", "review-assist", workspace.Change{Path: "reports/review/3C53TD.md", Old: []byte("a")}, "does not delete a file"},
		{"explore adds a conjecture", "explore", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(conjecture)}, ""},
		{"explore adds a computation", "explore", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(computation)}, ""},
		{"explore adds a counterexample", "explore", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(strings.Replace(conjecture, "kind: conjecture", "kind: counterexample", 1))}, ""},
		{"explore adds code", "explore", workspace.Change{Path: "compute/~/~1a2b3c4d/main.py", New: []byte("print(1)\n")}, ""},
		{"explore changes code", "explore", workspace.Change{Path: "compute/AAY/AAYEQE/main.g", Old: []byte("a"), New: []byte("b")}, "does not change code that exists"},
		{"explore adds code outside a computation", "explore", workspace.Change{Path: "compute/main.py", New: []byte("a")}, "is in compute/SHARD/ID/"},
		{"explore adds code in the wrong shard", "explore", workspace.Change{Path: "compute/AA/AAYEQE/main.g", New: []byte("a")}, "is in compute/SHARD/ID/"},
		{"explore adds code with no shard", "explore", workspace.Change{Path: "compute/~1a2b3c4d/main.py", New: []byte("a")}, "is in compute/SHARD/ID/"},
		{"explore adds a statement", "explore", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(lemma)}, "of the kind statement"},
		{"explore binds a conjecture", "explore", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(strings.Replace(conjecture, "---\n\n", "formal:\n  - {system: lean4, library: math2, decl: Math2.c, match: exact}\n---\n\n", 1))}, "no field formal"},
		{"explore explains", "explore", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(conjecture + "\n## Explanations\n\n### L2 agent\n\nA text.\n")}, "writes no explanations"},
		{"explore changes a node", "explore", workspace.Change{Path: path, Old: []byte(target), New: []byte(rebind)}, "changes no node that exists"},
		{"explore writes Lean", "explore", workspace.Change{Path: "lean/Math2/Conjectures/A.lean", New: []byte("a")}, "writes no Lean"},
		{"enumerate adds a mention", "enumerate", workspace.Change{Path: mentions, New: []byte(first)}, ""},
		{"enumerate adds a second mention", "enumerate", workspace.Change{Path: mentions, Old: []byte(first), New: []byte(first + second)}, ""},
		{"enumerate changes a mention", "enumerate", workspace.Change{Path: mentions, Old: []byte(first), New: []byte(strings.Replace(first, "Heine-Borel", "Heine Borel", 1))}, "does not change or remove"},
		{"enumerate removes a mention", "enumerate", workspace.Change{Path: mentions, Old: []byte(first + second), New: []byte(second)}, "does not change or remove"},
		{"enumerate does not sort", "enumerate", workspace.Change{Path: mentions, New: []byte(second + first)}, "not sorted"},
		{"enumerate adds a mention of an importer", "enumerate", workspace.Change{Path: mentions, New: []byte(mention("model-claude", "54D30.heine-borel-theorem", "m2/0.2.0 import"))}, "adds only its own mentions"},
		{"enumerate adds a mention of another source", "enumerate", workspace.Change{Path: inventory.MentionsDir + "/model-gemma/" + shard + ".jsonl", New: []byte(first)}, "the file is of model-gemma"},
		{"enumerate adds a mention to the wrong shard", "enumerate", workspace.Change{Path: inventory.MentionsDir + "/model-claude/000.jsonl", New: []byte(first)}, "is of the shard"},
		{"enumerate writes a source", "enumerate", workspace.Change{Path: inventory.MentionsDir + "/wikidata/" + shard + ".jsonl", New: []byte(first)}, "only the mentions of a model list"},
		{"enumerate adds a field", "enumerate", workspace.Change{Path: mentions, New: []byte(strings.Replace(first, `"by"`, `"score":1,"by"`, 1))}, "unknown field"},
		{"enumerate adds a node", "enumerate", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(lemma)}, "only the mentions of a model list"},
		{"enumerate writes Lean", "enumerate", workspace.Change{Path: "lean/Math2/A.lean", New: []byte("a")}, "only the mentions of a model list"},
		{"write adds a section", "write", workspace.Change{Path: path, Old: []byte(target), New: []byte(target + "\n## Remarks\n\nThe converse is false.\n")}, ""},
		{"write changes the statement", "write", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "finite group", "finite group $G$", 1))}, ""},
		{"write adds a use", "write", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "msc: [20D60]", "msc: [20D60]\nuses: [G5S1M9]", 1))}, ""},
		{"write removes a use", "write", workspace.Change{Path: path, Old: []byte(strings.Replace(target, "msc: [20D60]", "msc: [20D60]\nuses: [G5S1M9]", 1)), New: []byte(target)}, "removes G5S1M9"},
		{"write changes the name", "write", workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "name: Lagrange's", "name: The", 1))}, "the field name"},
		{"write rebinds", "write", workspace.Change{Path: path, Old: []byte(target), New: []byte(rebind)}, "the field formal"},
		{"write explains", "write", workspace.Change{Path: path, Old: []byte(target), New: []byte(explained)}, "writes no explanations"},
		{"write adds an example", "write", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(strings.Replace(example, "---\n\n", "illustrates: [3C53TD]\n---\n\n", 1))}, ""},
		{"write adds a non-example", "write", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(strings.Replace(example, "---\n\n", "not_instance_of:\n  - target: 3C53TD\n---\n\n", 1))}, ""},
		{"write adds an example of another node", "write", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(strings.Replace(example, "---\n\n", "instance_of: [G5S1M9]\n---\n\n", 1))}, "illustration of the target 3C53TD"},
		{"write adds a proof", "write", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(proof)}, ""},
		{"write proves another node", "write", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(strings.Replace(proof, "proves: 3C53TD", "proves: G5S1M9", 1))}, "proves the target"},
		{"write binds a proof", "write", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(strings.Replace(proof, "---\n\n", "formal: []\n---\n\n", 1))}, "no formal field"},
		{"write explains a proof", "write", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(proof + "\n## Explanations\n\n### L2 agent\n\nA text.\n")}, "writes no explanations"},
		{"write adds a lemma", "write", workspace.Change{Path: "nodes/~/~1a2b3c4d.md", New: []byte(lemma)}, "of the kind statement"},
		{"write writes another node", "write", workspace.Change{Path: "nodes/AA0/AA0001.md", Old: []byte(strings.Replace(target, "3C53TD", "AA0001", 1)), New: []byte(strings.Replace(target, "3C53TD", "AA0001", 1) + "\nMore.\n")}, "not the target"},
		{"write writes Lean", "write", workspace.Change{Path: "lean/Math2/A.lean", New: []byte("a")}, "writes no Lean"},
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
	if err := agent.Permit("planner", "3C53TD", nil); err == nil {
		t.Error("the role planner has no permission set, and Permit gave no error")
	}
}

func TestPermitInformalize(t *testing.T) {
	const decl = "mathlib:Subgroup.card_subgroup_dvd_card"
	path := "nodes/3C5/3C53TD.md"
	added := strings.Replace(strings.Replace(target, "tag: 3C53TD", "tag: ~1a2b3c4d", 1), "match: stronger", "match: exact", 1)
	newPath := "nodes/~/~1a2b3c4d.md"
	for _, tc := range []struct {
		name, target string
		c            workspace.Change
		err          string
	}{
		{"a new statement", decl, workspace.Change{Path: newPath, New: []byte(added)}, ""},
		{"a new definition", "mathlib:IsCyclic", workspace.Change{Path: newPath, New: []byte(strings.NewReplacer("kind: statement\nrole: theorem", "kind: definition", "Subgroup.card_subgroup_dvd_card", "IsCyclic", "## Statement", "## Definition").Replace(added))}, ""},
		{"a new concept", decl, workspace.Change{Path: newPath, New: []byte("---\ntag: ~1a2b3c4d\nkind: concept\nname: Cyclic group\n---\n\n## Summary\n\nA group that one element generates.\n")}, ""},
		{"a bound concept", decl, workspace.Change{Path: newPath, New: []byte(strings.Replace(added, "kind: statement\nrole: theorem", "kind: concept", 1))}, "a new concept of informalize has no field formal"},
		{"a new example", decl, workspace.Change{Path: newPath, New: []byte(strings.Replace(added, "kind: statement\nrole: theorem", "kind: example", 1))}, "of the kind example"},
		{"another declaration", decl, workspace.Change{Path: newPath, New: []byte(strings.Replace(added, "card_subgroup_dvd_card", "card_dvd", 1))}, "has one binding"},
		{"a binding that is not exact", decl, workspace.Change{Path: newPath, New: []byte(target)}, "has one binding"},
		{"two bindings", decl, workspace.Change{Path: newPath, New: []byte(strings.Replace(added, "match: exact\n", "match: exact\n  - system: lean4\n    library: math2\n    decl: Math2.A\n    match: exact\n", 1))}, "has one binding"},
		{"a reviewed binding", decl, workspace.Change{Path: newPath, New: []byte(strings.Replace(added, "match: exact\n", "match: exact\n    fidelity_reviews: [r-3C53TD-01]\n", 1))}, "has one binding"},
		{"a crosswalk edge", decl, workspace.Change{Path: newPath, New: []byte(strings.Replace(added, "---\n\n", "same_as: {wikidata: Q190546}\n---\n\n", 1))}, "no field same_as"},
		{"an explanation", decl, workspace.Change{Path: newPath, New: []byte(added + "\n## Explanations\n\n### L2 agent\n\nA text.\n")}, "writes no explanations"},
		{"a section of the bound node", decl, workspace.Change{Path: path, Old: []byte(target), New: []byte(target + "\n## Remarks\n\nThe converse is false.\n")}, ""},
		{"the name of the bound node", decl, workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "name: Lagrange's theorem", "name: Theorem of Lagrange", 1))}, ""},
		{"the msc of the bound node", decl, workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "[20D60]", "[20D99]", 1))}, "changes the field msc"},
		{"the binding of the bound node", decl, workspace.Change{Path: path, Old: []byte(target), New: []byte(strings.Replace(target, "match: stronger", "match: exact", 1))}, "changes the field formal"},
		{"a node that does not bind it", "mathlib:IsCyclic", workspace.Change{Path: path, Old: []byte(target), New: []byte(target + "\nMore.\n")}, "does not bind it"},
		{"a target that is a tag", "3C53TD", workspace.Change{Path: newPath, New: []byte(added)}, "not in the form mathlib:DECL"},
		{"Lean", decl, workspace.Change{Path: "lean/Math2/A.lean", New: []byte("a")}, "writes no Lean"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := agent.Permit("informalize", tc.target, []workspace.Change{tc.c})
			switch {
			case tc.err == "" && err != nil:
				t.Errorf("error %v", err)
			case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
				t.Errorf("error %v, want %q", err, tc.err)
			}
		})
	}
}

func TestPermitExercise(t *testing.T) {
	const path = "nodes/~/~1a2b3c4d.md"
	ex := "---\ntag: ~1a2b3c4d\nkind: example\nname: A large subgroup\nuses: [3C53TD]\n---\n\n## Example\n\nWhat is the order of $H$?\n\n## Remarks\n\nRole: exercise. About: 3C53TD. Difficulty: 2. Group: B. Check: exact.\n\n### Answer\n\n12\n"
	ref := `{"exercise":"~1a2b3c4d","about":["3C53TD"],"check":"exact","reason":"r","by":"m2-agent/exercise@0.1.0"}` + "\n"
	for _, tc := range []struct {
		name string
		c    workspace.Change
		err  string
	}{
		{"a new exercise", workspace.Change{Path: path, New: []byte(ex)}, ""},
		{"an example", workspace.Change{Path: path, New: []byte(strings.Replace(ex, "Role: exercise. ", "", 1))}, "writes only exercises"},
		{"no target in uses", workspace.Change{Path: path, New: []byte(strings.Replace(ex, "uses: [3C53TD]\n", "uses: [G5S1M9]\n", 1))}, "has it in its uses"},
		{"another field", workspace.Change{Path: path, New: []byte(strings.Replace(ex, "uses:", "illustrates: [3C53TD]\nuses:", 1))}, "it has illustrates"},
		{"an explanation", workspace.Change{Path: path, New: []byte(ex + "\n## Explanations\n\n### L2 agent\n\nA text.\n")}, "writes no explanations"},
		{"a section of an exercise", workspace.Change{Path: path, Old: []byte(ex), New: []byte(strings.Replace(ex, "12\n", "12, the order of $G$\n", 1))}, ""},
		{"the uses of an exercise", workspace.Change{Path: path, Old: []byte(ex), New: []byte(strings.Replace(ex, "[3C53TD]", "[3C53TD, G5S1M9]", 1))}, ""},
		{"the msc of an exercise", workspace.Change{Path: path, Old: []byte(ex), New: []byte(strings.Replace(ex, "uses:", "msc: [20D60]\nuses:", 1))}, "changes the field msc"},
		{"the target", workspace.Change{Path: "nodes/3C5/3C53TD.md", Old: []byte(target), New: []byte(target + "\nMore.\n")}, "writes only exercises"},
		{"a referral", workspace.Change{Path: agent.ExerciseQueue, New: []byte(ref)}, ""},
		{"a second referral", workspace.Change{Path: agent.ExerciseQueue, Old: []byte(ref), New: []byte(ref + ref)}, ""},
		{"a changed referral", workspace.Change{Path: agent.ExerciseQueue, Old: []byte(ref), New: []byte(strings.Replace(ref, `"r"`, `"s"`, 1))}, "changes a line"},
		{"a referral of another role", workspace.Change{Path: agent.ExerciseQueue, New: []byte(strings.Replace(ref, "exercise@", "write@", 1))}, "is by exercise"},
		{"Lean", workspace.Change{Path: "lean/Math2/A.lean", New: []byte("a")}, "writes no Lean"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := agent.Permit("exercise", "3C53TD", []workspace.Change{tc.c})
			switch {
			case tc.err == "" && err != nil:
				t.Errorf("error %v", err)
			case tc.err != "" && (err == nil || !strings.Contains(err.Error(), tc.err)):
				t.Errorf("error %v, want %q", err, tc.err)
			}
		})
	}
}
