// Package doab is the adapter of DOAB, the Directory of Open Access Books, and of the OAPEN Library (spec corpus resources RS4.1 and RS8.3, phase R2). The two sources run the same DSpace platform and give the same CSV export, so one package serves both. It keeps the books of mathematics, and it gives an ingest record and a source record for each book.
//
// Fetch reads the CSV export of the whole repository and writes one JSON object for each book of mathematics and each line, compressed with gzip. A book of mathematics has the Thema or the BIC class PB, or a subject heading of mathematics. The metadata of DOAB and OAPEN is under CC0 (RQ05), and each book has its own licence. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/doab"
package doab

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

// Adapter is the adapter of one DSpace repository of open-access books.
type Adapter struct {
	ID     string // the source id
	Name   string
	Home   string
	Export string // the URL of the CSV export
}

// DOAB is the adapter of the Directory of Open Access Books.
var DOAB = Adapter{ID: "doab", Name: "DOAB, the Directory of Open Access Books", Home: "https://directory.doabooks.org", Export: "https://directory.doabooks.org/download-export?format=csv"}

// OAPEN is the adapter of the OAPEN Library.
var OAPEN = Adapter{ID: "oapen", Name: "The OAPEN Library", Home: "https://library.oapen.org", Export: "https://library.oapen.org/download-export?format=csv"}

func init() {
	ingest.Register(DOAB)
	ingest.Register(OAPEN)
}

// Source returns the catalogue entry of the repository.
func (a Adapter) Source() ingest.Source {
	return ingest.Source{ID: a.ID, Name: a.Name, Home: a.Home, Licence: "CC0-1.0", Class: ingest.RecordOnly, Attribution: a.Name + ", " + a.Home}
}

// Version returns the version of the adapter.
func (a Adapter) Version() string { return a.ID + "/1" }

// Dump returns the URL of the CSV export.
func (a Adapter) Dump() string { return a.Export }

// Book is one book or chapter of the export, as one line of the snapshot.
type Book struct {
	ID        string   `json:"id"`
	Type      string   `json:"type,omitempty"`
	Title     string   `json:"title"`
	Authors   []string `json:"authors,omitempty"`
	Editors   []string `json:"editors,omitempty"`
	Date      string   `json:"date,omitempty"`
	Language  []string `json:"language,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	Series    string   `json:"series,omitempty"`
	Number    string   `json:"number,omitempty"`
	ISBN      []string `json:"isbn,omitempty"`
	DOI       string   `json:"doi,omitempty"`
	OCLC      []string `json:"oclc,omitempty"`
	Handle    string   `json:"handle,omitempty"`
	Classes   []string `json:"classes,omitempty"`  // as thema PBT or bic PB
	Headings  []string `json:"headings,omitempty"` // the free subjects
	Licence   string   `json:"licence,omitempty"`
}

// Required are the columns of the export that Fetch needs.
var Required = []string{"id", "dc.title", "dc.type", "dc.subject.classification"}

// code finds the code at the start of one level of a class path, as PBT in PBT Probability and statistics.
var code = regexp.MustCompile(`^([0-9A-Z][0-9A-Z.]*)\s`)

// Classes parses the class paths of the export, as thema EDItEUR::P Mathematics and Science::PB Mathematics::PBT Probability and statistics, and gives each as its scheme and its last code, as thema PBT.
func Classes(vs []string) []string {
	var out []string
	for _, v := range vs {
		parts := strings.Split(v, "::")
		scheme, _, _ := strings.Cut(strings.TrimSpace(parts[0]), " ")
		last := ""
		for _, p := range parts[1:] {
			if m := code.FindStringSubmatch(strings.TrimSpace(p) + " "); m != nil {
				last = m[1]
			}
		}
		if last != "" {
			out = append(out, strings.ToLower(scheme)+" "+last)
		}
	}
	return out
}

// Math reports whether a book is a book of mathematics: it has the Thema or the BIC class PB, or a subject heading of mathematics.
func Math(b Book) bool {
	for _, c := range b.Classes {
		if _, cd, _ := strings.Cut(c, " "); strings.HasPrefix(cd, "PB") {
			return true
		}
	}
	return fetch.MathHeading(b.Headings)
}

// Fetch reads the CSV export at req.URL and writes the books of mathematics as JSON Lines, compressed with gzip.
func (a Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	body, err := req.Get(ctx, req.URL)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	rd := csv.NewReader(bufio.NewReaderSize(body, 1<<20))
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	rd.ReuseRecord = true
	head, err := rd.Read()
	if err != nil {
		return fmt.Errorf("%s: the export has no header: %w", a.ID, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF"))] = i
	}
	for _, h := range Required {
		if _, ok := col[h]; !ok {
			return fmt.Errorf("%s: the export has no column %s", a.ID, h)
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
			return fmt.Errorf("%s: %w", a.ID, err)
		}
		get := func(h string) string {
			if i, ok := col[h]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		many := func(hs ...string) []string {
			var vs []string
			for _, h := range hs {
				for _, v := range strings.Split(get(h), "||") {
					if v = strings.TrimSpace(v); v != "" && !slices.Contains(vs, v) {
						vs = append(vs, v)
					}
				}
			}
			return vs
		}
		b := Book{ID: get("id"), Type: strings.ToLower(get("dc.type")), Title: strings.Join(strings.Fields(get("dc.title")), " "), Authors: many("dc.contributor.author"), Editors: many("dc.contributor.editor"),
			Date: get("dc.date.issued"), Language: many("dc.language"), Publisher: first(many("dc.publisher", "publisher.name")), Series: first(many("dc.relation.ispartofseries")), Number: first(many("oapen.series.number", "dc.seriesnumber")),
			ISBN: many("dc.identifier.isbn", "oapen.identifier.isbn", "BITSTREAM ISBN"), DOI: first(many("oapen.identifier.doi")), Classes: Classes(many("dc.subject.classification")),
			Headings: many("dc.subject.other"), Licence: first(many("BITSTREAM License", "dc.rights.uri"))}
		for _, v := range many("dc.identifier") {
			if n, ok := strings.CutPrefix(v, "OCN:"); ok {
				b.OCLC = append(b.OCLC, strings.TrimSpace(n))
			}
		}
		for _, u := range many("dc.identifier.uri") {
			if _, h, ok := strings.Cut(u, "/handle/"); ok {
				b.Handle = h
				break
			}
		}
		if b.ID == "" || b.Title == "" || !Math(b) {
			continue
		}
		if err := enc.Encode(b); err != nil {
			return err
		}
	}
	return zw.Close()
}

func first(vs []string) string {
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

func each(r io.Reader, fn func(Book) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 64<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var b Book
		if err := json.Unmarshal(sc.Bytes(), &b); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if b.ID == "" {
			return fmt.Errorf("line %d: the book has no id", line)
		}
		if err := fn(b); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (a Adapter) link(b Book) string {
	if b.Handle != "" {
		return a.Home + "/handle/" + b.Handle
	}
	return a.Home + "/items/" + b.ID
}

// Read reads the books and emits one ingest record for each book.
func (a Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(b Book) error {
		return emit(ingest.Record{ID: b.ID, Kind: Kind(b), Title: b.Title, URL: a.link(b)})
	})
}

// Records reads the books and emits one source record for each book (RS13.1).
func (a Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(b Book) error { return emit(a.SourceRecord(b)) })
}

// Kind gives the kind of a book: a chapter, an edited volume when the book has editors and no authors, else a monograph.
func Kind(b Book) string {
	switch {
	case b.Type == "chapter":
		return "chapter"
	case len(b.Authors) == 0 && len(b.Editors) > 0:
		return "edited-volume"
	}
	return "monograph"
}

// language finds the code in a language of the export, as eng in English[eng].
var language = regexp.MustCompile(`\[([a-z]{2,3})\]\s*$`)

// SourceRecord makes the source record of one book. A record of DOAB or OAPEN describes one open edition, so its level is manifestation.
func (a Adapter) SourceRecord(b Book) cluster.Record {
	rec := cluster.Record{NativeID: b.ID, Level: "manifestation", Kind: Kind(b), Title: b.Title, Date: b.Date, Publisher: b.Publisher, IDs: resource.IDs{}, Links: []string{a.link(b)}}
	if len(b.Language) > 0 {
		l := b.Language[0]
		if m := language.FindStringSubmatch(l); m != nil {
			l = m[1]
		}
		rec.Language = fetch.Lang(l)
	}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("doi", b.DOI)
	put("isbn", b.ISBN...)
	put("oclc", b.OCLC...)
	if b.Handle != "" {
		put("handle", b.Handle)
	}
	for _, n := range b.Authors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: n, Role: "author"})
	}
	for _, n := range b.Editors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: n, Role: "editor"})
	}
	for _, c := range b.Classes {
		s, cd, _ := strings.Cut(c, " ")
		rec.Subjects = append(rec.Subjects, resource.Subject{System: s, Code: cd})
	}
	if b.Series != "" {
		rec.Volume = b.Number
	}
	return rec
}
