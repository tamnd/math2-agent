package audit

import "github.com/tamnd/math2-agent/graph"

// g10 checks that each implies, equivalent and refutes edge names a proof or a computation in via (rule G10). A via that is not a node is a finding of rule G01.
func g10(a *auditor) outcome {
	var o outcome
	g := a.graphOf()
	for _, n := range a.scopeNodes() {
		for _, t := range []string{graph.Implies, graph.Equivalent, graph.Refutes} {
			for _, e := range g.Out(t, n.ID) {
				o.checked++
				line, col := keyPos(n, t)
				if e.Via == "" {
					o.add(n.Path, line, col, "the %s edge to %s names no via. Name the proof or the computation that supports it", t, e.To)
					continue
				}
				if v, ok := g.Node(e.Via); ok && v.Kind != "proof" && v.Kind != "computation" {
					o.add(n.Path, line, col, "the %s edge to %s has the via %s, which is a %s. The via must be a proof or a computation", t, e.To, e.Via, v.Kind)
				}
			}
		}
	}
	return o
}
