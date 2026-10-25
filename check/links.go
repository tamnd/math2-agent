package check

import (
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
