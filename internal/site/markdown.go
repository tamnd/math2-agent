package site

import (
	"bytes"
	"fmt"
	"html"
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

// nodeScheme marks a link to a node after the wiki links change to Markdown links.
const nodeScheme = "m2node:"

// displayMarker stands for a block of display math while goldmark parses the text.
const displayMarker = "M2DISPLAYMATH"

// linkFunc changes a link destination. It returns the new destination and false when the target does not exist.
type linkFunc func(dest string) (string, bool)

// markdown renders Markdown to HTML. A node body keeps its math as TeX in a span or a div, and the browser renders it with KaTeX.
type markdown struct {
	md    goldmark.Markdown
	nodes bool // change wiki links to node links
}

func newMarkdown(math bool) *markdown {
	inline := []util.PrioritizedValue{}
	if math {
		inline = append(inline, util.Prioritized(mathParser{}, 50))
	}
	md := goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Strikethrough, extension.Footnote),
		goldmark.WithParserOptions(parser.WithAutoHeadingID(), parser.WithInlineParsers(inline...)),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(util.Prioritized(mathRenderer{}, 50)), gmhtml.WithUnsafe()),
	)
	return &markdown{md: md, nodes: math}
}

// render renders src. Each link destination goes through links. A node link to a missing node gets the class missing.
func (m *markdown) render(src string, links linkFunc) (string, error) {
	src, blocks := takeDisplayMath(src)
	if m.nodes {
		src = wikiLinks(src)
	}
	doc := m.md.Parser().Parse(text.NewReader([]byte(src)))
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if l, ok := n.(*ast.Link); ok && links != nil {
			dest, ok := links(string(l.Destination))
			l.Destination = []byte(dest)
			if !ok {
				l.SetAttributeString("class", []byte("missing"))
				l.Title = []byte("This node does not exist yet")
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
		div := `<div class="math display">` + html.EscapeString(tex) + `</div>`
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
			lines[i] = wikiLink.ReplaceAllString(line, "[$1]("+nodeScheme+"$2)")
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

// mathKind is the AST kind of inline math.
var mathKind = ast.NewNodeKind("Math")

type mathNode struct {
	ast.BaseInline
	TeX     []byte
	Display bool
}

func (n *mathNode) Kind() ast.NodeKind            { return mathKind }
func (n *mathNode) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

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
	n := &mathNode{TeX: append([]byte(nil), line[open:end]...), Display: open == 2}
	block.Advance(end + open)
	return n
}

type mathRenderer struct{}

func (mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(mathKind, func(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		m := n.(*mathNode)
		class := "math"
		if m.Display {
			class = "math display"
		}
		_, _ = fmt.Fprintf(w, `<span class="%s">%s</span>`, class, html.EscapeString(string(m.TeX)))
		return ast.WalkSkipChildren, nil
	})
}
