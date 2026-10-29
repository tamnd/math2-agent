package site

import (
	"strings"
)

// readerBody is the Markdown body of a node as its page shows it to a person.
type readerBody struct {
	Lead    string // the first section, for example the definition or the statement
	Plain   string // the explanations at level L1, without their headings
	Rest    string // the sections after the first, and the explanations at the higher levels
	Excerpt string // the first paragraph of the summary, or else of the first section
}

// section is one part of a body at a heading of level two, with its heading line. The text before the first heading is a section with no heading.
type section struct {
	head  string // the text of the heading, "" for the text before the first heading
	lines []string
}

// sections splits a Markdown body at its headings of level n. A heading in a fenced code block does not split.
func sections(body string, n int) []section {
	mark := strings.Repeat("#", n) + " "
	var out []section
	cur := section{}
	fence := ""
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case fence != "":
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
		case strings.HasPrefix(t, "```"):
			fence = "```"
		case strings.HasPrefix(t, "~~~"):
			fence = "~~~"
		case strings.HasPrefix(line, mark):
			if cur.head != "" || len(strings.TrimSpace(strings.Join(cur.lines, ""))) > 0 {
				out = append(out, cur)
			}
			cur = section{head: strings.TrimSpace(strings.TrimPrefix(line, mark))}
		}
		cur.lines = append(cur.lines, line)
	}
	if cur.head != "" || len(strings.TrimSpace(strings.Join(cur.lines, ""))) > 0 {
		out = append(out, cur)
	}
	return out
}

func (s section) text() string { return strings.Join(s.lines, "\n") }

// content returns the section without its heading line.
func (s section) content() string {
	if s.head == "" {
		return s.text()
	}
	return strings.Join(s.lines[1:], "\n")
}

// levelTitles are the headings of the explanations above L1, for a reader. The levels are in document 04 of the spec.
var levelTitles = map[string]string{
	"L2": "For a student",
	"L3": "For a graduate student",
	"L4": "For a researcher",
}

// splitBody splits the body of a node for its page. The explanations at level L1, for a person who has not met the idea, leave the section Explanations and go to Plain. The explanations at the higher levels go to the end of Rest, in a section Going deeper, with a heading for each level that a reader understands. The word human or agent in a heading says who wrote the explanation, and the page does not show it: the notices of the page say which text no person has checked.
func splitBody(body string) readerBody {
	var r readerBody
	var rest, deeper []string
	first := true
	var summary, lead string
	for _, s := range sections(body, 2) {
		if strings.EqualFold(s.head, "Explanations") {
			var plain []string
			for _, sub := range sections(s.content(), 3) {
				c := strings.TrimSpace(sub.content())
				if c == "" {
					continue
				}
				level, _, _ := strings.Cut(sub.head, " ")
				if sub.head == "" || level == "L1" {
					plain = append(plain, c)
					continue
				}
				title := levelTitles[level]
				if title == "" {
					title = sub.head
				}
				deeper = append(deeper, "### "+title+"\n\n"+c)
			}
			r.Plain = strings.Join(plain, "\n\n")
			continue
		}
		if strings.EqualFold(s.head, "Summary") && summary == "" {
			summary = s.content()
		}
		if first {
			// The first section, or the text before the first heading, leads the page.
			r.Lead = s.text()
			lead = s.content()
			first = false
			continue
		}
		rest = append(rest, s.text())
	}
	if len(deeper) > 0 {
		rest = append(rest, "## Going deeper\n\n"+strings.Join(deeper, "\n\n"))
	}
	r.Rest = strings.Join(rest, "\n\n")
	if summary == "" {
		summary = lead
	}
	r.Excerpt = firstParagraph(summary)
	return r
}

// firstParagraph returns the first paragraph of Markdown text, a run of lines up to a blank line. A paragraph of display math only counts when no paragraph of text comes first.
func firstParagraph(md string) string {
	var paras []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			paras = append(paras, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, line := range strings.Split(md, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	for _, p := range paras {
		t := strings.TrimSpace(p)
		if strings.HasPrefix(t, "$$") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "|") || strings.HasPrefix(t, "```") {
			continue
		}
		return p
	}
	if len(paras) > 0 {
		return paras[0]
	}
	return ""
}
