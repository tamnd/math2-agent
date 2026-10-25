package audit

import (
	"regexp"
	"strings"

	"github.com/tamnd/math2-agent/check"
)

// explanationHeading is the heading of one explanation: a level from L1 to L4 and a marker that says who wrote it (spec 04.5).
var explanationHeading = regexp.MustCompile(`^L[1-4] (human|agent)$`)

// b05 finds the explanations that have no level or no marker (rule B05). In the Explanations section, each explanation starts with a level 3 heading such as "### L2 human" or "### L1 agent". Text before the first such heading is an explanation with no level.
func b05(a *auditor) outcome {
	var o outcome
	for _, n := range a.scopeNodes() {
		for _, s := range check.Sections(n) {
			if s.Name != "Explanations" {
				continue
			}
			o.checked++
			first := s.Line + 1 + strings.Count(s.Text, "\n") + 1 // after the section
			if len(s.Headings) > 0 {
				first = s.Headings[0].Line
			}
			for i, l := range strings.Split(s.Text, "\n") {
				if line := s.Line + 1 + i; line < first && strings.TrimSpace(l) != "" {
					o.add(n.Path, line, 1, "the Explanations section has text before the first explanation heading. Start each explanation with a heading such as \"### L2 human\"")
					break
				}
			}
			for _, h := range s.Headings {
				if h.Level == 3 && !explanationHeading.MatchString(h.Name) {
					o.add(n.Path, h.Line, 1, "the explanation heading %q has no level or no marker. Use a level from L1 to L4 and human or agent, for example \"### L2 human\"", h.Name)
				}
			}
		}
	}
	return o
}
