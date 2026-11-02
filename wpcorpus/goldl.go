package wpcorpus

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
)

// GoldSetL is gold set L of WP11.2: the links in the items of 200 articles.
const GoldSetL = "l"

// GoldLSize is the number of articles in gold set L.
const GoldLSize = 200

// GoldLMethods are the methods of WP6.3 that have the target of WP11.3 for
// the precision of the predicate.
var GoldLMethods = []string{"link-in-item", "link-in-proof", "anchor", "named-link"}

// goldLTarget is the target of WP11.3 for the predicate precision of each
// method of GoldLMethods.
const goldLTarget = 0.90

// GoldLink is one link of an article of gold set L. The draft gives the
// predicate of the extractor. A person writes the correct predicate.
type GoldLink struct {
	Span   [2]int `yaml:"span,flow"`
	Method string `yaml:"method"`
	// To is the target: a mention in scope, "page N" for a page out of the
	// scope, or the title of a target with no page id.
	To        string `yaml:"to"`
	Label     string `yaml:"label,omitempty"`
	Predicate string `yaml:"predicate"`
}

// edgeTo gives the target of an edge as GoldLink.To gives it.
func edgeTo(e *Edge) string {
	switch {
	case e.To != "":
		return e.To
	case e.Page != 0:
		return "page " + strconv.Itoa(e.Page)
	}
	return e.Target
}

// itemLinks gives the edges of an edge file that start at an item, as the
// draft of gold set L. An item mention has a tag after "#".
func itemLinks(f *EdgeFile) []GoldLink {
	var out []GoldLink
	for i := range f.Edges {
		e := &f.Edges[i]
		if !strings.Contains(e.From, "#") {
			continue
		}
		out = append(out, GoldLink{Span: e.Span, Method: e.Method, To: edgeTo(e), Label: e.Label, Predicate: e.Predicate})
	}
	return out
}

// DrawGoldL draws gold set L: GoldLSize articles that have links in their
// items. The articles are sorted by the type, the class and the MSC code,
// and the draw takes them at equal steps from a random start, so the types,
// the classes and the codes are in proportion to the scope (WP11.2). edges
// gives the edge file of a page, or nil when the page has none.
func DrawGoldL(as []*CensusArticle, edges func(page int) (*EdgeFile, error), seed uint64) ([]*GoldPage, error) {
	var pool []*GoldPage
	links := map[int][]GoldLink{}
	for _, a := range as {
		if a.Census == nil {
			continue
		}
		f, err := edges(a.Page)
		if err != nil {
			return nil, err
		}
		if f == nil || f.Revision != a.Census.Revision {
			continue
		}
		if ls := itemLinks(f); len(ls) > 0 {
			links[a.Page] = ls
			pool = append(pool, goldPage(a))
		}
	}
	slices.SortFunc(pool, func(a, b *GoldPage) int {
		for _, c := range []int{strings.Compare(a.Type, b.Type), strings.Compare(a.Class, b.Class), strings.Compare(a.MSC, b.MSC)} {
			if c != 0 {
				return c
			}
		}
		return a.Page - b.Page
	})
	r := rand.New(rand.NewPCG(seed, uint64(len(GoldSetL)))) // #nosec G404 -- a seeded sample must repeat, it is not a secret
	out := systematic(pool, GoldLSize, r)
	for _, g := range out {
		g.Links = links[g.Page]
	}
	slices.SortFunc(out, func(a, b *GoldPage) int { return a.Page - b.Page })
	return out, nil
}

// GoldLMeasure is the result of MeasureGoldL.
type GoldLMeasure struct {
	Pages, Marked, Stale int
	// ByMethod and ByPredicate give the precision of the predicate. The
	// predicate of an edge is right when it is the predicate of the gold set.
	ByMethod, ByPredicate map[string]*GoldRatio
	// Errors are the links whose predicate is wrong, and Misses are the
	// links of the gold set that no edge has.
	Errors, Misses []string
}

// MeasureGoldL compares the predicates of the marked articles of gold set L
// with the edge files of the same revisions (WP11.3).
func MeasureGoldL(root string, gs []*GoldPage) (GoldLMeasure, error) {
	m := GoldLMeasure{ByMethod: map[string]*GoldRatio{}, ByPredicate: map[string]*GoldRatio{}}
	for _, g := range gs {
		m.Pages++
		if !g.Marked {
			continue
		}
		m.Marked++
		f, err := ReadEdges(root, g.Page)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && (f == nil || f.Revision != g.Revision)) {
			m.Stale++
			continue
		}
		if err != nil {
			return m, err
		}
		type key struct {
			span [2]int
			to   string
		}
		have := map[key]*Edge{}
		for i := range f.Edges {
			e := &f.Edges[i]
			have[key{e.Span, edgeTo(e)}] = e
		}
		for _, l := range g.Links {
			e := have[key{l.Span, l.To}]
			if e == nil {
				m.Misses = append(m.Misses, fmt.Sprintf("%s: %s to %s at %d", g.Title, l.Method, l.To, l.Span[0]))
				continue
			}
			right := e.Predicate == l.Predicate
			ratio(m.ByMethod, e.Method).add(right)
			ratio(m.ByPredicate, e.Predicate).add(right)
			if !right {
				m.Errors = append(m.Errors, fmt.Sprintf("%s: %s to %s at %d gives %s, and the gold predicate is %s", g.Title, e.Method, l.To, l.Span[0], e.Predicate, l.Predicate))
			}
		}
	}
	return m, nil
}

// GoldLReport writes reports/gold-l.md.
func GoldLReport(w io.Writer, m GoldLMeasure) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# Gold set L\n\n")
	fmt.Fprintf(bw, "This report is made by `m2 wp gold measure l`. Gold set L has %d articles, and %d of them are marked. %d marked articles have no edge file of the marked revision, so they are not measured. The value is the precision of the predicate.\n\n", m.Pages, m.Marked, m.Stale)
	fmt.Fprintf(bw, "| Method | Right | Total | Value | Target |\n|---|---:|---:|---:|---:|\n")
	for _, k := range sortedKeys(m.ByMethod) {
		target := "none"
		if slices.Contains(GoldLMethods, k) {
			target = fmt.Sprintf("%.3f", goldLTarget)
		}
		r := m.ByMethod[k]
		fmt.Fprintf(bw, "| %s | %d | %d | %.3f | %s |\n", k, r.Right, r.Total, r.Value(), target)
	}
	if len(m.ByPredicate) > 0 {
		fmt.Fprintf(bw, "\n## Precision by predicate\n\n| Predicate | Right | Total | Value |\n|---|---:|---:|---:|\n")
		for _, k := range sortedKeys(m.ByPredicate) {
			r := m.ByPredicate[k]
			fmt.Fprintf(bw, "| %s | %d | %d | %.3f |\n", k, r.Right, r.Total, r.Value())
		}
	}
	writeList(bw, "Errors", "The extractor gives another predicate for these links.", m.Errors)
	writeList(bw, "Misses", "No edge has these links of the gold set.", m.Misses)
	return bw.Flush()
}
