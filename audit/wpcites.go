package audit

import (
	"os"
	"path/filepath"
	"strings"

	wpImport "github.com/tamnd/math2-agent/importer/wikipedia"
)

// y16 reports a citation of the corpus with an id that resolves to no record (rule Y16, corpus wikipedia WP10.2): an id that does not have the syntax of its system or fails its check digit, or an id that a live API does not know. It reads the citation files that m2 wp cites writes in the checkout, and it skips when there are none.
func y16(a *auditor) outcome {
	var o outcome
	if a.wikipedia == "" {
		o.reason = "no checkout of tamnd/wikipedia-math is given, so set -wikipedia to run the rule"
		return o
	}
	paths, err := filepath.Glob(filepath.Join(a.wikipedia, wpImport.CitationsDir, "*", "*.md"))
	if err != nil {
		o.add(wpImport.CitationsDir, 0, 0, "%v", err)
		return o
	}
	if len(paths) == 0 {
		o.reason = "the checkout has no citation files yet. Run m2 wp cites to make them"
		return o
	}
	for _, p := range paths {
		data, err := os.ReadFile(p) // #nosec G304 -- the path comes from a listing of the checkout.
		rel, _ := filepath.Rel(a.wikipedia, p)
		rel = filepath.ToSlash(rel)
		if err != nil {
			o.add(rel, 0, 0, "%v", err)
			continue
		}
		f, err := wpImport.ParseCiteFile(data)
		if err != nil {
			o.add(rel, 0, 0, "%v", err)
			continue
		}
		for _, c := range f.Citations {
			o.checked++
			if len(c.BadIDs) > 0 {
				o.add(rel, 0, 0, "the citation %s of %s has an id that is not valid: %s. Correct the id in the article", c.Ref, f.Title, strings.Join(c.BadIDs, ", "))
			}
			if len(c.DeadIDs) > 0 {
				o.add(rel, 0, 0, "the citation %s of %s has an id that the live API does not know: %s. Check the id in the article", c.Ref, f.Title, strings.Join(c.DeadIDs, ", "))
			}
		}
	}
	return o
}
