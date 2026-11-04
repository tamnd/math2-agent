// Package openlibrary is the adapter of Open Library (spec corpus resources RS4.1). It reads the monthly dumps of works and editions, one line for each record with five fields separated by tabs: the type, the key, the revision, the date of the last change and the record as JSON. It gives an ingest record and a source record for each work of mathematics and each edition of mathematics.
//
// A work is of mathematics when one of its LCC classes is in the class QA, one of its DDC numbers is in 510 to 519, or one of its subjects is a heading of mathematics. An edition is of mathematics when its own classes or subjects are, or when its work is a work of mathematics that came before it in the snapshot. The snapshot of Dump is the dump of the works and then the dump of the editions, so each work comes before its editions. A work gives a source record at the level work, and an edition gives a source record at the level manifestation with the key of its work, which joins the two in pass C5. The terms of the data of Open Library are not settled (RQ02), so the class is record.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/openlibrary"
package openlibrary

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
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

// The dumps of Open Library.
const (
	WorksDump    = "https://openlibrary.org/data/ol_dump_works_latest.txt.gz"
	EditionsDump = "https://openlibrary.org/data/ol_dump_editions_latest.txt.gz"
)

// Adapter is the adapter of Open Library.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of Open Library.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "openlibrary",
		Name:        "Open Library",
		Home:        "https://openlibrary.org",
		Licence:     "LicenseRef-unknown",
		Class:       ingest.RecordOnly,
		Lineage:     []string{"loc-marc"}, // Open Library took many of its records from the MARC records of the Library of Congress, so the two are one capture (RS11.4.3)
		Attribution: "Open Library, the Internet Archive, https://openlibrary.org",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "openlibrary/1" }

// Dump returns the URL of the dump of the works.
func (Adapter) Dump() string { return WorksDump }

// Fetch reads the dump at req.URL as a stream and writes the lines of the works and editions of mathematics, compressed with gzip. For the dump of the works, it reads the dump of the editions after it, so that each work of mathematics is known before its editions. The full dumps are too large to keep, and the snapshot keeps only the lines that Records reads. A line that is not a record of five fields, or whose JSON does not parse, is left out.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	urls := []string{req.URL}
	if req.URL == WorksDump {
		urls = append(urls, EditionsDump)
	}
	zw := gzip.NewWriter(w)
	works := map[string]bool{}
	for _, u := range urls {
		if err := keep(ctx, req.Get, u, works, zw); err != nil {
			return fmt.Errorf("%s: %w", u, err)
		}
	}
	return zw.Close()
}

// keep reads the dump at u, compressed with gzip, and writes each line of a work or an edition of mathematics to w. It adds the key of each work of mathematics to works.
func keep(ctx context.Context, get ingest.Getter, u string, works map[string]bool, w io.Writer) error {
	body, err := get(ctx, u)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	zr, err := gzip.NewReader(body)
	if err != nil {
		return err
	}
	sc := bufio.NewScanner(zr)
	sc.Buffer(nil, 64<<20)
	for n := 0; sc.Scan(); n++ {
		if n%100000 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		line := sc.Text()
		f := strings.SplitN(line, "\t", 5)
		if len(f) != 5 || (f[0] != "/type/work" && f[0] != "/type/edition") {
			continue
		}
		var d Doc
		if json.Unmarshal([]byte(f[4]), &d) != nil {
			continue
		}
		if d.Key == "" {
			d.Key = f[1]
		}
		if !math(f[0], &d, works) {
			continue
		}
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}
	return sc.Err()
}

// math reports whether a work or an edition is of mathematics. An edition is also of mathematics when one of its works is in works. It adds the key of a work of mathematics to works.
func math(typ string, d *Doc, works map[string]bool) bool {
	switch {
	case typ == "/type/work" && Math(d):
		works[d.Key] = true
		return true
	case typ == "/type/edition":
		return Math(d) || slices.ContainsFunc(d.Works, func(k keyRef) bool { return works[k.Key] })
	}
	return false
}

type keyRef struct {
	Key string `json:"key"`
}

// Doc is one work or edition of Open Library, with the fields that the adapter reads.
type Doc struct {
	Key       string   `json:"key"`
	Title     string   `json:"title"`
	Subtitle  string   `json:"subtitle"`
	Subjects  []string `json:"subjects"`
	LCC       []string `json:"lc_classifications"`
	DDC       []string `json:"dewey_number"`        // of a work
	DDCClass  []string `json:"dewey_decimal_class"` // of an edition
	FirstDate string   `json:"first_publish_date"`
	// The fields of an edition.
	Works       []keyRef `json:"works"`
	ISBN13      []string `json:"isbn_13"`
	ISBN10      []string `json:"isbn_10"`
	OCLC        []string `json:"oclc_numbers"`
	LCCN        []string `json:"lccn"`
	Publishers  []string `json:"publishers"`
	PublishDate string   `json:"publish_date"`
	Languages   []keyRef `json:"languages"`
	Edition     string   `json:"edition_name"`
}

// headings are the first words of the subjects of mathematics.
var headings = map[string]bool{
	"mathematics": true, "algebra": true, "geometry": true, "arithmetic": true, "calculus": true, "trigonometry": true, "number theory": true,
	"probabilities": true, "mathematical analysis": true, "differential equations": true, "functions": true, "topology": true, "combinatorial analysis": true,
	"mathematical statistics": true, "set theory": true, "logic, symbolic and mathematical": true, "group theory": true, "numerical analysis": true,
}

// Math reports whether the classes or the subjects of a work or an edition are of mathematics. An LCC class in QA75.5 to QA76.95, computer science, is not.
func Math(d *Doc) bool {
	for _, c := range d.LCC {
		c = strings.ToUpper(strings.ReplaceAll(fetch.LCC(c), " ", ""))
		if rest, ok := strings.CutPrefix(c, "QA"); ok && rest != "" && rest[0] >= '0' && rest[0] <= '9' && !computing(rest) {
			return true
		}
	}
	for _, c := range slices.Concat(d.DDC, d.DDCClass) {
		if c = strings.TrimSpace(c); len(c) >= 3 && c[:2] == "51" && c[2] >= '0' && c[2] <= '9' {
			return true
		}
	}
	for _, s := range d.Subjects {
		head, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(s)), " -- ")
		if headings[strings.TrimSuffix(head, ".")] {
			return true
		}
	}
	return false
}

// computing reports whether the number of an LCC class in QA is in 75.5 to 76.95.
func computing(n string) bool {
	end := strings.IndexFunc(n, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if end < 0 {
		end = len(n)
	}
	var x float64
	if _, err := fmt.Sscanf(strings.TrimSuffix(n[:end], "."), "%g", &x); err != nil {
		return false
	}
	return x >= 75.5 && x <= 76.95
}

// each calls fn for each work and edition of mathematics of the snapshot, in the order of the snapshot.
func each(r io.Reader, fn func(typ string, d *Doc) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 64<<20)
	works := map[string]bool{}
	for line := 1; sc.Scan(); line++ {
		f := strings.SplitN(sc.Text(), "\t", 5)
		if len(f) == 1 && strings.TrimSpace(f[0]) == "" {
			continue
		}
		if len(f) != 5 {
			return fmt.Errorf("line %d: want 5 fields separated by tabs, got %d", line, len(f))
		}
		typ := f[0]
		if typ != "/type/work" && typ != "/type/edition" {
			continue
		}
		var d Doc
		if err := json.Unmarshal([]byte(f[4]), &d); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if d.Key == "" {
			d.Key = f[1]
		}
		if !math(typ, &d, works) {
			continue
		}
		if err := fn(typ, &d); err != nil {
			return err
		}
	}
	return sc.Err()
}

var yearRE = regexp.MustCompile(`\b(1[4-9][0-9]{2}|20[0-9]{2})\b`)

// year gives the year of a date of Open Library, which is free text as March 1990 or 1990, or an empty string.
func year(s string) string { return yearRE.FindString(s) }

func short(key string) string { return key[strings.LastIndex(key, "/")+1:] }

func title(d *Doc) string {
	t := d.Title
	if d.Subtitle != "" && t != "" {
		t += ": " + d.Subtitle
	}
	return strings.Join(strings.Fields(t), " ")
}

// Read reads the dumps and emits one ingest record for each work and edition of mathematics.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(typ string, d *Doc) error {
		kind := "work"
		if typ == "/type/edition" {
			kind = "edition"
		}
		return emit(ingest.Record{ID: short(d.Key), Kind: kind, Title: title(d), URL: "https://openlibrary.org" + d.Key})
	})
}

// Records reads the dumps and emits one source record for each work and edition of mathematics (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(typ string, d *Doc) error { return emit(SourceRecord(typ, d)) })
}

// SourceRecord makes the source record of a work or an edition. The kind is monograph, because the dumps do not tell a textbook from a monograph. The date is the year, because the dates of Open Library are free text. The creators are left out, because the dumps name an author by a key of the dump of the authors.
func SourceRecord(typ string, d *Doc) cluster.Record {
	rec := cluster.Record{
		NativeID: short(d.Key),
		Level:    "work",
		Kind:     "monograph",
		Title:    title(d),
		Date:     year(d.FirstDate),
		IDs:      resource.IDs{},
		Links:    []string{"https://openlibrary.org" + d.Key},
	}
	add := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	for _, c := range d.LCC {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "lcc", Code: fetch.LCC(c)})
	}
	for _, c := range slices.Concat(d.DDC, d.DDCClass) {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "ddc", Code: strings.TrimSpace(c)})
	}
	if typ == "/type/work" {
		add("openlibrary_work", d.Key)
		return rec
	}
	rec.Level = "manifestation"
	rec.Date = year(d.PublishDate)
	rec.Edition = d.Edition
	add("openlibrary_edition", d.Key)
	for _, w := range d.Works {
		add("openlibrary_work", w.Key)
	}
	add("isbn", slices.Concat(d.ISBN13, d.ISBN10)...)
	add("oclc", d.OCLC...)
	add("lccn", d.LCCN...)
	if len(d.Publishers) > 0 {
		rec.Publisher = d.Publishers[0]
	}
	if len(d.Languages) > 0 {
		rec.Language = fetch.Lang(short(d.Languages[0].Key))
	}
	return rec
}
