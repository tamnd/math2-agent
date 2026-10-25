package audit

import (
	"strings"

	"github.com/tamnd/math2-agent/graph"
)

// g02 checks that no two live nodes have the same same_as id in one external system (rule G02). The other direction needs no check: same_as is a YAML mapping, so a node has at most one id for each system.
func g02(a *auditor) outcome {
	var o outcome
	g := a.graphOf()
	owners := map[string][]string{} // the live nodes of each system and id
	for _, e := range g.Edges {
		if e.Type != graph.SameAs {
			continue
		}
		if n, ok := g.Node(e.From); ok && !n.Retired {
			k := e.System + " " + e.To
			owners[k] = append(owners[k], e.From)
		}
	}
	for _, n := range a.scopeNodes() {
		if _, ok := n.Meta["same_as"]; !ok {
			continue
		}
		o.checked++
		for _, e := range g.Out(graph.SameAs, n.ID) {
			if tags := owners[e.System+" "+e.To]; len(tags) > 1 {
				line, col := keyPos(n, "same_as")
				o.add(n.Path, line, col, "the %s id %s is the same_as id of more than one live node: %s. Use close_to with a note for a node that is not the same thing", e.System, e.To, strings.Join(tags, ", "))
			}
		}
	}
	return o
}
