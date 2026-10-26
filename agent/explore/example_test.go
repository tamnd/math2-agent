package explore_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/explore"
	"github.com/tamnd/math2-agent/node"
)

// The novelty check reads the claim of an output from its Statement section. A run of integers in it gives the terms for the search of the OEIS.
func ExampleClaimOf() {
	n, err := node.Parse("nodes/~/~9c0d1e2f.md", []byte("---\ntag: ~9c0d1e2f\nkind: conjecture\nname: A count of trees\n---\n\n## Statement\n\nThe counts 1, 1, 2, 5, 14, 42 of the trees go on as $C_n$.\n"))
	if err != nil {
		fmt.Println(err)
		return
	}
	c := explore.ClaimOf(n)
	fmt.Println(c.Name)
	fmt.Println(c.Terms)
	fmt.Printf("%q\n", explore.ClaimsNew("A novel count of trees."))
	// Output:
	// A count of trees
	// [1 1 2 5 14 42]
	// "novel"
}
