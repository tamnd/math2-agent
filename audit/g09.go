package audit

import (
	"strings"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/graph"
)

// maxDefinitions is the number of definitions that a concept can have without a note (rule G09).
const maxDefinitions = 12

// g09 finds each concept with more than 12 definitions and no note that says why (rule G09). The note is the Remarks section of the concept.
func g09(a *auditor) outcome {
	var o outcome
	g := a.graphOf()
	for _, n := range a.scopeNodes() {
		if n.Kind() != "concept" {
			continue
		}
		o.checked++
		defs := len(g.In(graph.Defines, n.ID))
		if defs <= maxDefinitions {
			continue
		}
		note := false
		for _, s := range check.Sections(n) {
			note = note || s.Name == "Remarks" && strings.TrimSpace(s.Text) != ""
		}
		if !note {
			o.add(n.Path, 1, 1, "the concept has %d definitions and no Remarks section that says why. Add the note, or merge the definitions", defs)
		}
	}
	return o
}
