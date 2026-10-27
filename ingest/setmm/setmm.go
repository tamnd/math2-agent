// Package setmm is the ingest adapter of set.mm, the Metamath database of set theory and the mathematics built on it (corpus K6.3 and K6.6). It gives one record for each theorem, axiom and definition.
//
// The bulk channel is the file set.mm of the repository metamath/set.mm at a pinned commit. The adapter reads the database as Metamath tokens. It does not check the proofs. The database is in the public domain under CC0 1.0, and each statement has a comment that a person wrote. That comment is the text of the record, with attribution, which CC0 does not need and corpus K13.8 asks for.
//
// The id of a record is the label of the statement, for example id. The kind is statement for a theorem ($p) and an axiom (an $a whose label starts with ax-), and definition for a definition (an $a whose label starts with df-). The adapter skips the other $a statements, which give the syntax. The title is the label, the locator is set.mm and the line of the label, and the URL is the page of the label on the Metamath site. The meta of a record has these keys, each one only when the statement has a value:
//
//	keyword      theorem, axiom or definition
//	statement    the assertion after the typecode, as the database gives it, for example ( ph -> ph )
//	hypotheses   the assertions of the $e hypotheses in the scope of the statement, with " && " between them
//	section      the title of the last section header before the statement
//	contributed  the contributor and the date of the statement, from the comment
//	discouraged  yes when the comment says that new usage is discouraged
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/setmm"
package setmm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// Repository is the git repository of set.mm.
const Repository = "https://github.com/metamath/set.mm"

// Commit is the pinned commit of the repository. The licence was read in the header of set.mm at this commit on 2026-10-27.
const Commit = "f05c2caa5338ccd5b40b8c565d76c54fd8efdd81"

// Raw is the root of the raw files of the repository. A file is Raw + commit + "/" + path.
const Raw = "https://raw.githubusercontent.com/metamath/set.mm/"

// Site is the root of the pages of the labels on the Metamath site.
const Site = "https://us.metamath.org/mpeuni/"

// File is the name of the database in the repository.
const File = "set.mm"

// Adapter is the adapter of set.mm.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of set.mm.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:           "setmm",
		Name:         "Metamath Proof Explorer (set.mm)",
		Kind:         ingest.Library,
		Home:         "https://us.metamath.org/mpeuni/mmset.html",
		Licence:      "CC0-1.0",
		Class:        ingest.Open,
		Authored:     "human",
		Intake:       "crosswalk",
		IntakeReason: "a library in Metamath, not Lean 4, thus no bindings. CC0 allows its comments in the prose zone (corpus K6.6)",
		Channel:      "git",
		Attribution:  "The Metamath Proof Explorer (set.mm), the Metamath community, " + Repository + ", CC0 1.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "setmm/1" }

// Dump returns the file set.mm at the pinned commit.
func (Adapter) Dump() string { return Raw + Commit + "/" + File }

// Fetch gets the file at req.URL and sets req.Commit to the pinned commit. It fails when the URL does not name the pinned commit, because a person reads the licence again at each pin (corpus K6.3).
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	if !strings.Contains(req.URL, "/"+Commit+"/") {
		return fmt.Errorf("the URL %s does not name the pinned commit %s of set.mm. A person reads the licence at the new commit before the pin moves (corpus K6.3)", req.URL, Commit)
	}
	body, err := req.Get(ctx, req.URL)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	if _, err := io.Copy(w, body); err != nil {
		return err
	}
	req.Commit = Commit
	return nil
}

var (
	contributed = regexp.MustCompile(`\(Contributed by ([^()]+?)\.\)`)
	header      = regexp.MustCompile(`^[#*=\-.~]{6,}$`)
)

// Read reads the database and emits one record for each theorem, axiom and definition. Read fails when a statement or a comment does not end, when a label is in the file two times, and when the file has no statements.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	t := &tokens{sc: bufio.NewScanner(r), line: 1}
	t.sc.Buffer(nil, 1<<20)
	t.sc.Split(scanTokens(&t.newlines))
	var (
		comment, section string
		scopes           = [][]string{nil} // the $e assertions of each open scope
		seen             = map[string]int{}
		n                int
	)
	for {
		tok, line, ok := t.next()
		if !ok {
			break
		}
		switch tok {
		case "$(":
			text, err := t.until("$)", line)
			if err != nil {
				return err
			}
			switch title, isHeader := headerTitle(text); {
			case isHeader:
				section, comment = title, ""
			case strings.HasPrefix(strings.TrimSpace(text), "$t"), strings.HasPrefix(strings.TrimSpace(text), "$j"):
			default:
				comment = text
			}
			continue
		case "${":
			scopes = append(scopes, nil)
			continue
		case "$}":
			if len(scopes) == 1 {
				return fmt.Errorf("%s:%d: $} closes no scope", File, line)
			}
			scopes = scopes[:len(scopes)-1]
			continue
		case "$[":
			if _, err := t.until("$]", line); err != nil {
				return err
			}
			continue
		case "$c", "$v", "$d":
			if _, err := t.until("$.", line); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(tok, "$") {
			return fmt.Errorf("%s:%d: the keyword %s has no label", File, line, tok)
		}
		label := tok
		kw, _, ok := t.next()
		if !ok {
			return fmt.Errorf("%s:%d: the statement %s does not end", File, line, label)
		}
		switch kw {
		case "$f":
			if _, err := t.until("$.", line); err != nil {
				return err
			}
			continue
		case "$e":
			math, err := t.until("$.", line)
			if err != nil {
				return err
			}
			if typecode, rest, _ := strings.Cut(math, " "); typecode == "|-" {
				scopes[len(scopes)-1] = append(scopes[len(scopes)-1], rest)
			}
			continue
		case "$a", "$p":
		default:
			return fmt.Errorf("%s:%d: the label %s has the keyword %s", File, line, label, kw)
		}
		end := "$."
		if kw == "$p" {
			end = "$="
		}
		math, err := t.until(end, line)
		if err != nil {
			return err
		}
		if kw == "$p" {
			if _, err := t.until("$.", line); err != nil {
				return err
			}
		}
		desc := comment
		comment = ""
		if first, dup := seen[label]; dup {
			return fmt.Errorf("%s:%d: the label %s is also at line %d", File, line, label, first)
		}
		seen[label] = line
		typecode, assertion, _ := strings.Cut(math, " ")
		keyword := keywordOf(kw, label)
		if typecode != "|-" || keyword == "" {
			continue
		}
		rec := record(label, keyword, line, desc)
		meta := map[string]string{"keyword": keyword, "statement": assertion}
		var hyps []string
		for _, s := range scopes {
			hyps = append(hyps, s...)
		}
		if len(hyps) > 0 {
			meta["hypotheses"] = strings.Join(hyps, " && ")
		}
		if section != "" {
			meta["section"] = section
		}
		if m := contributed.FindStringSubmatch(rec.Text); m != nil {
			meta["contributed"] = m[1]
		}
		if strings.Contains(rec.Text, "(New usage is discouraged.)") {
			meta["discouraged"] = "yes"
		}
		rec.Meta = meta
		if err := emit(rec); err != nil {
			return err
		}
		n++
	}
	if err := t.sc.Err(); err != nil {
		return err
	}
	if n == 0 {
		return errors.New("the database has no statements")
	}
	return nil
}

// keywordOf gives the keyword of a statement: theorem for $p, axiom and definition for an $a by the prefix of its label. It gives "" for an $a of syntax.
func keywordOf(kw, label string) string {
	switch {
	case kw == "$p":
		return "theorem"
	case strings.HasPrefix(label, "ax-"):
		return "axiom"
	case strings.HasPrefix(label, "df-"):
		return "definition"
	}
	return ""
}

// record gives the record of a statement without its meta.
func record(label, keyword string, line int, desc string) ingest.Record {
	kind := "statement"
	if keyword == "definition" {
		kind = "definition"
	}
	return ingest.Record{
		ID:      label,
		Kind:    kind,
		Title:   label,
		URL:     Site + label + ".html",
		Locator: File + ":" + strconv.Itoa(line),
		Class:   ingest.Open,
		Text:    strings.Join(strings.Fields(desc), " "),
	}
}

// headerTitle reports whether a comment is a section header, a comment whose first line is a row of marks such as #*#*#*, and gives its title: the first line after the marks.
func headerTitle(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i == len(lines) || !header.MatchString(strings.TrimSpace(lines[i])) {
		return "", false
	}
	for _, l := range lines[i+1:] {
		if l = strings.TrimSpace(l); l != "" && !header.MatchString(l) {
			return l, true
		}
	}
	return "", true
}

// tokens reads the tokens of the database with their lines.
type tokens struct {
	sc       *bufio.Scanner
	line     int
	newlines int // the newlines before the last token, which the split function counts
}

// next gives the next token and its line.
func (t *tokens) next() (string, int, bool) {
	if !t.sc.Scan() {
		return "", t.line, false
	}
	t.line += t.newlines
	return t.sc.Text(), t.line, true
}

// until reads the tokens up to the token end and gives them with a space between them. A comment keeps its line breaks, because a section header has one title on a line. It fails when the file ends first.
func (t *tokens) until(end string, start int) (string, error) {
	var b strings.Builder
	last := t.line
	for {
		tok, line, ok := t.next()
		if !ok {
			return "", fmt.Errorf("%s:%d: the file ends before %s", File, start, end)
		}
		if tok == end {
			return b.String(), nil
		}
		if b.Len() > 0 {
			if end == "$)" && line > last {
				b.WriteString("\n")
			} else {
				b.WriteString(" ")
			}
		}
		b.WriteString(tok)
		last = line
	}
}

// scanTokens splits the input at white space, as Metamath does, and counts the newlines before each token in *newlines.
func scanTokens(newlines *int) bufio.SplitFunc {
	return func(data []byte, atEOF bool) (int, []byte, error) {
		*newlines = 0
		i := 0
		for i < len(data) && isSpace(data[i]) {
			if data[i] == '\n' {
				*newlines++
			}
			i++
		}
		for j := i; j < len(data); j++ {
			if isSpace(data[j]) {
				return j, data[i:j], nil
			}
		}
		if atEOF && i < len(data) {
			return len(data), data[i:], nil
		}
		if atEOF {
			return len(data), nil, nil
		}
		return 0, nil, nil // read more, and count the white space again
	}
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }
