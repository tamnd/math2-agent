package prov_test

import (
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/prov"
)

func ExampleChanges() {
	before, _ := node.Parse("nodes/7K/7K3QX2.md", []byte("---\ntag: 7K3QX2\nkind: statement\nname: Lagrange\n---\n\n## Statement\n\nThe order of a subgroup divides the order of the group.\n"))
	after, _ := node.Parse("nodes/7K/7K3QX2.md", []byte("---\ntag: 7K3QX2\nkind: statement\nname: Lagrange's theorem\nmsc: [20D60]\n---\n\n## Statement\n\nThe order of a subgroup divides the order of the group.\n"))
	who := prov.Line{Human: "github:writer", At: "2026-09-01T10:00:00Z"}
	for _, l := range prov.Changes(before, after, who) {
		fmt.Println(l.Op, l.Field)
	}
	// Output:
	// add msc
	// change name
}

func ExampleProduced() {
	log := `{"field":"body.explanations.L1","op":"add","agent":"m2-agent/explain@1.2.0","model":"claude-opus-5-5","prompt":"explain/7","run":"r-2026-09-01-1","human":null,"at":"2026-09-01T10:00:00Z"}
{"field":"body.explanations.L2","op":"add","agent":"m2-agent/explain@1.2.0","model":"claude-opus-5-5","prompt":"explain/7","run":"r-2026-09-01-1","human":null,"at":"2026-09-01T10:00:00Z"}
{"field":"body.explanations.L2","op":"confirm","agent":null,"human":"orcid:0000-0002-1825-0097","at":"2026-09-03T10:00:00Z"}
`
	lines, _ := prov.Parse(strings.NewReader(log))
	logs := map[string][]prov.Line{"7K3QX2": lines}
	for _, h := range prov.Produced(logs, nil, "claude-opus-5-5", "explain/7") {
		fmt.Println(h.Tag, h.Line.Field, h.Line.Run)
	}
	// Output:
	// 7K3QX2 body.explanations.L1 r-2026-09-01-1
}

func ExampleLine_Validate() {
	l := prov.Line{Field: "formal[0]", Op: prov.Add, Agent: "m2-agent/formalize@0.9.1", At: "2026-11-02T09:13:44Z"}
	fmt.Println(l.Validate())
	l.Run = "r-2026-11-02-8812"
	fmt.Println(l.Validate())
	// Output:
	// the line of m2-agent/formalize@0.9.1 names no run
	// <nil>
}

func ExampleHide() {
	n, _ := node.Parse("nodes/7K/7K3QX2.md", []byte("---\ntag: 7K3QX2\nkind: statement\n---\n\n## Statement\n\nThe order of a subgroup divides the order of the group.\n\n## Explanations\n\n### L1 agent\n\nA small group fits into a big group a whole number of times.\n"))
	hidden := func(field string) bool { return prov.Under(field, "body.explanations") }
	fmt.Print(prov.Hide(n, hidden, "This text is in quarantine."))
	// Output:
	// ## Statement
	//
	// The order of a subgroup divides the order of the group.
	//
	// ## Explanations
	//
	// ### L1 agent
	//
	// This text is in quarantine.
}

func ExampleDisclose() {
	n, _ := node.Parse("nodes/7K/7K3QX2.md", []byte("---\ntag: 7K3QX2\nkind: statement\nname: Lagrange's theorem\n---\n\n## Statement\n\nThe order of a subgroup divides the order of the group.\n"))
	log := []prov.Line{
		{Field: prov.Whole, Op: prov.Add, Human: "github:writer", At: "2026-09-01T10:00:00Z"},
		{Field: "body.explanations.L1", Op: prov.Add, Agent: "m2-agent/explain@1.2.0", Model: "claude-sonnet-5-5", Prompt: "explain/7", Run: "r-2026-09-02-1", At: "2026-09-02T10:00:00Z"},
	}
	runs := map[string]*prov.Run{"r-2026-09-02-1": {ID: "r-2026-09-02-1", Agent: "m2-agent/explain@1.2.0", Prompt: "explain/7", Models: []string{"claude-sonnet-5-5"}, OnBehalfOf: "github:writer", Started: "2026-09-02T09:58:00Z", Outcome: "success", Seconds: 120, SpendUSD: 0.04}}
	fmt.Print(prov.Disclose(prov.Facts{Node: n, Log: log, Runs: runs}))
	// Output:
	// This disclosure is about the math2 node m2://7K3QX2, Lagrange's theorem. These people changed it themselves: github:writer. The agent m2-agent/explain@1.2.0 changed 1 field in the run r-2026-09-02-1, on behalf of github:writer. It used the models claude-sonnet-5-5 and the prompt explain/7, and the outcome was success. The runs took 2 minutes and spent 0.04 US dollars in all. The kind of agent contribution is: literature search, formalization, rewriting or computation only. An agent wrote these fields and no person checked them: body.explanations.L1. The node has no formal binding. The full records of the runs, with the transcripts, are in the artefact store under the run ids.
}

func ExampleFormalization() {
	n, _ := node.Parse("nodes/7K/7K3QX2.md", []byte("---\ntag: 7K3QX2\nkind: statement\nname: Lagrange's theorem\nformal:\n  - system: lean4\n    library: mathlib\n    decl: Subgroup.card_subgroup_dvd_card\n    match: exact\n---\n\n## Statement\n\nThe order of a subgroup divides the order of the group.\n"))
	log := []prov.Line{{Field: "formal[0]", Op: prov.Add, Human: "github:writer", At: "2026-09-01T10:00:00Z"}}
	data, _ := prov.Formalization(prov.Facts{Node: n, Log: log})
	head, _, _ := strings.Cut(string(data), "repository:")
	fmt.Print(head)
	// Output:
	// version: v0.4
	// project:
	//   name: Lagrange's theorem
	//   description: 'The math2 node m2://7K3QX2, a statement: The order of a subgroup divides the order of the group.'
	//   authors:
	//     - github:writer
	//   license: Apache-2.0
}
