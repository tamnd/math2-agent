package audit

import (
	"strings"

	wpImport "github.com/tamnd/math2-agent/importer/wikipedia"
	"github.com/tamnd/math2-agent/inventory"
)

// y18 reports each item of the parity data that has no outcome, and each mention of an article of the parity data that the data does not list (rule Y18, corpus wikipedia WP9.7). The rule is n/a when the tree has no parity data.
func y18(a *auditor) outcome {
	var o outcome
	listed := map[string]bool{}
	pages := map[string]string{} // the article mention to the file of its first item
	err := wpImport.WalkParity(a.root, func(file string, line int, it wpImport.ParityItem) error {
		o.checked++
		listed[it.Mention] = true
		if _, ok := pages[it.Article]; !ok {
			pages[it.Article] = file
		}
		if it.Missing() {
			o.add(file, line, 1, "the item %s of the article %s has no outcome (%s)", it.Mention, it.Title, it.Outcome)
		}
		return nil
	})
	if err != nil {
		o.add(wpImport.ParityDir, 0, 0, "%v", err)
		return o
	}
	if o.checked == 0 {
		o.reason = "the tree has no parity data"
		return o
	}
	err = inventory.WalkMentions(a.root, func(_ string, _ int, m inventory.Mention) error {
		if m.Source != wpImport.Source {
			return nil
		}
		art, _, _ := strings.Cut(m.ID, "#")
		if file, ok := pages[art]; ok && !listed[m.ID] {
			o.add(file, 0, 0, "the parity data does not list the item %s of the article %s. Run m2 corpus parity again", m.ID, art)
		}
		return nil
	})
	if err != nil {
		o.add(inventory.MentionsDir, 0, 0, "%v", err)
	}
	return o
}
