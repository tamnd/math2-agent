package wpcorpus

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/wikipedia-cli/wiki/parse"
	"github.com/tamnd/wikipedia-cli/wiki/texvc"
)

// Resolver gives the page id of an article title, or 0 when it does not
// know the title.
type Resolver func(title string) int

// renderHeader is the YAML header of the Markdown rendering of a page. It
// gives the attribution that CC BY-SA asks for, as the header of the content
// file does.
type renderHeader struct {
	Page      int    `yaml:"page"`
	Title     string `yaml:"title"`
	Revision  int    `yaml:"revision"`
	Timestamp string `yaml:"timestamp,omitempty"`
	Source    string `yaml:"source"`
	History   string `yaml:"history"`
	Licence   string `yaml:"licence"`
	Renderer  string `yaml:"renderer"`
}

// block is one block of the rendering: a heading, a paragraph, a list item,
// a table or a note. start and end are the bytes of the wikitext that the
// block comes from.
type block struct {
	text       string
	start, end int
	// tight is true for a list item or a table row. Two tight blocks of the
	// same list or table are on lines that follow, with no empty line.
	tight bool
	// group is the number of the table of a row, or 0. A fence does not
	// start or end inside a table.
	group int
}

type renderer struct {
	x       *extractor
	r       *Result
	resolve Resolver
	blocks  []block
	notes   []string
	// labels gives the label of the note of a named ref.
	labels map[string]string
	// cites gives the ref of the citation object of each <ref> by its
	// first byte.
	cites  map[int]string
	nNotes int
	groups int
	// listMark is the marker of the last list block, so that a list of
	// one type continues with the same marker.
	listMark string
}

// Render gives the Markdown rendering of a page revision (WP3.12). It needs
// the result of Extract for the same page, because each object of the
// result becomes a fenced block. version names the renderer.
func Render(p *Page, m *Manifests, r *Result, resolve Resolver, version string) ([]byte, error) {
	doc, err := parse.Parse(p.Wikitext)
	if err != nil {
		return nil, fmt.Errorf("page %d: %w", p.ID, err)
	}
	x := &extractor{doc: doc, m: m, page: p, typ: ArticleType(p.Title), ver: version, unmapped: map[string]int{}, unheads: map[string]bool{}, refs: map[string]*object{}}
	rd := &renderer{x: x, r: r, resolve: resolve, labels: map[string]string{}, cites: map[int]string{}}
	for _, o := range r.Objects {
		if o.Kind == "citation" && o.Cue == "ref" {
			rd.cites[o.Span[0]] = o.Ref
			if o.Name != "" {
				rd.labels[o.Name] = o.Ref
			}
		}
	}
	for _, c := range doc.Root.Children {
		rd.section(c)
	}

	var b bytes.Buffer
	h := renderHeader{Page: p.ID, Title: p.Title, Revision: p.Revision, Timestamp: p.Timestamp,
		Source:   "https://en.wikipedia.org/w/index.php?oldid=" + strconv.Itoa(p.Revision),
		History:  fmt.Sprintf("https://en.wikipedia.org/w/index.php?curid=%d&action=history", p.ID),
		Licence:  licence,
		Renderer: version}
	if err := writeFrontMatter(&b, h); err != nil {
		return nil, err
	}
	fmt.Fprintf(&b, "\n# %s\n", escapeText(p.Title))
	rd.write(&b)
	if len(rd.notes) > 0 {
		b.WriteString("\n")
		for _, n := range rd.notes {
			b.WriteString(n + "\n")
		}
	}
	return b.Bytes(), nil
}

// section renders a section node: its heading and its blocks.
func (rd *renderer) section(n *parse.Node) {
	for _, c := range n.Children {
		switch c.Kind {
		case parse.KindHeading:
			text := rd.inline(c.Children)
			if text == "" {
				continue
			}
			h := strings.Repeat("#", min(max(c.Level, 2), 6)) + " " + text
			if a := parse.Anchor(rd.x.doc.HeadingText(n)); a != "" {
				h += " {#" + a + "}"
			}
			rd.add(block{text: h, start: c.Start, end: c.End})
		case parse.KindSection:
			rd.section(c)
		case parse.KindList:
			rd.list(c)
		case parse.KindTable:
			rd.table(c)
		case parse.KindRule:
			rd.add(block{text: "* * *", start: c.Start, end: c.End})
		case parse.KindComment:
		case parse.KindParagraph, parse.KindPre:
			rd.paragraph(c.Children, c.Start, c.End)
		default:
			rd.paragraph([]*parse.Node{c}, c.Start, c.End)
		}
	}
}

func (rd *renderer) add(b block) {
	if !b.tight {
		rd.listMark = ""
	}
	rd.blocks = append(rd.blocks, b)
}

// para collects the text of a run of inline nodes. A node that is a block of
// its own (a display formula, an image, an item template) ends the current
// piece and adds its block.
type para struct {
	rd         *renderer
	b          strings.Builder
	start, end int
	out        *[]block
	// inlineOnly is true for the text of a heading, a table cell or a
	// note. Its blocks are on one line, and a display formula is inline.
	inlineOnly bool
}

func (rd *renderer) paragraph(nodes []*parse.Node, start, end int) {
	p := &para{rd: rd, start: start, end: end, out: &rd.blocks}
	p.nodes(nodes)
	p.flush()
}

// inline gives the text of nodes on one line. The blocks inside the nodes
// are joined with a space.
func (rd *renderer) inline(nodes []*parse.Node) string {
	var out []block
	p := &para{rd: rd, out: &out, inlineOnly: true}
	p.nodes(nodes)
	p.flush()
	parts := make([]string, 0, len(out))
	for _, b := range out {
		parts = append(parts, b.text)
	}
	return strings.Join(parts, " ")
}

// flush ends the current piece at the end of the node.
func (p *para) flush() { p.flushTo(p.end) }

// flushTo ends the current piece at the byte end of the wikitext. An empty
// line in the text splits it into two paragraphs.
func (p *para) flushTo(end int) {
	s := p.b.String()
	p.b.Reset()
	for _, part := range splitParagraphs(s) {
		if t := collapse(part); t != "" {
			// A paragraph of one formula is a display formula, as an
			// indented formula on Wikipedia is.
			if m := loneMath.FindStringSubmatch(t); m != nil && !p.inlineOnly {
				t = "$$" + m[1] + m[2] + "$$"
			}
			if p.out == &p.rd.blocks {
				p.rd.listMark = ""
			}
			*p.out = append(*p.out, block{text: escapeStart(t), start: p.start, end: max(end, p.start)})
		}
	}
}

// own adds a block for one node, and the paragraph goes on after it.
func (p *para) own(n *parse.Node, text string) {
	if p.inlineOnly {
		if m := displayMath.FindStringSubmatch(text); m != nil {
			text = "$" + m[1] + "$"
		}
		p.b.WriteString(" " + text + " ")
		return
	}
	p.flushTo(n.Start)
	if text != "" {
		if p.out == &p.rd.blocks {
			p.rd.listMark = ""
		}
		*p.out = append(*p.out, block{text: text, start: n.Start, end: n.End})
	}
	if p.end > n.End {
		p.start = n.End
	}
}

var indentStart = regexp.MustCompile(`^[:;*#]+[ \t]*`)

var indentLine = regexp.MustCompile(`\n[:;*#]+[ \t]*`)

var magicWord = regexp.MustCompile(`__[A-Z]+__`)

func (p *para) nodes(nodes []*parse.Node) {
	rd, x := p.rd, p.rd.x
	for _, n := range nodes {
		switch n.Kind {
		case parse.KindText:
			t := magicWord.ReplaceAllString(x.doc.Raw(n), "")
			// An indent or a list mark at the start of a line in a
			// template parameter is not parsed as a list. It starts a new
			// paragraph.
			t = indentLine.ReplaceAllString(t, "\n\n")
			if strings.HasSuffix(p.b.String(), "\n") {
				t = indentStart.ReplaceAllString(t, "\n")
			}
			p.b.WriteString(escapeText(html.UnescapeString(t)))
		case parse.KindBold:
			p.b.WriteString(wrap(rd.inline(n.Children), "**"))
		case parse.KindItalic:
			p.b.WriteString(wrap(rd.inline(n.Children), "*"))
		case parse.KindLink:
			p.link(n)
		case parse.KindExtLink:
			label := rd.inline(n.Children)
			if label == "" {
				p.b.WriteString("<" + n.Name + ">")
			} else {
				p.b.WriteString("[" + label + "](" + escapeURL(n.Name) + ")")
			}
		case parse.KindTag:
			p.tag(n)
		case parse.KindTemplate:
			p.template(n)
		case parse.KindList, parse.KindTable, parse.KindParagraph, parse.KindSection, parse.KindPre, parse.KindListItem, parse.KindRow, parse.KindCell, parse.KindCaption, parse.KindHeading:
			p.nodes(n.Children)
			p.b.WriteString(" ")
		}
	}
}

func (p *para) link(n *parse.Node) {
	rd := p.rd
	l := linkTarget(n.Name)
	if l == nil {
		t := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(n.Name, ":")))
		switch {
		case strings.HasPrefix(t, "file:") || strings.HasPrefix(t, "image:"):
			if !strings.HasPrefix(strings.TrimSpace(n.Name), ":") {
				p.own(n, rd.image(n))
				return
			}
		case strings.HasPrefix(t, "category:"):
			return
		}
		// A link to another wiki or namespace keeps its label.
		label := rd.inline(n.Children)
		if label == "" {
			label = escapeText(strings.TrimPrefix(n.Name, ":"))
		}
		p.b.WriteString(label + escapeText(n.Trail))
		return
	}
	label := rd.inline(n.Children)
	if label == "" {
		label = escapeText(strings.TrimPrefix(strings.TrimSpace(n.Name), ":"))
	}
	label += escapeText(n.Trail)
	p.b.WriteString("[" + label + "](" + rd.linkURL(l) + ")")
}

func (rd *renderer) linkURL(l *Link) string {
	anchor := ""
	if l.Anchor != "" {
		anchor = "#" + escapeURL(l.Anchor)
	}
	if rd.resolve != nil {
		if id := rd.resolve(l.Title); id != 0 {
			return "wikipedia://en/" + strconv.Itoa(id) + anchor
		}
	}
	return "https://en.wikipedia.org/wiki/" + escapeURL(strings.ReplaceAll(l.Title, " ", "_")) + anchor
}

// image gives the placeholder of an image: the file name and the caption.
// The caption is the last part of the label, after the options.
func (rd *renderer) image(n *parse.Node) string {
	name := strings.TrimSpace(n.Name)
	if i := strings.IndexByte(name, ':'); i >= 0 {
		name = strings.TrimSpace(name[i+1:])
	}
	// The label is the options and the caption, split by "|" at the top
	// level. The caption is the last part that is not an option.
	segs := []string{""}
	for _, c := range n.Children {
		if c.Kind != parse.KindText {
			segs[len(segs)-1] += rd.inline([]*parse.Node{c})
			continue
		}
		parts := strings.Split(rd.x.doc.Raw(c), "|")
		for i, part := range parts {
			if i > 0 {
				segs = append(segs, "")
			}
			segs[len(segs)-1] += escapeText(html.UnescapeString(part))
		}
	}
	var caption string
	for i := len(segs) - 1; i >= 0 && caption == ""; i-- {
		if c := collapse(segs[i]); c != "" && !isImageOption(c) {
			caption = c
		}
	}
	out := "*Image: " + escapeText(name) + ".*"
	if caption != "" {
		out += " " + caption
	}
	return out
}

var imageOption = regexp.MustCompile(`^(thumb|thumbnail|frame|frameless|border|left|right|center|centre|none|upright(=[0-9.]+)?|[0-9]*x?[0-9]+px|(alt|link|lang|page|class)=.*)$`)

func isImageOption(s string) bool { return imageOption.MatchString(strings.TrimSpace(s)) }

func (p *para) tag(n *parse.Node) {
	rd, x := p.rd, p.rd.x
	switch n.Name {
	case "math":
		res := texvc.FromMathTag(x.doc.Inner(n), n.Attrs)
		p.math(n, res.TeX, res.Display)
	case "chem", "ce":
		p.math(n, `\ce{`+strings.TrimSpace(x.doc.Inner(n))+`}`, false)
	case "nowiki":
		p.b.WriteString(escapeText(html.UnescapeString(x.doc.Inner(n))))
	case "code", "tt", "kbd", "samp":
		inner := x.doc.Inner(n)
		if len(n.Children) > 0 {
			inner = x.doc.Plain(n.Children)
		}
		if s := strings.TrimSpace(html.UnescapeString(inner)); s != "" {
			p.b.WriteString(codeSpan(s))
		}
	case "pre", "syntaxhighlight", "source":
		code := strings.Trim(html.UnescapeString(x.doc.Inner(n)), "\n")
		if strings.TrimSpace(code) != "" {
			fence := "```"
			for strings.Contains(code, fence) {
				fence += "`"
			}
			p.own(n, fence+n.Attrs["lang"]+"\n"+code+"\n"+fence)
		}
	case "ref":
		p.b.WriteString(rd.note(n))
	case "references", "gallery", "timeline", "score", "graph", "templatestyles", "includeonly", "noinclude", "mapframe", "maplink", "imagemap":
	case "br":
		p.b.WriteString(" ")
	case "sup", "sub":
		if s := rd.inline(n.Children); s != "" {
			p.b.WriteString("<" + n.Name + ">" + s + "</" + n.Name + ">")
		}
	case "blockquote":
		var out []block
		q := &para{rd: rd, out: &out}
		q.nodes(n.Children)
		q.flush()
		var lines []string
		for _, b := range out {
			lines = append(lines, "> "+b.text)
		}
		p.own(n, strings.Join(lines, "\n>\n"))
	default:
		p.nodes(n.Children)
	}
}

func (p *para) math(n *parse.Node, tex string, display bool) {
	tex = strings.Join(strings.Fields(tex), " ")
	if tex == "" {
		return
	}
	if display {
		p.own(n, "$$"+tex+"$$")
		return
	}
	p.b.WriteString("$" + tex + "$")
}

// note writes the note of a <ref> and gives its mark. A named ref that is
// used again gets the mark of its first use.
func (rd *renderer) note(n *parse.Node) string {
	name := strings.TrimSpace(n.Attrs["name"])
	if n.SelfClosing || n.InnerEnd <= n.InnerStart {
		if name == "" {
			return ""
		}
		l, ok := rd.labels[name]
		if !ok {
			rd.nNotes++
			l = "n" + strconv.Itoa(rd.nNotes)
			rd.labels[name] = l
		}
		return "[^" + l + "]"
	}
	l := rd.cites[n.Start]
	if l == "" {
		if name != "" && rd.labels[name] != "" {
			l = rd.labels[name]
		} else {
			rd.nNotes++
			l = "n" + strconv.Itoa(rd.nNotes)
		}
	}
	if name != "" {
		rd.labels[name] = l
	}
	text := rd.inline(n.Children)
	if text == "" {
		return ""
	}
	mark := "[^" + l + "]"
	for _, have := range rd.notes {
		if strings.HasPrefix(have, mark+":") {
			return mark
		}
	}
	rd.notes = append(rd.notes, mark+": "+text)
	return mark
}

// charTemplates gives the text of the templates that write one character.
var charTemplates = map[string]string{"mdash": "—", "ndash": "–", "snd": " – ", "spnd": " – ", "spaced ndash": " – ", "nbsp": " ", "thinsp": " ", "hairsp": "", "sp": " ", "zwsp": "", "middot": "·", "dot": "·", "times": "×", "pm": "±", "minus": "−", "'": "'", "\"": "\"", "=": "=", "!": "|", "!!": "||"}

var listTemplates = map[string]bool{"unbulleted list": true, "ubl": true, "plainlist": true, "plain list": true, "flatlist": true, "flat list": true, "hlist": true, "bulleted list": true, "bulleted": true, "ordered list": true, "unordered list": true}

// layout gives the text of the layout templates that need more than one
// parameter, or false for the other templates.
func (rd *renderer) layout(n *parse.Node, key string) (string, bool) {
	text := func(name string) string {
		if v := n.Param(name); v != nil {
			return strings.TrimSpace(rd.inline(v.Value))
		}
		return ""
	}
	switch key {
	case "lang", "langx":
		return text("2"), true
	case "nihongo":
		return text("1"), true
	case "convert":
		return strings.TrimSpace(text("1") + " " + text("2")), true
	case "circa":
		return strings.TrimSpace("c. " + text("1")), true
	case "birth date", "death date", "birth date and age", "death date and age":
		parts := []string{text("1"), text("2"), text("3")}
		return strings.Trim(strings.Join(parts, "-"), "-"), true
	}
	return "", false
}

// val gives the text of {{val}}: the number, the uncertainty and the unit.
func (rd *renderer) val(n *parse.Node) string {
	text := func(name string) string {
		if v := n.Param(name); v != nil {
			return strings.TrimSpace(rd.inline(v.Value))
		}
		return ""
	}
	s := text("1")
	if u := text("2"); u != "" {
		if l := text("3"); l != "" {
			s += " +" + u + " " + l
		} else {
			s += " ± " + u
		}
	}
	if e := text("e"); e != "" {
		s += " × 10<sup>" + e + "</sup>"
	}
	if u := cmpOr(text("u"), text("ul")); u != "" {
		s += " " + u
	}
	return s
}

// numbered writes {{NumBlk}}: a display formula with its label as a tag.
func (p *para) numbered(n *parse.Node) {
	rd := p.rd
	v := n.Param("2")
	if v == nil {
		return
	}
	var out []block
	q := &para{rd: rd, out: &out}
	q.nodes(v.Value)
	q.flush()
	var parts []string
	for _, b := range out {
		parts = append(parts, b.text)
	}
	body := strings.Trim(strings.TrimSpace(strings.Join(parts, " ")), "$")
	label := ""
	if l := n.Param("3"); l != nil {
		label = strings.Trim(strings.TrimSpace(rd.inline(l.Value)), "()")
	}
	if body == "" {
		return
	}
	if !strings.Contains(body, "$") {
		if label != "" {
			body += ` \tag{` + label + `}`
		}
		p.own(n, "$$"+body+"$$")
		return
	}
	// The block has text and math. The label goes at the end.
	if label != "" {
		body += " (" + label + ")"
	}
	p.own(n, body)
}

// footnote adds a note with the text and gives its mark. Two notes with
// the same text have one mark.
func (rd *renderer) footnote(text string) string {
	for _, have := range rd.notes {
		if mark, t, _ := strings.Cut(have, ": "); t == text {
			return mark
		}
	}
	rd.nNotes++
	mark := "[^n" + strconv.Itoa(rd.nNotes) + "]"
	rd.notes = append(rd.notes, mark+": "+text)
	return mark
}

func (p *para) template(n *parse.Node) {
	rd, x := p.rd, p.rd.x
	key := parse.TemplateKey(n.Name)
	t := x.m.Template(key)
	if c, ok := charTemplates[key]; ok {
		p.b.WriteString(c)
		return
	}
	if listTemplates[key] {
		// A list template gives its items on one line.
		first := true
		for _, prm := range n.Params {
			if prm.Name != "" {
				continue
			}
			if txt := strings.TrimSpace(rd.inline(prm.Value)); txt != "" {
				if !first {
					p.b.WriteString("; ")
				}
				p.b.WriteString(txt)
				first = false
			}
		}
		return
	}
	if t == nil {
		// WP3.2: until a template is mapped, its text is kept. The first
		// positional parameter is the text of most inline templates.
		if v := n.Param("1"); v != nil && !strings.HasPrefix(key, "#") && !strings.Contains(key, ":") {
			p.nodes(v.Value)
		}
		return
	}
	switch t.Class {
	case ClassMath:
		switch t.Key {
		case "val":
			p.b.WriteString(rd.val(n))
			return
		case "numblk":
			p.numbered(n)
			return
		}
		var b strings.Builder
		x.renderMath(&b, n, t)
		s := b.String()
		switch {
		case strings.HasPrefix(s, "\n$$"):
			p.own(n, "$$"+strings.Join(strings.Fields(strings.Trim(strings.TrimSpace(s), "$")), " ")+"$$")
		case strings.HasPrefix(s, "$"):
			p.math(n, strings.TrimSuffix(strings.TrimPrefix(s, "$"), "$"), false)
		default:
			// numblk and the equation refs give text with math in it.
			p.b.WriteString(s)
		}
	case ClassLayout, ClassAnchor:
		if t.Class == ClassAnchor && t.Key != "vanchor" {
			return
		}
		if s, ok := rd.layout(n, t.Key); ok {
			p.b.WriteString(s)
			return
		}
		for _, role := range []string{"text", "link", "name"} {
			if name := t.Param(role); name != "" && n.Param(name) != nil && (t.Class == ClassAnchor || role != "name") {
				p.nodes(n.Param(name).Value)
				return
			}
		}
	case ClassTerm:
		if t.Key == "term" {
			for _, name := range []string{"1", "term"} {
				if v := n.Param(name); v != nil {
					p.own(n, "**"+strings.Trim(rd.inline(v.Value), "*")+"**")
					return
				}
			}
			return
		}
		if t.Key != "defn" {
			// {{dfn}} and the others mark a term in a sentence.
			if v := n.Param("1"); v != nil {
				p.b.WriteString(wrap(strings.Trim(rd.inline(v.Value), "*"), "**"))
			}
			return
		}
		p.flushTo(n.Start)
		for _, name := range []string{"1", "term", "defn", "content", "2"} {
			if v := n.Param(name); v != nil && t.Params[name] != "" && t.Params[name] != "flag" {
				q := &para{rd: rd, start: n.Start, end: n.End, out: p.out}
				q.nodes(v.Value)
				q.flush()
			}
		}
		p.start = n.End
	case ClassItem:
		if t.Key == "numblk" {
			p.numbered(n)
			return
		}
		p.flushTo(n.Start)
		head, body := itemParams(n, t)
		var title string
		if head != "" {
			title = rd.inline(n.Param(head).Value)
		}
		if t.Key == "math theorem" {
			if title == "" {
				title = "Theorem"
			}
			if v := n.Param("note"); v != nil {
				if note := rd.inline(v.Value); note != "" {
					title += " (" + note + ")"
				}
			}
		}
		if title = strings.Trim(title, "* "); title != "" {
			*p.out = append(*p.out, block{text: "**" + title + ".**", start: n.Start, end: n.End})
		}
		for _, name := range body {
			q := &para{rd: rd, start: n.Start, end: n.End, out: p.out}
			q.nodes(n.Param(name).Value)
			q.flush()
		}
		p.start = n.End
	case ClassCitation:
		c := rd.citation(n, t)
		if strings.HasPrefix(t.Key, "sfn") && c != "" {
			// A short footnote is a note, as a <ref> is.
			c = rd.footnote(strings.TrimSuffix(strings.TrimPrefix(c, "("), ")") + ".")
		}
		p.b.WriteString(c)
	}
}

// citation gives the text of a citation template: the authors, the year,
// the title, the work and the identifiers. A short citation (sfn, harv)
// gives the authors, the year and the page.
func (rd *renderer) citation(n *parse.Node, t *Template) string {
	f := rd.x.fields(n)
	if f == nil {
		return ""
	}
	if strings.HasPrefix(t.Key, "sfn") || strings.HasPrefix(t.Key, "harv") {
		var parts []string
		for i := 1; i <= 5; i++ {
			if v := f[strconv.Itoa(i)]; v != "" {
				parts = append(parts, v)
			}
		}
		s := strings.Join(parts, " ")
		if pg := cmpOr(f["p"], f["pp"], f["page"], f["pages"], f["loc"]); pg != "" {
			s += ", " + pg
		}
		return "(" + escapeText(s) + ")"
	}
	var authors []string
	if a := cmpOr(f["author"], f["authors"]); a != "" {
		authors = append(authors, a)
	}
	for i := 1; i <= 8; i++ {
		s := strconv.Itoa(i)
		last, first := cmpOr(f["last"+s], ifOne(i, f["last"]), f["surname"+s], ifOne(i, f["surname"])), cmpOr(f["first"+s], ifOne(i, f["first"]), f["given"+s], ifOne(i, f["given"]))
		if last == "" {
			if a := f["author"+s]; a != "" && i > 1 {
				authors = append(authors, a)
			}
			continue
		}
		if first != "" {
			last += ", " + first
		}
		authors = append(authors, last)
	}
	var out []string
	if len(authors) > 0 {
		out = append(out, escapeText(strings.Join(authors, "; ")))
	}
	if y := cmpOr(f["year"], f["date"]); y != "" {
		out = append(out, "("+escapeText(y)+")")
	}
	if title := cmpOr(f["title"], f["chapter"], f["text"]); title != "" {
		out = append(out, "\""+escapeText(title)+"\"")
	}
	if w := cmpOr(f["journal"], f["work"], f["website"], f["magazine"], f["book-title"], f["encyclopedia"], f["periodical"]); w != "" {
		out = append(out, "*"+escapeText(w)+"*")
	}
	if v := f["volume"]; v != "" {
		s := v
		if is := f["issue"]; is != "" {
			s += " (" + is + ")"
		}
		out = append(out, escapeText(s))
	}
	if pub := f["publisher"]; pub != "" {
		out = append(out, escapeText(pub))
	}
	if pg := cmpOr(f["pages"], f["page"], f["p"], f["pp"]); pg != "" {
		out = append(out, "p. "+escapeText(pg))
	}
	for _, id := range []string{"doi", "isbn", "arxiv", "mr", "zbl", "jstor"} {
		if v := f[id]; v != "" {
			out = append(out, id+":"+escapeText(v))
		}
	}
	if u := f["url"]; u != "" && f["doi"] == "" {
		out = append(out, "<"+u+">")
	}
	if len(out) == 0 {
		return ""
	}
	return joinSentences(out)
}

// joinSentences joins the parts of a citation with a full stop. A part
// that ends with a full stop gets no second one.
func joinSentences(parts []string) string {
	var b strings.Builder
	for i, s := range parts {
		if i > 0 {
			if !strings.HasSuffix(parts[i-1], ".") {
				b.WriteByte('.')
			}
			b.WriteByte(' ')
		}
		b.WriteString(s)
	}
	return b.String()
}

// itemParams gives the parameter of the title of an item template and the
// parameters of its text, in the order of the text. {{math theorem}} reads
// {{math theorem|Statement}} and {{math theorem|Name|Statement}}.
func itemParams(n *parse.Node, t *Template) (head string, body []string) {
	if t.Key == "math theorem" && n.Param("2") != nil && n.Param("math_statement") == nil && n.Param("name") == nil {
		return "1", []string{"2"}
	}
	for _, name := range []string{"name", "title"} {
		if n.Param(name) != nil && t.Params[name] != "" {
			head = name
			break
		}
	}
	for _, name := range []string{"term", "1", "math_statement", "proof", "statement", "content", "defn", "2"} {
		if r := t.Params[name]; n.Param(name) != nil && r != "" && r != "flag" && name != head && (t.Key != "math theorem" || name != "note") {
			body = append(body, name)
		}
	}
	return head, body
}

func ifOne(i int, v string) string {
	if i == 1 {
		return v
	}
	return ""
}

func cmpOr(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// list renders the items of a list. A bullet item or a numbered item is a
// tight block. A term (;) and an indented line (:) are blocks of their own.
func (rd *renderer) list(l *parse.Node) {
	// widths[d] is the width of the marker at the depth d.
	widths := map[int]int{}
	for _, it := range l.Children {
		if it.Kind != parse.KindListItem {
			rd.paragraph([]*parse.Node{it}, it.Start, it.End)
			continue
		}
		mark := it.Name
		if mark != "" {
			mark = mark[len(mark)-1:]
		}
		var out []block
		p := &para{rd: rd, start: it.Start, end: it.End, out: &out}
		p.nodes(it.Children)
		p.flush()
		if len(out) == 0 {
			continue
		}
		switch mark {
		case "*", "#":
			indent := 0
			for d := 1; d < it.Level; d++ {
				indent += widths[d]
			}
			m := "- "
			if mark == "#" {
				m = "1. "
			}
			widths[it.Level] = len(m)
			texts := make([]string, 0, len(out))
			for _, b := range out {
				texts = append(texts, b.text)
			}
			line := strings.Repeat(" ", indent) + m + strings.Join(texts, " ")
			if rd.listMark != "" && rd.listMark != mark && it.Level == 1 {
				// A new list type at the top needs an empty line, or the
				// Markdown reads it as one list.
				rd.add(block{text: "<!-- -->", start: it.Start, end: it.Start})
			}
			rd.blocks = append(rd.blocks, block{text: line, start: it.Start, end: it.End, tight: true})
			if it.Level == 1 {
				rd.listMark = mark
			}
		case ";":
			for i := range out {
				out[i].text = "**" + strings.Trim(out[i].text, "*") + "**"
			}
			for _, b := range out {
				rd.add(b)
			}
		default:
			for _, b := range out {
				rd.add(b)
			}
		}
	}
}

var droppedTable = regexp.MustCompile(`\b(infobox|navbox|sidebar|metadata|vertical-navbox|ambox|toccolours|plainlinks)\b`)

// table renders a table as a pipe table. The first row is the header.
func (rd *renderer) table(t *parse.Node) {
	if droppedTable.MatchString(t.Attrs["class"]) {
		return
	}
	var caption string
	var rows [][]string
	var spans [][2]int
	var carry []int
	for _, c := range t.Children {
		switch c.Kind {
		case parse.KindCaption:
			caption = rd.inline(c.Children)
		case parse.KindRow:
			var cells []string
			col := 0
			// carry gives the cells of the rows above that span this row.
			fill := func() {
				for col < len(carry) && carry[col] > 0 {
					carry[col]--
					cells = append(cells, "")
					col++
				}
			}
			for _, cell := range c.Children {
				if cell.Kind != parse.KindCell {
					continue
				}
				fill()
				text := strings.ReplaceAll(rd.inline(cell.Children), "|", `\|`)
				span := max(atoiOr(cell.Attrs["colspan"], 1), 1)
				rows := max(atoiOr(cell.Attrs["rowspan"], 1), 1)
				for k := 0; k < span; k++ {
					if k == 0 {
						cells = append(cells, text)
					} else {
						cells = append(cells, "")
					}
					for len(carry) <= col {
						carry = append(carry, 0)
					}
					carry[col] = rows - 1
					col++
				}
			}
			fill()
			if strings.Trim(strings.Join(cells, ""), " ") != "" {
				rows = append(rows, cells)
				spans = append(spans, [2]int{c.Start, c.End})
			}
		}
	}
	if len(rows) == 0 {
		return
	}
	if caption != "" {
		rd.add(block{text: "*" + strings.Trim(caption, "*") + "*", start: t.Start, end: t.Start})
	}
	cols := 0
	for _, r := range rows {
		cols = max(cols, len(r))
	}
	rd.groups++
	g := rd.groups
	line := func(r []string) string {
		for len(r) < cols {
			r = append(r, "")
		}
		return "| " + strings.Join(r, " | ") + " |"
	}
	head := line(rows[0]) + "\n|" + strings.Repeat("---|", cols)
	rd.add(block{text: head, start: spans[0][0], end: spans[0][1], tight: true, group: g})
	for i := 1; i < len(rows); i++ {
		rd.blocks = append(rd.blocks, block{text: line(rows[i]), start: spans[i][0], end: spans[i][1], tight: true, group: g})
	}
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(strings.Trim(strings.TrimSpace(s), `"'`)); err == nil {
		return n
	}
	return def
}

// fence is the fenced block of one object, over the blocks from first to
// last.
type fence struct {
	first, last int
	open        string
}

var noFence = map[string]bool{"section": true, "citation": true, "infobox": true}

// fences gives the fenced block of each object that can have one. An object
// covers each block that its span overlaps. An object that would break a
// table or cross another fence gets no fence. Its record is in the objects
// file.
func (rd *renderer) fences() []fence {
	objs := slices.Clone(rd.r.Objects)
	slices.SortStableFunc(objs, func(a, b Object) int {
		if a.Span[0] != b.Span[0] {
			return a.Span[0] - b.Span[0]
		}
		return b.Span[1] - a.Span[1]
	})
	var out []fence
	for _, o := range objs {
		if noFence[o.Kind] {
			continue
		}
		first, last := -1, -1
		for i, b := range rd.blocks {
			if b.end > o.Span[0] && b.start < o.Span[1] {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first < 0 {
			continue
		}
		if g := rd.blocks[first].group; g != 0 && first > 0 && rd.blocks[first-1].group == g {
			continue
		}
		if g := rd.blocks[last].group; g != 0 && last+1 < len(rd.blocks) && rd.blocks[last+1].group == g {
			continue
		}
		ok := true
		for _, f := range out {
			inside := f.first <= first && last <= f.last
			around := first <= f.first && f.last <= last
			apart := last < f.first || f.last < first
			if !inside && !around && !apart {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		out = append(out, fence{first: first, last: last, open: fenceOpen(&o)})
	}
	// Open the outer fence first. At the same blocks, the first object is
	// the outer one.
	slices.SortStableFunc(out, func(a, b fence) int {
		if a.first != b.first {
			return a.first - b.first
		}
		return b.last - a.last
	})
	return out
}

func fenceOpen(o *Object) string {
	kind := o.Kind
	class, _, _ := strings.Cut(kind, "/")
	s := "::: {#" + o.Ref + " ." + class
	if kind != class {
		s += " kind=" + kind
	}
	s += " cue=" + o.Cue
	if o.Name != "" {
		s += " name=\"" + strings.ReplaceAll(strings.ReplaceAll(o.Name, `\`, `\\`), `"`, `\"`) + "\""
	}
	return s + "}"
}

// write writes the blocks with their fences.
func (rd *renderer) write(b *bytes.Buffer) {
	fs := rd.fences()
	opens := map[int][]string{}
	closes := map[int]int{}
	for _, f := range fs {
		opens[f.first] = append(opens[f.first], f.open)
		closes[f.last]++
	}
	prevTight := false
	for i, bl := range rd.blocks {
		os := opens[i]
		switch {
		case len(os) == 0 && prevTight && bl.tight:
			b.WriteString("\n")
		default:
			b.WriteString("\n\n")
		}
		for _, o := range os {
			b.WriteString(o + "\n\n")
		}
		b.WriteString(bl.text)
		prevTight = bl.tight
		if n := closes[i]; n > 0 {
			b.WriteString(strings.Repeat("\n\n:::", n))
			prevTight = false
		}
	}
	b.WriteString("\n")
	// The output starts with "\n\n" before the first block, after the
	// title line. Make it one empty line.
	if s := b.String(); strings.Contains(s, "\n\n\n") {
		*b = *bytes.NewBufferString(strings.ReplaceAll(s, "\n\n\n", "\n\n"))
	}
}

var displayMath = regexp.MustCompile(`^\$\$(.+)\$\$$`)

var loneMath = regexp.MustCompile(`^\$([^$]+)\$([.,;]?)$`)

var blankLine = regexp.MustCompile(`\n[ \t]*\n`)

func splitParagraphs(s string) []string { return blankLine.Split(s, -1) }

// collapse makes each run of white space one space.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

var mdSpecial = strings.NewReplacer(`\`, `\\`, "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "$", `\$`, "<", `\<`, "`", "\\`")

// escapeText escapes the characters that Markdown reads as markup.
func escapeText(s string) string { return mdSpecial.Replace(s) }

var lineStart = regexp.MustCompile(`^([#>+=|-]|[0-9]+[.)])`)

// escapeStart escapes the first character of a paragraph when Markdown
// would read it as a heading, a quote, a list or a table.
func escapeStart(s string) string {
	if lineStart.MatchString(s) {
		return `\` + s
	}
	return s
}

func escapeURL(s string) string {
	return strings.NewReplacer(" ", "_", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E").Replace(strings.TrimSpace(s))
}

// wrap puts s in a mark of emphasis. The spaces at the ends stay outside.
func wrap(s, mark string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return s
	}
	lead := s[:strings.Index(s, t)]
	trail := s[len(lead)+len(t):]
	return lead + mark + t + mark + trail
}

func codeSpan(s string) string {
	fence := "`"
	for strings.Contains(s, fence) {
		fence += "`"
	}
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}
