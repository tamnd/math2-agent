// Package gutenberg is the adapter of Project Gutenberg (spec corpus resources RS4.1 and RS8.3, phase R2). It reads the catalogue file of Project Gutenberg, keeps the texts of mathematics, and gives an ingest record and a source record for each text.
//
// Fetch reads the catalogue file pg_catalog.csv and writes one JSON object for each text of mathematics and each line, compressed with gzip. A text of mathematics has the LCC class QA, or a subject or a bookshelf of mathematics. The texts of Project Gutenberg are in the public domain in the United States, and the catalogue is free to use. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/gutenberg"
package gutenberg

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Catalogue is the URL of the catalogue file.
const Catalogue = "https://www.gutenberg.org/cache/epub/feeds/pg_catalog.csv"

// Adapter is the adapter of Project Gutenberg.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of Project Gutenberg.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "gutenberg",
		Name:        "Project Gutenberg",
		Home:        "https://www.gutenberg.org",
		Licence:     "LicenseRef-public-domain",
		Class:       ingest.RecordOnly,
		Attribution: "Project Gutenberg, https://www.gutenberg.org",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "gutenberg/1" }

// Dump returns the URL of the catalogue file.
func (Adapter) Dump() string { return Catalogue }

// Text is one text of the catalogue, as one line of the snapshot.
type Text struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Issued      string   `json:"issued,omitempty"`
	Languages   []string `json:"languages,omitempty"`
	Authors     []string `json:"authors,omitempty"`
	Subjects    []string `json:"subjects,omitempty"`
	LCC         []string `json:"lcc,omitempty"`
	Bookshelves []string `json:"bookshelves,omitempty"`
}

// Columns are the columns of the catalogue file that Fetch reads.
var Columns = []string{"Text#", "Type", "Issued", "Title", "Language", "Authors", "Subjects", "LoCC", "Bookshelves"}

// Fetch reads the catalogue file at req.URL and writes the texts of mathematics as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	body, err := req.Get(ctx, req.URL)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	rd := csv.NewReader(bufio.NewReader(body))
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	head, err := rd.Read()
	if err != nil {
		return fmt.Errorf("gutenberg: the catalogue has no header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF"))] = i
	}
	for _, h := range Columns {
		if _, ok := col[h]; !ok {
			return fmt.Errorf("gutenberg: the catalogue has no column %s", h)
		}
	}
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	for {
		row, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("gutenberg: %w", err)
		}
		get := func(h string) string {
			if i := col[h]; i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		if get("Type") != "Text" {
			continue
		}
		t := Text{ID: get("Text#"), Title: strings.Join(strings.Fields(get("Title")), " "), Issued: get("Issued"), Languages: split(get("Language")), Authors: split(get("Authors")),
			Subjects: split(get("Subjects")), LCC: split(get("LoCC")), Bookshelves: split(get("Bookshelves"))}
		if t.ID == "" || !Math(t) {
			continue
		}
		if err := enc.Encode(t); err != nil {
			return err
		}
	}
	return zw.Close()
}

func split(s string) []string {
	var vs []string
	for _, v := range strings.Split(s, ";") {
		if v = strings.TrimSpace(v); v != "" {
			vs = append(vs, v)
		}
	}
	return vs
}

// Math reports whether a text is a text of mathematics: it has the LCC class QA, or a subject or a bookshelf of mathematics.
func Math(t Text) bool {
	if slices.ContainsFunc(t.LCC, func(c string) bool { return strings.HasPrefix(c, "QA") }) {
		return true
	}
	return fetch.MathHeading(append(slices.Clone(t.Subjects), t.Bookshelves...))
}

func each(r io.Reader, fn func(Text) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var t Text
		if err := json.Unmarshal(sc.Bytes(), &t); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if t.ID == "" {
			return fmt.Errorf("line %d: the text has no id", line)
		}
		if err := fn(t); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Read reads the texts and emits one ingest record for each text.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(t Text) error {
		return emit(ingest.Record{ID: t.ID, Kind: "monograph", Title: t.Title, URL: "https://www.gutenberg.org/ebooks/" + t.ID})
	})
}

// Records reads the texts and emits one source record for each text (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(t Text) error { return emit(SourceRecord(t)) })
}

// role finds the role of a creator in the catalogue, as [Translator].
var role = regexp.MustCompile(`\s*\[([A-Za-z ]+)\]\s*$`)

// dates finds the dates of a creator at the end of a name, as , 1851-1916 or , 1801?-.
var dates = regexp.MustCompile(`,\s*(?:active\s+)?(?:[0-9]{1,4}\??\s*(?:BCE|CE|AD)?\s*-\s*(?:[0-9]{1,4}\??\s*(?:BCE|CE|AD)?)?|[0-9]{1,4}\??\s*(?:BCE|CE|AD)?\s*-|-[0-9]{1,4})\s*$`)

// Creator parses a name of the catalogue, as Thompson, Silvanus P. (Silvanus Phillips), 1851-1916 [Editor]. The full form of the given names in parentheses goes.
func Creator(s string) resource.Creator {
	c := resource.Creator{Role: "author"}
	if m := role.FindStringSubmatch(s); m != nil {
		switch strings.ToLower(m[1]) {
		case "editor":
			c.Role = "editor"
		case "translator":
			c.Role = "translator"
		default:
			c.Role = "contributor"
		}
		s = s[:len(s)-len(m[0])]
	}
	s = dates.ReplaceAllString(s, "")
	if i := strings.Index(s, " ("); i > 0 && strings.HasSuffix(s, ")") {
		s = s[:i]
	}
	c.Name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), ","))
	return c
}

// SourceRecord makes the source record of one text. A text of Project Gutenberg is a manifestation of a book, and Issued is the date of the text in Project Gutenberg, not the date of the book, so the record has no date.
func SourceRecord(t Text) cluster.Record {
	rec := cluster.Record{NativeID: t.ID, Level: "manifestation", Kind: "monograph", Title: t.Title, Publisher: "Project Gutenberg", IDs: resource.IDs{},
		Links: []string{"https://www.gutenberg.org/ebooks/" + t.ID}}
	if n, err := ids.Normalize("gutenberg", t.ID); err == nil {
		rec.IDs["gutenberg"] = []string{n}
	}
	if len(t.Languages) > 0 {
		rec.Language = fetch.Lang(t.Languages[0])
	}
	for _, a := range t.Authors {
		if c := Creator(a); c.Name != "" {
			rec.Creators = append(rec.Creators, c)
		}
	}
	for _, c := range t.LCC {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "lcc", Code: c})
	}
	return rec
}
