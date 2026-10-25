package audit

import "github.com/tamnd/math2-agent/graph"

// g05 checks that each live statement has a source, a proof or a formal binding (rule G05). A statement with none of them is an assertion with no evidence.
func g05(a *auditor) outcome {
	var o outcome
	g := a.graphOf()
	for _, n := range a.scopeNodes() {
		if n.Kind() != "statement" || retired(n) {
			continue
		}
		o.checked++
		if len(g.Out(graph.Source, n.ID)) == 0 && len(g.In(graph.Proves, n.ID)) == 0 && len(g.In(graph.Formalizes, n.ID)) == 0 {
			line, col := keyPos(n, "kind")
			o.add(n.Path, line, col, "the statement has no source, no proof and no formal binding. Add at least one of them")
		}
	}
	return o
}
