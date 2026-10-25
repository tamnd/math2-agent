package schema_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
)

func ExampleSchema_Validate() {
	data := []byte("---\ntag: 9TX3RB\nkind: proof\nname: Proof of Lagrange's theorem\nstatus: proved\n---\n\n## Proof\n\nCount the cosets.\n")
	n, err := node.Parse("nodes/9T/9TX3RB.md", data)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, f := range schema.Embedded().Validate(n) {
		fmt.Println(f)
	}
	// Output:
	// nodes/9T/9TX3RB.md:2:1: S02: the field "proves" is missing, and a node of kind proof needs it
	// nodes/9T/9TX3RB.md:5:1: S03: the field "status" states a status, but the build computes the status of each node
}

func ExampleSchema_Sections() {
	s, _ := schema.Embedded().Sections("statement")
	fmt.Println(s.Required, s.Allowed)
	// Output: [Statement] [Statement Remarks History Explanations]
}

func ExampleSchema_Refs() {
	fmt.Println(schema.Embedded().Refs[:4])
	// Output: [about defines instance_of methods]
}
