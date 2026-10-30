package textbook

import (
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// IndexEnv is the environment of the record of an index term. The locator of its mention has this kind.
const IndexEnv = "index"

// indexTerm is one term of the back-of-book index of a book (RS4.6). Node is the key of the node of the table of contents that holds the term, or empty before the first heading.
type indexTerm struct {
	term, node, file string
	line             int
}

// texWrap matches a command that only wraps its argument, as \emph{word}.
var texWrap = regexp.MustCompile(`\\(?:emph|textbf|textit|textrm|textsf|texttt|textsc|textup|mathrm|mathbf|mathit|mbox|text)\s*\{([^{}]*)\}`)

// here gives the key of the deepest open node of the table of contents, or "".
func (t *toc) here() string {
	for i := len(t.last) - 1; i >= 0; i-- {
		if t.last[i] != nil {
			return t.last[i].Key
		}
	}
	return ""
}

// latexTerm adds the term of the \index command at m in the file s.
func (t *toc) latexTerm(file, s string, m []int) {
	arg, ok := braced(s[m[1]-1:])
	if !ok {
		return
	}
	if term := makeindex(arg); term != "" {
		t.terms = append(t.terms, indexTerm{term: term, node: t.here(), file: file, line: 1 + strings.Count(s[:m[0]], "\n")})
	}
}

// makeindex gives the term of the argument of \index, in the syntax of makeindex. A subentry follows !, as group!term, and the term gives "group, term". The text after @ is the printed form of a level. A cross reference (|see, |seealso) and the end of a range, |), give no term, because they are not a place of the term in the book.
func makeindex(arg string) string {
	entry, encap := splitAt(arg, '|')
	if strings.HasPrefix(encap, "see") || encap == ")" {
		return ""
	}
	var levels []string
	for _, lvl := range splitAll(entry, '!') {
		if _, shown := splitAt(lvl, '@'); shown != "" {
			lvl = shown
		}
		if lvl = cleanTeX(unquote(lvl)); lvl != "" {
			levels = append(levels, lvl)
		}
	}
	return strings.Join(levels, ", ")
}

// splitAt splits s at the first c that is not quoted with " and not in braces. The quote characters stay in the parts, and unquote removes them.
func splitAt(s string, c byte) (string, string) {
	parts := splitN(s, c, 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func splitAll(s string, c byte) []string { return splitN(s, c, -1) }

func splitN(s string, c byte, n int) []string {
	var out []string
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; {
		case (ch == '"' || ch == '\\') && i+1 < len(s):
			b.WriteByte(ch)
			i++
			b.WriteByte(s[i])
			continue
		case ch == '{':
			depth++
		case ch == '}':
			depth--
		case ch == c && depth == 0 && (n < 0 || len(out) < n-1):
			out = append(out, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(s[i])
	}
	return append(out, b.String())
}

// unquote removes the quote character " of makeindex before a character.
func unquote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '"' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// cleanTeX removes the commands that only wrap text and the braces, and collapses the spaces. Mathematics in $ stays.
func cleanTeX(s string) string {
	for {
		t := texWrap.ReplaceAllString(s, "$1")
		if t == s {
			break
		}
		s = t
	}
	s = strings.NewReplacer("{", "", "}", "", "~", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// pretextTerm reads an idx element of a PreTeXt book and adds its term. The levels are the h elements, or the text of the element when it has no h. An idx with see or seealso is a cross reference, and it gives no term.
func (p *pretext) pretextTerm(d *xml.Decoder, file string, line int) error {
	var levels []string
	var text strings.Builder
	see := false
	err := p.children(d, file, &sink{b: &text, refs: new([]string)}, func(c xml.StartElement, _ int) error {
		switch c.Name.Local {
		case "h":
			var b strings.Builder
			err := p.children(d, file, &sink{b: &b, refs: new([]string)}, nil)
			levels = append(levels, strings.Join(strings.Fields(b.String()), " "))
			return err
		case "see", "seealso":
			see = true
			return d.Skip()
		}
		return p.node(d, file, c, line, &sink{b: &text, refs: new([]string)})
	})
	if err != nil || see {
		return err
	}
	if len(levels) == 0 {
		levels = []string{strings.Join(strings.Fields(text.String()), " ")}
	}
	var keep []string
	for _, l := range levels {
		if l != "" {
			keep = append(keep, l)
		}
	}
	if len(keep) > 0 {
		p.toc.terms = append(p.toc.terms, indexTerm{term: strings.Join(keep, ", "), node: p.toc.here(), file: file, line: line})
	}
	return nil
}

// termRecords gives one record of kind concept for each term of the index and each node that holds it (RS4.6). The record has the term and its place and no text, because the index gives terms only. Its id is index/, the node, a slash and the term in lower case with a dash for each run of other characters, so that it stays the same when the book adds a term.
func (b Book) termRecords(terms []indexTerm, commit string) []ingest.Record {
	commit = cmpOr(commit, b.Commit)
	seen := map[string]bool{}
	ids := map[string]bool{}
	var out []ingest.Record
	for _, t := range terms {
		if seen[t.node+"\x00"+t.term] {
			continue
		}
		seen[t.node+"\x00"+t.term] = true
		id := "index/" + cmpOr(t.node, "front") + "/" + slug(t.term)
		for base, i := id, 2; ids[id]; i++ {
			id = base + "-" + strconv.Itoa(i)
		}
		ids[id] = true
		meta := map[string]string{"environment": IndexEnv}
		if t.node != "" {
			meta["node"] = t.node
		}
		out = append(out, ingest.Record{
			ID:      id,
			Kind:    "concept",
			Title:   t.term,
			Locator: t.file + ":" + strconv.Itoa(t.line),
			URL:     b.blob(commit, t.file, t.line),
			Meta:    meta,
		})
	}
	return out
}

// slug gives s in lower case with one dash for each run of characters that are not letters or digits.
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	return cmpOr(b.String(), "term")
}
