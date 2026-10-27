// Package afp is the ingest adapter of the Archive of Formal Proofs, the library of Isabelle entries (corpus K6.3). It gives one record for each entry and for each named lemma and definition of its theories.
//
// The bulk channel is the git archive of isabelle-prover/mirror-afp-devel, the mirror of the AFP on GitHub, at a pinned commit. The release tarball has no commit, thus the adapter takes the mirror. The adapter reads the metadata of the entries and the theory files as text. It does not build the theories.
//
// Each entry has its own licence: BSD-3-Clause, or LGPL-2.1-only for an entry with license = "lgpl" in its metadata. Corpus K6.6 lets the text of a BSD entry into the prose zone, and not the text of an LGPL entry. Thus the record of a BSD entry has the abstract as its text, and the record of an LGPL entry has no text. The records of the lemmas and the definitions have no text.
//
// The id of the record of an entry is the name of the entry, for example Prime_Number_Theorem, and its kind is entry. The title is the title of the entry, and the URL is its page on the AFP site. The meta of an entry has these keys:
//
//	topics   the topics of the entry, with "|" between them
//	date     the date of the entry, YYYY-MM-DD
//	authors  the names of the authors, with "|" between them
//
// The id of the record of a lemma or a definition is the entry, the theory, the locale and the name, for example Prime_Number_Theorem.Mertens_Theorems.mertens_first_theorem. The locale is in the id only when the declaration is in a locale or a class, by (in X) or in a context X. The kind is statement for a lemma, theorem, corollary or proposition, and definition for a definition, fun, function, primrec, inductive, inductive_set, coinductive, datatype, codatatype, typedef, record, locale or class. The adapter skips the declarations with no name and the ones in an experiment or a notepad. The title is the name, the locator is the file and the line, and the URL is the line at the pinned commit. The meta has these keys, each one only when the declaration has a value:
//
//	keyword  the Isabelle keyword, for example lemma
//	entry    the entry
//	theory   the theory
//	locale   the locale or the class
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/afp"
package afp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// Repository is the git repository of the mirror of the AFP.
const Repository = "https://github.com/isabelle-prover/mirror-afp-devel"

// Commit is the pinned commit of the mirror. The licences were read in thys/LICENSE and thys/LICENSE.LGPL at this commit on 2026-10-27.
const Commit = "a2248ecf461df9deb8b6e507d073c7b296535371"

// Site is the root of the pages of the entries.
const Site = "https://www.isa-afp.org/entries/"

// The licences of the entries.
const (
	BSD  = "BSD-3-Clause"
	LGPL = "LGPL-2.1-only"
)

// Adapter is the adapter of the AFP.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the AFP.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:             "afp",
		Name:           "Archive of Formal Proofs",
		Kind:           ingest.Library,
		Home:           "https://www.isa-afp.org",
		Licence:        BSD,
		Class:          ingest.Open,
		PerItemLicence: true,
		Authored:       "human",
		Intake:         "crosswalk",
		IntakeReason:   "a library in Isabelle, not Lean 4, thus no bindings. Each entry is under BSD-3-Clause or LGPL-2.1, and only the abstracts of the BSD entries go into the prose zone (corpus K6.6)",
		Channel:        "git",
		Attribution:    "The Archive of Formal Proofs, the authors of each entry, https://www.isa-afp.org, BSD-3-Clause or LGPL-2.1 for each entry",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "afp/1" }

// Dump returns the git archive of the pinned commit.
func (Adapter) Dump() string { return Repository + "/archive/" + Commit + ".tar.gz" }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit. It fails when the archive is not of the pinned commit, because a person reads the licences again at each pin (corpus K6.3).
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	if err := ingest.FetchArchive(ctx, req, w); err != nil {
		return err
	}
	if req.Commit != Commit {
		return fmt.Errorf("the archive is of the commit %s, and afp is pinned at %s. A person reads the licences at the new commit before the pin moves (corpus K6.3)", req.Commit, Commit)
	}
	return nil
}

// entry is the metadata of one entry.
type entry struct {
	title, date, abstract, licence string
	topics, authors                []string
}

// Read reads a git archive of the mirror and emits the record of each entry and then the records of the declarations, by entry. A name that two places give keeps its first place. The declarations of a directory with no metadata are not read. A theory with a comment, a string or a cartouche that does not end is not read either: Isabelle cannot load it, thus no session has it, because the AFP builds each theory of its sessions. Read fails when the metadata of an entry does not parse, when the archive has no commit and when it has no entries.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	entries := map[string]entry{}
	names := map[string]string{} // the name of each author id
	decls := map[string][]decl{} // the declarations of each entry
	commit, err := ingest.WalkArchive(r, func(name string, f io.Reader) error {
		switch {
		case name == "metadata/authors.toml":
			src, err := io.ReadAll(f)
			if err != nil {
				return err
			}
			for id, t := range tables(string(src)) {
				if n := t["name"]; n != "" {
					names[id] = n
				}
			}
		case strings.HasPrefix(name, "metadata/entries/") && path.Ext(name) == ".toml":
			src, err := io.ReadAll(f)
			if err != nil {
				return err
			}
			e, err := parseEntry(string(src))
			if err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			entries[strings.TrimSuffix(path.Base(name), ".toml")] = e
		case strings.HasPrefix(name, "thys/") && path.Ext(name) == ".thy":
			parts := strings.Split(name, "/")
			if len(parts) < 3 {
				return nil
			}
			src, err := io.ReadAll(f)
			if err != nil {
				return err
			}
			ds, err := declarations(string(src))
			if err != nil {
				return nil // not a theory of a session, because the AFP builds each theory of its sessions
			}
			for i := range ds {
				ds[i].file = name
				ds[i].theory = strings.TrimSuffix(path.Base(name), ".thy")
			}
			decls[parts[1]] = append(decls[parts[1]], ds...)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if commit == "" {
		return errors.New("the archive does not record its commit")
	}
	if len(entries) == 0 {
		return errors.New("the archive has no entries")
	}
	ids := make([]string, 0, len(entries))
	for id := range entries {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	seen := map[string]bool{}
	for _, id := range ids {
		e := entries[id]
		rec := ingest.Record{ID: id, Kind: "entry", Title: e.title, URL: Site + id + ".html", Locator: "metadata/entries/" + id + ".toml", Licence: e.licence, Class: ingest.ClassOf(e.licence)}
		if rec.Class == ingest.Open {
			rec.Text = plain(e.abstract)
		}
		var authors []string
		for _, a := range e.authors {
			authors = append(authors, cmp.Or(names[a], a))
		}
		rec.Meta = meta(map[string]string{"topics": strings.Join(e.topics, "|"), "date": e.date, "authors": strings.Join(authors, "|")})
		seen[id] = true
		if err := emit(rec); err != nil {
			return err
		}
		for _, d := range decls[id] {
			parts := []string{id, d.theory}
			if d.locale != "" {
				parts = append(parts, d.locale)
			}
			did := strings.Join(append(parts, d.name), ".")
			if seen[did] {
				continue
			}
			seen[did] = true
			rec := ingest.Record{
				ID:      did,
				Kind:    d.kind,
				Title:   d.name,
				URL:     Repository + "/blob/" + commit + "/" + d.file + "#L" + strconv.Itoa(d.line),
				Locator: d.file + ":" + strconv.Itoa(d.line),
				Licence: e.licence,
				Class:   ingest.ClassOf(e.licence),
				Meta:    meta(map[string]string{"keyword": d.keyword, "entry": id, "theory": d.theory, "locale": d.locale}),
			}
			if err := emit(rec); err != nil {
				return err
			}
		}
	}
	return nil
}

// meta returns m without its empty values, or nil when m has none.
func meta(m map[string]string) map[string]string {
	for k, v := range m {
		if v == "" {
			delete(m, k)
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

var (
	block = regexp.MustCompile(`(?i)</?(?:p|br|li|ul|ol|div|h[1-6]|blockquote|pre|table|tr|td|dl|dt|dd)\b[^>]*>`)
	tag   = regexp.MustCompile(`<[^>]*>`)
)

// plain gives the text of an abstract in HTML, with no tags, the entities decoded and the white space folded. A tag of a block is a space, and another tag is nothing.
func plain(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tag.ReplaceAllString(block.ReplaceAllString(s, " "), ""))), " ")
}

// parseEntry reads the metadata of an entry.
func parseEntry(src string) (entry, error) {
	var e entry
	top := tables(src)[""]
	e.title, e.date = top["title"], top["date"]
	e.abstract = top["abstract"]
	switch top["license"] {
	case "bsd":
		e.licence = BSD
	case "lgpl":
		e.licence = LGPL
	default:
		return e, fmt.Errorf("the licence %q is not bsd or lgpl", top["license"])
	}
	if e.title == "" {
		return e, errors.New("the entry has no title")
	}
	for v := range strings.SplitSeq(top["topics"], "\x00") {
		if v != "" {
			e.topics = append(e.topics, v)
		}
	}
	for _, l := range strings.Split(src, "\n") {
		l = strings.TrimSpace(l)
		if a, ok := strings.CutPrefix(l, "[authors."); ok && strings.HasSuffix(a, "]") && !strings.Contains(a, ".") {
			e.authors = append(e.authors, strings.TrimSuffix(a, "]"))
		}
	}
	return e, nil
}

// tables reads the key = value lines of the TOML of the AFP metadata, by table. The values are strings, multi-line strings in """, dates and arrays of strings. An array gives its strings with NUL between them. A key of a table [a.b] is in the table a.b.
func tables(src string) map[string]map[string]string {
	out := map[string]map[string]string{"": {}}
	cur := ""
	lines := strings.Split(src, "\n")
	for i := 0; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") && !strings.Contains(l, "=") {
			cur = strings.Trim(l, "[]")
			if out[cur] == nil {
				out[cur] = map[string]string{}
			}
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch {
		case strings.HasPrefix(v, `"""`):
			v = strings.TrimPrefix(v, `"""`)
			var b strings.Builder
			for {
				if before, _, done := strings.Cut(v, `"""`); done {
					b.WriteString(before)
					break
				}
				b.WriteString(v + "\n")
				i++
				if i == len(lines) {
					break
				}
				v = lines[i]
			}
			v = b.String()
		case strings.HasPrefix(v, "["):
			for !strings.Contains(v, "]") && i+1 < len(lines) {
				i++
				v += " " + strings.TrimSpace(lines[i])
			}
			var items []string
			for _, m := range quoted.FindAllStringSubmatch(v, -1) {
				items = append(items, unquote(m[1]))
			}
			v = strings.Join(items, "\x00")
		case strings.HasPrefix(v, `"`):
			if m := quoted.FindStringSubmatch(v); m != nil {
				v = unquote(m[1])
			}
		}
		out[cur][k] = v
	}
	return out
}

var quoted = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

func unquote(s string) string {
	if u, err := strconv.Unquote(`"` + s + `"`); err == nil {
		return u
	}
	return s
}

// kinds gives the kind of each keyword of a declaration.
var kinds = map[string]string{
	"lemma": "statement", "theorem": "statement", "corollary": "statement", "proposition": "statement",
	"definition": "definition", "fun": "definition", "function": "definition", "primrec": "definition",
	"inductive": "definition", "inductive_set": "definition", "coinductive": "definition",
	"datatype": "definition", "codatatype": "definition", "typedef": "definition", "record": "definition",
	"locale": "definition", "class": "definition",
}

// opens has the commands that open a context at their begin. The context of an experiment or a notepad hides its declarations.
var opens = map[string]bool{"theory": true, "context": true, "locale": true, "class": true, "instantiation": true, "overloading": true, "experiment": true, "notepad": true, "bundle": true}

// notNames has the words after a keyword that start an unnamed statement.
var notNames = map[string]bool{"assumes": true, "shows": true, "fixes": true, "obtains": true, "includes": true, "notes": true, "defines": true, "where": true, "for": true, "and": true, "is": true, "begin": true, "imports": true}

// decl is one declaration of a theory.
type decl struct {
	file, theory, name, kind, keyword, locale string
	line                                      int
}

// scope is one context that a begin opens.
type scope struct {
	locale string
	hidden bool
}

// declarations gives the named declarations of the theory src, in order. Its file and theory are empty.
func declarations(src string) ([]decl, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}
	var (
		out     []decl
		scopes  []scope
		pending *scope // the context that the next begin opens
	)
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.kind != wordTok {
			continue
		}
		switch t.text {
		case "begin":
			s := scope{}
			if pending != nil {
				s = *pending
			}
			if len(scopes) > 0 && scopes[len(scopes)-1].hidden {
				s.hidden = true
			}
			scopes = append(scopes, s)
			pending = nil
			continue
		case "end":
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
			}
			pending = nil
			continue
		}
		if opens[t.text] {
			s := scope{hidden: t.text == "experiment" || t.text == "notepad"}
			if (t.text == "context" || t.text == "locale" || t.text == "class") && i+1 < len(toks) && toks[i+1].kind == wordTok && !notNames[toks[i+1].text] {
				s.locale = toks[i+1].text
			}
			pending = &s
		}
		kind, ok := kinds[t.text]
		if !ok {
			continue
		}
		d := decl{kind: kind, keyword: t.text, line: t.line}
		j := i + 1
		for j < len(toks) && toks[j].kind == openTok {
			if j+3 < len(toks) && toks[j+1].text == "in" && toks[j+3].kind == closeTok {
				d.locale = toks[j+2].text
				j += 4
				continue
			}
			j = skipGroup(toks, j)
		}
		for j < len(toks) && toks[j].kind == wordTok && strings.HasPrefix(toks[j].text, "'") {
			j++
		}
		if j == len(toks) || toks[j].kind != wordTok || notNames[toks[j].text] || kinds[toks[j].text] != "" {
			continue
		}
		d.name = toks[j].text
		if d.kind == "statement" && (j+1 == len(toks) || toks[j+1].text != ":" && toks[j+1].kind != bracketTok) {
			continue
		}
		hidden := false
		for k := len(scopes) - 1; k >= 0; k-- {
			hidden = hidden || scopes[k].hidden
			if d.locale == "" && scopes[k].locale != "" {
				d.locale = scopes[k].locale
			}
		}
		if hidden {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// skipGroup gives the index after the parenthesized group that starts at toks[j].
func skipGroup(toks []token, j int) int {
	depth := 0
	for ; j < len(toks); j++ {
		switch toks[j].kind {
		case openTok:
			depth++
		case closeTok:
			depth--
			if depth == 0 {
				return j + 1
			}
		}
	}
	return j
}

// The kinds of tokens.
const (
	wordTok = iota
	openTok
	closeTok
	bracketTok
	symbolTok
	textTok // a string or a cartouche
)

// token is one token of the outer syntax of a theory.
type token struct {
	kind int
	text string
	line int
}

// tokenize splits a theory into the tokens of its outer syntax. Comments are dropped, and a string or a cartouche is one token. Comments and cartouches nest. A backquote is a symbol, because a fact in backquotes is in a proof, and a theory can have one with no end.
func tokenize(src string) ([]token, error) {
	var out []token
	line := 1
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case strings.HasPrefix(src[i:], "(*"):
			start := line
			depth := 0
			for {
				if i >= len(src) {
					return nil, fmt.Errorf("line %d: a comment does not end", start)
				}
				switch {
				case strings.HasPrefix(src[i:], "(*"):
					depth++
					i += 2
				case strings.HasPrefix(src[i:], "*)"):
					depth--
					i += 2
				default:
					if src[i] == '\n' {
						line++
					}
					i++
				}
				if depth == 0 {
					break
				}
			}
		case strings.HasPrefix(src[i:], `\<open>`):
			start := line
			depth := 0
			j := i
			for {
				if j >= len(src) {
					return nil, fmt.Errorf("line %d: a cartouche does not end", start)
				}
				switch {
				case strings.HasPrefix(src[j:], `\<open>`):
					depth++
					j += len(`\<open>`)
				case strings.HasPrefix(src[j:], `\<close>`):
					depth--
					j += len(`\<close>`)
				default:
					if src[j] == '\n' {
						line++
					}
					j++
				}
				if depth == 0 {
					break
				}
			}
			out = append(out, token{kind: textTok, line: start})
			i = j
		case c == '"':
			start := line
			j := i + 1
			for j < len(src) && src[j] != '"' {
				if src[j] == '\\' {
					j++
				}
				if j < len(src) && src[j] == '\n' {
					line++
				}
				j++
			}
			if j >= len(src) {
				return nil, fmt.Errorf("line %d: a string does not end", start)
			}
			out = append(out, token{kind: textTok, line: start})
			i = j + 1
		case c == '(':
			out = append(out, token{kind: openTok, text: "(", line: line})
			i++
		case c == ')':
			out = append(out, token{kind: closeTok, text: ")", line: line})
			i++
		case c == '[':
			out = append(out, token{kind: bracketTok, text: "[", line: line})
			i++
		case isWord(c):
			j := i
			for j < len(src) && (isWord(src[j]) || src[j] == '.') {
				j++
			}
			out = append(out, token{kind: wordTok, text: strings.TrimRight(src[i:j], "."), line: line})
			i = j
		default:
			out = append(out, token{kind: symbolTok, text: string(c), line: line})
			i++
		}
	}
	return out, nil
}

func isWord(c byte) bool {
	return c == '_' || c == '\'' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
