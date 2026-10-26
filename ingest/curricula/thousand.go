package curricula

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/crosswalk"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/msc"
)

// ThousandHome is the site of the 1000+ theorems project.
const ThousandHome = "https://1000-plus.github.io"

// ThousandRepository is the git repository of the site.
const ThousandRepository = "https://github.com/1000-plus/1000-plus.github.io"

// ThousandArchive is the git archive of the main branch of the site.
const ThousandArchive = ThousandRepository + "/archive/refs/heads/main.tar.gz"

// Systems are the formal systems of the 1000+ theorems, as the keys of an entry name them.
var Systems = []string{"hol_light", "isabelle", "lean", "metamath", "mizar", "rocq"}

// Thousand is the adapter of the 1000+ theorems project. It gives one record for each file _thm/<id>.md of the site. The project takes its entries from the Wikipedia list of theorems, and the key of an entry is its Wikidata item, with a letter after it when two entries have the same item.
type Thousand struct{}

func init() { ingest.Register(Thousand{}) }

// Source returns the catalogue entry of the project.
func (Thousand) Source() ingest.Source {
	return ingest.Source{
		ID:          "thousand_plus",
		Name:        "1000+ theorems",
		Kind:        "curriculum",
		Home:        ThousandHome,
		Licence:     "Unlicense",
		Class:       ingest.Open,
		Attribution: "The contributors of the 1000+ theorems project, " + ThousandRepository + ", the Unlicense",
	}
}

// Version returns the version of the adapter.
func (Thousand) Version() string { return "thousand_plus/1" }

// Dump returns the git archive of the main branch.
func (Thousand) Dump() string { return ThousandArchive }

// Fetch gets the git archive at req.URL and sets req.Commit to its commit.
func (Thousand) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	return ingest.FetchArchive(ctx, req, w)
}

// theorem is the front matter of one entry.
type theorem struct {
	Wikidata  string   `yaml:"wikidata"`
	Suffix    string   `yaml:"id_suffix"`
	MSC       string   `yaml:"msc_classification"`
	Wikipedia []string `yaml:"wikipedia_links"`
	HOLLight  []formal `yaml:"hol_light"`
	Isabelle  []formal `yaml:"isabelle"`
	Lean      []formal `yaml:"lean"`
	Metamath  []formal `yaml:"metamath"`
	Mizar     []formal `yaml:"mizar"`
	Rocq      []formal `yaml:"rocq"`
}

// formal is one formalization of a theorem in one system.
type formal struct {
	Status      string   `yaml:"status"`
	Identifiers []string `yaml:"identifiers"`
}

var (
	qid        = regexp.MustCompile(`^Q[1-9][0-9]*$`)
	suffix     = regexp.MustCompile(`^[A-Z]?$`)
	mscClass   = regexp.MustCompile(`^[0-9]{2}$`)
	wikiLink   = regexp.MustCompile(`^\[\[([^\[\]|]+)(?:\|[^\[\]]*)?\]\]`)
	titleLine  = regexp.MustCompile(`(?m)^# +(.+)$`)
	frontDelim = []byte("---")
)

// Read reads a git archive of the site and emits one record for each entry. Read fails on an entry whose front matter does not parse, whose key is not the name of its file, or whose status is not formalized or statement, and on an archive with no entries.
func (Thousand) Read(r io.Reader, emit func(ingest.Record) error) error {
	n := 0
	_, err := ingest.WalkArchive(r, func(name string, f io.Reader) error {
		dir, file := path.Split(name)
		if dir != "_thm/" || !strings.HasSuffix(file, ".md") {
			return nil
		}
		b, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		rec, err := entryRecord(name, b)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		n++
		return emit(rec)
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("the archive has no entries in _thm/")
	}
	return nil
}

// entryRecord makes the record of the entry in the file name with the bytes b.
func entryRecord(name string, b []byte) (ingest.Record, error) {
	front, ok := frontMatter(b)
	if !ok {
		return ingest.Record{}, fmt.Errorf("the file has no front matter between --- lines")
	}
	var t theorem
	if err := yaml.Unmarshal(front, &t); err != nil {
		return ingest.Record{}, err
	}
	id := t.Wikidata + t.Suffix
	switch {
	case !qid.MatchString(t.Wikidata):
		return ingest.Record{}, fmt.Errorf("the wikidata key %q is not a QID", t.Wikidata)
	case !suffix.MatchString(t.Suffix):
		return ingest.Record{}, fmt.Errorf("the id_suffix %q is not one capital letter", t.Suffix)
	case path.Base(name) != id+".md":
		return ingest.Record{}, fmt.Errorf("the entry has the id %s, and the file name is not %s.md", id, id)
	}
	tm := titleLine.FindSubmatch(front)
	if tm == nil {
		return ingest.Record{}, fmt.Errorf("the entry has no title line # <title>")
	}
	m := map[string]string{"wikidata": t.Wikidata}
	if t.MSC != "" {
		code := t.MSC + "-XX"
		if !mscClass.MatchString(t.MSC) || !msc.Valid(code) {
			return ingest.Record{}, fmt.Errorf("the msc_classification %q is not two digits", t.MSC)
		}
		m["msc"] = code
	}
	var pages []string
	for _, l := range t.Wikipedia {
		// A link is [[page]] or [[page|text]], with some words after it in a few entries, or the title of the page alone.
		page := strings.TrimSpace(l)
		if lm := wikiLink.FindStringSubmatch(page); lm != nil {
			page = lm[1]
		}
		// A link to a section names a page about something larger, for example the theorem of Lamé in the page on the Euclidean algorithm, and a link to a disambiguation page names a list of pages. Neither page is the theorem, so the record does not give them.
		page = strings.Join(strings.Fields(strings.ReplaceAll(page, "_", " ")), " ")
		if strings.Contains(page, "#") || strings.HasSuffix(page, "(disambiguation)") || crosswalk.Check("wikipedia", page) != nil {
			continue
		}
		pages = append(pages, page)
	}
	m["wikipedia"] = strings.Join(slices.Compact(pages), "|")
	var proved, stated, decls []string
	for i, fs := range [][]formal{t.HOLLight, t.Isabelle, t.Lean, t.Metamath, t.Mizar, t.Rocq} {
		sys := Systems[i]
		has := map[string]bool{}
		for _, f := range fs {
			if f.Status != "formalized" && f.Status != "statement" {
				return ingest.Record{}, fmt.Errorf("the %s status %q is not formalized or statement", sys, f.Status)
			}
			has[f.Status] = true
			if sys == "lean" {
				decls = append(decls, f.Identifiers...)
			}
		}
		switch {
		case has["formalized"]:
			proved = append(proved, sys)
		case has["statement"]:
			stated = append(stated, sys)
		}
	}
	m["formalized"] = strings.Join(proved, "|")
	m["stated"] = strings.Join(stated, "|")
	m["mathlib"] = strings.Join(decls, "|")
	return ingest.Record{
		ID:      id,
		Kind:    "statement",
		Title:   clean(string(tm[1])),
		URL:     ThousandHome + "/all?length=-1#" + t.Wikidata,
		Locator: name,
		Meta:    meta(m),
	}, nil
}

// frontMatter returns the bytes between the first two lines that are ---.
func frontMatter(b []byte) ([]byte, bool) {
	lines := bytes.SplitAfter(b, []byte("\n"))
	start := -1
	off := 0
	for _, l := range lines {
		if bytes.Equal(bytes.TrimSpace(l), frontDelim) {
			if start >= 0 {
				return b[start:off], true
			}
			start = off + len(l)
		}
		off += len(l)
	}
	return nil, false
}
