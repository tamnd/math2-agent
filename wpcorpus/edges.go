package wpcorpus

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/wikipedia-cli/wiki/parse"
	"go.yaml.in/yaml/v3"
)

// EdgesDir is the directory of the edge files in the analysis plane (WP6.4).
const EdgesDir = "analysis/edges"

// EdgesPath gives the path of the edge file of a page. The file holds the
// edges of the revision of record only.
func EdgesPath(page int) string {
	return filepath.Join(EdgesDir, Shard(page), strconv.Itoa(page)+".md")
}

// The methods of an edge (WP6.1). The method link-in-text is a link in the
// text of a section that is not in an item.
const (
	MethodLinkInItem  = "link-in-item"
	MethodLinkInProof = "link-in-proof"
	MethodAnchor      = "anchor"
	MethodNamedLink   = "named-link"
	MethodHatnote     = "hatnote"
	MethodLinkInLead  = "link-in-lead"
	MethodLinkInText  = "link-in-text"
	MethodSeeAlso     = "see-also"
	MethodNav         = "nav"
	MethodCategory    = "category"
	MethodList        = "list"
	MethodGlossary    = "glossary"
	MethodInfobox     = "infobox"
)

// The predicates of an edge (WP6.3).
const (
	PredUses        = "uses"
	PredRestates    = "restates"
	PredRelated     = "related"
	PredInField     = "in-field"
	PredInNav       = "in-nav"
	PredKindOf      = "kind-of"
	PredListedIn    = "listed-in"
	PredGlossaryOf  = "glossary-of"
	PredGeneralizes = "generalizes"
	PredSpecializes = "specializes"
	PredProvedBy    = "proved-by"
	PredStatedBy    = "stated-by"
)

// The confidences of an edge (WP6.3).
const (
	ConfCertain = "certain"
	ConfHigh    = "high"
	ConfMedium  = "medium"
	ConfLow     = "low"
)

// FindingRedLink is the finding of a link to a page that does not exist
// (WP6.6).
const FindingRedLink = "red-link"

// LinkUse is one link of an article before resolution: a wiki link, a
// hatnote, a navbox, a category or a reference to an equation. The
// extractor gives the uses in the order of the page.
type LinkUse struct {
	// From is the ref of the innermost object with a mention that holds the
	// link, or "" for the article.
	From string
	// Title is the target: an article, "Category:…" or "Template:…". It is
	// "" for an anchor of the same article.
	Title  string
	Anchor string
	// Local is the ref of the object of the same article that an anchor
	// names. It is "" when no object has the anchor.
	Local string
	Label string
	Span  [2]int
	// Method is the method of WP6.1. Predicate and Confidence are set when
	// the method or a phrase decides them without the target.
	Method     string
	Predicate  string
	Confidence string
	// Hatnote is the key of the hatnote template.
	Hatnote string
}

// EdgePhrase is one phrase of manifests/wikipedia/edges.yaml. A link after
// the phrase gets the predicate of the phrase (WP6.3).
type EdgePhrase struct {
	Phrase    string `yaml:"phrase"`
	Predicate string `yaml:"predicate"`
}

// defaultEdgePhrases are the phrases when the corpus has no edges.yaml.
var defaultEdgePhrases = []EdgePhrase{
	{"special case of", PredSpecializes},
	{"special cases of", PredSpecializes},
	{"generalization of", PredGeneralizes},
	{"generalisation of", PredGeneralizes},
	{"generalizes", PredGeneralizes},
	{"generalises", PredGeneralizes},
	{"analogous to", PredRelated},
	{"analogue of", PredRelated},
	{"compare", PredRelated},
	{"cf.", PredRelated},
	{"not to be confused with", PredRelated},
	{"in contrast to", PredRelated},
}

// mentionKind tells if an object of this kind gets a mention (WP5.1).
func mentionKind(kind string) bool {
	switch kindFamily(kind) {
	case "section", "equation", "citation", "infobox", "figure", "table", "table-row":
		return false
	}
	return true
}

var (
	namedLead = regexp.MustCompile(`\b(by|from|using|apply|applying|applies|via|invoking)\s+(the\s+|a\s+|an\s+)?$`)
	kindOfCat = regexp.MustCompile(`^(theorems|lemmas|conjectures|unsolved problems|mathematical problems|paradoxes|algorithms|inequalities|mathematical identities|equations|integer sequences|mathematical constants|probability distributions|types of|classes of)\b`)
	skipCat   = regexp.MustCompile(`^(articles|pages|wikipedia|cs1|all |use |short description|webarchive|harv and sfn|commons category)|\b(births|deaths|people|alumni|faculty|fellows|members|recipients|winners|stubs)\b`)
)

// links collects the link uses of the page. It runs after the objects are
// made, so that each use knows the object that holds it.
func (x *extractor) links() {
	x.items = map[*object]bool{}
	for _, c := range x.cues {
		if c.obj != nil && mentionKind(c.obj.Kind) {
			x.items[c.obj] = true
		}
	}
	eqs := map[string]*object{}
	parse.Walk(x.doc.Root, func(n *parse.Node) bool {
		if n.Kind == parse.KindTemplate && parse.TemplateKey(n.Name) == "equationref" {
			if p := n.Param("1"); p != nil {
				eqs[strings.TrimSpace(x.doc.ParamValue(p))] = x.holder(n.Start, n.End, false)
			}
		}
		return true
	})
	x.walkLinks(x.doc.Root, eqs)
}

// holder gives the innermost object with a mention that holds the span. The
// subject of the lead is the article, so it gives nil for the subject
// unless subject is true.
func (x *extractor) holder(start, end int, subject bool) *object {
	var best *object
	for o := range x.items {
		if o.Span[0] <= start && end <= o.Span[1] && (best == nil || o.Span[1]-o.Span[0] < best.Span[1]-best.Span[0] || (o.Span[1]-o.Span[0] == best.Span[1]-best.Span[0] && o.order > best.order)) {
			best = o
		}
	}
	if best != nil && best.Kind == "subject" && !subject {
		return nil
	}
	return best
}

// linkUse is a LinkUse with the objects before they have their refs.
type linkUse struct {
	LinkUse
	from, local *object
}

func (x *extractor) use(u LinkUse, from *object) {
	x.uses = append(x.uses, linkUse{LinkUse: u, from: from})
}

func (x *extractor) walkLinks(n *parse.Node, eqs map[string]*object) {
	switch n.Kind {
	case parse.KindTag:
		switch n.Name {
		case "ref", "math", "chem", "ce", "gallery", "imagemap", "timeline", "score", "syntaxhighlight", "source", "pre", "nowiki":
			return
		}
	case parse.KindLink:
		x.linkUse(n)
		return
	case parse.KindTemplate:
		if x.templateUse(n, eqs) {
			return
		}
	}
	for _, p := range n.Params {
		for _, c := range p.Value {
			x.walkLinks(c, eqs)
		}
	}
	for _, c := range n.Children {
		x.walkLinks(c, eqs)
	}
}

// templateUse makes the uses of a template. It tells if the walk must not
// go into the template.
func (x *extractor) templateUse(n *parse.Node, eqs map[string]*object) bool {
	key := parse.TemplateKey(n.Name)
	if key == "equationnote" {
		label := ""
		if p := n.Param("1"); p != nil {
			label = strings.TrimSpace(x.doc.ParamValue(p))
		}
		x.use(LinkUse{Anchor: label, Span: [2]int{n.Start, n.End}, Method: MethodAnchor, Predicate: PredUses, Confidence: ConfCertain}, x.holder(n.Start, n.End, false))
		if o := eqs[label]; o != nil {
			x.uses[len(x.uses)-1].local = o
		}
		return true
	}
	t := x.m.Template(key)
	if t == nil {
		return false
	}
	switch t.Class {
	case ClassCitation, ClassMeta, ClassMaintenance, ClassIgnore:
		return true
	case ClassNav:
		x.use(LinkUse{Title: "Template:" + upperFirst(strings.TrimSpace(strings.ReplaceAll(n.Name, "_", " "))), Span: [2]int{n.Start, n.End}, Method: MethodNav, Predicate: PredInNav, Confidence: ConfLow}, nil)
		return true
	case ClassHatnote:
		pred, conf := PredRelated, ConfMedium
		switch t.Key {
		case "main":
			pred, conf = PredRestates, ConfHigh
		case "further", "see also", "also", "see", "details":
		default:
			// The other hatnotes (about, for, distinguish) tell the reader
			// of other pages with a similar name. They are not edges.
			return true
		}
		from := x.sectionObject(n.Start)
		for _, p := range n.Params {
			if p.Name != "" {
				continue
			}
			l := linkTarget(x.doc.ParamValue(p))
			if l == nil {
				continue
			}
			x.use(LinkUse{Title: l.Title, Anchor: l.Anchor, Span: [2]int{p.Start, p.End}, Method: MethodHatnote, Predicate: pred, Confidence: conf, Hatnote: t.Key}, from)
		}
		return true
	case ClassItem:
		if strings.HasPrefix(t.Key, "infobox ") {
			x.infoboxUse(n)
			return true
		}
	}
	return false
}

// infoboxFields gives the predicate of each field of an infobox of a
// statement that names other items (WP6.3).
var infoboxFields = map[string]string{
	"generalizations": PredSpecializes, "generalization": PredSpecializes,
	"special cases": PredGeneralizes, "special case": PredGeneralizes,
	"first proof by": PredProvedBy, "proved by": PredProvedBy,
	"conjectured by": PredStatedBy, "stated by": PredStatedBy,
	"field":        PredInField,
	"consequences": PredRelated, "implied by": PredRelated, "equivalent to": PredRelated,
}

func (x *extractor) infoboxUse(n *parse.Node) {
	for _, p := range n.Params {
		pred, ok := infoboxFields[strings.ToLower(strings.TrimSpace(strings.ReplaceAll(p.Name, "_", " ")))]
		if !ok {
			for _, c := range p.Value {
				x.walkLinks(c, nil)
			}
			continue
		}
		for _, c := range p.Value {
			parse.Walk(c, func(m *parse.Node) bool {
				if m.Kind != parse.KindLink {
					return m.Kind != parse.KindTag || m.Name != "ref"
				}
				if l := linkTarget(m.Name); l != nil && !isMediaLink(m.Name) {
					x.use(LinkUse{Title: l.Title, Anchor: l.Anchor, Label: x.label(m, l), Span: [2]int{m.Start, m.End}, Method: MethodInfobox, Predicate: pred, Confidence: ConfHigh}, nil)
				}
				return false
			})
		}
	}
}

// sectionObject gives the item object of the section at off, or nil for
// the article. A hatnote under a heading speaks for the section.
func (x *extractor) sectionObject(off int) *object {
	for s := x.sectionAt(off); s != nil; s = s.parent {
		if s.item != nil && x.items[s.item] {
			return s.item
		}
	}
	return nil
}

func (x *extractor) seeAlso(s *section) bool {
	for ; s != nil; s = s.parent {
		if s.obj != nil && strings.EqualFold(strings.Join(strings.Fields(s.obj.Name), " "), "see also") {
			return true
		}
	}
	return false
}

func (x *extractor) label(n *parse.Node, l *Link) string {
	label := strings.TrimSpace(x.doc.Plain(n.Children))
	if label == "" || strings.EqualFold(label, l.Title) || upperFirst(label) == l.Title {
		return ""
	}
	return label
}

func (x *extractor) linkUse(n *parse.Node) {
	target := strings.TrimSpace(n.Name)
	span := [2]int{n.Start, n.End}
	if c, ok := strings.CutPrefix(strings.ToLower(target), "category:"); ok {
		name := strings.TrimSpace(target[len(target)-len(c):])
		if i := strings.IndexByte(name, '|'); i >= 0 {
			name = name[:i]
		}
		name = upperFirst(strings.Join(strings.Fields(strings.ReplaceAll(name, "_", " ")), " "))
		low := strings.ToLower(name)
		if name == "" || skipCat.MatchString(low) {
			return
		}
		pred := PredInField
		if kindOfCat.MatchString(low) {
			pred = PredKindOf
		}
		x.use(LinkUse{Title: "Category:" + name, Span: span, Method: MethodCategory, Predicate: pred, Confidence: ConfMedium}, nil)
		return
	}
	if isMediaLink(target) {
		return
	}
	s := x.sectionAt(n.Start)
	if s != nil && x.inBack(s) && !x.seeAlso(s) {
		return
	}
	if strings.HasPrefix(target, "#") {
		a := normAnchor(decodeAnchor(target[1:]))
		x.use(LinkUse{Anchor: strings.ReplaceAll(a, " ", "_"), Label: strings.TrimSpace(x.doc.Plain(n.Children)), Span: span, Method: MethodAnchor, Predicate: PredUses, Confidence: ConfCertain}, x.holder(n.Start, n.End, false))
		x.uses[len(x.uses)-1].local = x.anchorObject(a)
		return
	}
	l := linkTarget(target)
	if l == nil {
		return
	}
	u := LinkUse{Title: l.Title, Anchor: l.Anchor, Label: x.label(n, l), Span: span}
	if s != nil && x.seeAlso(s) {
		u.Method, u.Predicate, u.Confidence = MethodSeeAlso, PredRelated, ConfLow
		x.use(u, nil)
		return
	}
	o := x.holder(n.Start, n.End, true)
	switch {
	case o != nil && o.Kind == "subject":
		u.Method, o = MethodLinkInLead, nil
	case o != nil && o.Kind == "proof":
		u.Method = MethodLinkInProof
	case o != nil && o.Kind == "entry" && o.Cue == "list-entry" && x.firstLinkOf(o) == n:
		u.Method, u.Predicate, u.Confidence = MethodList, PredListedIn, ConfCertain
	case o != nil && o.Kind == "entry" && o.Cue == "glossary-term" && x.firstLinkOf(o) == n:
		u.Method, u.Predicate, u.Confidence = MethodGlossary, PredGlossaryOf, ConfCertain
	case o != nil:
		u.Method = MethodLinkInItem
	case s == nil || s.node.Level == 0:
		u.Method = MethodLinkInLead
	default:
		u.Method = MethodLinkInText
		if row := x.tableRow(n); row != nil && x.typ == "list" && x.firstLinkOf(row) == n {
			u.Method, u.Predicate, u.Confidence = MethodList, PredListedIn, ConfCertain
		}
	}
	if u.Predicate == "" {
		before := strings.ToLower(x.doc.Src[max(0, n.Start-80):n.Start])
		if i := strings.LastIndexAny(before, "\n.;"); i >= 0 && !strings.HasSuffix(before[:i+1], "cf.") {
			before = before[i+1:]
		}
		for _, p := range x.m.EdgePhrases() {
			if strings.Contains(before, p.Phrase) {
				u.Predicate, u.Confidence = p.Predicate, ConfMedium
				if p.Predicate == PredRelated {
					u.Confidence = ConfLow
				}
				break
			}
		}
		if u.Predicate == "" && (u.Method == MethodLinkInItem || u.Method == MethodLinkInProof || u.Method == MethodLinkInText) && namedLead.MatchString(before) {
			name := strings.ToLower(u.Label)
			if name == "" {
				name = strings.ToLower(l.Title)
			}
			if resultWord.MatchString(name) {
				u.Method, u.Predicate, u.Confidence = MethodNamedLink, PredUses, ConfHigh
			}
		}
	}
	x.use(u, o)
}

// firstLinkOf gives the first wiki link in the span of an object.
func (x *extractor) firstLinkOf(o *object) *parse.Node {
	var out *parse.Node
	parse.Walk(x.doc.Root, func(m *parse.Node) bool {
		if out != nil || m.End <= o.Span[0] || m.Start >= o.Span[1] {
			return false
		}
		if m.Kind == parse.KindLink && m.Start >= o.Span[0] && !isMediaLink(m.Name) {
			out = m
			return false
		}
		return true
	})
	return out
}

// tableRow gives the table row object that holds a link.
func (x *extractor) tableRow(n *parse.Node) *object {
	for _, o := range x.objs {
		if o.Kind == "table-row" && o.Span[0] <= n.Start && n.End <= o.Span[1] {
			return o
		}
	}
	return nil
}

// anchorObject gives the object of the page that an anchor names: the
// object of an anchor template, or the item or the section of a heading.
// A name that differs only in case is the second choice.
func (x *extractor) anchorObject(a string) *object {
	if o := x.anchors[a]; o != nil {
		return o
	}
	for _, fold := range []bool{false, true} {
		for _, s := range x.secs {
			if s.anchor != "" && (normAnchor(s.anchor) == a || fold && strings.EqualFold(normAnchor(s.anchor), a)) {
				if s.item != nil {
					return s.item
				}
				return s.obj
			}
		}
		for k, o := range x.anchors {
			if fold && strings.EqualFold(k, a) {
				return o
			}
		}
	}
	return nil
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}

// Edge is one edge of the edge store (WP6.4).
type Edge struct {
	From string `yaml:"from"`
	// To is the mention of the target in scope. Page is the page id of a
	// target outside the scope. Target is the title of a target with no
	// page id: a category, a navbox, a red link, or a page that the
	// command could not resolve.
	To         string `yaml:"to,omitempty"`
	Page       int    `yaml:"page,omitempty"`
	Target     string `yaml:"target,omitempty"`
	Predicate  string `yaml:"predicate"`
	Method     string `yaml:"method"`
	Confidence string `yaml:"confidence"`
	Span       [2]int `yaml:"span,flow"`
	Revision   int    `yaml:"revision"`
	Label      string `yaml:"label,omitempty"`
	Finding    string `yaml:"finding,omitempty"`
}

// EdgeFile is the edge file of one article.
type EdgeFile struct {
	Page      int    `yaml:"page"`
	Revision  int    `yaml:"revision"`
	Title     string `yaml:"title"`
	Extractor string `yaml:"extractor"`
	Edges     []Edge `yaml:"edges"`
}

// EdgeContext gives what the resolution of WP6.2 needs to know of the other
// pages. Each field can be nil.
type EdgeContext struct {
	// Scope gives the page id of an article in scope, after the redirects.
	Scope Resolver
	// Page gives the page id of any article, after one redirect. It gives
	// false when no page has the title. When it is nil, a link out of the
	// scope keeps its title, and no link is a red link.
	Page func(title string) (int, bool)
	// Objects gives the objects, with their tags, of an article in scope. It
	// gives nil when the article is not extracted.
	Objects func(page int) []Object
	// Type gives the article type of WP1.5 of an article in scope.
	Type func(page int) string
}

// MentionID gives the mention id of an article, or of an object when tag
// is not "" (WP5.2).
func MentionID(page int, tag string) string {
	if tag == "" {
		return fmt.Sprintf("wikipedia://en/%d", page)
	}
	return fmt.Sprintf("wikipedia://en/%d#%s", page, tag)
}

// BuildEdges resolves the link uses of a result to edges (WP6.2 and WP6.3).
// The objects of the result must have their tags.
func BuildEdges(r *Result, ctx EdgeContext) *EdgeFile {
	c := &r.Census
	f := &EdgeFile{Page: c.Page, Revision: c.Revision, Title: c.Title, Extractor: c.Extractor, Edges: []Edge{}}
	byRef := map[string]*Object{}
	for i := range r.Objects {
		byRef[r.Objects[i].Ref] = &r.Objects[i]
	}
	mention := func(page int, o *Object) string {
		if o == nil || o.Tag == "" || o.Kind == "subject" || !mentionKind(o.Kind) {
			return MentionID(page, "")
		}
		return MentionID(page, o.Tag)
	}
	for _, u := range r.Links {
		e := Edge{From: mention(c.Page, byRef[u.From]), Predicate: u.Predicate, Method: u.Method, Confidence: u.Confidence, Span: u.Span, Revision: c.Revision, Label: u.Label}
		class := ""
		switch {
		case u.Title == "":
			e.To = MentionID(c.Page, "")
			if o := byRef[u.Local]; o != nil {
				e.To = mention(c.Page, o)
			} else {
				e.Finding = FindingBrokenAnchor
			}
		case strings.HasPrefix(u.Title, "Category:") || strings.HasPrefix(u.Title, "Template:"):
			e.Target = u.Title
		default:
			page := 0
			if ctx.Scope != nil {
				page = ctx.Scope(u.Title)
			}
			switch {
			case page != 0:
				e.To = MentionID(page, "")
				if ctx.Type != nil {
					class = typeClass(ctx.Type(page))
				}
				var objs []Object
				if u.Anchor != "" && ctx.Objects != nil {
					objs = ctx.Objects(page)
				}
				// A page with no objects is not extracted yet, so its
				// anchors are not known.
				if len(objs) > 0 {
					o, found := anchorIn(objs, u.Anchor)
					switch {
					case !found:
						e.Finding = FindingBrokenAnchor
					case o != nil:
						e.To = mention(page, o)
						if k := kindClass(o.Kind); k != "" {
							class = k
						}
					}
				}
			case ctx.Page != nil:
				if id, ok := ctx.Page(u.Title); ok {
					e.Page = id
				} else {
					e.Target, e.Finding = u.Title, FindingRedLink
				}
			default:
				e.Target = u.Title
			}
			if page == c.Page && u.Anchor == "" {
				// A link to the article itself, often through a redirect.
				continue
			}
		}
		if e.Predicate == "" {
			e.Predicate, e.Confidence = decide(u.Method, class)
		}
		f.Edges = append(f.Edges, e)
	}
	return f
}

// typeClass gives the class of a target from its article type.
func typeClass(t string) string {
	switch t {
	case TypeConcept, TypeObject, TypeMethod:
		return "definition"
	case TypeStatement, TypeProblem:
		return "statement"
	case TypeField, TypeList, TypeBiography:
		return t
	}
	return ""
}

// kindClass gives the class of a target from the kind of its object.
func kindClass(k string) string {
	switch kindFamily(k) {
	case "definition":
		return "definition"
	case "statement", "conjecture", "problem":
		return "statement"
	}
	return ""
}

// decide gives the predicate and the confidence of a link from its method
// and the class of its target (WP6.3).
func decide(method, class string) (string, string) {
	if class == TypeBiography || class == TypeList {
		return PredRelated, ConfLow
	}
	switch method {
	case MethodLinkInItem:
		switch class {
		case "definition":
			return PredUses, ConfHigh
		case TypeField:
			return PredInField, ConfMedium
		}
		return PredUses, ConfMedium
	case MethodLinkInProof:
		switch class {
		case "statement":
			return PredUses, ConfHigh
		case TypeField:
			return PredRelated, ConfLow
		}
		return PredUses, ConfMedium
	case MethodLinkInLead:
		if class == TypeField {
			return PredInField, ConfHigh
		}
		return PredUses, ConfMedium
	}
	return PredRelated, ConfLow
}

// anchorIn gives the object of a page that an anchor names. It gives true
// when the anchor names an object or a section. The object is nil for a
// section, which has no mention.
// A name that differs only in case is the second choice.
func anchorIn(objs []Object, anchor string) (*Object, bool) {
	a := normAnchor(decodeAnchor(anchor))
	for _, fold := range []bool{false, true} {
		same := func(b string) bool {
			b = normAnchor(b)
			return b != "" && (b == a || fold && strings.EqualFold(b, a))
		}
		for i := range objs {
			if same(objs[i].Anchor) && mentionKind(objs[i].Kind) {
				return &objs[i], true
			}
		}
		found := false
		for i := range objs {
			o := &objs[i]
			if !same(o.SectionAnchor) {
				continue
			}
			if (o.Cue == "heading" || o.Cue == "named-heading") && mentionKind(o.Kind) {
				return o, true
			}
			if o.Kind == "section" || same(o.Anchor) {
				found = true
			}
		}
		if found {
			return nil, true
		}
	}
	return nil, false
}

// MarshalEdges gives the edge file of an article: a YAML header with the
// edges and a body with the counts.
func MarshalEdges(f *EdgeFile) ([]byte, error) {
	var b bytes.Buffer
	if err := writeFrontMatter(&b, f); err != nil {
		return nil, err
	}
	n := map[string]int{}
	var methods []string
	red := 0
	for _, e := range f.Edges {
		k := e.Method + "\x00" + e.Predicate + "\x00" + e.Confidence
		if n[k] == 0 {
			methods = append(methods, k)
		}
		n[k]++
		if e.Finding == FindingRedLink {
			red++
		}
	}
	sort.Strings(methods)
	fmt.Fprintf(&b, "\n# Edges of %s\n\nThe article has %s and %s (WP06).\n", f.Title, count(len(f.Edges), "edge"), count(red, "red link"))
	if len(methods) > 0 {
		b.WriteString("\n| Method | Predicate | Confidence | Edges |\n|---|---|---|---:|\n")
		for _, k := range methods {
			p := strings.Split(k, "\x00")
			fmt.Fprintf(&b, "| %s | %s | %s | %d |\n", p[0], p[1], p[2], n[k])
		}
	}
	return b.Bytes(), nil
}

// WriteEdges writes the edge file of an article.
func WriteEdges(root string, f *EdgeFile) error {
	data, err := MarshalEdges(f)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(root, EdgesPath(f.Page)), data)
}

// ReadEdges reads the edge file of an article.
func ReadEdges(root string, page int) (*EdgeFile, error) {
	data, err := os.ReadFile(filepath.Join(root, EdgesPath(page))) // #nosec G304 -- the path is in the corpus that the user names
	if err != nil {
		return nil, err
	}
	head, _, err := splitFrontMatter(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", EdgesPath(page), err)
	}
	var f EdgeFile
	if err := yaml.Unmarshal(head, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", EdgesPath(page), err)
	}
	return &f, nil
}

// EdgeSummary is the sum of the edge files of the corpus.
type EdgeSummary struct {
	Pages    int
	Edges    int
	ByMethod map[string]int
	ByPred   map[string]int
	ByConf   map[string]int
	Broken   int
	RedLinks map[string]int // by title: the number of articles that link to it
	// Outside gives, for each page outside the scope, the number of
	// articles in scope that point to it.
	Outside map[int]int
}

// Add adds the edges of one article to the summary.
func (s *EdgeSummary) Add(f *EdgeFile) {
	if s.ByMethod == nil {
		s.ByMethod, s.ByPred, s.ByConf, s.RedLinks, s.Outside = map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}, map[int]int{}
	}
	s.Pages++
	red, out := map[string]bool{}, map[int]bool{}
	for _, e := range f.Edges {
		s.Edges++
		s.ByMethod[e.Method]++
		s.ByPred[e.Predicate]++
		s.ByConf[e.Confidence]++
		switch e.Finding {
		case FindingBrokenAnchor:
			s.Broken++
		case FindingRedLink:
			red[e.Target] = true
		}
		if e.Page != 0 {
			out[e.Page] = true
		}
	}
	for t := range red {
		s.RedLinks[t]++
	}
	for p := range out {
		s.Outside[p]++
	}
}

// OutsideMin is the number of articles in scope that must point to a page
// outside the scope before the page goes to the classifier (signal S9).
const OutsideMin = 3

// EdgeReport writes reports/edges.md. titles gives the title of a page
// outside the scope, when the command knows it.
func EdgeReport(w io.Writer, s EdgeSummary, titles map[int]string) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintf(bw, "# Edges\n\nThis report is made by `m2 wp edges`. The edge files of %d articles have %d edges. %d edges point to an anchor that the target does not have (`broken-anchor`). %d titles are red links (WP6.6).\n", s.Pages, s.Edges, s.Broken, len(s.RedLinks))
	for _, by := range []struct {
		name string
		m    map[string]int
	}{{"method", s.ByMethod}, {"predicate", s.ByPred}, {"confidence", s.ByConf}} {
		fmt.Fprintf(bw, "\n## Edges by %s\n\n| %s | Edges |\n|---|---:|\n", by.name, upperFirst(by.name))
		for _, k := range sortedKeys(by.m) {
			fmt.Fprintf(bw, "| %s | %d |\n", k, by.m[k])
		}
	}
	type row struct {
		name string
		n    int
	}
	var out []row
	for p, n := range s.Outside {
		if n >= OutsideMin {
			name := strconv.Itoa(p)
			if t := titles[p]; t != "" {
				name = fmt.Sprintf("%s (%d)", t, p)
			}
			out = append(out, row{name, n})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].n > out[j].n || (out[i].n == out[j].n && out[i].name < out[j].name) })
	fmt.Fprintf(bw, "\n## Pages outside the scope\n\nThese %d pages are not in scope, and %d or more articles in scope point to them. They go to the classifier as signal S9 (WP6.2).\n\n", len(out), OutsideMin)
	if len(out) > 0 {
		fmt.Fprintf(bw, "| Page | Articles |\n|---|---:|\n")
		for _, r := range out {
			fmt.Fprintf(bw, "| %s | %d |\n", r.name, r.n)
		}
	}
	var red []row
	for t, n := range s.RedLinks {
		red = append(red, row{t, n})
	}
	sort.Slice(red, func(i, j int) bool { return red[i].n > red[j].n || (red[i].n == red[j].n && red[i].name < red[j].name) })
	fmt.Fprintf(bw, "\n## Red links\n\nEach red link is a gap signal of WP4.7.\n\n")
	if len(red) > 0 {
		fmt.Fprintf(bw, "| Title | Articles |\n|---|---:|\n")
		for _, r := range red {
			fmt.Fprintf(bw, "| %s | %d |\n", r.name, r.n)
		}
	}
	return bw.Flush()
}

// EdgesReportPath is the path of the edge report in the corpus.
const EdgesReportPath = "reports/edges.md"

// JoinTags sets the tag of each object of a result from the objects file of
// the same revision. The objects must be the same, by ref and by hash, so
// that the edges name the tags that the register holds.
func JoinTags(r *Result, objs []Object) error {
	if len(objs) != len(r.Objects) {
		return fmt.Errorf("the objects file has %d objects and the extractor gives %d. Run m2 wp extract again", len(objs), len(r.Objects))
	}
	for i := range r.Objects {
		o, f := &r.Objects[i], &objs[i]
		if o.Ref != f.Ref || o.Hash != f.Hash {
			return fmt.Errorf("object %s of the objects file is not the object that the extractor gives. Run m2 wp extract again", f.Ref)
		}
		o.Tag = f.Tag
	}
	return nil
}

// Slim keeps only the fields of the objects that the resolution of the
// edges reads, so that a run over the scope can hold all the pages.
func (r *Result) Slim() {
	for i := range r.Objects {
		o := &r.Objects[i]
		r.Objects[i] = Object{Ref: o.Ref, Tag: o.Tag, Kind: o.Kind, Cue: o.Cue, Anchor: o.Anchor, SectionAnchor: o.SectionAnchor}
	}
	r.TeX = nil
	r.Census = Census{Page: r.Census.Page, Revision: r.Census.Revision, Title: r.Census.Title, Extractor: r.Census.Extractor}
}
