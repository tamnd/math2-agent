// Package mathcomp is the ingest adapter of the Mathematical Components library, a library of Rocq (corpus K6.3). It gives one record for each lemma and each definition.
//
// The bulk channel is the git archive of math-comp/math-comp at a pinned commit. The adapter reads the Rocq source as text. It does not build the library. The library is under CeCILL-B. Corpus K6.6 does not name CeCILL-B among the licences whose text goes into the prose zone, thus the records keep the names and the places, and no text.
//
// The id of a record is the full Rocq name: the logical path of the file, the modules that hold the declaration and the name, for example mathcomp.boot.ssrnat.addnC. The adapter skips the files under test_suite/, doc/, docs/, etc/ and all/, and the declarations between HB.builders and HB.end, which build an instance and are not part of the interface. The kind is statement for a Lemma, Theorem, Corollary, Fact, Remark or Proposition, and definition for a Definition, Fixpoint, Inductive, CoInductive, Variant, Record or Structure, also with the HB.mixin, HB.factory, HB.structure and HB.lock forms. The title is the name, the locator is the file and the line, and the URL is the line at the pinned commit. The meta of a record has these keys, each one only when the declaration has a value:
//
//	keyword  the Rocq keyword, for example Lemma or HB.mixin Record
//	file     the logical path of the file, for example mathcomp.boot.ssrnat
//	section  the sections that hold the declaration, with " > " between them
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/mathcomp"
package mathcomp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// Repository is the git repository of the library.
const Repository = "https://github.com/math-comp/math-comp"

// Tag is the pinned release, and Commit is its commit. The licence was read in the file LICENCE at this commit on 2026-10-27.
const (
	Tag    = "mathcomp-2.6.0"
	Commit = "7cde45afa55ead3410e17081d9ab4bdd53def3e7"
)

// Licence is the SPDX identifier of the licence of the library.
const Licence = "CECILL-B"

// Root is the logical root of the files, from -R . mathcomp in _CoqProject.
const Root = "mathcomp"

// skip has the top directories that are not the library.
var skip = map[string]bool{"test_suite": true, "doc": true, "docs": true, "etc": true, "all": true}

// Adapter is the adapter of the library.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the library.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:           "mathcomp",
		Name:         "The Mathematical Components library",
		Kind:         ingest.Library,
		Home:         "https://math-comp.github.io",
		Licence:      Licence,
		Class:        ingest.ClassOf(Licence),
		Authored:     "human",
		Intake:       "crosswalk",
		IntakeReason: "a library in Rocq, not Lean 4, thus no bindings. Corpus K6.6 does not name CeCILL-B for the prose zone, thus the records keep only the names and the places",
		Edition:      Tag,
		Channel:      "git",
		Attribution:  "The Mathematical Components team, " + Repository + ", CeCILL-B",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "mathcomp/1" }

// Dump returns the git archive of the pinned commit.
func (Adapter) Dump() string { return Repository + "/archive/" + Commit + ".tar.gz" }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit. It fails when the archive is not of the pinned commit, because a person reads the licence again at each pin (corpus K6.3).
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	if err := ingest.FetchArchive(ctx, req, w); err != nil {
		return err
	}
	if req.Commit != Commit {
		return fmt.Errorf("the archive is of the commit %s, and mathcomp is pinned at %s. A person reads the licence at the new commit before the pin moves (corpus K6.3)", req.Commit, Commit)
	}
	return nil
}

// Read reads a git archive of the library and emits one record for each declaration. A name that two files or two places give keeps its first place. Read fails when a file has a comment or a string that does not end, when the archive has no commit and when it has no declarations.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	var recs []ingest.Record
	seen := map[string]bool{}
	commit, err := ingest.WalkArchive(r, func(name string, f io.Reader) error {
		if path.Ext(name) != ".v" || skip[strings.SplitN(name, "/", 2)[0]] || strings.HasPrefix(path.Base(name), "all_") {
			return nil
		}
		src, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		decls, err := declarations(string(src))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		file := Root + "." + strings.ReplaceAll(strings.TrimSuffix(name, ".v"), "/", ".")
		for _, d := range decls {
			id := strings.Join(append([]string{file}, append(d.modules, d.name)...), ".")
			if seen[id] {
				continue
			}
			seen[id] = true
			recs = append(recs, ingest.Record{
				ID:      id,
				Kind:    d.kind,
				Title:   d.name,
				Locator: name + ":" + strconv.Itoa(d.line),
				Class:   ingest.ClassOf(Licence),
				Meta:    meta(d, file),
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	if commit == "" {
		return errors.New("the archive does not record its commit")
	}
	if len(recs) == 0 {
		return errors.New("the archive has no declarations")
	}
	for _, rec := range recs {
		file, line, _ := strings.Cut(rec.Locator, ":")
		rec.URL = Repository + "/blob/" + commit + "/" + file + "#L" + line
		if err := emit(rec); err != nil {
			return err
		}
	}
	return nil
}

func meta(d decl, file string) map[string]string {
	m := map[string]string{"keyword": d.keyword, "file": file}
	if len(d.sections) > 0 {
		m["section"] = strings.Join(d.sections, " > ")
	}
	return m
}

// keywords gives the kind of each keyword of a declaration.
var keywords = map[string]string{
	"Lemma": "statement", "Theorem": "statement", "Corollary": "statement", "Fact": "statement", "Remark": "statement", "Proposition": "statement",
	"Definition": "definition", "Fixpoint": "definition", "Inductive": "definition", "CoInductive": "definition", "Variant": "definition", "Record": "definition", "Structure": "definition",
	"HB.mixin Record": "definition", "HB.factory Record": "definition", "HB.structure Definition": "definition", "HB.lock Definition": "definition",
}

// decl is one declaration of a file.
type decl struct {
	name, kind, keyword string
	line                int
	modules, sections   []string
}

// declarations gives the declarations of the Rocq source src, in order.
func declarations(src string) ([]decl, error) {
	sents, err := sentences(src)
	if err != nil {
		return nil, err
	}
	type scope struct {
		name   string
		module bool
	}
	var (
		out      []decl
		scopes   []scope
		builders bool
	)
	for _, s := range sents {
		words := strings.Fields(s.text)
		words = trimAttributes(words)
		if len(words) == 0 {
			continue
		}
		switch {
		case words[0] == "HB.builders":
			builders = true
			continue
		case words[0] == "HB.end":
			builders = false
			continue
		case words[0] == "Section" && len(words) == 2:
			scopes = append(scopes, scope{name: words[1]})
			continue
		case words[0] == "Module" && !strings.Contains(s.text, ":="):
			ws := words[1:]
			if len(ws) > 0 && (ws[0] == "Import" || ws[0] == "Export" || ws[0] == "Type") {
				ws = ws[1:]
			}
			if len(ws) > 0 {
				scopes = append(scopes, scope{name: ws[0], module: true})
			}
			continue
		case words[0] == "End" && len(words) == 2:
			for i := len(scopes) - 1; i >= 0; i-- {
				if scopes[i].name == words[1] {
					scopes = scopes[:i]
					break
				}
			}
			continue
		}
		if builders {
			continue
		}
		keyword, rest := words[0], words[1:]
		if strings.HasPrefix(keyword, "HB.") && len(rest) > 0 {
			keyword, rest = keyword+" "+rest[0], rest[1:]
		}
		kind, ok := keywords[keyword]
		if !ok || len(rest) == 0 {
			continue
		}
		name := rest[0]
		if i := strings.IndexAny(name, ":({[|"); i >= 0 {
			name = name[:i]
		}
		if name == "" || name == "_" || !isIdent(name) {
			continue
		}
		d := decl{name: name, kind: kind, keyword: keyword, line: s.line}
		for _, sc := range scopes {
			if sc.module {
				d.modules = append(d.modules, sc.name)
			} else {
				d.sections = append(d.sections, sc.name)
			}
		}
		out = append(out, d)
	}
	return out, nil
}

// trimAttributes removes the attributes and the locality words at the start of a sentence, for example #[local] and Local.
func trimAttributes(words []string) []string {
	for len(words) > 0 {
		switch w := words[0]; {
		case strings.HasPrefix(w, "#["):
			for len(words) > 0 && !strings.HasSuffix(words[0], "]") {
				words = words[1:]
			}
			if len(words) > 0 {
				words = words[1:]
			}
		case w == "Local", w == "Global", w == "Program", w == "Polymorphic", w == "Monomorphic":
			words = words[1:]
		default:
			return words
		}
	}
	return words
}

// isIdent reports whether s is a Rocq identifier.
func isIdent(s string) bool {
	for i, c := range s {
		switch {
		case c == '_' || c == '\'' && i > 0:
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		case c >= 0x80:
		default:
			return false
		}
	}
	return s != ""
}

// sentence is one Rocq sentence, without its comments, and the line of its first word.
type sentence struct {
	text string
	line int
}

// sentences splits the Rocq source into sentences. A sentence ends at a period before white space or the end of the source. Comments nest, and a string in a comment is a string.
func sentences(src string) ([]sentence, error) {
	var (
		out   []sentence
		b     strings.Builder
		line  = 1
		start = 0 // the line of the first word of the sentence, or 0
	)
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '(' && i+1 < len(src) && src[i+1] == '*':
			end, lines, err := comment(src, i)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			line += lines
			i = end
			b.WriteByte(' ')
			continue
		case c == '"':
			end, lines, err := str(src, i)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", line, err)
			}
			if start == 0 {
				start = line
			}
			b.WriteString(src[i : end+1])
			line += lines
			i = end
			continue
		case c == '.' && (i+1 == len(src) || isSpace(src[i+1])):
			if start != 0 {
				out = append(out, sentence{text: b.String(), line: start})
			}
			b.Reset()
			start = 0
			continue
		case c == '\n':
			line++
		}
		if start == 0 && !isSpace(c) {
			start = line
		}
		b.WriteByte(c)
	}
	return out, nil
}

// comment gives the index of the closing parenthesis of the comment at src[i:], and the count of its newlines.
func comment(src string, i int) (int, int, error) {
	depth, lines := 0, 0
	for ; i < len(src); i++ {
		switch {
		case src[i] == '(' && i+1 < len(src) && src[i+1] == '*':
			depth++
			i++
		case src[i] == '*' && i+1 < len(src) && src[i+1] == ')':
			depth--
			i++
			if depth == 0 {
				return i, lines, nil
			}
		case src[i] == '"':
			end, n, err := str(src, i)
			if err != nil {
				return 0, 0, err
			}
			i, lines = end, lines+n
		case src[i] == '\n':
			lines++
		}
	}
	return 0, 0, errors.New("a comment does not end")
}

// str gives the index of the closing quote of the string at src[i:], and the count of its newlines. Two quotes in a string are one quote.
func str(src string, i int) (int, int, error) {
	lines := 0
	for i++; i < len(src); i++ {
		switch src[i] {
		case '"':
			if i+1 < len(src) && src[i+1] == '"' {
				i++
				continue
			}
			return i, lines, nil
		case '\n':
			lines++
		}
	}
	return 0, 0, errors.New("a string does not end")
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
