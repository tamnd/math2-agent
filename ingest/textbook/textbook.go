// Package textbook has the ingest adapters of the open textbooks of Table A of corpus K5.4.1, the books that the prose zone can take. It gives one record for each definition, statement, proof, example and exercise in the LaTeX or PreTeXt source of a book, with its locator.
//
// Each book is one source, with its own licence, and the package registers one adapter for each book in Books. The bulk channel of a book is the git archive of its repository at a pinned commit. A book can change its licence between editions (K5.4.4), so Fetch refuses an archive of another commit: a person reads the licence at the new commit and moves the pin. Hefferon's Linear Algebra is not in Books, because the LICENSE file of its repository names CC BY-SA 2.5 and Table A names CC BY-SA 3.0 US, and a person must settle that first. The books in PDF and CNXML are for another adapter.
//
// A LaTeX book gives a record for each theorem environment in the files that it names. The kind of an environment comes from its name in \newtheorem, \declaretheorem or \crefname in any file of the archive, for example \newtheorem{thm}{Theorem} gives the kind statement to thm. An environment that no file defines, such as the proof of amsthm, has the kind of its usual name. A PreTeXt book gives a record for each element of these kinds, in the files that its main file includes with xi:include. An element with a component attribute is a variant for one platform, and Read does not read it. Read also does not read the pictures, the code and the index entries, because a PreFigure diagram in a picture has definition elements that are not definitions of mathematics. An item with no text, such as an example that the author did not write yet, gives no record.
//
// The id of a record is the path of its file, #, and its label, for example ch-der.tex#thm:rolle. An item with no label has the name of its environment and its number in the file in place of the label, for example ch-der.tex#exercise-3. The kind is one of definition, statement, conjecture, proof, example, counterexample and exercise. The title is the title of the item, and it is empty when the item has none. The locator is the path of the file and the line of the item, for example ch-der.tex:19, and the URL is that line at the commit, on the site of the repository. The text is the source of the statement: LaTeX without the comments, or for PreTeXt the text of the statement with the mathematics in dollars. The licence gate keeps it with its fingerprints, and the inventory keeps it only when the licence is not share-alike. The meta of a record has these keys, each one only when the item has a value:
//
//	environment  the LaTeX environment or the PreTeXt element, for example thm or theorem
//	label        the label of the item
//	section      the title of the division that holds the item
//	proves       of a proof: the id of the statement that it proves
//	uses         the ids of the items that the item refers to, with | between them. They are candidate uses edges (K5.4.5).
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/textbook"
package textbook

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// Format is the format of the source of a book.
type Format string

// The formats that the adapters read.
const (
	LaTeX   Format = "latex"
	PreTeXt Format = "pretext"
)

// Book is an open textbook and its adapter.
type Book struct {
	ID          string   // the id of the source, for example hott_book
	Name        string   // the title of the book
	Authors     []string // the authors of the book
	Home        string   // the home page of the book
	Repository  string   // the repository of the source, for example https://github.com/HoTT/book
	Commit      string   // the pinned commit
	Edition     string   // the pinned edition, where the editions differ in licence
	Licence     string   // the SPDX expression of the licence, as the source states it
	Election    string   // the option that math2 takes from a choice of licences
	Attribution string   // the attribution line that the licence asks for
	Format      Format

	// For a LaTeX book: Files matches the paths of the files to read, and Skip matches the paths among them not to read. Skip can be nil. Macros are more commands of the book that define an environment as \newtheorem does, with the name and the display name as their first two arguments.
	Files, Skip *regexp.Regexp
	Macros      []string

	// For a PreTeXt book: Main is the path of the main file, and Read reads the files that it includes.
	Main string
}

func re(s string) *regexp.Regexp { return regexp.MustCompile(s) }

// Books are the books of Table A of corpus K5.4.1 with a source in LaTeX or PreTeXt. Each licence was read at the source, in the repository at the commit, on 2026-10-27.
var Books = []Book{
	{
		ID: "open_logic", Name: "The Open Logic Project", Authors: []string{"The Open Logic Project"},
		Home: "https://openlogicproject.org", Repository: "https://github.com/OpenLogicProject/OpenLogic", Commit: "1e960beff9ed7835bf3e3f1335e21af3439cd107",
		Licence: "CC-BY-4.0", Attribution: "The Open Logic Project, The Open Logic Text, https://openlogicproject.org, CC BY 4.0",
		Format: LaTeX, Files: re(`^content/.*\.tex$`),
	},
	{
		ID: "hott_book", Name: "Homotopy Type Theory: Univalent Foundations of Mathematics", Authors: []string{"The Univalent Foundations Program"},
		Home: "https://homotopytypetheory.org/book/", Repository: "https://github.com/HoTT/book", Commit: "578b85cc8d586b1677ec4335148adeb443057d24",
		Licence:     "CC-BY-SA-3.0",
		Attribution: "The Univalent Foundations Program, Homotopy Type Theory: Univalent Foundations of Mathematics, https://homotopytypetheory.org/book/, CC BY-SA 3.0",
		Format:      LaTeX, Files: re(`^[a-z]+\.tex$`), Skip: re(`^(errata|exercise_solutions)\.tex$`), Macros: []string{"defthm"},
	},
	{
		ID: "lebl_ra", Name: "Basic Analysis I and II: Introduction to Real Analysis", Authors: []string{"Jiří Lebl"},
		Home: "https://www.jirka.org/ra/", Repository: "https://github.com/jirilebl/ra", Commit: "e21ec524ca7d54f800c693b948020c188d21d01f",
		Licence: "CC-BY-NC-SA-4.0 OR CC-BY-SA-4.0", Election: "CC-BY-SA-4.0",
		Attribution: "Jiří Lebl, Basic Analysis I and II: Introduction to Real Analysis, https://www.jirka.org/ra/, CC BY-SA 4.0",
		Format:      LaTeX, Files: re(`^(ch|frag)-[^/]*\.tex$`),
	},
	{
		ID: "lebl_diffyqs", Name: "Notes on Diffy Qs: Differential Equations for Engineers", Authors: []string{"Jiří Lebl"},
		Home: "https://www.jirka.org/diffyqs/", Repository: "https://github.com/jirilebl/diffyqs", Commit: "658bcae9fb710f3fae2c9da4ca4524ce157453af",
		Licence: "CC-BY-NC-SA-4.0 OR CC-BY-SA-4.0", Election: "CC-BY-SA-4.0",
		Attribution: "Jiří Lebl, Notes on Diffy Qs: Differential Equations for Engineers, https://www.jirka.org/diffyqs/, CC BY-SA 4.0",
		Format:      LaTeX, Files: re(`^(ch|ap)-[^/]*\.tex$`),
	},
	{
		ID: "lebl_scv", Name: "Tasty Bits of Several Complex Variables", Authors: []string{"Jiří Lebl"},
		Home: "https://www.jirka.org/scv/", Repository: "https://github.com/jirilebl/scv", Commit: "78d7d41554695c389552c301398803d4633b9208",
		Licence: "CC-BY-NC-SA-4.0 OR CC-BY-SA-4.0", Election: "CC-BY-SA-4.0",
		Attribution: "Jiří Lebl, Tasty Bits of Several Complex Variables, https://www.jirka.org/scv/, CC BY-SA 4.0",
		Format:      LaTeX, Files: re(`^scv\.tex$`),
	},
	{
		ID: "napkin", Name: "An Infinitely Large Napkin", Authors: []string{"Evan Chen"},
		Home: "https://web.evanchen.cc/napkin.html", Repository: "https://github.com/vEnhance/napkin", Commit: "2bdf25b3bc8c9e9d1043bcbc495bff4c7d10f586",
		// The text is under CC BY-SA 4.0 and the source files are under the GPL v3. math2 takes the text only.
		Licence: "CC-BY-SA-4.0", Attribution: "Evan Chen, An Infinitely Large Napkin, https://web.evanchen.cc/napkin.html, CC BY-SA 4.0",
		Format: LaTeX, Files: re(`^tex/.*\.tex$`), Skip: re(`^tex/(preamble|macros|Qcircuit)\.tex$|^tex/backmatter/`),
	},
	{
		ID: "levin_dmoi", Name: "Discrete Mathematics: An Open Introduction", Authors: []string{"Oscar Levin"}, Edition: "3rd edition",
		Home: "https://discrete.openmathbooks.org/dmoi3.html", Repository: "https://github.com/oscarlevin/discrete-book", Commit: "a8e4949bc45daf100930c1182983778f56ab689f",
		// The tag 3rd-ed. The 4th edition is under CC BY-NC-SA 4.0.
		Licence: "CC-BY-SA-4.0", Attribution: "Oscar Levin, Discrete Mathematics: An Open Introduction, 3rd edition, https://discrete.openmathbooks.org/dmoi3.html, CC BY-SA 4.0",
		Format: PreTeXt, Main: "ptx/dmoi.ptx",
	},
	{
		ID: "applied_combinatorics", Name: "Applied Combinatorics", Authors: []string{"Mitchel T. Keller", "William T. Trotter"},
		Home: "https://appliedcombinatorics.org/appcomb/", Repository: "https://github.com/mitchkeller/applied-combinatorics", Commit: "ebd95d6bd59e23c633c65cf560bcb93efa912aae",
		Licence: "CC-BY-SA-4.0", Attribution: "Mitchel T. Keller and William T. Trotter, Applied Combinatorics, https://appliedcombinatorics.org/appcomb/, CC BY-SA 4.0",
		Format: PreTeXt, Main: "source/main.ptx",
	},
	{
		ID: "ernst_ibl", Name: "An Introduction to Proof via Inquiry-Based Learning", Authors: []string{"Dana C. Ernst"},
		Home: "https://github.com/dcernst/IBL-IntroToProof", Repository: "https://github.com/dcernst/IBL-IntroToProof", Commit: "2315fb79ee07cd541dcb7de084d3ca9921dfae74",
		Licence: "CC-BY-SA-4.0", Attribution: "Dana C. Ernst, An Introduction to Proof via Inquiry-Based Learning, https://github.com/dcernst/IBL-IntroToProof, CC BY-SA 4.0",
		Format: LaTeX, Files: re(`^MAAPressVersion/[^/]*\.tex$`),
	},
	{
		ID: "austin_ula", Name: "Understanding Linear Algebra", Authors: []string{"David Austin"},
		Home: "https://understandinglinearalgebra.org", Repository: "https://github.com/davidaustinm/ula", Commit: "b6ea263ef23703072b7007485bb7a2d89c5f399b",
		Licence: "CC-BY-4.0", Attribution: "David Austin, Understanding Linear Algebra, https://understandinglinearalgebra.org, CC BY 4.0",
		Format: PreTeXt, Main: "src/ula.xml",
	},
	{
		ID: "active_calculus", Name: "Active Calculus", Authors: []string{"Matthew Boelkins", "David Austin", "Steven Schlicker"},
		Home: "https://activecalculus.org", Repository: "https://github.com/active-calculus/active-calculus-single-mbx", Commit: "e597daa657ba5ec0953157aa8c44834dceb63eb2",
		Licence: "CC-BY-SA-4.0", Attribution: "Matthew Boelkins, David Austin and Steven Schlicker, Active Calculus, https://activecalculus.org, CC BY-SA 4.0",
		Format: PreTeXt, Main: "source/main.ptx",
	},
}

func init() {
	for _, b := range Books {
		ingest.Register(b)
	}
}

// Source returns the catalogue entry of the book.
func (b Book) Source() ingest.Source {
	return ingest.Source{
		ID:          b.ID,
		Name:        b.Name,
		Authors:     b.Authors,
		Kind:        "textbook",
		Home:        b.Home,
		Licence:     b.Licence,
		Election:    b.Election,
		Class:       ingest.Open,
		Authored:    "human",
		Edition:     b.Edition,
		Channel:     "git",
		Attribution: b.Attribution,
	}
}

// Version returns the version of the adapter.
func (Book) Version() string { return "textbook/1" }

// Dump returns the git archive of the repository at the pinned commit.
func (b Book) Dump() string { return b.Repository + "/archive/" + b.Commit + ".tar.gz" }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit. It fails when the commit is not the pinned commit of the book.
func (b Book) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	if err := ingest.FetchArchive(ctx, req, w); err != nil {
		return err
	}
	if req.Commit != b.Commit {
		return fmt.Errorf("the archive is of the commit %s, and %s is pinned at %s. A person reads the licence at the new commit before the pin moves (corpus K5.4.4)", req.Commit, b.ID, b.Commit)
	}
	return nil
}

// item is one environment that a reader found, before Read gives it an id.
type item struct {
	file, env, kind, label, title, section, text string
	line                                         int
	refs                                         []string // the labels that the item refers to
}

// Read reads a git archive of the book and emits one record for each item, in the order of the files and of the items in each file. Then it emits one record of kind concept for each term of the back-of-book index, with the node of the table of contents that holds it (RS4.6). It fails when a file does not parse, when the main file of a PreTeXt book is not in the archive, and when the archive has no items. An item with no text is not read.
func (b Book) Read(r io.Reader, emit func(ingest.Record) error) error {
	files, order, commit, err := b.load(r)
	if err != nil {
		return err
	}
	var items []item
	switch b.Format {
	case LaTeX:
		items = readLaTeX(files, order, b.Files, b.Skip, b.Macros)
	case PreTeXt:
		items, err = readPreTeXt(files, b.Main)
	default:
		err = fmt.Errorf("the format %q is not known", b.Format)
	}
	if err != nil {
		return err
	}
	// An item with no text, such as an example that the author did not write yet, is not a record.
	items = slices.DeleteFunc(items, func(it item) bool { return it.text == "" })
	if len(items) == 0 {
		return errors.New("the archive has no definitions, statements, proofs, examples or exercises")
	}
	t, err := b.build(files, order, true)
	if err != nil {
		return err
	}
	for _, rec := range append(b.records(items, commit), b.termRecords(t.terms, commit)...) {
		if err := emit(rec); err != nil {
			return err
		}
	}
	return nil
}

// load reads the files of a git archive that the book wants, with their paths in the order of the archive and the commit of the archive.
func (b Book) load(r io.Reader) (files map[string][]byte, order []string, commit string, err error) {
	files = map[string][]byte{}
	commit, err = ingest.WalkArchive(r, func(name string, f io.Reader) error {
		if !b.wants(name) {
			return nil
		}
		data, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		files[name] = data
		order = append(order, name)
		return nil
	})
	return files, order, commit, err
}

// wants reports whether Read keeps a file of the archive: a source of the format, or for LaTeX a file that can define environments.
func (b Book) wants(name string) bool {
	switch ext := path.Ext(name); b.Format {
	case LaTeX:
		return ext == ".tex" || ext == ".sty" || ext == ".cls"
	case PreTeXt:
		return ext == ".ptx" || ext == ".xml"
	}
	return false
}

// records gives the ids, the proofs and the references of the items.
func (b Book) records(items []item, commit string) []ingest.Record {
	commit = cmpOr(commit, b.Commit)
	ids := make([]string, len(items))
	seen := map[string]int{}
	byLabel := map[string][]int{} // the items with a label, in the order of the book
	for i, it := range items {
		key := it.label
		if key == "" {
			seen[it.file+"\x00"+it.env]++
			key = it.env + "-" + strconv.Itoa(seen[it.file+"\x00"+it.env])
		} else {
			byLabel[it.label] = append(byLabel[it.label], i)
		}
		id := it.file + "#" + key
		if n := seen[id]; n > 0 {
			id += "-" + strconv.Itoa(n+1)
		}
		seen[id]++
		ids[i] = id
	}
	out := make([]ingest.Record, len(items))
	last := map[string]int{} // the last statement of each file
	for i, it := range items {
		rec := ingest.Record{
			ID:      ids[i],
			Kind:    it.kind,
			Title:   it.title,
			Locator: it.file + ":" + strconv.Itoa(it.line),
			URL:     b.blob(commit, it.file, it.line),
			Text:    it.text,
			Meta:    map[string]string{"environment": it.env, "label": it.label, "section": it.section},
		}
		proves := -1
		if j, ok := last[it.file]; ok && it.kind == "proof" {
			proves = j
			rec.Meta["proves"] = ids[j]
		}
		if it.kind == "statement" {
			last[it.file] = i
		}
		var uses []string
		for _, l := range it.refs {
			if j, ok := resolve(byLabel[l], items, it.file); ok && j != i && j != proves && !slices.Contains(uses, ids[j]) {
				uses = append(uses, ids[j])
			}
		}
		rec.Meta["uses"] = strings.Join(uses, "|")
		for k, v := range rec.Meta {
			if v == "" {
				delete(rec.Meta, k)
			}
		}
		out[i] = rec
	}
	return out
}

// resolve gives the item of a label: the one in the same file, else the only one in the book. A label that two files have and the file of the reference does not have is not resolved.
func resolve(cands []int, items []item, file string) (int, bool) {
	for _, j := range cands {
		if items[j].file == file {
			return j, true
		}
	}
	if len(cands) == 1 {
		return cands[0], true
	}
	return 0, false
}

// blob gives the URL of a line of a file at the commit, on GitHub.
func (b Book) blob(commit, file string, line int) string {
	return b.Repository + "/blob/" + commit + "/" + file + "#L" + strconv.Itoa(line)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// kindOf gives the kind of an item from the name of its environment, for example Theorem or Skeleton Proof. It tries the whole name in lower case, then its last word, then the last word without a final s. It gives "" for a name of another kind, such as Remark.
func kindOf(name string) string {
	name = strings.ToLower(strings.Join(strings.Fields(name), " "))
	if k, ok := kinds[name]; ok {
		return k
	}
	words := strings.Fields(name)
	if len(words) == 0 {
		return ""
	}
	w := words[len(words)-1]
	if k, ok := kinds[w]; ok {
		return k
	}
	return kinds[strings.TrimSuffix(w, "s")]
}

// kinds maps the name of an environment in lower case to its kind.
var kinds = map[string]string{
	"theorem": "statement", "lemma": "statement", "proposition": "statement", "corollary": "statement", "claim": "statement",
	"fact": "statement", "axiom": "statement", "principle": "statement", "identity": "statement", "law": "statement",
	"definition":  "definition",
	"conjecture":  "conjecture",
	"proof":       "proof",
	"example":     "example",
	"non-example": "counterexample", "counterexample": "counterexample",
	"exercise": "exercise", "problem": "exercise", "question": "exercise", "activity": "exercise",
}
