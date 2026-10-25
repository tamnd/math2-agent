// Package formalconjectures is the ingest adapter of formal-conjectures, the Lean statements of open and solved problems by Google DeepMind and its contributors (spec 09.2). It gives one record for each theorem with a category attribute in the files under FormalConjectures/, except the statements of the category test.
//
// The bulk channel is the git archive of the main branch, and the catalogue records its commit. The adapter reads the Lean source as text. It does not build the project, because the project pins its own Mathlib. Thus the adapter follows the form of the files of the repository: each declaration starts a line, and its attributes and docstring come before it.
//
// The id of a record is the full name of the theorem, for example Erdos1.erdos_1, and the kind is statement. The text is the docstring, which states the problem in words. The URL is the line of the theorem at the commit of the snapshot. The meta of a record has these keys:
//
//	category      research open, research solved or textbook
//	ams           the AMS subject classes, with a space between them
//	question      yes, or value for a question that asks for a value
//	formal_proof  the link to a formal proof, for a solved problem
//	statement     the Lean statement after the name, on one line, for example ": 1 = 1"
//	module        the Lean module of the file
//	title         the title of the module docstring, for example Erdős Problem 1
//	references    the links in the module docstring, with a space between them
//	erdos         the numbers of the Erdős problems that the links name
//	oeis          the OEIS sequences that the links name
//	mathoverflow  the MathOverflow questions that the links name
//	wikipedia     the Wikipedia articles that the links name, with "|" between them
//	posed_by      the person who posed the problem, when the directory of the file names one (rule G06)
//
// A key is not in the meta when the file has no value for it. The repository is under the Apache License 2.0.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/ingest/formalconjectures"
package formalconjectures

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
)

// Repository is the git repository of formal-conjectures.
const Repository = "https://github.com/google-deepmind/formal-conjectures"

// Archive is the git archive of the main branch of the repository.
const Archive = Repository + "/archive/refs/heads/main.tar.gz"

// Dir is the directory of the statements in the repository.
const Dir = "FormalConjectures/"

// posers gives the person who posed the problems of a directory under Dir.
var posers = map[string]string{
	"ErdosProblems":   "Paul Erdős",
	"HilbertProblems": "David Hilbert",
}

// Adapter is the adapter of formal-conjectures.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of formal-conjectures.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "formal_conjectures",
		Name:        "Formal Conjectures",
		Home:        Repository,
		Licence:     "Apache-2.0",
		Class:       ingest.Open,
		Attribution: "The Formal Conjectures Authors, " + Repository + ", Apache License 2.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "formal_conjectures/1" }

// Dump returns the git archive of the main branch.
func (Adapter) Dump() string { return Archive }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	return ingest.FetchArchive(ctx, req, w)
}

// Read reads a git archive of the repository and emits one record for each statement. Read fails when the archive has no commit, when two statements have the same name and when the archive has no statements.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	var recs []ingest.Record
	seen := map[string]string{}
	commit, err := ingest.WalkArchive(r, func(name string, f io.Reader) error {
		if !strings.HasPrefix(name, Dir) || path.Ext(name) != ".lean" {
			return nil
		}
		src, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		for _, rec := range records(name, string(src)) {
			if first, dup := seen[rec.ID]; dup {
				return fmt.Errorf("%s and %s both have the statement %s", first, rec.Locator, rec.ID)
			}
			seen[rec.ID] = rec.Locator
			recs = append(recs, rec)
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
		return errors.New("the archive has no statements")
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

var (
	category    = regexp.MustCompile(`\bcategory\s+(research\s+open|research\s+solved|textbook|test)\b`)
	ams         = regexp.MustCompile(`\bAMS((?:\s+[0-9]+)+)`)
	question    = regexp.MustCompile(`\bquestion(\s+value)?\b`)
	formalProof = regexp.MustCompile(`\bformal_proof\s+using\s+\w+\s+at\s+"([^"]+)"`)
	link        = regexp.MustCompile(`https?://[^\s)\]>"]+`)
	erdos       = regexp.MustCompile(`^https?://(?:www\.)?erdosproblems\.com/(?:forum/thread/)?([0-9]+)$`)
	oeis        = regexp.MustCompile(`^https?://oeis\.org/(A[0-9]{6})$`)
	mo          = regexp.MustCompile(`^https?://mathoverflow\.net/(?:questions|q|a)/([0-9]+)`)
	wikipedia   = regexp.MustCompile(`^https?://en\.wikipedia\.org/wiki/([^#?]+)`)
)

// records gives the records of the statements of one Lean file. The locator of a record is the path and the line, and the URL is set later.
func records(name, src string) []ingest.Record {
	f := scan(src)
	module := strings.ReplaceAll(strings.TrimSuffix(name, ".lean"), "/", ".")
	shared := map[string]string{"module": module}
	for _, l := range strings.Split(f.moduleDoc, "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(l), "# "); ok {
			shared["title"] = strings.TrimSpace(t)
			break
		}
	}
	refs := links(f.moduleDoc)
	ids := map[string][]string{}
	for _, u := range refs {
		for key, re := range map[string]*regexp.Regexp{"erdos": erdos, "oeis": oeis, "mathoverflow": mo, "wikipedia": wikipedia} {
			if m := re.FindStringSubmatch(u); m != nil {
				id := m[1]
				if key == "wikipedia" {
					if t, err := url.PathUnescape(id); err == nil {
						id = t
					}
				}
				if !slices.Contains(ids[key], id) {
					ids[key] = append(ids[key], id)
				}
			}
		}
	}
	if len(refs) > 0 {
		shared["references"] = strings.Join(refs, " ")
	}
	for key, vs := range ids {
		sep := " "
		if key == "wikipedia" {
			sep = "|"
		}
		shared[key] = strings.Join(vs, sep)
	}
	dir, _, _ := strings.Cut(strings.TrimPrefix(name, Dir), "/")
	if p, ok := posers[dir]; ok {
		shared["posed_by"] = p
	}

	var out []ingest.Record
	for _, d := range f.decls {
		m := category.FindStringSubmatch(d.attrs)
		if m == nil {
			continue
		}
		cat := strings.Join(strings.Fields(m[1]), " ")
		if cat == "test" {
			continue
		}
		meta := map[string]string{"category": cat}
		for k, v := range shared {
			meta[k] = v
		}
		if m := ams.FindStringSubmatch(d.attrs); m != nil {
			meta["ams"] = strings.Join(strings.Fields(m[1]), " ")
		}
		if m := question.FindStringSubmatch(d.attrs); m != nil {
			meta["question"] = "yes"
			if m[1] != "" {
				meta["question"] = "value"
			}
		}
		if m := formalProof.FindStringSubmatch(d.attrs); m != nil {
			meta["formal_proof"] = m[1]
		}
		if d.statement != "" {
			meta["statement"] = d.statement
		}
		out = append(out, ingest.Record{
			ID:      d.name,
			Kind:    "statement",
			Title:   shared["title"],
			Locator: name + ":" + strconv.Itoa(d.line),
			Text:    d.doc,
			Meta:    meta,
		})
	}
	return out
}

// links returns the links in a text, in order, each one time.
func links(text string) []string {
	var out []string
	for _, u := range link.FindAllString(text, -1) {
		u = strings.TrimRight(u, ".,;:")
		if !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}
