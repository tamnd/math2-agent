package check

import (
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/node"
)

// Link is a [[text|target]] link in the body of a node.
type Link struct {
	Text    string
	Target  string // the text after the last |, without spaces at the ends. It is empty when the link has no |.
	Section string // the level 2 section of the link, or ""
	Line    int    // the file line of the link
	Col     int    // the column of the [[, in runes
}

// Links returns the links in the body of n, in order. A link in a code block or in a code span is text, and Links does not return it.
func Links(n *node.Node) []Link {
	b := parseBody(n)
	var out []Link
	for i, l := range b.masked {
		for _, m := range wikiLink.FindAllStringSubmatchIndex(l, -1) {
			inner := b.lines[i][m[2]:m[3]]
			link := Link{Text: inner, Section: b.section[i]}
			if bar := strings.LastIndex(inner, "|"); bar >= 0 {
				link.Text, link.Target = inner[:bar], strings.TrimSpace(inner[bar+1:])
			}
			link.Line, link.Col = b.place(i, m[0])
			out = append(out, link)
		}
	}
	return out
}

// Section is a level 2 section in the body of a node.
type Section struct {
	Name     string
	Line     int       // the file line of the heading
	Text     string    // the lines after the heading, up to the next heading of level 1 or 2, as they are in the file
	Headings []Heading // the headings of level 3 to 6 in the section, in order
}

// Heading is a heading in a section.
type Heading struct {
	Level int
	Name  string
	Line  int // the file line of the heading
}

// Sections returns the level 2 sections in the body of n, in order. A heading in a code block is text, and Sections does not return it. The text before the first section is not in a section.
func Sections(n *node.Node) []Section {
	b := parseBody(n)
	var out []Section
	var lines []string
	in := false // the line is in the last section of out
	flush := func() {
		if in {
			out[len(out)-1].Text = strings.Join(lines, "\n")
		}
		lines, in = nil, false
	}
	h := 0
	for i, l := range b.lines {
		if h < len(b.heads) && b.heads[h].index == i {
			head := b.heads[h]
			h++
			if head.level <= 2 {
				flush()
				if head.level == 2 {
					out = append(out, Section{Name: head.name, Line: b.start + i})
					in = true
				}
				continue
			}
			if in {
				s := &out[len(out)-1]
				s.Headings = append(s.Headings, Heading{Level: head.level, Name: head.name, Line: b.start + i})
			}
		}
		if in {
			lines = append(lines, l)
		}
	}
	flush()
	return out
}

// StatementSections returns the names of the sections that state the mathematics. Rule B02 does not allow URLs in them.
func StatementSections() []string { return slices.Clone(statementSections) }
