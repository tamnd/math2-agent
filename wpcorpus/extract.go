package wpcorpus

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/tamnd/wikipedia-cli/wiki/parse"
	"github.com/tamnd/wikipedia-cli/wiki/texvc"
)

// Result is the output of Extract for one page revision.
type Result struct {
	Objects []Object
	Census  Census
}

// ArticleType gives the article type of WP1.5 that the title shows. The
// types that need Wikidata (concept, person and the others) are "article"
// here.
func ArticleType(title string) string {
	t := strings.ToLower(title)
	switch {
	case strings.HasPrefix(t, "glossary of "):
		return "glossary"
	case strings.HasPrefix(t, "list of "), strings.HasPrefix(t, "lists of "):
		return "list"
	case strings.HasPrefix(t, "outline of "):
		return "outline"
	case strings.HasPrefix(t, "timeline of "):
		return "timeline"
	}
	if w := resultWord.FindString(strings.ToLower(baseTitle(title))); w != "" {
		return "statement"
	}
	return "article"
}

// resultWord finds a word that names a result in a title or a heading.
var resultWord = regexp.MustCompile(`\b(theorem|lemma|conjecture|corollary|proposition|identity|inequality|formula|principle|law|axiom|hypothesis|criterion|paradox)s?\b`)

// headWord finds an inline head: a bold text or a definition-list term such
// as "Theorem.", "Lemma 2", "Definition (group)" or "Proof:".
var headWord = regexp.MustCompile(`(?i)^(theorem|lemma|proposition|corollary|definition|conjecture|example|proof|claim|axiom|remark|problem|exercise)\b[^.:]{0,60}[.:]?$`)

type nameKind struct {
	re   *regexp.Regexp
	kind string
}

var nameKinds []nameKind

func init() {
	// The order matters: "Proof of Zorn's lemma" is a proof, and
	// "Counterexample to the theorem" is a counterexample.
	for _, k := range [][2]string{
		{"proof", "proof"},
		{"counterexample", "example/counterexample"},
		{"lemma", "statement/lemma"},
		{"corollary", "statement/corollary"},
		{"proposition", "statement/proposition"},
		{"conjecture", "statement/conjecture"},
		{"hypothesis", "statement/conjecture"},
		{"axiom", "statement/axiom"},
		{"claim", "statement/claim"},
		{"definition", "definition"},
		{"example", "example"},
		{"remark", "remark"},
		{"problem", "problem"},
		{"exercise", "exercise"},
		{"theorem", "statement/theorem"},
	} {
		nameKinds = append(nameKinds, nameKind{regexp.MustCompile(`\b` + k[0] + `s?\b`), k[1]})
	}
}

// kindOfName gives the kind that a name such as "Lemma 2" or "Sylow
// theorems" shows, or def when the name shows none.
func kindOfName(name, def string) string {
	s := strings.ToLower(name)
	for _, k := range nameKinds {
		if k.re.MatchString(s) {
			return k.kind
		}
	}
	if resultWord.MatchString(s) {
		return "statement"
	}
	return def
}

var roleKinds = map[string]string{
	"item/definition":     "definition",
	"item/statement":      "statement",
	"item/proof":          "proof",
	"item/example":        "example",
	"item/counterexample": "example/counterexample",
	"item/property":       "statement/fact",
	"item/generalization": "statement/generalization",
	"item/construction":   "definition/construction",
	"item/conjecture":     "statement/conjecture",
	"item/notation":       "definition/notation",
	"item/application":    "application",
}

type section struct {
	node   *parse.Node
	path   string
	anchor string
	role   string
	item   *object
	parent *section
}

type object struct {
	Object
	target *object
	order  int
}

type cue struct {
	Cue
	obj *object
}

type extractor struct {
	doc  *parse.Document
	m    *Manifests
	page *Page
	typ  string
	ver  string

	objs     []*object
	cues     []*cue
	secs     []*section
	counts   Counts
	unmapped map[string]int
	unheads  map[string]bool
	refs     map[string]*object
}

// Extract reads the wikitext of a page and gives its objects and its census
// (WP3 and WP4). version names the version of the extractor, and it goes
// into each record.
func Extract(p *Page, m *Manifests, version string) (*Result, error) {
	doc, err := parse.Parse(p.Wikitext)
	if err != nil {
		return nil, fmt.Errorf("page %d: %w", p.ID, err)
	}
	x := &extractor{
		doc: doc, m: m, page: p, typ: ArticleType(p.Title), ver: version,
		unmapped: map[string]int{}, unheads: map[string]bool{}, refs: map[string]*object{},
		counts: Counts{ItemTemplates: map[string]int{}, ItemHeadings: map[string]int{}, Math: map[string]int{}, Maintenance: map[string]int{}},
	}
	x.sections(doc.Root, nil)
	x.lead()
	x.blocks()
	x.templates()
	x.attachProofs()
	return x.result(), nil
}

// sections walks the section tree. It makes an object for each section, and
// an item object for each item heading and each heading that names an item.
func (x *extractor) sections(n *parse.Node, parent *section) {
	for _, c := range n.Children {
		if c.Kind != parse.KindSection {
			continue
		}
		s := &section{node: c, parent: parent}
		h := x.doc.HeadingText(c)
		if parent != nil {
			s.path = parent.path
		}
		if h != "" {
			if s.path != "" {
				s.path += " > "
			}
			s.path += h
			s.anchor = parse.Anchor(h)
			s.role = x.m.HeadingRole(h, x.page.Title)
			if s.role == "" && strings.HasPrefix(strings.ToLower(h), "proof") {
				s.role = "item/proof"
			}
		}
		x.secs = append(x.secs, s)
		if h != "" {
			x.heading(s, h)
		}
		x.sections(c, s)
	}
}

func (x *extractor) heading(s *section, h string) {
	sec := x.add(Object{Kind: "section", Cue: "section", Strength: StrengthStrong, Name: h}, s.node.Start, s.node.End)
	sec.Text = ""
	if x.inBack(s) {
		return
	}
	switch {
	case strings.HasPrefix(s.role, "item/"):
		x.counts.ItemHeadings[s.role]++
		s.item = x.item("heading", "", s.role, Object{Kind: roleKinds[s.role], Cue: "heading", Strength: StrengthStrong, Name: h}, s.node.Start, s.node.End)
	case s.role == "" && x.typ != "list" && x.typ != "glossary" && resultWord.MatchString(strings.ToLower(h)):
		x.counts.NamedHeadings++
		s.item = x.item("named-heading", "", "", Object{Kind: kindOfName(h, "statement"), Cue: "named-heading", Strength: StrengthNormal, Name: h}, s.node.Start, s.node.End)
	case s.role == "":
		x.unheads[normalHeading(h, x.page.Title)] = true
	}
	if s.item != nil {
		for p := s.parent; p != nil; p = p.parent {
			if p.item != nil {
				s.item.target = p.item
				break
			}
		}
	}
}

func (x *extractor) inBack(s *section) bool {
	for ; s != nil; s = s.parent {
		if s.role == "section/back" || s.role == "section/works" {
			return true
		}
	}
	return false
}

// lead makes the subject object from the first paragraph of the lead that
// has text.
func (x *extractor) lead() {
	if len(x.doc.Root.Children) == 0 {
		return
	}
	l := x.doc.Root.Children[0]
	if l.Kind != parse.KindSection || l.Level != 0 {
		return
	}
	for _, c := range l.Children {
		if c.Kind == parse.KindParagraph && x.doc.Plain(c.Children) != "" {
			o := x.item("lead", "", "", Object{Kind: "subject", Cue: "lead", Strength: StrengthNormal, Name: baseTitle(x.page.Title)}, c.Start, c.End)
			if o != nil && len(o.Math) == 0 {
				o.Flags = append(o.Flags, "informal")
			}
			return
		}
	}
}

// blocks reads the blocks of each section: inline heads, glossary and list
// entries, and table rows.
func (x *extractor) blocks() {
	for _, s := range x.secs {
		if x.inBack(s) {
			continue
		}
		var heads []*parse.Node
		for _, c := range s.node.Children {
			switch c.Kind {
			case parse.KindParagraph:
				if x.isHead(c) {
					heads = append(heads, c)
				}
			case parse.KindList:
				x.list(c, &heads)
			case parse.KindTable:
				x.table(c)
			}
		}
		end := s.node.End
		for _, c := range s.node.Children {
			if c.Kind == parse.KindSection {
				end = c.Start
				break
			}
		}
		for i, h := range heads {
			e := end
			if i+1 < len(heads) {
				e = heads[i+1].Start
			}
			name := strings.TrimRight(x.headText(h), ".: ")
			x.counts.InlineHeads++
			x.item("inline-head", "", "", Object{Kind: kindOfName(name, "statement"), Cue: "inline-head", Strength: StrengthNormal, Name: name}, h.Start, e)
		}
	}
}

func (x *extractor) headText(n *parse.Node) string {
	if n.Kind == parse.KindListItem {
		return x.doc.Plain(n.Children)
	}
	for _, c := range n.Children {
		if c.Kind == parse.KindText && strings.TrimSpace(x.doc.Raw(c)) == "" {
			continue
		}
		if c.Kind == parse.KindBold {
			return x.doc.Plain(c.Children)
		}
		return ""
	}
	return ""
}

func (x *extractor) isHead(n *parse.Node) bool {
	return headWord.MatchString(strings.TrimSpace(x.headText(n)))
}

func (x *extractor) list(l *parse.Node, heads *[]*parse.Node) {
	items := l.Children
	for i, it := range items {
		switch {
		case it.Kind != parse.KindListItem:
		case it.Name == ";" && x.typ == "glossary":
			e := i + 1
			for e < len(items) && items[e].Name != ";" {
				e++
			}
			x.counts.GlossaryEntries++
			x.item("glossary-term", "", "", Object{Kind: "entry", Cue: "glossary-term", Strength: StrengthNormal, Name: x.doc.Plain(it.Children)}, it.Start, items[e-1].End)
		case it.Name == ";" && x.isHead(it):
			*heads = append(*heads, it)
		case (it.Name == "*" || it.Name == "#") && x.typ == "list" && x.firstLink(it.Children) != nil:
			x.counts.ListEntries++
			x.item("list-entry", "", "", Object{Kind: "entry", Cue: "list-entry", Strength: StrengthNormal, Name: x.linkName(x.firstLink(it.Children))}, it.Start, it.End)
		}
	}
}

func (x *extractor) table(t *parse.Node) {
	if !strings.Contains(t.Attrs["class"], "wikitable") {
		return
	}
	tab := x.add(Object{Kind: "table", Cue: "table", Strength: StrengthStrong}, t.Start, t.End)
	for _, c := range t.Children {
		if c.Kind == parse.KindCaption {
			tab.Name = x.doc.Plain(c.Children)
		}
	}
	for _, r := range t.Children {
		if r.Kind != parse.KindRow || len(r.Children) == 0 {
			continue
		}
		header := true
		for _, c := range r.Children {
			if !c.Header {
				header = false
			}
		}
		first := r.Children[0]
		link := x.firstLink(first.Children)
		if header || link == nil {
			continue
		}
		x.counts.TableRows++
		o := x.item("table-row", "", "", Object{Kind: "table-row", Cue: "table-row", Strength: StrengthNormal, Name: x.doc.Plain(first.Children)}, r.Start, r.End)
		if o != nil {
			o.target = tab
		}
	}
}

func (x *extractor) firstLink(nodes []*parse.Node) *parse.Node {
	var out *parse.Node
	for _, n := range nodes {
		parse.Walk(n, func(m *parse.Node) bool {
			if out != nil || m.Kind == parse.KindTemplate || (m.Kind == parse.KindTag && m.Name == "ref") {
				return false
			}
			if m.Kind == parse.KindLink && linkTarget(m.Name) != nil {
				out = m
			}
			return out == nil
		})
	}
	return out
}

func (x *extractor) linkName(n *parse.Node) string {
	if len(n.Children) > 0 {
		return x.doc.Plain(n.Children) + n.Trail
	}
	return linkTarget(n.Name).Title
}

// templates walks each template and tag of the page. It counts the math, the
// citations and the flags, and it makes the objects of the item, term,
// anchor, infobox and citation templates.
func (x *extractor) templates() {
	var term *object
	var collapse *parse.Node
	parse.Walk(x.doc.Root, func(n *parse.Node) bool {
		switch n.Kind {
		case parse.KindTag:
			switch n.Name {
			case "math":
				x.counts.Math["math"]++
			case "chem", "ce":
				x.counts.Math["chem"]++
			case "ref":
				if !n.SelfClosing && n.InnerEnd > n.InnerStart {
					x.counts.Citations++
					o := x.add(Object{Kind: "citation", Cue: "ref", Strength: StrengthStrong, Name: n.Attrs["name"]}, n.Start, n.End)
					o.Fields = x.citeFields(n.Children)
					x.fill(o)
					if o.Name != "" {
						x.refs[o.Name] = o
					}
				}
				return false
			}
		case parse.KindTemplate:
			key := parse.TemplateKey(n.Name)
			if strings.HasPrefix(key, "#") || strings.Contains(key, ":") || key == "" {
				return true
			}
			t := x.m.Template(key)
			if t == nil {
				x.unmapped[key]++
				return true
			}
			switch t.Class {
			case ClassMath:
				switch t.Key {
				case "math":
					x.counts.Math["template"]++
					return false
				case "mvar", "tmath":
					x.counts.Math[t.Key]++
					return false
				}
			case ClassMaintenance:
				x.counts.Maintenance[t.Key]++
			case ClassAnchor:
				x.counts.Anchors++
				x.anchor(n)
			case ClassCitation:
				// The walk does not go into a <ref>, so a citation
				// template here is a short citation or an entry of
				// the bibliography.
				cueName := ""
				switch {
				case strings.HasPrefix(t.Key, "sfn"), strings.HasPrefix(t.Key, "harv"):
					cueName = t.Key
				case strings.HasPrefix(t.Key, "cite "), t.Key == "citation":
					cueName = "bibliography"
				}
				if cueName != "" {
					x.counts.Citations++
					o := x.add(Object{Kind: "citation", Cue: cueName, Strength: StrengthStrong}, n.Start, n.End)
					o.Fields = x.fields(n)
					x.fill(o)
				}
				return false
			case ClassMeta:
				if strings.HasPrefix(t.Key, "infobox ") {
					o := x.add(Object{Kind: "infobox", Cue: "infobox", Strength: StrengthStrong, Name: t.Key}, n.Start, n.End)
					o.Fields = x.fields(n)
				}
			case ClassLayout:
				switch t.Key {
				case "collapse top", "hidden begin":
					title := x.paramText(n, "title", "1", "header")
					if strings.Contains(strings.ToLower(title), "proof") {
						collapse = n
					}
				case "collapse bottom", "hidden end":
					if collapse != nil {
						x.counts.ProofBlocks++
						title := strings.TrimSpace(x.paramText(collapse, "title", "1", "header"))
						o := x.item("collapse", t.Key, "", Object{Kind: "proof", Cue: "collapse", Strength: StrengthNormal, Name: title}, collapse.Start, n.End)
						if o != nil {
							o.Flags = append(o.Flags, "collapsed")
						}
						collapse = nil
					}
				}
			case ClassItem, ClassTerm:
				x.itemTemplate(n, t, &term)
			}
		}
		return true
	})
}

func (x *extractor) itemTemplate(n *parse.Node, t *Template, term **object) {
	counted := t.Cue != ""
	strength := t.Cue
	if !counted {
		strength = StrengthNormal
	}
	if counted {
		x.counts.ItemTemplates[t.Key]++
	}
	cueName := strings.ReplaceAll(t.Key, " ", "-")
	o := Object{Kind: t.Kind, Cue: cueName, Strength: strength}
	start, end := n.Start, n.End
	span := func(names ...string) {
		for _, name := range names {
			if p := n.Param(name); p != nil && x.doc.ParamValue(p) != "" {
				start, end = p.ValueStart, p.End
				return
			}
		}
	}
	switch t.Key {
	case "math theorem":
		// The template reads {{math theorem|Statement}} and
		// {{math theorem|Name|Statement}}. With no name, it shows
		// "Theorem".
		if n.Param("2") != nil && n.Param("math_statement") == nil {
			o.Name = x.paramText(n, "name", "1")
			span("2")
		} else {
			o.Name = x.paramText(n, "name")
			span("math_statement", "1", "2")
		}
		o.Kind = kindOfName(o.Name, t.Kind)
		if o.Name == "" {
			o.Name = "Theorem"
		}
		if note := x.paramText(n, "note"); note != "" {
			o.Name += " (" + note + ")"
		}
	case "math proof":
		o.Name = x.paramText(n, "title")
		span("proof", "1")
	case "infobox mathematical statement":
		o.Name = x.paramText(n, "name")
		if o.Name == "" {
			o.Name = baseTitle(x.page.Title)
		}
		o.Kind = kindOfName(x.paramText(n, "type"), t.Kind)
		o.Fields = x.fields(n)
	case "unsolved":
		span("2")
	case "numblk":
		x.counts.NumberedBlocks++
		if p := n.Param("3"); p != nil {
			o.Name = x.doc.Plain(p.Value)
			for _, v := range p.Value {
				if v.Kind == parse.KindTemplate {
					if q := v.Param("1"); q != nil {
						o.Name = x.doc.ParamValue(q)
					}
				}
			}
		}
		span("2")
	case "term":
		x.counts.GlossaryEntries++
		o.Kind = "entry"
		o.Name = x.paramText(n, "term", "1")
		*term = x.item(cueName, t.Key, "", o, start, end)
		x.cues[len(x.cues)-1].Span = [2]int{n.Start, n.End}
		return
	case "defn":
		if *term != nil {
			x.addCue(cueName, o.Kind, strength, n.Start, n.End, *term)
			(*term).Span[1] = n.End
			x.fill(*term)
			return
		}
		span("defn", "1")
	case "dfn", "dfni":
		o.Name = x.paramText(n, "1")
		if p := x.enclosing(n); p != nil {
			start, end = p.Start, p.End
		}
	default:
		o.Name = x.paramText(n, "name", "title")
	}
	if !counted {
		x.add(o, start, end)
		return
	}
	x.item(cueName, t.Key, "", o, start, end)
	x.cues[len(x.cues)-1].Span = [2]int{n.Start, n.End}
}

// enclosing returns the paragraph or list item that holds a node.
func (x *extractor) enclosing(n *parse.Node) *parse.Node {
	var out *parse.Node
	parse.Walk(x.doc.Root, func(m *parse.Node) bool {
		if m.Start > n.Start || m.End < n.End {
			return false
		}
		if m.Kind == parse.KindParagraph || m.Kind == parse.KindListItem || m.Kind == parse.KindCell {
			out = m
		}
		return true
	})
	return out
}

// anchor gives the anchor name to the innermost object that holds the
// template.
func (x *extractor) anchor(n *parse.Node) {
	p := n.Param("1")
	if p == nil {
		return
	}
	name := x.doc.ParamValue(p)
	var best *object
	for _, o := range x.objs {
		if o.Kind != "citation" && o.Span[0] <= n.Start && n.End <= o.Span[1] && (best == nil || o.Span[1]-o.Span[0] <= best.Span[1]-best.Span[0]) {
			best = o
		}
	}
	if best != nil && best.Anchor == "" {
		best.Anchor = name
	}
}

func (x *extractor) paramText(n *parse.Node, names ...string) string {
	for _, name := range names {
		if p := n.Param(name); p != nil {
			if s := x.doc.Plain(p.Value); s != "" {
				return s
			}
		}
	}
	return ""
}

// fields gives the parameters of a template as plain text, for a citation
// or an infobox.
func (x *extractor) fields(n *parse.Node) map[string]string {
	out := map[string]string{}
	for _, p := range n.Params {
		k := p.Name
		if k == "" {
			k = fmt.Sprint(p.Index)
		}
		if v := strings.TrimSpace(x.doc.Plain(p.Value)); v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// citeFields gives the fields of the first citation template in a <ref>, or
// the text of the ref when it has no template.
func (x *extractor) citeFields(nodes []*parse.Node) map[string]string {
	for _, n := range nodes {
		var tpl *parse.Node
		parse.Walk(n, func(m *parse.Node) bool {
			if tpl == nil && m.Kind == parse.KindTemplate {
				if t := x.m.Template(parse.TemplateKey(m.Name)); t != nil && t.Class == ClassCitation {
					tpl = m
				}
			}
			return tpl == nil
		})
		if tpl != nil {
			f := x.fields(tpl)
			if f == nil {
				f = map[string]string{}
			}
			f["template"] = parse.TemplateKey(tpl.Name)
			return f
		}
	}
	if s := x.doc.Plain(nodes); s != "" {
		return map[string]string{"text": s}
	}
	return nil
}

// item makes the object of a counted cue and the cue of the census. When a
// waiver matches the cue, it makes no object and the cue names the waiver.
func (x *extractor) item(cueName, template, role string, o Object, start, end int) *object {
	if w := x.m.Waive(template, role, x.typ); w != nil && (template != "" || role != "") {
		c := x.addCue(cueName, o.Kind, o.Strength, start, end, nil)
		c.Waiver, c.Reason = w.ID, w.Reason
		return nil
	}
	r := x.add(o, start, end)
	x.addCue(cueName, o.Kind, o.Strength, start, end, r)
	return r
}

func (x *extractor) addCue(cueName, kind, strength string, start, end int, o *object) *cue {
	c := &cue{Cue: Cue{Cue: cueName, Kind: kind, Strength: strength, Span: [2]int{start, end}}, obj: o}
	x.cues = append(x.cues, c)
	return c
}

// add makes an object with the given span and fills its section, text,
// math, links, flags and hash.
func (x *extractor) add(o Object, start, end int) *object {
	r := &object{Object: o, order: len(x.objs)}
	r.Page, r.Revision, r.Extractor = x.page.ID, x.page.Revision, x.ver
	r.Span = [2]int{start, end}
	if s := x.sectionAt(start); s != nil {
		r.Section, r.SectionAnchor = s.path, s.anchor
	}
	x.fill(r)
	x.objs = append(x.objs, r)
	return r
}

func (x *extractor) sectionAt(off int) *section {
	var out *section
	for _, s := range x.secs {
		if s.node.Start <= off && off < s.node.End {
			out = s
		}
	}
	return out
}

// fill sets the text, the math, the links, the flags and the hash of an
// object from its span.
func (x *extractor) fill(r *object) {
	nodes := x.nodesIn(x.doc.Root, r.Span[0], r.Span[1], nil)
	r.Math, r.Links = nil, nil
	flags := map[string]bool{}
	for _, f := range r.Flags {
		flags[f] = true
	}
	seen := map[Link]bool{}
	for _, n := range nodes {
		parse.Walk(n, func(m *parse.Node) bool {
			switch m.Kind {
			case parse.KindTag:
				switch m.Name {
				case "math":
					res := texvc.FromMathTag(x.doc.Inner(m), m.Attrs)
					r.Math = append(r.Math, Math{Form: "math", TeX: res.TeX, Source: x.doc.Raw(m), Display: res.Display, Span: [2]int{m.Start, m.End}, Errors: res.Errors})
				case "ref":
					if name := m.Attrs["name"]; name != "" {
						r.Citations = appendOnce(r.Citations, "ref:"+name)
					}
					return false
				}
			case parse.KindLink:
				if l := linkTarget(m.Name); l != nil && !seen[*l] {
					seen[*l] = true
					r.Links = append(r.Links, *l)
				}
			case parse.KindTemplate:
				t := x.m.Template(parse.TemplateKey(m.Name))
				if t == nil {
					return true
				}
				switch t.Class {
				case ClassMath:
					if mt, ok := x.mathTemplate(m, t); ok {
						r.Math = append(r.Math, mt)
						return false
					}
				case ClassMaintenance:
					flags[strings.ReplaceAll(t.Key, " ", "-")] = true
				case ClassCitation:
					return false
				}
			}
			return true
		})
	}
	for _, mt := range r.Math {
		if len(mt.Errors) > 0 {
			flags["unparsed-math"] = true
		}
	}
	r.Flags = r.Flags[:0]
	for f := range flags {
		r.Flags = append(r.Flags, f)
	}
	sort.Strings(r.Flags)
	if len(r.Flags) == 0 {
		r.Flags = nil
	}
	text := x.render(nodes)
	if r.Kind == "citation" {
		keys := make([]string, 0, len(r.Fields))
		for k := range r.Fields {
			keys = append(keys, k+"="+r.Fields[k])
		}
		sort.Strings(keys)
		text = strings.Join(keys, "\n")
	}
	r.Hash = hash(text)
	switch r.Kind {
	case "section":
		// A section holds its subsections, so its math and its links are
		// in the objects inside it. The record keeps the structure only.
		r.Math, r.Links = nil, nil
	case "citation":
	default:
		r.Text = text
	}
}

func (x *extractor) mathTemplate(m *parse.Node, t *Template) (Math, bool) {
	src := x.doc.Raw(m)
	var res texvc.Result
	var form string
	switch t.Key {
	case "math":
		res, form = texvc.FromMathTemplate(src), "template"
	case "mvar":
		res, form = texvc.FromMvar(src), "mvar"
	case "tmath":
		res, form = texvc.FromTmath(src), "tmath"
	default:
		return Math{}, false
	}
	return Math{Form: form, TeX: res.TeX, Source: src, Display: res.Display, Span: [2]int{m.Start, m.End}, Errors: res.Errors}, true
}

// nodesIn returns the largest nodes that are inside the span [start, end).
// It goes into the values of template parameters, so that the span of one
// parameter gives the nodes of that parameter.
func (x *extractor) nodesIn(n *parse.Node, start, end int, out []*parse.Node) []*parse.Node {
	if n.End <= start || n.Start >= end {
		return out
	}
	if n.Start >= start && n.End <= end && n.Kind != parse.KindDocument {
		return append(out, n)
	}
	for _, p := range n.Params {
		for _, c := range p.Value {
			out = x.nodesIn(c, start, end, out)
		}
	}
	for _, c := range n.Children {
		out = x.nodesIn(c, start, end, out)
	}
	return out
}

// render gives the text of nodes as a reader sees it, with the math as $…$
// in TeX. It is a simple form of the Markdown of WP3.12.
func (x *extractor) render(nodes []*parse.Node) string {
	var b strings.Builder
	x.renderTo(&b, nodes)
	var lines []string
	for _, l := range strings.Split(b.String(), "\n") {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

func (x *extractor) renderTo(b *strings.Builder, nodes []*parse.Node) {
	for _, n := range nodes {
		switch n.Kind {
		case parse.KindText:
			b.WriteString(html.UnescapeString(x.doc.Raw(n)))
		case parse.KindHeading:
			b.WriteString("\n" + strings.Repeat("#", max(n.Level, 1)) + " ")
			x.renderTo(b, n.Children)
			b.WriteString("\n")
		case parse.KindParagraph, parse.KindList, parse.KindTable, parse.KindSection, parse.KindPre:
			b.WriteString("\n")
			x.renderTo(b, n.Children)
			b.WriteString("\n")
		case parse.KindListItem, parse.KindRow, parse.KindCaption:
			b.WriteString("\n")
			if n.Name == "*" || n.Name == "#" {
				b.WriteString("- ")
			}
			x.renderTo(b, n.Children)
			b.WriteString("\n")
		case parse.KindCell:
			b.WriteString(" | ")
			x.renderTo(b, n.Children)
		case parse.KindBold, parse.KindItalic, parse.KindExtLink:
			x.renderTo(b, n.Children)
		case parse.KindLink:
			if linkTarget(n.Name) == nil && isMediaLink(n.Name) {
				continue
			}
			if len(n.Children) > 0 {
				x.renderTo(b, n.Children)
			} else {
				b.WriteString(strings.TrimPrefix(n.Name, ":"))
			}
			b.WriteString(n.Trail)
		case parse.KindTag:
			switch n.Name {
			case "math":
				res := texvc.FromMathTag(x.doc.Inner(n), n.Attrs)
				writeMath(b, res.TeX, res.Display)
			case "nowiki", "code", "pre", "syntaxhighlight", "source":
				b.WriteString(html.UnescapeString(x.doc.Inner(n)))
			case "ref", "references", "gallery", "timeline", "score", "graph", "templatestyles", "includeonly":
			case "br":
				b.WriteByte(' ')
			default:
				x.renderTo(b, n.Children)
			}
		case parse.KindTemplate:
			key := parse.TemplateKey(n.Name)
			t := x.m.Template(key)
			if t == nil {
				// WP3.2: until a template is mapped, its text is
				// kept. The first positional parameter is the
				// text of most inline templates.
				if p := n.Param("1"); p != nil && !strings.HasPrefix(key, "#") && !strings.Contains(key, ":") {
					x.renderTo(b, p.Value)
				}
				continue
			}
			switch t.Class {
			case ClassMath:
				x.renderMath(b, n, t)
			case ClassLayout, ClassAnchor:
				if t.Class == ClassAnchor && t.Key != "vanchor" {
					continue
				}
				for _, role := range []string{"text", "link", "name"} {
					if name := t.Param(role); name != "" && n.Param(name) != nil && (t.Class == ClassAnchor || role != "name") {
						x.renderTo(b, n.Param(name).Value)
						break
					}
				}
			case ClassTerm:
				for _, name := range []string{"1", "term", "defn", "content"} {
					if p := n.Param(name); p != nil && t.Params[name] != "" {
						x.renderTo(b, p.Value)
						b.WriteByte(' ')
					}
				}
			case ClassItem:
				for _, name := range []string{"name", "title", "term", "1", "math_statement", "proof", "statement", "content", "defn", "2"} {
					if p := n.Param(name); p != nil && t.Params[name] != "" && t.Params[name] != "flag" {
						b.WriteString("\n")
						x.renderTo(b, p.Value)
						b.WriteString("\n")
					}
				}
			}
		}
	}
}

// renderMath writes a template of class math. A template that is not
// {{math}}, {{mvar}} or {{tmath}} ({{pi}}, {{sfrac}} and the others) is read
// as the content of a {{math}}.
func (x *extractor) renderMath(b *strings.Builder, n *parse.Node, t *Template) {
	if mt, ok := x.mathTemplate(n, t); ok {
		writeMath(b, mt.TeX, mt.Display)
		return
	}
	switch t.Key {
	case "numblk":
		if p := n.Param("2"); p != nil {
			x.renderTo(b, p.Value)
		}
		return
	case "equationref", "equationnote":
		label := x.paramText(n, "2", "1")
		if label != "" {
			b.WriteString("(" + label + ")")
		}
		return
	}
	res := texvc.FromMathTemplate("{{math|" + x.doc.Raw(n) + "}}")
	if len(res.Errors) == 0 && res.TeX != "" {
		writeMath(b, res.TeX, false)
	}
}

func writeMath(b *strings.Builder, tex string, display bool) {
	if display {
		b.WriteString("\n$$" + tex + "$$\n")
		return
	}
	b.WriteString("$" + tex + "$")
}

var otherNamespaces = map[string]bool{
	"category": true, "file": true, "image": true, "media": true, "wikt": true, "wiktionary": true, "s": true, "q": true, "d": true,
	"b": true, "v": true, "n": true, "c": true, "commons": true, "mw": true, "meta": true, "m": true, "w": true, "wikipedia": true, "wp": true,
	"help": true, "template": true, "portal": true, "user": true, "talk": true, "special": true, "draft": true, "module": true, "wikisource": true,
	"wikiquote": true, "wikibooks": true, "wikiversity": true, "species": true, "wikidata": true, "user talk": true, "template talk": true,
}

func isMediaLink(target string) bool {
	t := strings.ToLower(strings.TrimSpace(target))
	return strings.HasPrefix(t, "file:") || strings.HasPrefix(t, "image:") || strings.HasPrefix(t, "category:")
}

// linkTarget gives the article and the anchor of a wiki link, or nil when
// the link goes to another namespace, another wiki or another language.
func linkTarget(target string) *Link {
	t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(target), ":"))
	if i := strings.IndexByte(t, ':'); i > 0 {
		p := strings.ToLower(strings.TrimSpace(t[:i]))
		if otherNamespaces[p] || (len(p) <= 3 && p == strings.ToLower(t[:i]) && !strings.Contains(p, " ")) {
			return nil
		}
	}
	title, anchor, _ := strings.Cut(t, "#")
	title = strings.Join(strings.Fields(strings.ReplaceAll(title, "_", " ")), " ")
	if title == "" {
		return nil
	}
	if r := []rune(title); len(r) > 0 {
		title = strings.ToUpper(string(r[0])) + string(r[1:])
	}
	return &Link{Title: title, Anchor: strings.ReplaceAll(strings.TrimSpace(anchor), " ", "_")}
}

func appendOnce(s []string, v string) []string {
	if slices.Contains(s, v) {
		return s
	}
	return append(s, v)
}

// hash gives the normalized hash of a text: lower case, with each run of
// white space as one space.
func hash(text string) string {
	s := strings.ToLower(strings.Join(strings.Fields(text), " "))
	sum := sha256.Sum256([]byte(s))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// attachProofs attaches each proof to the statement that it proves, by the
// rules of WP3.7. A proof with no target gets the flag "orphan".
func (x *extractor) attachProofs() {
	var items, named []*object
	var subject *object
	statements := 0
	for _, c := range x.cues {
		if c.obj == nil || slices.Contains(items, c.obj) {
			continue
		}
		items = append(items, c.obj)
		switch {
		case c.obj.Kind == "subject":
			subject = c.obj
			named = append(named, c.obj)
		case strings.HasPrefix(c.obj.Kind, "statement"):
			statements++
			named = append(named, c.obj)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Span[0] < items[j].Span[0] })
	for i, o := range items {
		if o.Kind != "proof" {
			continue
		}
		// Rule 1: the title or the heading names the statement.
		if t := namedTarget(o.Name, named); t != nil {
			o.target = t
			continue
		}
		// Rule 2: the proof is directly after a statement.
		parent := o.target
		o.target = nil
		for j := i - 1; j >= 0; j-- {
			p := items[j]
			if contains(p, o) {
				continue
			}
			if strings.HasPrefix(p.Kind, "statement") {
				o.target = p
			}
			break
		}
		// Rule 3: a proof heading under a statement heading. The
		// section walk set parent to the item heading above.
		if o.target == nil && parent != nil && strings.HasPrefix(parent.Kind, "statement") {
			o.target = parent
		}
		// Rule 3 also holds for a proof template or head in the
		// section of a statement heading. The last match is the
		// innermost one.
		if o.target == nil {
			for _, p := range items {
				if p != o && (p.Cue == "heading" || p.Cue == "named-heading") && strings.HasPrefix(p.Kind, "statement") && contains(p, o) {
					o.target = p
				}
			}
		}
		// Rule 4: an article of type statement with at most one
		// statement cue.
		if o.target == nil && x.typ == "statement" && statements <= 1 {
			o.target = subject
			for _, p := range named {
				if p != subject {
					o.target = p
				}
			}
		}
		if o.target == nil {
			o.Flags = append(o.Flags, "orphan")
		}
	}
}

func contains(p, o *object) bool {
	return p.Span[0] <= o.Span[0] && o.Span[1] <= p.Span[1]
}

func namedTarget(title string, statements []*object) *object {
	t := strings.ToLower(title)
	t = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(t, "proof"), "s"))
	t = strings.TrimSpace(strings.TrimPrefix(t, "of"))
	t = strings.TrimSpace(strings.TrimPrefix(t, "the"))
	if t == "" {
		return nil
	}
	// The longest name that the title holds wins, so that "Proof of
	// the Sylow theorems" goes to "Sylow theorems" and not to a heading
	// "Theorems". A name of one word, such as "Theorem (1)" or
	// "Statement", names no statement.
	var best *object
	for _, s := range statements {
		n := strings.ToLower(s.Name)
		if len(strings.Fields(n)) < 2 || headWord.MatchString(n) || !strings.Contains(t, n) {
			continue
		}
		if best == nil || len(n) > len(best.Name) {
			best = s
		}
	}
	return best
}

// result orders the objects by span, gives each one its ref, and makes the
// census.
func (x *extractor) result() *Result {
	sort.SliceStable(x.objs, func(i, j int) bool {
		a, b := x.objs[i], x.objs[j]
		if a.Span[0] != b.Span[0] {
			return a.Span[0] < b.Span[0]
		}
		if a.Span[1] != b.Span[1] {
			return a.Span[1] > b.Span[1]
		}
		return a.order < b.order
	})
	for i, o := range x.objs {
		o.Ref = fmt.Sprintf("o%d", i+1)
	}
	res := &Result{}
	for _, o := range x.objs {
		if o.target != nil {
			o.AttachedTo = o.target.Ref
		}
		o.Citations = x.citations(o)
		res.Objects = append(res.Objects, o.Object)
	}
	c := &res.Census
	c.Page, c.Revision, c.Title, c.Type, c.Extractor = x.page.ID, x.page.Revision, x.page.Title, x.typ, x.ver
	c.Manifests = map[string]int{"templates": x.m.TemplatesVersion, "headings": x.m.HeadingsVersion, "waivers": x.m.WaiversVersion}
	c.Strength = StrengthWeak
	c.Counts = x.counts
	c.Cues = []Cue{}
	for _, q := range x.cues {
		if q.obj != nil {
			q.Object = q.obj.Ref
		}
		c.Cues = append(c.Cues, q.Cue)
	}
	sort.SliceStable(c.Cues, func(i, j int) bool { return c.Cues[i].Span[0] < c.Cues[j].Span[0] })
	if len(x.unmapped) > 0 {
		c.Unmapped = x.unmapped
	}
	for h := range x.unheads {
		if h != "" {
			c.UnmappedHeadings = append(c.UnmappedHeadings, h)
		}
	}
	sort.Strings(c.UnmappedHeadings)
	return res
}

// citations gives the refs of the citations that support an object: the
// citation objects in its span and the named refs that it uses again.
func (x *extractor) citations(o *object) []string {
	if o.Kind == "citation" {
		return nil
	}
	var out []string
	for _, c := range x.objs {
		if c.Kind == "citation" && contains(o, c) {
			out = appendOnce(out, c.Ref)
		}
	}
	for _, name := range o.Citations {
		if r := x.refs[strings.TrimPrefix(name, "ref:")]; r != nil {
			out = appendOnce(out, r.Ref)
		}
	}
	return out
}
