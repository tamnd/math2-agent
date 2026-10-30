package wpcorpus

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

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
	have, inTemplates := map[string]int{}, map[string]int{}
	for _, f := range rd.Math {
		if f.Transcluded {
			inTemplates[NormTeX(f.AltText)]++
		} else {
			w.Math++
			have[NormTeX(f.AltText)]++
		}
	}
	// Parsoid marks a formula in a template parameter as transcluded when
	// the parameter has the TeX in another form. A transcluded formula with
	// the TeX of a W-text formula is a formula of the page.
	for _, tex := range r.TeX {
		w.TeX++
		switch t := NormTeX(tex); {
		case have[t] > 0:
			have[t]--
		case inTemplates[t] > 0:
			inTemplates[t]--
			w.Math++
		default:
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
	case html || in && agree:
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

// Difference is one comparison of WP4.3 where W-text and W-html differ.
// Cause names the comparison: math, tex, sections, or the key of an item
// template.
type Difference struct {
	Cause string
	Text  string
}

// The causes of a difference that are not item templates.
const (
	CauseMath     = "math count"
	CauseTeX      = "math text"
	CauseSections = "sections"
)

// HTMLDifferences gives each comparison of WP4.3 where W-text and W-html
// differ by more than 1 element or 2%, whichever is larger. It gives nil
// when the census has no witness W-html.
func (c *Census) HTMLDifferences() []string {
	var out []string
	for _, d := range c.Differences() {
		out = append(out, d.Text)
	}
	return out
}

// Differences is HTMLDifferences with the cause of each difference.
func (c *Census) Differences() []Difference {
	w := c.HTML
	if w == nil {
		return nil
	}
	var out []Difference
	add := func(cause, format string, args ...any) {
		out = append(out, Difference{cause, fmt.Sprintf(format, args...)})
	}
	text := 0
	for _, n := range c.Counts.Math {
		text += n
	}
	if over(text, w.Math) {
		add(CauseMath, "the math count is %d in W-text and %d in W-html", text, w.Math)
	}
	if over(w.TeX, w.TeX-w.TeXMismatches) {
		add(CauseTeX, "%d of the %d <math> elements have no equal alttext in W-html", w.TeXMismatches, w.TeX)
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
			add("{{"+k+"}}", "the template {{%s}} has %d uses in W-text and %d transclusions in W-html", k, a, b)
		}
	}
	if over(c.Counts.Sections, w.Sections) {
		add(CauseSections, "the page has %d sections in W-text and %d in W-html", c.Counts.Sections, w.Sections)
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
	texAliases = strings.NewReplacer(`\ne `, `\neq `, `\le `, `\leq `, `\ge `, `\geq `)
	texBraces  = strings.NewReplacer("{", "", "}", "")
	texCommand = regexp.MustCompile(`\\[A-Za-z]+`)
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
	s = texAliases.Replace(s + " ")
	// The scripts change places before the braces go, because a braced
	// script such as ^{-\infty} is one token only while it has its braces.
	return texBraces.Replace(strings.Join(strings.Fields(swapScripts(s)), ""))
}

// swapScripts writes each ^a_b of s as _b^a. A script is a braced group, a
// command or one character. The scripts in a braced group change places
// too.
func swapScripts(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '^' {
			a, j := texToken(s, i+1)
			if k := skipSpace(s, j); a != "" && k < len(s) && s[k] == '_' {
				if c, l := texToken(s, k+1); c != "" {
					b.WriteString("_" + c + "^" + a)
					i = l
					continue
				}
			}
		}
		if s[i] == '{' {
			if g, j := texToken(s, i); g != "" {
				b.WriteString(g)
				i = j
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// texToken gives the token of s that starts at i after the spaces, and the
// index after it. A braced group comes back with its scripts swapped. The
// token is empty when s has no full token at i.
func texToken(s string, i int) (string, int) {
	i = skipSpace(s, i)
	switch {
	case i >= len(s) || s[i] == '}' || s[i] == '^' || s[i] == '_':
		return "", i
	case s[i] == '{':
		depth := 0
		for j := i; j < len(s); j++ {
			switch s[j] {
			case '{':
				depth++
			case '}':
				if depth--; depth == 0 {
					return "{" + swapScripts(s[i+1:j]) + "}", j + 1
				}
			}
		}
		return "", i
	case s[i] == '\\':
		if m := texCommand.FindString(s[i:]); m != "" {
			return m, i + len(m)
		}
		if i+1 < len(s) {
			return s[i : i+2], i + 2
		}
		return "", i
	}
	_, n := utf8.DecodeRuneInString(s[i:])
	return s[i : i+n], i + n
}

func skipSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n') {
		i++
	}
	return i
}
