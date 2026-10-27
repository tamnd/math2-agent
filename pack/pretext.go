package pack

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"

	"github.com/tamnd/math2-agent/markup"
)

// PreTeXt is the format of the PreTeXt XML output (corpus K11.4).
const PreTeXt = "pretext"

// MainPTX is the file of the PreTeXt output.
const MainPTX = "main.ptx"

// PreTeXt returns the files of the PreTeXt output, by slash path: main.ptx, one PreTeXt document. Each item is the PreTeXt element of its kind, with its tag as its xml:id when the tag is an XML name, else m2- and the tag. A macro of the notation register is a notation element at the item that introduces it, and the back matter has the list of notation and the attribution. The PreTeXt tools make the web, PDF and EPUB outputs from the file. The output is the same each time for the same document.
func (d *Document) PreTeXt() (map[string][]byte, error) {
	w := &ptxWriter{d: d, b: &strings.Builder{}}
	return map[string][]byte{MainPTX: []byte(w.main())}, nil
}

// ptxWriter writes main.ptx.
type ptxWriter struct {
	d   *Document
	b   *strings.Builder
	src []byte
}

func (w *ptxWriter) printf(format string, args ...any) { fmt.Fprintf(w.b, format, args...) }

func (w *ptxWriter) main() string {
	d, v := w.d, w.d.Volume
	article := v.Kind == "article" || v.Kind == "survey"
	root, chapter, section := "book", "chapter", "section"
	if article {
		root, chapter, section = "article", "section", "subsection"
	}
	w.printf("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!-- m2 pack wrote this file from the volume %s of math2. Do not change it by hand. -->\n<pretext xml:lang=\"en-US\">\n", xmlText(v.ID))
	if macros := w.macros(); macros != "" {
		w.printf("<docinfo>\n<macros>\n%s</macros>\n</docinfo>\n", xmlText(macros))
	}
	w.printf("<%s xml:id=\"%s\">\n<title>%s</title>\n<frontmatter>\n", root, xmlID("volume-"+v.ID), xmlText(v.Title))
	if len(v.Editors) > 0 {
		w.printf("<bibinfo>\n")
		for _, e := range v.Editors {
			w.printf("<editor><personname>%s</personname></editor>\n", xmlText(e))
		}
		w.printf("<date>math2 release %s</date>\n</bibinfo>\n", xmlText(d.Release))
	}
	w.printf("<titlepage><titlepage-items/></titlepage>\n")
	front := len(d.Preface) > 0 || len(d.Assumed) > 0
	if front && !article {
		w.printf("<preface xml:id=\"preface\">\n")
		w.front()
		w.printf("</preface>\n")
	}
	w.printf("</frontmatter>\n")
	if front && article {
		w.printf("<introduction xml:id=\"preface\">\n")
		w.front()
		w.printf("</introduction>\n")
	}
	for _, c := range d.Chapters {
		w.printf("<%s xml:id=\"%s\">\n<title>%s</title>\n", chapter, xmlID(c.ID), xmlText(c.Title))
		if len(c.Glue) > 0 {
			w.printf("<introduction>\n")
			for _, g := range c.Glue {
				w.markdown(g.Text)
			}
			w.printf("</introduction>\n")
		}
		for _, s := range c.Sections {
			w.printf("<%s xml:id=\"%s\">\n<title>%s</title>\n", section, xmlID(s.ID), xmlText(s.Title))
			for _, g := range s.Glue {
				w.markdown(g.Text)
			}
			for _, it := range s.Items {
				w.item(it)
			}
			w.printf("</%s>\n", section)
		}
		w.printf("</%s>\n", chapter)
	}
	w.back()
	w.printf("</%s>\n</pretext>\n", root)
	return w.b.String()
}

// macros returns the macros for docinfo, one \newcommand on each line, with the variants of the volume.
func (w *ptxWriter) macros() string {
	m := w.d.macros()
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(m)) {
		b.WriteString(`\newcommand{` + name + `}` + arity(m[name]) + `{` + m[name] + "}\n")
	}
	return b.String()
}

// front writes the preface and the nodes that the reader knows.
func (w *ptxWriter) front() {
	for _, g := range w.d.Preface {
		w.markdown(g.Text)
	}
	if len(w.d.Assumed) > 0 {
		w.printf("<p>The reader knows these nodes:<ul>\n")
		for _, r := range w.d.Assumed {
			w.printf("<li><url href=\"%s\">%s</url>, <c>%s</c></li>\n", xmlText(r.URL), xmlText(r.Name), xmlText(r.URI))
		}
		w.printf("</ul></p>\n")
	}
}

// ptxElement returns the PreTeXt element of an item, and the content model of the element: statement for an element with a statement, blocks for an element with blocks.
func ptxElement(it *Item) (element string, statement bool) {
	switch it.Kind {
	case "definition", "concept":
		return "definition", true
	case "statement":
		switch {
		case it.Claim != "":
			return "claim", true
		case slices.Contains([]string{"theorem", "lemma", "corollary", "fact", "axiom"}, it.Role):
			return it.Role, true
		}
		return "proposition", true
	case "conjecture":
		return "conjecture", true
	case "problem", "example", "counterexample", "computation", "exercise":
		if it.Kind == "counterexample" {
			return "example", false
		}
		return it.Kind, false
	}
	return "remark", false
}

func (w *ptxWriter) item(it *Item) {
	el, statement := ptxElement(it)
	title := it.Name
	if it.Kind == "counterexample" {
		title = strings.TrimSuffix("Counterexample: "+title, ": ")
	}
	if it.Claim != "" {
		title = strings.TrimPrefix(title+", "+it.Claim, ", ")
	}
	w.printf("<%s xml:id=\"%s\">\n", el, xmlID(it.anchor()))
	if title != "" {
		w.printf("<title>%s</title>\n", xmlText(title))
	}
	var notes []NotationUse
	for _, u := range w.d.Notation {
		if u.Introduced != "" && u.Introduced == it.Number {
			notes = append(notes, u)
		}
	}
	if el == "definition" {
		for _, u := range notes {
			w.notation(u)
		}
		notes = nil
	}
	if statement {
		w.printf("<statement>\n")
	}
	start := w.b.Len()
	if it.Characterizes != "" {
		w.printf("<p>An equivalent characterization of the definition in %s.</p>\n", w.ref(it.Characterizes, xmlText(it.Characterizes)))
	}
	w.markdown(it.Main)
	if w.b.Len() == start {
		w.printf("<p/>\n")
	}
	if statement {
		w.printf("</statement>\n")
	}
	if el != "axiom" && el != "conjecture" && statement {
		for _, p := range it.Proofs {
			w.printf("<proof xml:id=\"%s\">\n", xmlID(p.anchor()))
			start := w.b.Len()
			w.markdown(p.Main)
			if w.b.Len() == start {
				w.printf("<p/>\n")
			}
			w.printf("</proof>\n")
		}
	}
	w.printf("</%s>\n", el)
	if it.Remarks != "" {
		w.printf("<remark>\n<title>Remarks</title>\n")
		w.markdown(it.Remarks)
		w.printf("</remark>\n")
	}
	if len(notes) > 0 {
		w.printf("<p>")
		for _, u := range notes {
			w.notation(u)
		}
		w.printf("</p>\n")
	}
	if len(it.Sources) > 0 {
		w.printf("<p>Sources: ")
		for i, c := range it.Sources {
			if i > 0 {
				w.printf("; ")
			}
			if c.Locator != "" {
				w.printf("<xref ref=\"%s\" detail=\"%s\"/>", c.Key, xmlText(c.Locator))
			} else {
				w.printf("<xref ref=\"%s\"/>", c.Key)
			}
		}
		w.printf(".</p>\n")
	}
	w.printf("<p><url href=\"%s\"><c>m2://%s</c></url></p>\n", xmlText(w.d.ref(it.Tag).URL), it.Tag)
}

func (w *ptxWriter) notation(u NotationUse) {
	w.printf("<notation><usage><m>%s</m></usage><description>%s</description></notation>\n", xmlText(notationTeX(u)), xmlText(u.Reads))
}

// ref returns a link to a node: to its item when the volume has it, else to its page on the site.
func (w *ptxWriter) ref(tag, text string) string {
	if it := w.d.homes[tag]; it != nil {
		return `<xref ref="` + xmlID(it.anchor()) + `" text="custom">` + text + `</xref>`
	}
	if w.d.tree.nodes[tag] != nil {
		return `<url href="` + xmlText(w.d.ref(tag).URL) + `">` + text + `</url>`
	}
	return text
}

// back writes the back matter: the conventions, the notation, the cited nodes and the attribution as appendices, then the sources.
func (w *ptxWriter) back() {
	d := w.d
	w.printf("<backmatter>\n")
	if len(d.Conventions) > 0 {
		w.printf("<appendix xml:id=\"conventions\">\n<title>Conventions</title>\n<p><dl>\n")
		for _, c := range d.Conventions {
			w.printf("<li><title>%s</title><p>This volume uses %s", xmlText(c.ID), xmlText(c.Option))
			if c.Default {
				w.printf(", the default of math2")
			}
			w.printf(".")
			for _, x := range c.Differ {
				w.printf(" %s uses %s", w.ref(x.Tag, xmlText(x.Name)), xmlText(x.Option))
				if x.Note != "" {
					w.printf(": %s", xmlText(x.Note))
				} else {
					w.printf(".")
				}
			}
			w.printf("</p></li>\n")
		}
		w.printf("</dl></p>\n</appendix>\n")
	}
	if len(d.Notation) > 0 {
		w.printf("<appendix xml:id=\"notation\">\n<title>Notation</title>\n")
		var rest []NotationUse
		for _, u := range d.Notation {
			if u.Introduced == "" {
				rest = append(rest, u)
			}
		}
		if len(rest) > 0 {
			w.printf("<p>")
			for _, u := range rest {
				w.notation(u)
			}
			w.printf("</p>\n")
		}
		w.printf("<notation-list/>\n</appendix>\n")
	}
	if len(d.Cited) > 0 {
		w.printf("<appendix xml:id=\"cited\">\n<title>Cited nodes</title>\n<p><ul>\n")
		for _, r := range d.Cited {
			w.printf("<li><url href=\"%s\">%s</url>, <c>%s</c></li>\n", xmlText(r.URL), xmlText(r.Name), xmlText(r.URI))
		}
		w.printf("</ul></p>\n</appendix>\n")
	}
	w.printf("<appendix xml:id=\"attribution\">\n<title>Attribution</title>\n<p>%s</p>\n", xmlText(licence))
	a := d.Attribution
	if len(a.People) > 0 {
		w.printf("<p><ul>\n")
		for _, c := range a.People {
			w.printf("<li>%s</li>\n", xmlText(c.String()))
		}
		w.printf("</ul></p>\n")
	}
	if len(a.Agents) > 0 {
		w.printf("<p>Agent runs that helped:<ul>\n")
		for _, r := range a.Agents {
			w.printf("<li>%s</li>\n", xmlText(r))
		}
		w.printf("</ul></p>\n")
	}
	if len(a.NoLog) > 0 {
		var refs []string
		for _, t := range a.NoLog {
			refs = append(refs, w.ref(t, xmlText(t)))
		}
		w.printf("<p>These nodes have no provenance log: %s.</p>\n", strings.Join(refs, ", "))
	}
	w.printf("</appendix>\n")
	if len(d.Sources) > 0 {
		w.printf("<references xml:id=\"sources\">\n<title>Sources</title>\n")
		for _, s := range d.Sources {
			w.printf("<biblio type=\"raw\" xml:id=\"%s\">%s", s.Key, xmlText(s.Title))
			if s.URL != "" {
				w.printf(", <url href=\"%s\"/>", xmlText(s.URL))
			}
			w.printf(".</biblio>\n")
		}
		w.printf("</references>\n")
	}
	w.printf("</backmatter>\n")
}

// markdown writes a body or a part of the glue as PreTeXt blocks.
func (w *ptxWriter) markdown(src string) {
	if strings.TrimSpace(src) == "" {
		return
	}
	doc, source := markup.Parse(src)
	w.src = source
	w.blocks(doc)
}

func (w *ptxWriter) blocks(parent ast.Node) {
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		w.block(n)
	}
}

func (w *ptxWriter) block(n ast.Node) {
	switch n := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		w.printf("<p>")
		w.inlines(n)
		w.printf("</p>\n")
	case *ast.Heading:
		w.printf("<p><alert>")
		w.inlines(n)
		w.printf("</alert></p>\n")
	case *ast.CodeBlock, *ast.FencedCodeBlock:
		w.printf("<pre>")
		lines := n.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			w.printf("%s", xmlText(string(seg.Value(w.src))))
		}
		w.printf("</pre>\n")
	case *ast.Blockquote:
		w.printf("<blockquote>\n")
		w.blocks(n)
		w.printf("</blockquote>\n")
	case *ast.List:
		w.printf("<p>")
		w.list(n)
		w.printf("</p>\n")
	case *east.Table:
		w.printf("<tabular>\n")
		for row := n.FirstChild(); row != nil; row = row.NextSibling() {
			if _, ok := row.(*east.TableHeader); ok {
				w.printf("<row header=\"yes\">")
			} else {
				w.printf("<row>")
			}
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				w.printf("<cell>")
				w.inlines(cell)
				w.printf("</cell>")
			}
			w.printf("</row>\n")
		}
		w.printf("</tabular>\n")
	case *east.FootnoteList, *ast.HTMLBlock, *ast.ThematicBreak:
		// The footnotes are at their links. The reviewed HTML of a body has no PreTeXt form.
	default:
		w.blocks(n)
	}
}

// list writes a list. An item with one line of text is text, and an item with more is paragraphs.
func (w *ptxWriter) list(n *ast.List) {
	el := "ul"
	if n.IsOrdered() {
		el = "ol"
	}
	w.printf("<%s>\n", el)
	for li := n.FirstChild(); li != nil; li = li.NextSibling() {
		w.printf("<li>")
		if _, ok := li.FirstChild().(*ast.TextBlock); ok && li.ChildCount() == 1 {
			w.inlines(li.FirstChild())
		} else {
			w.blocks(li)
		}
		w.printf("</li>\n")
	}
	w.printf("</%s>", el)
}

func (w *ptxWriter) inlines(parent ast.Node) {
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		w.inline(n)
	}
}

func (w *ptxWriter) inline(n ast.Node) {
	switch n := n.(type) {
	case *ast.Text:
		w.printf("%s", xmlText(string(n.Segment.Value(w.src))))
		if n.SoftLineBreak() || n.HardLineBreak() {
			w.printf("\n")
		}
	case *ast.String:
		w.printf("%s", xmlText(string(n.Value)))
	case *markup.Math:
		el := "m"
		if n.Display {
			el = "md"
		}
		w.printf("<%s>%s</%s>", el, xmlText(strings.TrimSpace(string(n.TeX))), el)
	case *ast.CodeSpan:
		w.printf("<c>")
		w.inlines(n)
		w.printf("</c>")
	case *ast.Emphasis:
		el := "em"
		if n.Level > 1 {
			el = "alert"
		}
		w.printf("<%s>", el)
		w.inlines(n)
		w.printf("</%s>", el)
	case *ast.Link:
		saved := w.b
		w.b = &strings.Builder{}
		w.inlines(n)
		text := w.b.String()
		w.b = saved
		dest := string(n.Destination)
		t, ok := strings.CutPrefix(dest, markup.NodeScheme)
		if !ok {
			t, ok = strings.CutPrefix(dest, "m2://")
		}
		if ok {
			w.printf("%s", w.ref(t, text))
		} else {
			w.printf("<url href=\"%s\">%s</url>", xmlText(dest), text)
		}
	case *ast.AutoLink:
		w.printf("<url href=\"%s\"/>", xmlText(string(n.URL(w.src))))
	case *east.FootnoteLink:
		w.printf("<fn>")
		if fn := footnote(n); fn != nil {
			for p := fn.FirstChild(); p != nil; p = p.NextSibling() {
				if p != fn.FirstChild() {
					w.printf(" ")
				}
				w.inlines(p)
			}
		}
		w.printf("</fn>")
	case *ast.RawHTML, *east.FootnoteBacklink:
	default:
		w.inlines(n)
	}
}

// xmlText escapes text for XML content and attributes. It drops the control characters that XML does not allow.
func xmlText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r < 0x20 && r != '\t' && r != '\n' && r != '\r', r == 0xFFFE, r == 0xFFFF:
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// xmlID returns an id as an XML name: the id when it is one, else m2- and the id with each other character as a hyphen.
func xmlID(id string) string {
	ok := id != ""
	out := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' {
			return r
		}
		ok = false
		return '-'
	}, id)
	if !ok || out[0] < 'A' || out[0] > 'z' || out[0] > 'Z' && out[0] < 'a' && out[0] != '_' {
		return "m2-" + out
	}
	return out
}
