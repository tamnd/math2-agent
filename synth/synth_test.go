package synth_test

import (
	"testing"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/synth"
	"github.com/tamnd/math2-agent/tag"
)

func TestMake(t *testing.T) {
	d := synth.Make(synth.Options{Nodes: 20000, Seed: 1})
	g := d.Graph
	if len(g.Nodes) < 20000 || len(g.Nodes) > 24000 {
		t.Errorf("Make gives %d nodes, and the options ask for 20000", len(g.Nodes))
	}
	kinds := map[string]int{}
	for _, n := range g.Nodes {
		kinds[n.Kind]++
		if !tag.IsTag(n.Tag) {
			t.Fatalf("%q is not a tag", n.Tag)
		}
	}
	t.Logf("%d nodes, %d edges, kinds %v", len(g.Nodes), len(g.Edges), kinds)
	if again := synth.Make(synth.Options{Nodes: 20000, Seed: 1}); again.Graph.Hash() != g.Hash() {
		t.Error("two graphs with the same options have different hashes")
	}
	deepest := 0
	for _, n := range g.Nodes {
		if n.Kind != "statement" {
			continue
		}
		if a, ok := g.Closure(n.Tag, graph.MinDepth); ok {
			deepest = max(deepest, a.Depth)
		}
	}
	if deepest < 20 || deepest > 40 {
		t.Errorf("the deepest closure has the depth %d, and it must be about 30", deepest)
	}
	t.Logf("the deepest closure has the depth %d", deepest)
}
