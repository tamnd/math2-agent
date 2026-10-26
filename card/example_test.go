package card_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/card"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
)

func ExampleCards_Assess() {
	data := []byte("---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nuses: [G5S1M9]\nsources:\n  - url: https://en.wikipedia.org/wiki/Lagrange%27s_theorem_(group_theory)\n    locator: Proof\n---\n\n## Statement\n\nLet $G$ be a finite group and $H$ a subgroup of $G$. Then $|H|$ divides $|G|$.\n")
	n, err := node.Parse("nodes/3C5/3C53TD.md", data)
	if err != nil {
		fmt.Println(err)
		return
	}
	env := card.Env{Graph: graph.Build([]*node.Node{n}), Row: func(string) bool { return true }}
	r := card.Embedded().Assess(n, env)
	fmt.Println(r.Level)
	for _, need := range r.Next {
		fmt.Println(need.What, need.Have, need.Want)
	}
	// Output:
	// W1
	// edge about out 0 1
	// edge proves in 0 1
	// edge instance_of or illustrates in 0 1
	// explanation L2 0 1
}
