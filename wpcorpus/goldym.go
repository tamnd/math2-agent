package wpcorpus

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// The gold sets of WP11.2 that have a file for each article.
const (
	GoldSetY = "y"
	GoldSetM = "m"
)

// GoldVersion is the version of gold sets Y and M. A version is frozen when
// the people mark it. A new draw makes a new version.
const GoldVersion = 1

// GoldYSize is the number of articles of each type in gold set Y, and
// GoldMSize is the number of formulas of each form in gold set M.
const (
	GoldYSize = 50
	GoldMSize = 1000
)

// CueProse is the cue of an object that a model pass found in running text,
// with no cue of WP3.4.
const CueProse = "prose"

// GoldDir gives the directory of a version of a gold set.
func GoldDir(set string, version int) string {
	return filepath.Join("gold", set, "v"+strconv.Itoa(version))
}

// GoldPath gives the path of the file of a page in a gold set.
func GoldPath(set string, version, page int) string {
	return filepath.Join(GoldDir(set, version), Shard(page), strconv.Itoa(page)+".md")
}

// GoldItem is one item of an article of gold set Y.
type GoldItem struct {
	Kind string `yaml:"kind"`
	Name string `yaml:"name,omitempty"`
	Span [2]int `yaml:"span,flow"`
	// Cue is the cue of WP3.4, or "" for an item in running text.
	Cue string `yaml:"cue,omitempty"`
	// Target is the span of the item that a proof proves.
	Target *[2]int `yaml:"target,omitempty,flow"`
	// Text is the start of the text of the item, so that a person can find
	// it. The measure does not use it.
	Text string `yaml:"text,omitempty"`
}

// GoldFormula is one formula of gold set M. TeX is the correct TeX.
type GoldFormula struct {
	Form   string `yaml:"form"`
	Span   [2]int `yaml:"span,flow"`
	Source string `yaml:"source"`
	TeX    string `yaml:"tex"`
}

// GoldPage is the file of one article in gold set Y or M. The draw fills it
// with the output of the extractor as a draft. A person corrects the draft,
// adds what the extractor did not find, and sets Marked. Only the marked
// files are measured.
type GoldPage struct {
	Page     int           `yaml:"page"`
	Title    string        `yaml:"title"`
	Revision int           `yaml:"revision"`
	Type     string        `yaml:"type,omitempty"`
	Class    string        `yaml:"class,omitempty"`
	MSC      string        `yaml:"msc,omitempty"`
	Draft    string        `yaml:"draft"`
	Marked   bool          `yaml:"marked"`
	Marker   string        `yaml:"marker,omitempty"`
	Date     string        `yaml:"date,omitempty"`
	Items    []GoldItem    `yaml:"items,omitempty"`
	Formulas []GoldFormula `yaml:"formulas,omitempty"`
}

// MarshalGoldPage gives the file of an article in a gold set.
func MarshalGoldPage(set string, g *GoldPage) ([]byte, error) {
	var b bytes.Buffer
	if err := writeFrontMatter(&b, g); err != nil {
		return nil, err
	}
	fmt.Fprintf(&b, "\n# %s\n\n", g.Title)
	what := "each item of the article: the kind, the span in the wikitext, the name, the cue, and for a proof the span of its target. An item in running text with no cue has no cue field"
	if set == GoldSetM {
		what = "the correct TeX of each formula"
	}
	fmt.Fprintf(&b, "The draft is the output of the extractor %s for the revision %d (https://en.wikipedia.org/w/index.php?oldid=%d). A person checks %s. Then the person removes the wrong entries, adds the missing entries, and sets marked to true with the name of the marker and the date.\n", g.Draft, g.Revision, g.Revision, what)
	return b.Bytes(), nil
}

// WriteGoldPage writes the file of an article in a gold set.
func WriteGoldPage(root, set string, g *GoldPage) error {
	data, err := MarshalGoldPage(set, g)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(root, GoldPath(set, GoldVersion, g.Page)), data)
}

// ReadGoldPages reads the files of a gold set, in the order of the pages.
func ReadGoldPages(root, set string) ([]*GoldPage, error) {
	dir := filepath.Join(root, GoldDir(set, GoldVersion))
	var out []*GoldPage
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		data, err := os.ReadFile(path) // #nosec G304 G122 -- the path is in the corpus that the user names
		if err != nil {
			return err
		}
		head, _, err := splitFrontMatter(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		var g GoldPage
		if err := yaml.Unmarshal(head, &g); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, &g)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b *GoldPage) int { return a.Page - b.Page })
	return out, nil
}

func goldPage(a *CensusArticle) *GoldPage {
	g := &GoldPage{Page: a.Page, Title: a.Title, Revision: a.Census.Revision, Type: a.Type, Draft: a.Census.Extractor}
	if a.Meta != nil {
		g.Class = a.Meta.Class
		if len(a.Meta.MSC) > 0 && len(a.Meta.MSC[0]) >= 2 {
			g.MSC = a.Meta.MSC[0][:2]
		}
	}
	return g
}

// DrawGoldY draws gold set Y from the parsed articles: GoldYSize articles of
// each type of WP1.5. In a type, the articles are sorted by the class and the
// MSC code, and the draw takes them at equal steps from a random start, so
// the classes and the codes are in proportion to the scope (WP11.2).
func DrawGoldY(as []*CensusArticle, seed uint64) []*GoldPage {
	by := map[string][]*CensusArticle{}
	for _, a := range as {
		if a.Census != nil {
			by[a.Type] = append(by[a.Type], a)
		}
	}
	var out []*GoldPage
	for _, t := range Types {
		pool := by[t]
		gs := make([]*GoldPage, len(pool))
		for i, a := range pool {
			gs[i] = goldPage(a)
		}
		slices.SortFunc(gs, func(a, b *GoldPage) int {
			if c := strings.Compare(a.Class, b.Class); c != 0 {
				return c
			}
			if c := strings.Compare(a.MSC, b.MSC); c != 0 {
				return c
			}
			return a.Page - b.Page
		})
		byPage := map[int]*CensusArticle{}
		for _, a := range pool {
			byPage[a.Page] = a
		}
		r := rand.New(rand.NewPCG(seed, uint64(len(t)))) // #nosec G404 -- a seeded sample must repeat, it is not a secret
		for _, g := range systematic(gs, GoldYSize, r) {
			g.Items = draftItems(byPage[g.Page].Objects)
			out = append(out, g)
		}
	}
	slices.SortFunc(out, func(a, b *GoldPage) int { return a.Page - b.Page })
	return out
}

// systematic takes n elements at equal steps from a random start, or all
// the elements when there are n or fewer.
func systematic[T any](xs []T, n int, r *rand.Rand) []T {
	if len(xs) <= n {
		return xs
	}
	step := float64(len(xs)) / float64(n)
	start := r.Float64() * step
	out := make([]T, 0, n)
	for i := range n {
		out = append(out, xs[int(start+float64(i)*step)])
	}
	return out
}

// draftItems gives the items of the objects as the draft of gold set Y.
func draftItems(objs []Object) []GoldItem {
	span := map[string][2]int{}
	for _, o := range objs {
		span[o.Ref] = o.Span
	}
	var out []GoldItem
	for i := range objs {
		o := &objs[i]
		if !IsItem(o) {
			continue
		}
		g := GoldItem{Kind: o.Kind, Name: o.Name, Span: o.Span, Cue: o.Cue, Text: excerpt(o.Text, 80)}
		if o.Cue == CueProse {
			g.Cue = ""
		}
		if s, ok := span[o.AttachedTo]; ok && o.AttachedTo != "" && kindFamily(o.Kind) == "proof" {
			g.Target = &s
		}
		out = append(out, g)
	}
	return out
}

// excerpt gives the first n runes of a text on one line, with "..." when
// the text is longer.
func excerpt(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimSpace(string(r[:n])) + "..."
}

// mathForm gives the stratum of gold set M of a formula form. tmath is TeX,
// as <math> is, so it is in the same stratum.
func mathForm(form string) string {
	if form == "tmath" {
		return "math"
	}
	return form
}

// GoldMForms are the strata of gold set M: the three forms of WP3.3.
var GoldMForms = []string{"math", "template", "mvar"}

// DrawGoldM draws gold set M: GoldMSize formulas of each form of WP3.3 from
// the objects of the parsed articles. A formula that two objects hold is
// one formula.
func DrawGoldM(as []*CensusArticle, seed uint64) []*GoldPage {
	type formula struct {
		a *CensusArticle
		m Math
	}
	by := map[string][]formula{}
	for _, a := range as {
		if a.Census == nil {
			continue
		}
		seen := map[[2]int]bool{}
		for _, o := range a.Objects {
			for _, m := range o.Math {
				if !seen[m.Span] {
					seen[m.Span] = true
					by[mathForm(m.Form)] = append(by[mathForm(m.Form)], formula{a, m})
				}
			}
		}
	}
	pages := map[int]*GoldPage{}
	for _, form := range GoldMForms {
		pool := by[form]
		slices.SortFunc(pool, func(x, y formula) int {
			if x.a.Page != y.a.Page {
				return x.a.Page - y.a.Page
			}
			return x.m.Span[0] - y.m.Span[0]
		})
		r := rand.New(rand.NewPCG(seed, uint64(len(form)))) // #nosec G404 -- a seeded sample must repeat, it is not a secret
		r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
		for _, f := range pool[:min(GoldMSize, len(pool))] {
			g := pages[f.a.Page]
			if g == nil {
				g = goldPage(f.a)
				pages[f.a.Page] = g
			}
			g.Formulas = append(g.Formulas, GoldFormula{Form: f.m.Form, Span: f.m.Span, Source: f.m.Source, TeX: f.m.TeX})
		}
	}
	var out []*GoldPage
	for _, g := range pages {
		slices.SortFunc(g.Formulas, func(x, y GoldFormula) int { return x.Span[0] - y.Span[0] })
		out = append(out, g)
	}
	slices.SortFunc(out, func(a, b *GoldPage) int { return a.Page - b.Page })
	return out
}

// GoldRatio is a count of right answers out of a total.
type GoldRatio struct{ Right, Total int }

func (r *GoldRatio) add(ok bool) {
	r.Total++
	if ok {
		r.Right++
	}
}

// Value gives the ratio, or 0 when the total is 0.
func (r GoldRatio) Value() float64 {
	if r.Total == 0 {
		return 0
	}
	return float64(r.Right) / float64(r.Total)
}

// GoldYMeasure is the result of MeasureGoldY.
type GoldYMeasure struct {
	Pages, Marked int
	// Stale counts the marked articles whose objects file of the marked
	// revision is not in the corpus.
	Stale                       int
	CueRecall, CuePrecision     GoldRatio
	ProseRecall, ProsePrecision GoldRatio
	Attachment                  GoldRatio
	// ProseByType and ProseByMSC give the prose recall for rule Y17.
	ProseByType, ProseByMSC map[string]*GoldRatio
	// Misses are the items of the gold set that no object matches.
	Misses []string
}

// overlap gives the length of the common part of two spans.
func overlap(a, b [2]int) int {
	return max(0, min(a[1], b[1])-max(a[0], b[0]))
}

// kindFamily gives the kind of an item without its subkind.
func kindFamily(k string) string {
	f, _, _ := strings.Cut(k, "/")
	return f
}

// matches tells if an object is the item of the gold set: the same kind
// and spans that overlap by half of the shorter one.
func matches(g *GoldItem, o *Object) bool {
	n := min(g.Span[1]-g.Span[0], o.Span[1]-o.Span[0])
	return kindFamily(g.Kind) == kindFamily(o.Kind) && n > 0 && 2*overlap(g.Span, o.Span) >= n
}

// MeasureGoldY compares the marked articles of gold set Y with the objects
// of the same revisions (WP11.3).
func MeasureGoldY(root string, gs []*GoldPage) (GoldYMeasure, error) {
	m := GoldYMeasure{ProseByType: map[string]*GoldRatio{}, ProseByMSC: map[string]*GoldRatio{}}
	for _, g := range gs {
		m.Pages++
		if !g.Marked {
			continue
		}
		m.Marked++
		objs, err := ReadObjects(root, g.Page, g.Revision)
		if errors.Is(err, fs.ErrNotExist) {
			m.Stale++
			continue
		}
		if err != nil {
			return m, err
		}
		var items []*Object
		for i := range objs {
			if IsItem(&objs[i]) {
				items = append(items, &objs[i])
			}
		}
		// Each item of the gold set takes the free object with the largest
		// overlap.
		of := make([]*Object, len(g.Items))
		used := map[*Object]bool{}
		for i := range g.Items {
			best := 0
			for _, o := range items {
				if n := overlap(g.Items[i].Span, o.Span); !used[o] && matches(&g.Items[i], o) && n > best {
					best, of[i] = n, o
				}
			}
			if of[i] != nil {
				used[of[i]] = true
			}
		}
		for i, it := range g.Items {
			found := of[i] != nil
			if it.Cue != "" {
				m.CueRecall.add(found)
			} else {
				m.ProseRecall.add(found)
				ratio(m.ProseByType, g.Type).add(found)
				if g.MSC != "" {
					ratio(m.ProseByMSC, g.MSC).add(found)
				}
			}
			if !found {
				m.Misses = append(m.Misses, fmt.Sprintf("%s: %s %q at %d", g.Title, it.Kind, it.Name, it.Span[0]))
			}
			if kindFamily(it.Kind) == "proof" && it.Target != nil {
				var target *Object
				for j := range g.Items {
					if g.Items[j].Span == *it.Target {
						target = of[j]
					}
				}
				m.Attachment.add(found && target != nil && of[i].AttachedTo == target.Ref)
			}
		}
		for _, o := range items {
			if o.Cue == CueProse {
				m.ProsePrecision.add(used[o])
			} else {
				m.CuePrecision.add(used[o])
			}
		}
	}
	return m, nil
}

func ratio(m map[string]*GoldRatio, key string) *GoldRatio {
	if m[key] == nil {
		m[key] = &GoldRatio{}
	}
	return m[key]
}

// GoldMMeasure is the result of MeasureGoldM.
type GoldMMeasure struct {
	Pages, Marked, Stale int
	ByForm               map[string]*GoldRatio
	// Errors are the formulas whose TeX is not the TeX of the gold set.
	Errors []string
}

// normTeX gives the form of a TeX that the measure compares: the spaces
// are one space.
func normTeX(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// MeasureGoldM compares the TeX of the marked formulas of gold set M with
// the TeX of the objects of the same revisions (WP11.3).
func MeasureGoldM(root string, gs []*GoldPage) (GoldMMeasure, error) {
	m := GoldMMeasure{ByForm: map[string]*GoldRatio{}}
	for _, g := range gs {
		m.Pages++
		if !g.Marked {
			continue
		}
		m.Marked++
		objs, err := ReadObjects(root, g.Page, g.Revision)
		if errors.Is(err, fs.ErrNotExist) {
			m.Stale++
			continue
		}
		if err != nil {
			return m, err
		}
		tex := map[[2]int]string{}
		for _, o := range objs {
			for _, f := range o.Math {
				tex[f.Span] = f.TeX
			}
		}
		for _, f := range g.Formulas {
			have, ok := tex[f.Span]
			right := ok && normTeX(have) == normTeX(f.TeX)
			ratio(m.ByForm, mathForm(f.Form)).add(right)
			if !right {
				m.Errors = append(m.Errors, fmt.Sprintf("%s: %s gives %s, and the gold TeX is %s", g.Title, f.Source, have, f.TeX))
			}
		}
	}
	return m, nil
}

// The targets of WP11.3.
var goldTargets = map[string]float64{"cue recall": 0.995, "cue precision": 0.99, "prose recall": 0.90, "prose precision": 0.90, "attachment": 0.98, "math": 0.995, "template": 0.98}

func writeRatio(w io.Writer, name string, r GoldRatio) {
	target := "none"
	if t, ok := goldTargets[name]; ok {
		target = fmt.Sprintf("%.3f", t)
	}
	fmt.Fprintf(w, "| %s | %d | %d | %.3f | %s |\n", name, r.Right, r.Total, r.Value(), target)
}

func writeList(w io.Writer, heading, intro string, rows []string) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(w, "\n## %s\n\n%s", heading, intro)
	if len(rows) > reportRows {
		fmt.Fprintf(w, " The list shows %d of %d.", reportRows, len(rows))
	}
	fmt.Fprintf(w, "\n\n")
	for _, r := range rows[:min(reportRows, len(rows))] {
		fmt.Fprintf(w, "- %s\n", strings.ReplaceAll(r, "\n", " "))
	}
}

// GoldYReport writes reports/gold-y.md.
func GoldYReport(w io.Writer, m GoldYMeasure) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# Gold set Y\n\n")
	fmt.Fprintf(bw, "This report is made by `m2 wp gold measure y`. Gold set Y has %d articles, and %d of them are marked. %d marked articles have no objects file of the marked revision, so they are not measured.\n\n", m.Pages, m.Marked, m.Stale)
	fmt.Fprintf(bw, "| Measure | Right | Total | Value | Target |\n|---|---:|---:|---:|---:|\n")
	writeRatio(bw, "cue recall", m.CueRecall)
	writeRatio(bw, "cue precision", m.CuePrecision)
	writeRatio(bw, "prose recall", m.ProseRecall)
	writeRatio(bw, "prose precision", m.ProsePrecision)
	writeRatio(bw, "attachment", m.Attachment)
	for _, by := range []struct {
		name string
		m    map[string]*GoldRatio
	}{{"article type", m.ProseByType}, {"MSC code", m.ProseByMSC}} {
		if len(by.m) == 0 {
			continue
		}
		fmt.Fprintf(bw, "\n## Prose recall by %s\n\nRule Y17 reports a value below 0.900.\n\n| %s | Found | Items | Recall |\n|---|---:|---:|---:|\n", by.name, by.name)
		for _, k := range sortedKeys(by.m) {
			r := by.m[k]
			fmt.Fprintf(bw, "| %s | %d | %d | %.3f |\n", k, r.Right, r.Total, r.Value())
		}
	}
	writeList(bw, "Misses", "No object matches these items of the gold set.", m.Misses)
	return bw.Flush()
}

// GoldMReport writes reports/gold-m.md.
func GoldMReport(w io.Writer, m GoldMMeasure) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# Gold set M\n\n")
	fmt.Fprintf(bw, "This report is made by `m2 wp gold measure m`. Gold set M has formulas from %d articles, and %d of the articles are marked. %d marked articles have no objects file of the marked revision, so they are not measured.\n\n", m.Pages, m.Marked, m.Stale)
	fmt.Fprintf(bw, "| Form | Right | Total | Value | Target |\n|---|---:|---:|---:|---:|\n")
	for _, f := range GoldMForms {
		if r := m.ByForm[f]; r != nil {
			writeRatio(bw, f, *r)
		}
	}
	writeList(bw, "Errors", "The extractor gives another TeX for these formulas.", m.Errors)
	return bw.Flush()
}
