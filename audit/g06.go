package audit

import "github.com/tamnd/math2-agent/graph"

// g06 checks that each live conjecture and problem has a source and names who posed it in history.posed_by (rule G06).
func g06(a *auditor) outcome {
	var o outcome
	g := a.graphOf()
	for _, n := range a.scopeNodes() {
		if k := n.Kind(); k != "conjecture" && k != "problem" || retired(n) {
			continue
		}
		o.checked++
		line, col := keyPos(n, "kind")
		if len(g.Out(graph.Source, n.ID)) == 0 {
			o.add(n.Path, line, col, "the %s has no source. Add the source where it was posed", n.Kind())
		}
		if h, _ := n.Meta["history"].(map[string]any); h == nil || str(h["posed_by"]) == "" {
			if hl, hc := keyPos(n, "history"); hl > 1 {
				line, col = hl, hc
			}
			o.add(n.Path, line, col, "the %s does not name who posed it. Add history.posed_by", n.Kind())
		}
	}
	return o
}
