package graph_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/synth"
)

// TestQueriesRepeat checks that Needs and Dependents give the same result on a graph that ran other queries before, and on a new graph. The queries use the arrays of the calls before them again.
func TestQueriesRepeat(t *testing.T) {
	d := synth.Make(synth.Options{Nodes: 3000, Seed: 5})
	g := d.Graph
	for _, n := range g.Nodes {
		g.Needs(n.Tag)
		g.Dependents(n.Tag)
	}
	for i := 0; i < len(g.Nodes); i += 97 {
		tag := g.Nodes[i].Tag
		fresh := graph.New(g.Nodes, g.Edges)
		if got, want := g.Needs(tag), fresh.Needs(tag); !slices.Equal(got, want) {
			t.Errorf("Needs(%s) gives %d nodes after other queries, and %d on a new graph", tag, len(got), len(want))
		}
		if got, want := g.Dependents(tag), fresh.Dependents(tag); !slices.Equal(got, want) {
			t.Errorf("Dependents(%s) gives %d nodes after other queries, and %d on a new graph", tag, len(got), len(want))
		}
	}
}

// TestNeedsReference checks Needs against its definition, on a synthetic graph and on small random graphs with cycles.
func TestNeedsReference(t *testing.T) {
	g := synth.Make(synth.Options{Nodes: 3000, Seed: 7}).Graph
	for i := 0; i < len(g.Nodes); i += 211 {
		checkNeeds(t, g, g.Nodes[i].Tag)
	}
	r := rand.New(rand.NewPCG(9, 0)) // #nosec G404 -- the graphs must repeat.
	for range 5000 {
		g := random(r)
		for _, n := range g.Nodes {
			checkNeeds(t, g, n.Tag)
		}
	}
}

// checkNeeds compares Needs(x) with a reference. To make x false, the reference graph adds a uses edge from x to a node that uses itself, so that x has no alternative. A node needs x when it has an alternative in the graph and none in the reference graph.
func checkNeeds(t *testing.T, g *graph.Graph, x string) {
	t.Helper()
	const z = "ZZZZZZ"
	h := graph.New(append(slices.Clone(g.Nodes), graph.Node{Tag: z, Kind: "statement"}), append(slices.Clone(g.Edges), graph.Edge{Type: graph.Uses, From: x, To: z}, graph.Edge{Type: graph.Uses, From: z, To: z}))
	var want []string
	for _, n := range g.Nodes {
		if n.Tag == x {
			continue
		}
		if _, ok := g.Closure(n.Tag, graph.MinDepth); !ok {
			continue
		}
		if _, ok := h.Closure(n.Tag, graph.MinDepth); !ok {
			want = append(want, n.Tag)
		}
	}
	if got := g.Needs(x); !slices.Equal(got, want) {
		t.Errorf("Needs(%s) gives %v, and the reference gives %v, in the graph %v", x, got, want, g.Edges)
	}
}

// random makes a small graph of statements and proofs with random uses edges, so it can have cycles.
func random(r *rand.Rand) *graph.Graph {
	n := 2 + r.IntN(7)
	var nodes []graph.Node
	var edges []graph.Edge
	tag := func(kind string, i int) string { return fmt.Sprintf("5%c%04d", kind[0]-32, i) }
	for i := range n {
		nodes = append(nodes, graph.Node{Tag: tag("statement", i), Kind: "statement"})
		for k := range r.IntN(3) {
			p := tag("proof", i*3+k)
			nodes = append(nodes, graph.Node{Tag: p, Kind: "proof"})
			edges = append(edges, graph.Edge{Type: graph.Proves, From: p, To: tag("statement", i)})
		}
	}
	for range r.IntN(2 * len(nodes)) {
		from, to := nodes[r.IntN(len(nodes))].Tag, nodes[r.IntN(len(nodes))].Tag
		edges = append(edges, graph.Edge{Type: graph.Uses, From: from, To: to})
	}
	return graph.New(nodes, edges)
}

// TestClosureIndex checks that Index gives the position of each node of a closure in the nodes of the graph.
func TestClosureIndex(t *testing.T) {
	g := synth.Make(synth.Options{Nodes: 3000, Seed: 5}).Graph
	g = graph.New(g.Nodes, append(slices.Clone(g.Edges), graph.Edge{Type: graph.Uses, From: g.Nodes[len(g.Nodes)-1].Tag, To: "ZZZZZZ"}))
	for i := len(g.Nodes) - 1; i >= 0; i -= 101 {
		alt, ok := g.Closure(g.Nodes[i].Tag, graph.MinDepth)
		if !ok {
			continue
		}
		if len(alt.Index) != len(alt.Nodes) {
			t.Fatalf("the closure of %s has %d nodes and %d positions", alt.Tag, len(alt.Nodes), len(alt.Index))
		}
		for k, tag := range alt.Nodes {
			want := slices.IndexFunc(g.Nodes, func(n graph.Node) bool { return n.Tag == tag })
			if alt.Index[k] != want {
				t.Errorf("the closure of %s gives the position %d for %s, and the position is %d", alt.Tag, alt.Index[k], tag, want)
			}
		}
	}
}
