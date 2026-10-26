package check

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/tag"
)

// body is the body of a node, split into lines.
type body struct {
	path    string
	start   int      // the file line of the first body line
	lines   []string // the lines as they are in the file
	masked  []string // the lines with code blocks and code spans changed to spaces
	section []string // the section of each line, or ""
	heads   []heading
}

// heading is a section heading outside code.
type heading struct {
	index int // the index in lines
	level int
	name  string
}

var headingLine = regexp.MustCompile(`^(#{1,6})[ \t]+(.*?)[ \t#]*$`)

func parseBody(n *node.Node) *body {
	b := &body{path: n.Path, start: n.BodyLine}
	b.lines = strings.Split(strings.TrimSuffix(n.Body, "\n"), "\n")
	b.masked = make([]string, len(b.lines))
	b.section = make([]string, len(b.lines))
	fence := ""
	current := ""
	for i, l := range b.lines {
		trim := strings.TrimLeft(l, " ")
		switch {
		case fence != "":
			b.masked[i] = blank(l)
			if strings.HasPrefix(trim, fence) && strings.Trim(trim, fence[:1]) == "" {
				fence = ""
			}
		case strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~"):
			b.masked[i] = blank(l)
			fence = trim[:3]
		default:
			b.masked[i] = maskCode(l)
			if m := headingLine.FindStringSubmatch(b.masked[i]); m != nil && len(l)-len(trim) < 4 {
				h := heading{index: i, level: len(m[1]), name: m[2]}
				b.heads = append(b.heads, h)
				if h.level <= 2 {
					current = h.name
				}
			}
		}
		b.section[i] = current
	}
	return b
}

// blank returns s with each byte changed to a space.
func blank(s string) string { return strings.Repeat(" ", len(s)) }

// maskCode changes each code span in the line to spaces. The length of the line does not change.
func maskCode(l string) string {
	if !strings.Contains(l, "`") {
		return l
	}
	out := []byte(l)
	for i := 0; i < len(l); {
		if l[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(l) && l[j] == '`' {
			j++
		}
		run := l[i:j]
		end := strings.Index(l[j:], run)
		if end < 0 {
			i = j
			continue
		}
		end += j + len(run)
		for k := i; k < end; k++ {
			out[k] = ' '
		}
		i = end
	}
	return string(out)
}

// offsets returns a function that gives the file line and the column of an offset in the masked lines joined with newlines.
func (b *body) offsets() func(off int) (line, col int) {
	starts := make([]int, len(b.lines))
	for i, off := 1, 0; i < len(b.lines); i++ {
		off += len(b.lines[i-1]) + 1
		starts[i] = off
	}
	return func(off int) (int, int) {
		i := 0
		for i+1 < len(starts) && starts[i+1] <= off {
			i++
		}
		return b.place(i, off-starts[i])
	}
}

// place returns the file line and the column of the byte at offset in line i. The column counts runes.
func (b *body) place(i, offset int) (line, col int) {
	return b.start + i, utf8.RuneCountInString(b.lines[i][:offset]) + 1
}

// sections checks the section headings against sec (rule B03). It returns the first problem only, because one problem often causes others.
func (b *body) sections(kind string, sec schema.Sections, s *schema.Schema) (Finding, bool) {
	bad := func(i int, msg string) (Finding, bool) {
		line, col := b.start-1, 1 // the closing line of the front matter
		if i >= 0 {
			line, col = b.place(i, 0)
		}
		return Finding{Rule: "B03", Path: b.path, Line: line, Col: col, Msg: msg}, true
	}
	first := len(b.lines)
	if len(b.heads) > 0 {
		first = b.heads[0].index
	}
	for i := range first {
		if strings.TrimSpace(b.masked[i]) != "" {
			return bad(i, "text comes before the first section heading")
		}
	}
	pos := -1
	var seen []string
	for _, h := range b.heads {
		switch {
		case h.level == 1:
			return bad(h.index, fmt.Sprintf("the section heading %q uses #, and a section heading uses ##", h.name))
		case h.level > 2:
			continue
		}
		i := slices.Index(sec.Allowed, h.name)
		if i < 0 {
			for _, a := range sec.Allowed {
				if strings.EqualFold(a, h.name) {
					return bad(h.index, fmt.Sprintf("the section heading %q must be %q", h.name, a))
				}
			}
			for _, k := range s.Kinds() {
				if other, _ := s.Sections(k); slices.Contains(other.Allowed, h.name) {
					return bad(h.index, fmt.Sprintf("the section %s is not allowed in a node of kind %s", h.name, kind))
				}
			}
			return bad(h.index, fmt.Sprintf("the section %s is not in the schema", h.name))
		}
		if slices.Contains(seen, h.name) {
			return bad(h.index, fmt.Sprintf("the section %s comes two times", h.name))
		}
		if i < pos {
			return bad(h.index, fmt.Sprintf("the section %s must come before %s", h.name, sec.Allowed[pos]))
		}
		pos = i
		seen = append(seen, h.name)
	}
	for _, r := range sec.Required {
		if !slices.Contains(seen, r) {
			return bad(-1, fmt.Sprintf("the required section %s is missing", r))
		}
	}
	return Finding{}, false
}

// statementSections are the sections that state the mathematics. They must have no URLs (rule B02).
var statementSections = []string{"Statement", "Definition", "Problem"}

var (
	wikiLink     = regexp.MustCompile(`\[\[(.*?)\]\]`)
	markdownLink = regexp.MustCompile(`\[[^\]]*\]\(\s*<?([^)\s>]*)>?[^)]*\)`)
	bareURL      = regexp.MustCompile(`(?i)\b(?:https?|ftp)://[^\s)>\]]+|\bwww\.[^\s)>\]]+`)
)

// isURL reports whether s is a URL and not a tag or a path in the site.
func isURL(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "://") || strings.HasPrefix(s, "www.") || strings.HasPrefix(s, "mailto:")
}

// links checks the links of the body (rule B02).
func (r *run) links(n *node.Node, b *body) {
	inStatement := ""
	for i, l := range b.masked {
		sec := b.section[i]
		if slices.Contains(statementSections, sec) && sec != inStatement {
			r.checked["B02"]++
		}
		inStatement = sec
		for _, m := range wikiLink.FindAllStringSubmatchIndex(l, -1) {
			r.checked["B02"]++
			line, col := b.place(i, m[0])
			if msg := r.linkProblem(l[m[2]:m[3]]); msg != "" {
				r.add("B02", n.Path, line, col, msg)
			}
		}
		if !slices.Contains(statementSections, sec) {
			continue
		}
		l = wikiLink.ReplaceAllStringFunc(l, blank)
		for _, m := range markdownLink.FindAllStringSubmatchIndex(l, -1) {
			if isURL(l[m[2]:m[3]]) {
				line, col := b.place(i, m[0])
				r.add("B02", n.Path, line, col, fmt.Sprintf("the %s section has a Markdown link to a URL. Put the source in the sources field", sec))
			}
		}
		l = markdownLink.ReplaceAllStringFunc(l, blank)
		for _, m := range bareURL.FindAllStringIndex(l, -1) {
			line, col := b.place(i, m[0])
			r.add("B02", n.Path, line, col, fmt.Sprintf("the %s section has a URL. Put the source in the sources field", sec))
		}
	}
}

// linkProblem returns the problem with the link [[inner]], or "".
func (r *run) linkProblem(inner string) string {
	i := strings.LastIndex(inner, "|")
	if i < 0 {
		return fmt.Sprintf("the link [[%s]] has no target. Write [[text|TAG]]", inner)
	}
	t := strings.TrimSpace(inner[i+1:])
	switch {
	case isURL(t):
		return fmt.Sprintf("the link target %s is a URL. A link targets a tag", t)
	case !tag.IsTag(t) && !tag.IsProvisional(t):
		return fmt.Sprintf("the link target %q is not a tag", t)
	}
	e, ok := r.lookup(t)
	switch {
	case !ok:
		return fmt.Sprintf("the link target %s is not a node", t)
	case e.Retired && len(e.Successors) == 0:
		return fmt.Sprintf("the link target %s is retired and has no successor", t)
	}
	return ""
}
