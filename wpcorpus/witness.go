package wpcorpus

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/wikipedia-cli/wiki/parse"
	"github.com/tamnd/wikipedia-cli/wiki/parsoid"
)

// Witness is the witness W-html of a census (WP4.3): the counts of the
// Parsoid HTML of the same revision.
type Witness struct {
	Revision int `json:"revision" yaml:"revision"`
	// Math is the number of formulas: the MathML elements that the page
	// writes, and the transclusions of the templates math and mvar.
	Math int `json:"math" yaml:"math"`
	// TeX is the number of <math> elements of W-text, and TeXMismatches is
	// the number of them whose TeX has no equal alttext in W-html.
	TeX           int            `json:"tex" yaml:"tex"`
	TeXMismatches int            `json:"tex_mismatches,omitempty" yaml:"tex_mismatches,omitempty"`
	ItemTemplates map[string]int `json:"item_templates,omitempty" yaml:"item_templates,omitempty"`
	Sections      int            `json:"sections" yaml:"sections"`
}

// ReadParsoid reads the Parsoid HTML of a page revision from work/parsoid.
// It gives nil and no error when the corpus does not have the HTML.
func ReadParsoid(root string, page, rev int) (*parsoid.Reading, error) {
	f, err := os.Open(filepath.Join(root, ParsoidPath(page, rev))) // #nosec G304 -- the path is in the corpus that the user names
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return parsoid.Read(f)
}

// JoinWitness adds the witness W-html to the census of r when the corpus has
// the Parsoid HTML of the revision (WP2.5).
func JoinWitness(root string, r *Result, m *Manifests) error {
	rd, err := ReadParsoid(root, r.Census.Page, r.Census.Revision)
	if err != nil || rd == nil {
		return err
	}
	return AddWitness(r, m, rd)
}

// AddWitness compares the census of r with the Parsoid reading of the same
// revision, and puts the witness W-html into the census. The strength of
// the census becomes normal when the two readings agree within the
// threshold of WP4.3.
func AddWitness(r *Result, m *Manifests, rd *parsoid.Reading) error {
	c := &r.Census
	if rd.Revision != c.Revision {
		return fmt.Errorf("the Parsoid HTML is of the revision %d, and the census is of the revision %d", rd.Revision, c.Revision)
	}
	w := &Witness{Revision: rd.Revision, ItemTemplates: map[string]int{}}
	have := map[string]int{}
	for _, f := range rd.Math {
		if !f.Transcluded {
			w.Math++
			have[NormTeX(f.AltText)]++
		}
	}
	for _, tex := range r.TeX {
		w.TeX++
		if t := NormTeX(tex); have[t] > 0 {
			have[t]--
		} else {
			w.TeXMismatches++
		}
	}
	for _, tr := range rd.Transclusions {
		for _, t := range tr.Templates {
			if w.count(m, t.Name) {
				for _, v := range t.Params {
					w.countNested(m, v)
				}
			}
		}
	}
	if len(w.ItemTemplates) == 0 {
		w.ItemTemplates = nil
	}
	for _, s := range rd.Sections {
		if s.ID > 0 {
			w.Sections++
		}
	}
	c.HTML = w
	c.Witnesses = appendOnce(c.Witnesses, WitnessHTML)
	// An anchor that only the HTML has is made by a template that W-text
	// does not read. The pointer to it is not a broken anchor.
	anchors := map[string]bool{}
	for _, a := range rd.Anchors {
		anchors[normAnchor(a)] = true
	}
	for i := range c.Pointers {
		if p := &c.Pointers[i]; p.Finding == FindingBrokenAnchor && anchors[normAnchor(p.Anchor)] {
			p.Finding = FindingTemplateAnchor
		}
	}
	c.SetStrength()
	return nil
}

// SetStrength sets the strength of the census from its witnesses (WP4.5).
// It is strong when the three witnesses were read and agree, and normal
// when W-text and one more witness were read and agree. It is weak when
// only W-text was read, or when W-html differs by more than the threshold.
// W-in agrees when each pointer matches an object.
func (c *Census) SetStrength() {
	html := slices.Contains(c.Witnesses, WitnessHTML)
	in := slices.Contains(c.Witnesses, WitnessIn)
	if html && len(c.HTMLDifferences()) > 0 {
		c.Strength = StrengthWeak
		return
	}
	agree := true
	for _, p := range c.Pointers {
		if p.Object == "" {
			agree = false
		}
	}
	switch {
	case html && in && agree:
		c.Strength = StrengthStrong
	case html || in:
		c.Strength = StrengthNormal
	default:
		c.Strength = StrengthWeak
	}
}

// count counts a template when it is a counted item template or the
// template math or mvar, as the walk of W-text does. It gives false when
// the walk does not go into the parameters of the template. The template
// tmath makes a MathML element, so the count of the MathML has it.
func (w *Witness) count(m *Manifests, name string) bool {
	t := m.Template(parse.TemplateKey(name))
	switch {
	case t == nil:
	case t.Class == ClassMath && (t.Key == "math" || t.Key == "mvar"):
		w.Math++
		return false
	case t.Class == ClassMath && t.Key == "tmath":
		return false
	case (t.Class == ClassItem || t.Class == ClassTerm) && t.Cue != "":
		w.ItemTemplates[t.Key]++
	}
	return true
}

// countNested counts the templates in the wikitext of a parameter.
func (w *Witness) countNested(m *Manifests, wikitext string) {
	if !strings.Contains(wikitext, "{{") {
		return
	}
	doc, err := parse.Parse(wikitext)
	if err != nil {
		return
	}
	parse.Walk(doc.Root, func(n *parse.Node) bool {
		if n.Kind == parse.KindTemplate {
			return w.count(m, n.Name)
		}
		return n.Kind != parse.KindTag || n.Name != "math"
	})
}

// HTMLDifferences gives each comparison of WP4.3 where W-text and W-html
// differ by more than 1 element or 2%, whichever is larger. It gives nil
// when the census has no witness W-html.
func (c *Census) HTMLDifferences() []string {
	w := c.HTML
	if w == nil {
		return nil
	}
	var out []string
	text := 0
	for _, n := range c.Counts.Math {
		text += n
	}
	if over(text, w.Math) {
		out = append(out, fmt.Sprintf("the math count is %d in W-text and %d in W-html", text, w.Math))
	}
	if over(w.TeX, w.TeX-w.TeXMismatches) {
		out = append(out, fmt.Sprintf("%d of the %d <math> elements have no equal alttext in W-html", w.TeXMismatches, w.TeX))
	}
	keys := map[string]bool{}
	for k := range c.Counts.ItemTemplates {
		keys[k] = true
	}
	for k := range w.ItemTemplates {
		keys[k] = true
	}
	for _, k := range slices.Sorted(mapKeys(keys)) {
		if a, b := c.Counts.ItemTemplates[k], w.ItemTemplates[k]; over(a, b) {
			out = append(out, fmt.Sprintf("the template {{%s}} has %d uses in W-text and %d transclusions in W-html", k, a, b))
		}
	}
	if over(c.Counts.Sections, w.Sections) {
		out = append(out, fmt.Sprintf("the page has %d sections in W-text and %d in W-html", c.Counts.Sections, w.Sections))
	}
	return out
}

// over is true when a and b differ by more than 1 or 2% of the larger one,
// whichever is larger.
func over(a, b int) bool {
	d := max(a-b, b-a)
	return d > 1 && float64(d) > 0.02*float64(max(a, b))
}

func mapKeys(m map[string]bool) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

var (
	texAliases = strings.NewReplacer(`\ne `, `\neq `, `\le `, `\leq `, `\ge `, `\geq `, "{", "", "}", "")
	texCommand = regexp.MustCompile(`\\[A-Za-z]+`)
	texScripts = regexp.MustCompile(`\^(\\[A-Za-z]+|[^\\_^])_(\\[A-Za-z]+|[^\\_^])`)
)

// NormTeX gives the form of a formula that the comparison of the TeX of
// W-text and the alttext of W-html uses. It removes the {\displaystyle …}
// of the alttext, the spaces and the braces, writes the texvc aliases \ne,
// \le and \ge as \neq, \leq and \geq, and writes x^a_b as x_b^a, because
// MediaWiki writes the alttext in that form.
func NormTeX(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{`{\displaystyle `, `{\textstyle `} {
		if strings.HasPrefix(s, p) && strings.HasSuffix(s, "}") {
			s = s[len(p) : len(s)-1]
		}
	}
	// A space after each command keeps \le x apart from \lex when the
	// spaces go.
	s = texCommand.ReplaceAllString(s, "$0 ")
	s = strings.Join(strings.Fields(texAliases.Replace(s+" ")), "")
	return texScripts.ReplaceAllString(s, "_${2}^${1}")
}
