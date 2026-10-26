package formalize_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/formalize"
	"github.com/tamnd/math2-agent/lean"
)

// A statement node binds to a definition of a proposition when it has no proof, and the checks must not refute it.
func ExampleVerify() {
	r := lean.Record{Decl: "Math2.R.s", Kind: "def", Type: "s 0\n", Checks: &lean.Checks{}}
	fmt.Println(formalize.Verify("statement", []lean.Record{r}))

	r.Checks.Refuted = "decide"
	fmt.Println(formalize.Verify("statement", []lean.Record{r}))
	// Output:
	// <nil>
	// Math2.R.s is false: decide proves its negation
}
