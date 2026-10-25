package audit

import "github.com/tamnd/math2-agent/graph"

// minFamily is the number of members that a hub family needs (rule G07).
const minFamily = 3

// g07 finds each hub family with fewer than three members (rule G07). A hub is a live statement with the role fact, no formal binding and at least one generalizes edge (spec 05.4). The members are the nodes at the other end of its generalizes edges.
func g07(a *auditor) outcome {
	var o outcome
	g := a.graphOf()
	for _, n := range a.scopeNodes() {
		if n.Kind() != "statement" || n.Get("role") != "fact" || retired(n) || len(g.In(graph.Formalizes, n.ID)) > 0 {
			continue
		}
		members := map[string]bool{}
		for _, e := range g.Out(graph.Generalizes, n.ID) {
			members[e.To] = true
		}
		for _, e := range g.In(graph.Generalizes, n.ID) {
			members[e.From] = true
		}
		if len(members) == 0 {
			continue
		}
		o.checked++
		if len(members) < minFamily {
			line, col := keyPos(n, "generalizes", "role")
			o.add(n.Path, line, col, "the hub family has %d members, and a family needs at least %d. Link the members to each other instead", len(members), minFamily)
		}
	}
	return o
}
