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
