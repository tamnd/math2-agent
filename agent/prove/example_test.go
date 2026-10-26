package prove_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/prove"
	"github.com/tamnd/math2-agent/lean"
)

// A pin checks that the proof proves the pinned statement, and that the run has not changed the statement.
func ExamplePin_Verify() {
	p := &prove.Pin{Target: "7K3QX2", Statement: "Math2.T.s", Closure: map[string]string{"Math2.T.s": "h1"}}
	s := lean.Record{Decl: "Math2.T.s", Kind: "def", ValueHash: "h1"}
	proof := lean.Record{Decl: "Math2.T.t", Kind: "theorem", Type: p.ProofType(), Axioms: []string{"propext"}}
	fmt.Println(p.Verify("Math2.T.t", []lean.Record{proof, s}))

	proof.Axioms = []string{"sorryAx"}
	fmt.Println(p.Verify("Math2.T.t", []lean.Record{proof, s}))
	// Output:
	// <nil>
	// the proof Math2.T.t uses sorry
}
