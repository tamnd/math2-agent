package audit

import (
	"strings"

	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/node"
)

// graphOf returns the graph of every node file that parses.
func (a *auditor) graphOf() *graph.Graph {
	if a.graph == nil {
		a.graph = graph.Build(a.nodes)
	}
	return a.graph
}

// g03 checks that the specializes edges have no cycle, and that the uses edges have no cycle through one proof or through no proof (rule G03). A finding goes to the first node of the cycle. In a pull request, only a cycle through a node in scope is a finding.
func g03(a *auditor) outcome {
	var o outcome
	byID := map[string]*node.Node{}
	for _, n := range a.nodes {
		byID[n.ID] = n
	}
	o.checked = len(a.nodes)
	for _, c := range a.graphOf().Cycles() {
		var at *node.Node
		for _, id := range c.Nodes {
			if n := byID[id]; n != nil && (at == nil || a.inScope(n.Path) && !a.inScope(at.Path)) {
				at = n
			}
		}
		if at == nil || !a.inScope(at.Path) {
			continue
		}
		path := strings.Join(append(c.Nodes, c.Nodes[0]), " -> ")
		line, col := keyPos(at, c.Type, "proofs", "proves")
		if c.Type == graph.Specializes {
			o.add(at.Path, line, col, "the specializes edges make a cycle: %s", path)
		} else {
			o.add(at.Path, line, col, "the uses edges make a cycle that does not go through two different proofs: %s", path)
		}
	}
	return o
}

// keyPos returns the line and the column of the first of the top-level keys that the front matter of n has, or the first line of the file when it has none of them.
func keyPos(n *node.Node, keys ...string) (line, col int) {
	if n.Front == nil || len(n.Front.Content) == 0 {
		return 1, 1
	}
	m := n.Front.Content[0]
	for _, key := range keys {
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == key {
				return m.Content[i].Line, m.Content[i].Column
			}
		}
	}
	return 1, 1
}
