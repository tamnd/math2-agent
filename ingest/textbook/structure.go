package textbook

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/resource"
)

// The patterns of the headings of the LaTeX reader of the table of contents.
var (
	headingCmd = regexp.MustCompile(`\\(part|chapter|section|subsection|olchapter|olsection|olsubsection)(\*?)\s*(?:\[[^\]]*\]\s*)?\{|\\(appendix|frontmatter|mainmatter|backmatter)\b`)
	beginDoc   = regexp.MustCompile(`\\begin\s*\{document\}`)
	labelAfter = regexp.MustCompile(`^\s*\\label\s*\{([^{}]+)\}`)
	indexCmd   = regexp.MustCompile(`\\index\s*(?:\[[^\]]*\]\s*)?\{`)
)

// Structure reads a git archive of the book and gives the table of contents of the book (RS4.5) and the commit of the archive. The tree has the parts, the chapters, the sections and the subsections, with their numbers and titles. It does not have the text between the headings. It reads the same files as Read, in the same order. It fails when a file does not parse, when the main file of a PreTeXt book is not in the archive, and when the archive has no headings.
//
// The numbers are the numbers that LaTeX gives. A part has a Roman number. The chapters have one counter for the book, the sections have one counter for each chapter, and the subsections have one counter for each section. After \appendix, or in the appendices of a PreTeXt book, a chapter has a letter. A book with no chapters gives the letter to the section. A starred heading, and a chapter in the front matter or the back matter, has no number. The key of a node is its number, with part- before the number of a part. A heading with no number has its label as key, or when it has no label, the key of its parent, a dot, u and its place among the headings of the parent with no number, for example 2.u1.
func (b Book) Structure(r io.Reader) ([]resource.Node, string, error) {
	files, order, commit, err := b.load(r)
	if err != nil {
		return nil, "", err
	}
	t, err := b.build(files, order, false)
	if err != nil {
		return nil, "", err
	}
	if len(t.roots) == 0 {
		return nil, "", errors.New("the archive has no parts, chapters or sections")
	}
	return nodes(t.roots), cmpOr(commit, b.Commit), nil
}

// build builds the table of contents of the files of the book. When index is true, it also collects the terms of the back-of-book index, each with the node of the table of contents that holds it (RS4.6).
func (b Book) build(files map[string][]byte, order []string, index bool) (*toc, error) {
	t := &toc{keys: map[string]bool{}, index: index, places: map[string][]place{}}
	var err error
	switch b.Format {
	case LaTeX:
		latexTOC(t, files, order, b.Files, b.Skip)
	case PreTeXt:
		err = pretextTOC(t, files, b.Main)
	default:
		err = fmt.Errorf("the format %q is not known", b.Format)
	}
	return t, err
}

// latexTOC adds the headings of the files of order that match matches and skip does not match, in that order. In a file with \begin{document}, the text before it is the preamble, and it has no headings. A heading with # in its title is in the definition of a macro, and it is not a heading of the book.
func latexTOC(t *toc, files map[string][]byte, order []string, match, skip *regexp.Regexp) {
	for _, name := range order {
		if !match.MatchString(name) || skip != nil && skip.MatchString(name) {
			continue
		}
		s := stripComments(string(files[name]))
		base := 0 // the lines before s in the file
		if m := beginDoc.FindStringIndex(s); m != nil {
			base = strings.Count(s[:m[1]], "\n")
			s = s[m[1]:]
		}
		t.at(name, 0)
		line := func(at int) int { return base + 1 + strings.Count(s[:at], "\n") }
		var terms [][]int
		if t.index {
			terms = indexCmd.FindAllStringIndex(s, -1)
		}
		for _, m := range headingCmd.FindAllStringSubmatchIndex(s, -1) {
			for len(terms) > 0 && terms[0][0] < m[0] {
				t.latexTerm(name, s, terms[0])
				terms = terms[1:]
			}
			if m[6] >= 0 {
				t.mark(s[m[6]:m[7]])
				t.at(name, line(m[0]))
				continue
			}
			title, ok := braced(s[m[1]-1:])
			if !ok || strings.Contains(title, "#") {
				continue
			}
			label := ""
			if l := labelAfter.FindStringSubmatch(s[m[1]+len(title)+1:]); l != nil {
				label = strings.TrimSpace(l[1])
			}
			kind := strings.TrimPrefix(s[m[2]:m[3]], "ol")
			t.add(kind, m[5] > m[4], strings.Join(strings.Fields(title), " "), label)
			t.at(name, line(m[0]))
		}
		for _, m := range terms {
			t.latexTerm(name, s, m)
		}
	}
}

// pretextTOC adds the divisions of the main file of a PreTeXt book and of the files that it includes, in the order of the book.
func pretextTOC(t *toc, files map[string][]byte, main string) error {
	if _, ok := files[main]; !ok {
		return fmt.Errorf("the main file %s is not in the archive", main)
	}
	p := &pretext{files: files, open: map[string]bool{}, toc: t}
	return p.file(main, nil)
}

// levels are the levels of the kinds of headings in the tree.
var levels = map[string]int{"part": 0, "chapter": 1, "section": 2, "subsection": 3}

// toc builds the tree of headings of a book from the headings in the order of the book. It numbers the headings as LaTeX does.
type toc struct {
	roots      []*tocNode
	last       [4]*tocNode // the last heading of each level, or nil when a heading of a higher level closed it
	count      [4]int      // the counters of the levels
	num        [4]string   // the number of the last numbered heading of each level
	chapters   bool        // the book has a numbered chapter
	appendix   bool        // the headings are in the appendices
	unnumbered bool        // the headings are in the front matter or the back matter
	keys       map[string]bool
	index      bool               // collect the terms of the index
	terms      []indexTerm        // the terms of the index, in the order of the book
	places     map[string][]place // for each file of a LaTeX book, the node that is open from each line
}

// place is a line of a file and the key of the deepest open node of the table of contents from that line, or "".
type place struct {
	line int
	key  string
}

// at records that the node open now is open from the line of the file.
func (t *toc) at(file string, line int) {
	t.places[file] = append(t.places[file], place{line, t.here()})
}

// nodeAt gives the key of the node of the table of contents that holds the line of the file, or "" when the walk did not read the file.
func (t *toc) nodeAt(file string, line int) string {
	ps := t.places[file]
	i := sort.Search(len(ps), func(i int) bool { return ps[i].line > line })
	if i == 0 {
		return ""
	}
	return ps[i-1].key
}

// tocNode is a node of the tree while the tree grows.
type tocNode struct {
	resource.Node
	kids  []*tocNode
	loose int // the number of children with no number
}

// mark changes the state of the numbers at \appendix, \frontmatter, \mainmatter and \backmatter.
func (t *toc) mark(cmd string) {
	switch cmd {
	case "appendix":
		t.startAppendix()
	case "frontmatter", "backmatter":
		t.unnumbered = true
	case "mainmatter":
		t.unnumbered = false
	}
}

// startAppendix starts the appendices: the counter of the chapters, or of the sections in a book with no chapters, starts again.
func (t *toc) startAppendix() {
	if t.appendix {
		return
	}
	t.appendix = true
	if t.chapters {
		t.count[1] = 0
	} else {
		t.count[2] = 0
	}
}

// add adds a heading of one of the kinds in levels, or an appendix of a PreTeXt book, and returns its node. A starred heading has no number.
func (t *toc) add(kind string, starred bool, title, label string) *tocNode {
	if kind == "appendix" {
		t.startAppendix()
		kind = "section"
		if t.chapters {
			kind = "chapter"
		}
	}
	lvl := levels[kind]
	n := &tocNode{Node: resource.Node{Kind: kind, Title: title}}
	if !starred && (!t.unnumbered || lvl != 1) {
		if t.appendix && (lvl == 1 || lvl == 2 && !t.chapters) {
			n.Kind = "appendix"
		}
		t.count[lvl]++
		// The chapters do not start again in a new part.
		for i := max(lvl+1, 2); i < len(t.count); i++ {
			t.count[i] = 0
		}
		c := t.count[lvl]
		switch {
		case lvl == 0:
			n.Number = roman(c)
		case n.Kind == "appendix":
			n.Number = letter(c)
		case lvl == 1:
			n.Number = strconv.Itoa(c)
		default:
			n.Number = strconv.Itoa(c)
			if p := t.num[lvl-1]; p != "" {
				n.Number = p + "." + n.Number
			}
		}
		t.num[lvl] = n.Number
		for i := lvl + 1; i < len(t.num); i++ {
			t.num[i] = ""
		}
		if lvl == 1 {
			t.chapters = true
		}
	}
	var parent *tocNode
	for i := lvl - 1; i >= 0 && parent == nil; i-- {
		parent = t.last[i]
	}
	for i := lvl; i < len(t.last); i++ {
		t.last[i] = nil
	}
	t.last[lvl] = n
	switch {
	case n.Number != "" && lvl == 0:
		n.Key = "part-" + n.Number
	case n.Number != "":
		n.Key = n.Number
	case label != "":
		n.Key = label
	case parent != nil:
		parent.loose++
		n.Key = parent.Key + ".u" + strconv.Itoa(parent.loose)
	default:
		n.Key = "u" + strconv.Itoa(t.looseRoots())
	}
	for k, i := n.Key, 2; t.keys[n.Key]; i++ {
		n.Key = k + "-" + strconv.Itoa(i)
	}
	t.keys[n.Key] = true
	if parent != nil {
		parent.kids = append(parent.kids, n)
	} else {
		t.roots = append(t.roots, n)
	}
	return n
}

// close closes the node of a division at the end of the division, so that the next division is not in it.
func (t *toc) close(n *tocNode) {
	for i, l := range t.last {
		if l == n {
			for j := i; j < len(t.last); j++ {
				t.last[j] = nil
			}
		}
	}
}

// looseRoots gives the place of a new heading with no number and no parent among the top headings with no number.
func (t *toc) looseRoots() int {
	n := 1
	for _, r := range t.roots {
		if r.Number == "" {
			n++
		}
	}
	return n
}

// nodes gives the tree of resource nodes.
func nodes(ns []*tocNode) []resource.Node {
	out := make([]resource.Node, len(ns))
	for i, n := range ns {
		out[i] = n.Node
		if len(n.kids) > 0 {
			out[i].Children = nodes(n.kids)
		}
	}
	return out
}

// roman gives a number in Roman numerals, as \Roman does.
func roman(n int) string {
	vals := []int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	syms := []string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range vals {
		for n >= v {
			b.WriteString(syms[i])
			n -= v
		}
	}
	return b.String()
}

// letter gives a number as a capital letter, as \Alph does. A number after 26 stays a number.
func letter(n int) string {
	if n >= 1 && n <= 26 {
		return string(rune('A' + n - 1))
	}
	return strconv.Itoa(n)
}
