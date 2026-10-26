package style_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/style"
)

func ExampleNode() {
	data := []byte("---\ntag: 3C53TD\nkind: statement\nname: Lagrange's theorem\n---\n\n## Statement\n\nLet $G$ be a finite group and $H$ a subgroup of $G$. Then clearly $|H|$ divides $|G|$, as we saw in Section 2.\n")
	n, err := node.Parse("nodes/3C5/3C53TD.md", data)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, f := range style.Node(n) {
		fmt.Println(f)
	}
	// Output:
	// nodes/3C5/3C53TD.md:9:59: Y02: "clearly" hides a step: give the step, or make it a lemma
	// nodes/3C5/3C53TD.md:9:91: Y03: "we" is not used in the section Statement
	// nodes/3C5/3C53TD.md:9:101: Y04: "Section 2" is a reference to place, and a node has no place: link to the node
}
