package graph_test

import (
	"fmt"
	"log"
	"strings"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
)

// nodes parses node files. Each item is front matter that starts with the tag line.
func nodes(src ...string) []*node.Node {
	var out []*node.Node
	for _, s := range src {
		first, _, _ := strings.Cut(s, "\n")
		id := strings.TrimPrefix(first, "tag: ")
		n, err := node.Parse("nodes/"+id[:2]+"/"+id+".md", []byte("---\n"+s+"\n---\n\nText.\n"))
		if err != nil {
			log.Fatal(err)
		}
		out = append(out, n)
	}
	return out
}

func ExampleBuild() {
	g := graph.Build(nodes(
		"tag: G5S1M9\nkind: concept\nname: Group",
		"tag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nuses: [G5S1M9]",
	))
	for _, e := range g.Edges {
		fmt.Println(e.Type, e.From, e.To, graph.Family(e.Type))
	}
	// Output:
	// uses 3C53TD G5S1M9 logical
}

func ExampleGraph_Closure() {
	g := graph.Build(nodes(
		"tag: 5S0000\nkind: statement\nproofs: [5P1000, 5P2000]",
		"tag: 5P1000\nkind: proof\nproves: 5S0000\nuses: [5A0000, 5B0000, 5C0000]",
		"tag: 5P2000\nkind: proof\nproves: 5S0000\nuses: [5D0000]",
		"tag: 5D0000\nkind: definition\nuses: [5A0000]",
		"tag: 5A0000\nkind: concept",
		"tag: 5B0000\nkind: concept",
		"tag: 5C0000\nkind: concept",
	))
	deep, _ := g.Closure("5S0000", graph.MinDepth)
	fmt.Println(deep.Proofs, deep.Depth, deep.Nodes)
	small, _ := g.Closure("5S0000", graph.MinSize)
	fmt.Println(small.Proofs, small.Depth, small.Nodes)
	// Output:
	// [5P1000] 2 [5A0000 5B0000 5C0000 5P1000 5S0000]
	// [5P2000] 3 [5A0000 5D0000 5P2000 5S0000]
}

func ExampleGraph_Cycles() {
	g := graph.Build(nodes(
		"tag: 9S0000\nkind: statement\nproofs: [9P0000]",
		"tag: 9P0000\nkind: proof\nproves: 9S0000\nuses: [9T0000]",
		"tag: 9T0000\nkind: statement\nuses: [9S0000]",
	))
	for _, c := range g.Cycles() {
		fmt.Println(c.Type, c.Nodes)
	}
	// Output:
	// uses [9P0000 9T0000 9S0000]
}

func ExampleGraph_Hash() {
	g := graph.Build(nodes("tag: G5S1M9\nkind: concept\nname: Group"))
	fmt.Println(g.Hash()[:7], len(g.Hash()))
	// Output:
	// blake3: 71
}
