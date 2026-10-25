// Package crosswalk gives the syntax of the identifiers of the external systems that a node names in same_as (spec 05.3). Rule A01 of the audit uses it, and each ingest adapter of a system gives ids that pass it.
//
// A system with a fixed form of id, such as Wikidata, OEIS or the Stacks Project, has a pattern. A system whose ids are page titles, such as nLab or Wikipedia, has the rules of a page title. The other systems have the rules that each id obeys: an id is not empty, has no space at its ends and has no control characters.
package crosswalk

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// A system is the syntax of the ids of one external system.
type system struct {
	form    string         // the form of an id, for the error message
	pattern *regexp.Regexp // nil when the system has no fixed form
	title   bool           // the ids are page titles
}

func pattern(form, expr string) system {
	return system{form: form, pattern: regexp.MustCompile(`^(?:` + expr + `)$`)}
}

// msc is an MSC code: 11N05, 11Nxx, 11-XX or 11-02.
const msc = `[0-9]{2}(?:[A-Z][0-9]{2}|[A-Z]xx|-XX|-[0-9]{2})`

var systems = map[string]system{
	"wikidata":           pattern("a QID such as Q180967", `Q[1-9][0-9]*`),
	"mardi":              pattern("a MaRDI QID such as Q180967", `Q[1-9][0-9]*`),
	"oeis":               pattern("an A-number such as A000045", `A[0-9]{6}`),
	"stacks":             pattern("a tag of four digits and capital letters such as 0BRS", `[0-9A-Z]{4}`),
	"kerodon":            pattern("a tag of four digits and capital letters such as 00XD", `[0-9A-Z]{4}`),
	"lmfdb":              pattern("the path of the object on the LMFDB, such as EllipticCurve/Q/11.a2", `[A-Z][A-Za-z0-9]*(?:/[0-9A-Za-z][0-9A-Za-z.+_-]*)+`),
	"zbmath":             pattern("a document id such as 6383667, a Zbl number such as 1306.11073, an author id such as maynard.james or an MSC code", `[1-9][0-9]*|[0-9]{4}\.[0-9]{5}|[a-z][a-z-]*(?:\.[a-z][a-z-]*)+(?:\.[0-9]+)?|`+msc),
	"mr":                 pattern("an MR number such as MR1234567", `(?:MR)?[1-9][0-9]*`),
	"arxiv":              pattern("an arXiv id such as 1311.4600 or math/0211159, with an optional version and result tag", `(?:[0-9]{4}\.[0-9]{4,5}|[a-z-]+(?:\.[A-Z]{2})?/[0-9]{7})(?:v[1-9][0-9]*)?(?:#[A-Za-z0-9._:-]+)?`),
	"erdos":              pattern("a problem number such as 1", `[1-9][0-9]*`),
	"mathoverflow":       pattern("a question id such as 4", `[1-9][0-9]*`),
	"mathse":             pattern("a question id such as 4", `[1-9][0-9]*`),
	"house_of_graphs":    pattern("a graph id such as 660", `[1-9][0-9]*`),
	"pibase":             pattern("a space, property or theorem id such as S000001", `[SPT][0-9]{6}`),
	"findstat":           pattern("a statistic or map id such as St000001 or Mp00001", `St[0-9]{6}|Mp[0-9]{5}`),
	"nlab":               {form: "an nLab page name", title: true},
	"wikipedia":          {form: "a Wikipedia page title", title: true},
	"proofwiki":          {form: "a ProofWiki page title", title: true},
	"formal_conjectures": {form: "a formal-conjectures declaration name such as Erdos1.erdos_1"},
	"palomar":            pattern("a Palomar entry id such as PALOMAR-2026-10-23-000018", `PALOMAR-[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]{6}`),
	"theoremsearch":      {form: "a TheoremSearch statement id"},
	"hexagon":            {form: "a Hexagon id and version"},
	"dart":               {form: "a DaRT id"},
	"gap":                {form: "a GAP small group id"},
	"atlas":              {form: "an ATLAS id"},
}

// Systems returns the names of the external systems, in order of name. They are the keys of same_as in schema node.v1.
func Systems() []string {
	out := make([]string, 0, len(systems))
	for k := range systems {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Known reports whether name is an external system.
func Known(name string) bool {
	_, ok := systems[name]
	return ok
}

// ErrUnknown is the error of Check for a system that is not in Systems.
var ErrUnknown = errors.New("crosswalk: the system is not known")

// Check reports whether id has the syntax of the ids of the system. The error says what form the id must have.
func Check(name, id string) error {
	s, ok := systems[name]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknown, name)
	}
	switch {
	case id == "":
		return fmt.Errorf("the %s id is empty", name)
	case strings.TrimSpace(id) != id:
		return fmt.Errorf("the %s id %q has space at its ends", name, id)
	case strings.ContainsFunc(id, unicode.IsControl):
		return fmt.Errorf("the %s id %q has a control character", name, id)
	case s.pattern != nil && !s.pattern.MatchString(id):
		return fmt.Errorf("the %s id %q is not %s", name, id, s.form)
	case s.title && strings.ContainsAny(id, "#<>[]|{}"):
		return fmt.Errorf("the %s id %q is not %s: a title has no # < > [ ] | { }", name, id, s.form)
	}
	return nil
}
