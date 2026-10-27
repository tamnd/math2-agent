// Package markup renders Markdown to HTML: the bodies of nodes, with their math and their links to nodes, and the documents of the content repository.
//
// A node body links to a node with a wiki link, [[text|TAG]]. Render changes each wiki link to a Markdown link with the destination NodeScheme and the id, so that the caller can resolve it. Math is $...$ and $$...$$ on one line, or a block that starts and ends with a line of $$. The caller renders the math, for example with package katex.
package markup

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// wikiLink is a link to a node: [[text|TAG]].
var wikiLink = regexp.MustCompile(`\[\[([^\]|]+)\|([^\]|\s]+)\]\]`)

// NodeScheme starts the destination of a link to a node, for example m2node:G5S1M9, after the wiki links change to Markdown links.
const NodeScheme = "m2node:"

// displayMarker stands for a block of display math while goldmark parses the text.
const displayMarker = "M2DISPLAYMATH"

// Target is where a link goes.
type Target struct {
	Dest  string // the new destination
	Title string // the title of the link, or ""
	Class string // the class of the link, or ""
}

// Links resolves the destination of a link. It returns false when the target does not exist. Render then gives the link the class missing and, when the target has no title, a title that says so.
type Links func(dest string) (Target, bool)

// TeX returns the HTML of a formula.
type TeX func(tex string, display bool) string

// Markdown renders Markdown to HTML. A Markdown is safe for concurrent use when its TeX function is.
type Markdown struct {
	md  goldmark.Markdown
	tex TeX
}

// New returns a renderer. With a TeX function, the renderer reads math and wiki links, as in a node body. With nil, it reads plain Markdown, as in a document.
func New(tex TeX) *Markdown {
	inline := []util.PrioritizedValue{}
	if tex != nil {
		inline = append(inline, util.Prioritized(mathParser{}, 50))
	}
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Footnote),
		goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithInlineParsers(inline...)),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(mathRenderer{tex}, 50)), gmhtml.WithUnsafe()),
	)
	return &Markdown{md: md, tex: tex}
}

// Render renders src. Each link destination goes through links, when links is not nil. The HTML of the source is kept, because the content of math2 is reviewed.
func (m *Markdown) Render(src string, links Links) (string, error) {
	var blocks []string
	if m.tex != nil {
		src, blocks = takeDisplayMath(src)
		src = wikiLinks(src)
	}
	doc := m.md.Parser().Parse(text.NewReader([]byte(src)))
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if l, ok := n.(*ast.Link); ok && links != nil {
			t, ok := links(string(l.Destination))
			l.Destination = []byte(t.Dest)
			if !ok {
				t.Class = strings.TrimSpace(t.Class + " missing")
				if t.Title == "" {
					t.Title = "This node does not exist yet"
				}
			}
			if t.Class != "" {
				l.SetAttributeString("class", []byte(t.Class))
			}
			if t.Title != "" {
				l.Title = []byte(t.Title)
			}
		}
		return ast.WalkContinue, nil
	})
	var buf bytes.Buffer
	if err := m.md.Renderer().Render(&buf, []byte(src), doc); err != nil {
		return "", err
	}
	out := buf.String()
	for i, tex := range blocks {
		marker := fmt.Sprintf("%s%d", displayMarker, i)
		div := `<div class="math display">` + m.tex(tex, true) + `</div>`
		out = strings.Replace(out, "<p>"+marker+"</p>", div, 1)
	}
	return out, nil
}

// wikiLinks changes each wiki link outside fenced code to a Markdown link with the node scheme.
func wikiLinks(src string) string {
	lines := strings.Split(src, "\n")
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case fence == "" && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")):
			fence = trimmed[:3]
		case fence != "" && strings.HasPrefix(trimmed, fence):
			fence = ""
		case fence == "":
			lines[i] = wikiLink.ReplaceAllString(line, "[$1]("+NodeScheme+"$2)")
		}
	}
	return strings.Join(lines, "\n")
}

// takeDisplayMath replaces each block of display math that starts with a line of $$ by a marker paragraph. It does not look inside fenced code.
func takeDisplayMath(src string) (string, []string) {
	var out strings.Builder
	var blocks []string
	lines := strings.Split(src, "\n")
	fence := ""
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if fence == "" && (strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~")) {
			fence = trimmed[:3]
		} else if fence != "" && strings.HasPrefix(trimmed, fence) {
			fence = ""
		}
		if fence == "" && strings.HasPrefix(trimmed, "$$") && (len(trimmed) <= 4 || !strings.HasSuffix(trimmed, "$$")) {
			var tex []string
			if rest := strings.TrimPrefix(trimmed, "$$"); rest != "" {
				tex = append(tex, rest)
			}
			j := i + 1
			for ; j < len(lines); j++ {
				t := strings.TrimSpace(lines[j])
				if before, found := strings.CutSuffix(t, "$$"); found {
					if before != "" {
						tex = append(tex, before)
					}
					break
				}
				tex = append(tex, lines[j])
			}
			if j < len(lines) {
				fmt.Fprintf(&out, "\n%s%d\n", displayMarker, len(blocks))
				blocks = append(blocks, strings.Join(tex, "\n"))
				i = j
				if i < len(lines)-1 {
					out.WriteByte('\n')
				}
				continue
			}
		}
		out.WriteString(line)
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.String(), blocks
}

// KindMath is the AST kind of a formula.
var KindMath = ast.NewNodeKind("Math")

// Math is a formula in a tree that Parse returns. A block of display math is a paragraph with one Math in it.
type Math struct {
	ast.BaseInline
	TeX     []byte
	Display bool
}

// Kind returns KindMath.
func (n *Math) Kind() ast.NodeKind { return KindMath }

// Dump writes the formula for debugging.
func (n *Math) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"TeX": string(n.TeX)}, nil)
}

// body parses a node body for Parse.
var body = goldmark.New(
	goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Footnote),
	goldmark.WithParserOptions(parser.WithInlineParsers(util.Prioritized(mathParser{}, 50))),
)

// Parse parses src as a node body, for a back end that does not write HTML. It returns the tree and the source that the text of the tree points into. The math is Math, and each wiki link is a link with the destination NodeScheme and the id. The tree has the nodes of goldmark and of its extensions Table, Strikethrough and Footnote.
func Parse(src string) (ast.Node, []byte) {
	src, blocks := takeDisplayMath(src)
	source := []byte(wikiLinks(src))
	doc := body.Parser().Parse(text.NewReader(source))
	var marks []*ast.Paragraph
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if p, ok := n.(*ast.Paragraph); ok && entering {
			marks = append(marks, p)
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	for _, p := range marks {
		t, ok := p.FirstChild().(*ast.Text)
		if !ok || p.ChildCount() != 1 {
			continue
		}
		var i int
		if _, err := fmt.Sscanf(string(t.Segment.Value(source)), displayMarker+"%d", &i); err != nil || i >= len(blocks) || string(t.Segment.Value(source)) != fmt.Sprintf("%s%d", displayMarker, i) {
			continue
		}
		p.RemoveChildren(p)
		p.AppendChild(p, &Math{TeX: []byte(blocks[i]), Display: true})
	}
	return doc, source
}

// mathParser reads $...$ and $$...$$ on one line. A backslash escapes the next character.
type mathParser struct{}

func (mathParser) Trigger() []byte { return []byte{'$'} }

func (mathParser) Parse(_ ast.Node, block text.Reader, _ parser.Context) ast.Node {
	line, _ := block.PeekLine()
	open := 1
	if len(line) > 1 && line[1] == '$' {
		open = 2
	}
	end := -1
	for i := open; i < len(line); i++ {
		switch {
		case line[i] == '\\':
			i++
		case line[i] == '$' && (open == 1 || (i+1 < len(line) && line[i+1] == '$')):
			end = i
		}
		if end >= 0 {
			break
		}
	}
	if end <= open {
		return nil
	}
	n := &Math{TeX: append([]byte(nil), line[open:end]...), Display: open == 2}
	block.Advance(end + open)
	return n
}

type mathRenderer struct{ tex TeX }

func (r mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindMath, func(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		m := n.(*Math)
		class := "math"
		if m.Display {
			class = "math display"
		}
		_, _ = fmt.Fprintf(w, `<span class="%s">%s</span>`, class, r.tex(string(m.TeX), m.Display))
		return ast.WalkSkipChildren, nil
	})
}
