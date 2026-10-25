package audit

import "github.com/tamnd/math2-agent/graph"

// g08 finds each topic in which 2% or more of the live nodes are orphans (rule G08). An orphan is a node that no other node has an edge to (spec 04.7). The nodes of a topic are the nodes with a part_of edge to it.
func g08(a *auditor) outcome {
	var o outcome
	g := a.graphOf()
	for _, n := range a.scopeNodes() {
		if n.Kind() != "topic" || retired(n) {
			continue
		}
		o.checked++
		members, orphans := 0, 0
		for _, e := range g.In(graph.PartOf, n.ID) {
			m, ok := g.Node(e.From)
			if !ok || m.Retired {
				continue
			}
			members++
			if !linked(g, m.Tag) {
				orphans++
			}
		}
		if orphans > 0 && orphans*50 >= members {
			line, col := keyPos(n, "kind")
			o.add(n.Path, line, col, "%d of the %d nodes of the topic are orphans, and the limit is less than 2%%. Link the orphans from other nodes", orphans, members)
		}
	}
	return o
}

// linked reports whether another node of g has an edge to the node with the tag. A part_of edge does not count, because it goes to the topic.
func linked(g *graph.Graph, tag string) bool {
	for _, t := range graph.Types() {
		for _, e := range g.In(t, tag) {
			if _, ok := g.Node(e.From); ok && e.From != tag {
				return true
			}
		}
	}
	return false
}
