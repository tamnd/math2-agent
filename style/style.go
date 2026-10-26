// Package style checks the body of a node against the house style of corpus spec K8.2.
//
// The checks are mechanical. Y01 counts the words in each sentence: at most 25 in the main section of the kind, and at most 30 in the other sections. A formula and a code span count as one word. Y02 finds the filler words that hide a step. Y03 finds "we" in the sections Statement and Definition. Y04 finds references to place, such as "Section 3" or "the previous chapter". The other mechanical rules of K8.2, undeclared symbols and links that are not tags, are the rules B01 and B02 of package check.
//
// The text of a code block, a heading and a table row is not prose, and the checks do not read it. A blank line, a heading, a table row, a code block and the start of a list item end a sentence. A period ends a sentence, except after a known abbreviation or an initial.
package style

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
)

// The word limits of a sentence (corpus K8.2).
const (
	MainWords  = 25 // in the main section of the kind
	OtherWords = 30 // in the other sections
)

var rules = []check.Rule{
	{ID: "Y01", Text: "each sentence has at most 25 words in the main section and at most 30 in the other sections, with a formula as one word"},
	{ID: "Y02", Text: "the body does not use the filler words that hide a step"},
	{ID: "Y03", Text: "the sections Statement and Definition do not use \"we\""},
	{ID: "Y04", Text: "the body has no references to place"},
}

// Rules returns the rules of the house style, in report order. Each rule is soft: Lint makes a finding an error only for a node at the strict level.
func Rules() []check.Rule { return slices.Clone(rules) }

// Main maps each kind to its main section, the section that its card needs for W1 (corpus K8.4).
var Main = map[string]string{
	"concept":        "Summary",
	"definition":     "Definition",
	"statement":      "Statement",
	"conjecture":     "Statement",
	"problem":        "Problem",
	"proof":          "Proof",
	"example":        "Example",
	"counterexample": "Counterexample",
	"object":         "Description",
	"computation":    "Description",
	"topic":          "Summary",
}

// The phrases of rule Y02. A space matches each run of white space.
var filler = phrases("clearly", "obviously", "trivially", "it is easy to see", "the reader can verify")

// The words of rule Y03.
var we = phrases("we", "us", "our", "ours", "ourselves")

// The patterns of rule Y04.
var place = regexp.MustCompile(`(?i)\b(?:sections?|chapters?|paragraphs?|appendix|pages?|figures?|tables?|equations?)\s+\(?\d|§\s*\d|\b(?:the\s+)(?:previous|next|preceding|following|last|first)\s+(?:sections?|chapters?|paragraphs?|pages?|nodes?)\b|\b(?:this|that)\s+(?:section|chapter|paragraph|page)\b`)

func phrases(list ...string) *regexp.Regexp {
	alt := make([]string, len(list))
	for i, p := range list {
		alt[i] = strings.Join(strings.Fields(regexp.QuoteMeta(p)), `\s+`)
	}
	return regexp.MustCompile(`(?i)\b(?:` + strings.Join(alt, "|") + `)\b`)
}

// Lint checks the body of each node, and gives one result for each rule in the order of Rules, and the findings in path, line and column order. A rule fails when it has a finding in a node for which strict reports true. Else a finding only reports. A nil strict reports false for each node. Corpus K8.2 makes the findings warnings for W1 and errors for W3, so strict is true for a node at W3.
func Lint(nodes []*node.Node, strict func(tag string) bool) ([]check.Result, []check.Finding) {
	var out []check.Finding
	checked := map[string]int{}
	failed := map[string]bool{}
	for _, n := range nodes {
		fs := lint(n, checked)
		if strict != nil && strict(n.ID) {
			for _, f := range fs {
				failed[f.Rule] = true
			}
		}
		out = append(out, fs...)
	}
	slices.SortFunc(out, func(a, b check.Finding) int {
		return cmp.Or(strings.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Col, b.Col), strings.Compare(a.Rule, b.Rule))
	})
	count := map[string]int{}
	for _, f := range out {
		count[f.Rule]++
	}
	res := make([]check.Result, 0, len(rules))
	for _, r := range rules {
		x := check.Result{Rule: r.ID, Checked: checked[r.ID], Findings: count[r.ID]}
		switch {
		case failed[r.ID]:
			x.State = check.Fail
		case x.Findings > 0:
			x.State = check.Found
		case x.Checked == 0:
			x.State = check.NA
		default:
			x.State = check.Pass
		}
		res = append(res, x)
	}
	return res, out
}

// Node checks the body of one node, and gives the findings in line and column order.
func Node(n *node.Node) []check.Finding {
	_, out := Lint([]*node.Node{n}, nil)
	return out
}

func lint(n *node.Node, checked map[string]int) []check.Finding {
	t := prose(n)
	var out []check.Finding
	add := func(rule string, at int, msg string) {
		line, col := t.place(at)
		out = append(out, check.Finding{Rule: rule, Path: n.Path, Line: line, Col: col, Msg: msg})
	}
	main := Main[n.Kind()]
	for _, s := range t.sentences() {
		checked["Y01"]++
		limit := OtherWords
		if s.section == main && main != "" {
			limit = MainWords
		}
		if s.words > limit {
			in := "outside the sections"
			if s.section != "" {
				in = "in the section " + s.section
			}
			add("Y01", s.start, fmt.Sprintf("the sentence has %d words, and the limit %s is %d", s.words, in, limit))
		}
	}
	for _, sec := range t.sections {
		checked["Y02"]++
		checked["Y04"]++
		text := t.text[sec.start:sec.end]
		for _, m := range filler.FindAllStringIndex(text, -1) {
			add("Y02", sec.start+m[0], fmt.Sprintf("%q hides a step: give the step, or make it a lemma", strings.Join(strings.Fields(text[m[0]:m[1]]), " ")))
		}
		for _, m := range place.FindAllStringIndex(text, -1) {
			add("Y04", sec.start+m[0], fmt.Sprintf("%q is a reference to place, and a node has no place: link to the node", strings.Join(strings.Fields(text[m[0]:m[1]]), " ")))
		}
		if sec.name == "Statement" || sec.name == "Definition" {
			checked["Y03"]++
			for _, m := range we.FindAllStringIndex(text, -1) {
				add("Y03", sec.start+m[0], fmt.Sprintf("%q is not used in the section %s", text[m[0]:m[1]], sec.name))
			}
		}
	}
	return out
}

// mark is the byte that stands for a formula or a code span in the prose, and pad fills the rest of it. A formula counts as one word, and a word that a formula touches, as in "$n$th", stays one word.
const (
	mark = '\x00'
	pad  = '\x01'
)

// text is the body of a node as prose. It has the same length as the body, so that an offset in it is an offset in the body. Each byte that is not prose is a space, a newline or a mark.
type text struct {
	body     string
	text     string
	start    int   // the file line of the first body line
	lines    []int // the offset of each line
	breaks   []int // the offsets where a sentence ends without a period, in order
	sections []section
}

type section struct {
	name       string // the level 2 section, or "" before the first one
	start, end int
}

type sentence struct {
	section string
	start   int // the offset of the first word
	words   int
}

// place returns the file line and the column of the byte at offset. The column counts runes.
func (t *text) place(offset int) (line, col int) {
	i := max(lastAtMost(t.lines, offset), 0)
	return t.start + i, utf8.RuneCountInString(t.body[t.lines[i]:offset]) + 1
}

// lastAtMost returns the index of the last value in list that is at most v, or -1.
func lastAtMost(list []int, v int) int {
	i, found := slices.BinarySearch(list, v)
	if found {
		return i
	}
	return i - 1
}

var (
	heading  = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*?)[ \t#]*$`)
	listItem = regexp.MustCompile(`^[ \t]*(?:[-*+]|\d{1,3}[.)])[ \t]+`)
	wikiLink = regexp.MustCompile(`\[\[([^\]]*?)\]\]`)
	mdLink   = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// prose reads the body of n as prose.
func prose(n *node.Node) *text {
	t := &text{body: n.Body, start: n.BodyLine}
	b := []byte(n.Body)
	blankRange := func(i, j int) {
		for k := i; k < j; k++ {
			if b[k] != '\n' {
				b[k] = ' '
			}
		}
	}
	// The lines: code blocks, headings and table rows are not prose.
	fence := ""
	current := section{start: 0}
	off := 0
	for _, l := range strings.SplitAfter(n.Body, "\n") {
		if l == "" {
			break
		}
		t.lines = append(t.lines, off)
		trim := strings.TrimLeft(strings.TrimRight(l, "\n"), " ")
		end := off + len(l)
		switch {
		case fence != "":
			blankRange(off, end)
			if strings.HasPrefix(trim, fence) && strings.Trim(trim, fence[:1]) == "" {
				fence = ""
				t.breaks = append(t.breaks, end)
			}
		case strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~"):
			blankRange(off, end)
			fence = trim[:3]
			t.breaks = append(t.breaks, off)
		case heading.MatchString(strings.TrimRight(l, "\n")):
			m := heading.FindStringSubmatch(strings.TrimRight(l, "\n"))
			blankRange(off, end)
			t.breaks = append(t.breaks, off)
			if len(m[1]) <= 2 {
				current.end = off
				t.sections = append(t.sections, current)
				current = section{start: end}
				if len(m[1]) == 2 {
					current.name = m[2]
				}
			}
		case strings.HasPrefix(trim, "|"):
			blankRange(off, end)
			t.breaks = append(t.breaks, off)
		case strings.TrimSpace(l) == "":
			t.breaks = append(t.breaks, off)
		default:
			if m := listItem.FindStringIndex(l); m != nil {
				blankRange(off, off+m[1])
				t.breaks = append(t.breaks, off)
			} else if strings.HasPrefix(trim, ">") {
				k := strings.Index(l, ">")
				blankRange(off+k, off+k+1)
			}
		}
		off = end
	}
	current.end = len(b)
	t.sections = append(t.sections, current)
	t.sections = slices.DeleteFunc(t.sections, func(s section) bool { return s.name == "" && strings.TrimSpace(string(b[s.start:s.end])) == "" })

	maskMath(b)
	// A link reads as its text.
	s := string(b)
	for _, m := range wikiLink.FindAllStringSubmatchIndex(s, -1) {
		inner := s[m[2]:m[3]]
		if bar := strings.LastIndex(inner, "|"); bar >= 0 {
			blankRange(m[2]+bar, m[1])
		} else {
			blankRange(m[1]-2, m[1])
		}
		blankRange(m[0], m[2])
	}
	s = string(b)
	for _, m := range mdLink.FindAllStringSubmatchIndex(s, -1) {
		blankRange(m[0], m[2])
		blankRange(m[3], m[1])
	}
	// Emphasis marks are not words.
	for i, c := range b {
		if c == '*' || c == '_' {
			if i == 0 || i == len(b)-1 || !isWordByte(b[i-1]) || !isWordByte(b[i+1]) {
				b[i] = ' '
			}
		}
	}
	t.text = string(b)
	return t
}

func isWordByte(c byte) bool {
	return c >= 0x80 || c == mark || unicode.IsLetter(rune(c)) || unicode.IsDigit(rune(c))
}

// maskMath changes each formula and code span in b to a mark followed by pads. A newline in a formula stays. A formula that ends with a period, as a display formula at the end of a sentence does, keeps the period.
func maskMath(b []byte) {
	put := func(i, j int, math bool) {
		stop := math && strings.HasSuffix(strings.TrimRight(string(b[i:j]), "$\\)] \t\n"), ".")
		b[i] = mark
		for k := i + 1; k < j; k++ {
			if b[k] != '\n' {
				b[k] = pad
			}
		}
		if stop && j-1 > i {
			b[j-1] = '.'
		}
	}
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == '\\' && i+1 < len(b) && (b[i+1] == '(' || b[i+1] == '['):
			closer := `\)`
			if b[i+1] == '[' {
				closer = `\]`
			}
			if k := strings.Index(string(b[i+2:]), closer); k >= 0 {
				j := i + 2 + k + 2
				put(i, j, true)
				i = j
				continue
			}
		case c == '\\':
			i += 2
			continue
		case c == '$':
			open := "$"
			if i+1 < len(b) && b[i+1] == '$' {
				open = "$$"
			}
			if j := closeMath(b, i+len(open), open); j > 0 {
				put(i, j, true)
				i = j
				continue
			}
		case c == '`':
			j := i
			for j < len(b) && b[j] == '`' {
				j++
			}
			run := string(b[i:j])
			if k := strings.Index(string(b[j:]), run); k >= 0 {
				end := j + k + len(run)
				put(i, end, false)
				i = end
				continue
			}
			i = j
			continue
		}
		i++
	}
}

// closeMath returns the offset after the delimiter that closes a formula opened with open, or -1. An inline formula ends at its line or at a blank line.
func closeMath(b []byte, from int, open string) int {
	for k := from; k < len(b); k++ {
		switch {
		case b[k] == '\\':
			k++
		case open == "$" && b[k] == '\n':
			return -1
		case strings.HasPrefix(string(b[k:min(k+len(open), len(b))]), open):
			if open == "$$" || k+1 >= len(b) || b[k+1] != '$' {
				return k + len(open)
			}
		}
	}
	return -1
}

// abbrev are the words that end with a period and do not end a sentence.
var abbrev = map[string]bool{
	"e.g.": true, "i.e.": true, "cf.": true, "etc.": true, "vs.": true, "resp.": true, "viz.": true, "al.": true, "approx.": true,
	"thm.": true, "prop.": true, "def.": true, "lem.": true, "cor.": true, "ex.": true, "fig.": true, "eq.": true, "ch.": true, "sec.": true,
	"p.": true, "pp.": true, "vol.": true, "no.": true, "ed.": true, "eds.": true, "dr.": true, "mr.": true, "mrs.": true, "st.": true, "jr.": true,
}

// sentences returns the sentences of the prose, in order.
func (t *text) sentences() []sentence {
	var out []sentence
	var cur *sentence
	end := func() {
		if cur != nil && cur.words > 0 {
			out = append(out, *cur)
		}
		cur = nil
	}
	br := 0
	s := t.text
	for i := 0; i < len(s); {
		if s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\n' && s[j] != '\r' {
			j++
		}
		for br < len(t.breaks) && t.breaks[br] <= i {
			end()
			br++
		}
		word := s[i:j]
		if cur == nil {
			cur = &sentence{section: t.sectionAt(i), start: i}
		}
		if strings.ContainsFunc(word, func(r rune) bool { return r == mark || unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			cur.words++
		}
		if endsSentence(word) {
			end()
		}
		i = j
	}
	end()
	return out
}

// endsSentence reports whether the word ends a sentence.
func endsSentence(w string) bool {
	w = strings.TrimRight(w, `)"'”’]`)
	if w == "" {
		return false
	}
	switch w[len(w)-1] {
	case '!', '?':
		return true
	case '.':
	default:
		return false
	}
	if abbrev[strings.ToLower(strings.TrimLeft(w, `("'“‘[`))] {
		return false
	}
	// An initial, as in "J. Smith", and a part of an abbreviation, as in "e.g.", do not end a sentence.
	core := strings.TrimLeft(w, `("'“‘[`)
	if r, size := utf8.DecodeRuneInString(core); size+1 == len(core) && unicode.IsUpper(r) {
		return false
	}
	return true
}

// sectionAt returns the name of the section at offset.
func (t *text) sectionAt(offset int) string {
	for _, s := range t.sections {
		if offset >= s.start && offset < s.end {
			return s.name
		}
	}
	return ""
}
