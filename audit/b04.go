package audit

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/check"
)

// maxStatement is the length of a statement section, in characters, that rule B04 finds.
const maxStatement = 2000

// b04 finds each statement section of 2,000 characters or more (rule B04). A long statement is usually two statements. The statement sections are those of check.StatementSections, and the length does not count the blank lines at the ends.
func b04(a *auditor) outcome {
	var o outcome
	names := check.StatementSections()
	for _, n := range a.scopeNodes() {
		for _, s := range check.Sections(n) {
			if !slices.Contains(names, s.Name) {
				continue
			}
			o.checked++
			if c := utf8.RuneCountInString(strings.TrimSpace(s.Text)); c >= maxStatement {
				o.add(n.Path, s.Line, 1, "the %s section has %d characters. Make it shorter than %d characters, or split it into two nodes", s.Name, c, maxStatement)
			}
		}
	}
	return o
}
