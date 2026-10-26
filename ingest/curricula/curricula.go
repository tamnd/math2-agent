// Package curricula has the ingest adapters of the open curricula and theorem lists of corpus K5.8. A curriculum names items in an order, and the corpus takes it as data, not as text (K10.3). Each item that a curriculum names is a mention, with the curriculum as its source.
//
// The package registers five adapters:
//
//	mathlib_undergrad  the undergraduate mathematics of Mathlib, docs/undergrad.yaml
//	mathlib_overview   the overview of Mathlib, docs/overview.yaml
//	mathlib_100        Freek Wiedijk's 100 theorems in Mathlib, docs/100.yaml
//	mathlib_1000       the 1000+ theorems in Mathlib, docs/1000.yaml
//	thousand_plus      the 1000+ theorems project, with the formalizations in six systems
//
// The four lists of Mathlib are under the Apache License 2.0. The bulk channel of each is the raw file in the mathlib4 repository. A snapshot is pinned when the URL names a commit, for example
//
//	https://raw.githubusercontent.com/leanprover-community/mathlib4/2b2e96747ca44b376def99bf9571ef72cc6a7f2d/docs/undergrad.yaml
//
// and the catalogue then records the commit. The 1000+ theorems project is under the Unlicense. Its bulk channel is the git archive of its site.
//
// The meta keys of the records are these. A key is not in the meta when the item has no value for it.
//
//	mathlib     the Mathlib declarations of the item, with "|" between them
//	section     the sections of a tree list that hold the item, with " > " between them
//	order       the place of the item in the list, from 1
//	link        the page that a tree list gives for an item with no declaration
//	status      formalized, or statement when Mathlib has only the statement
//	authors     the authors of the formalization
//	date        the date of the formalization
//	links       other formalizations of the item, with "|" between them
//	note        the note of the list on the item
//	wikidata    the Wikidata item of the theorem
//	wikipedia   the Wikipedia pages of the theorem, with "|" between them
//	msc         the MSC2020 class of the theorem, for example 20-XX
//	formalized  the systems with a formal proof, with "|" between them
//	stated      the systems with only a formal statement, with "|" between them
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/curricula"
package curricula

import (
	"regexp"
	"strings"
)

// hex40 matches a full git commit.
var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

// commitOf returns the segment of the URL that is a full git commit, or "" when it has none.
func commitOf(url string) string {
	for s := range strings.SplitSeq(url, "/") {
		if hex40.MatchString(s) {
			return s
		}
	}
	return ""
}

// clean folds the runs of white space in s to one space and trims its ends.
func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

// meta returns m, or nil when m is empty.
func meta(m map[string]string) map[string]string {
	for k, v := range m {
		if v == "" {
			delete(m, k)
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}
