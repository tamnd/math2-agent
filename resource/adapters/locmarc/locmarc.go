// Package locmarc is the adapter of the MARC records of the Library of Congress (spec corpus resources RS4.1 and RS8.3, phase R2). It reads the books of mathematics in the MDSConnect files, and it gives an ingest record and a source record for each book.
//
// Fetch reads the page of the Books All files of MDSConnect, takes the parts of the newest year in MARC 21 with UTF-8, and reads each part as a stream. It keeps the records with the LCC class QA1 to QA939 without QA75.5 to QA76.95, or the DDC class 510 to 519, as the test S3 of RS9.2 does. It writes one JSON object for each book and each line, compressed with gzip. The snapshot keeps the contents note (505), because RS4.3 takes the tables of contents from it. The records of the Library of Congress are US government works, and the FAQ of MDSConnect asks for credit to the Library of Congress (RQ03). The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/locmarc"
package locmarc

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/marc"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Page is the page of the Books All files of MDSConnect.
const Page = "https://www.loc.gov/cds/products/MDSConnect-books_all.html"

// Adapter is the adapter of the MARC records of the Library of Congress.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the Library of Congress.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "loc-marc",
		Name:        "Library of Congress, the MARC records of MDSConnect",
		Home:        "https://www.loc.gov/cds/products/marcDist.php",
		Licence:     "LicenseRef-us-government",
		Class:       ingest.RecordOnly,
		Attribution: "Library of Congress, MDSConnect, https://www.loc.gov/cds/",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "loc-marc/1" }

// Dump returns the page of the Books All files.
func (Adapter) Dump() string { return Page }

// Book is one book, as one line of the snapshot.
type Book = marc.Book

// parts finds the links to the parts in MARC 21 with UTF-8, as BooksAll.2016.part01.utf8.gz.
var parts = regexp.MustCompile(`href="([^"]*BooksAll\.([0-9]{4})\.part[0-9]+\.utf8\.gz)"`)

// Parts gives the URLs of the parts of the newest year on the page, in order.
func Parts(base, page string) ([]string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	byYear := map[string][]string{}
	newest := ""
	for _, m := range parts.FindAllStringSubmatch(page, -1) {
		u, err := b.Parse(m[1])
		if err != nil {
			return nil, err
		}
		if !slices.Contains(byYear[m[2]], u.String()) {
			byYear[m[2]] = append(byYear[m[2]], u.String())
		}
		newest = max(newest, m[2])
	}
	if newest == "" {
		return nil, fmt.Errorf("loc-marc: the page %s has no part in MARC 21 with UTF-8", base)
	}
	out := byYear[newest]
	slices.Sort(out)
	return out, nil
}

// Fetch reads the page at req.URL, reads each part of the newest year, and writes the books of mathematics as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	body, err := req.Get(ctx, req.URL)
	if err != nil {
		return err
	}
	page, err := io.ReadAll(io.LimitReader(body, 16<<20))
	_ = body.Close()
	if err != nil {
		return err
	}
	us, err := Parts(req.URL, string(page))
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	for _, u := range us {
		if err := part(ctx, req.Get, u, enc); err != nil {
			return fmt.Errorf("loc-marc: %s: %w", u, err)
		}
	}
	return zw.Close()
}

func part(ctx context.Context, get ingest.Getter, u string, enc *json.Encoder) error {
	body, err := get(ctx, u)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	zr, err := gzip.NewReader(body)
	if err != nil {
		return err
	}
	rd := marc.NewReader(zr)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rec, err := rd.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		b := FromMARC(rec)
		if b.ID == "" || !Math(b) {
			continue
		}
		if err := enc.Encode(b); err != nil {
			return err
		}
	}
}

// classNumber finds the class letters and the number of an LCC class, as QA and 76.73 in QA76.73.J38.
var classNumber = regexp.MustCompile(`^([A-Z]{1,3})\s*([0-9]+(?:\.[0-9]+)?)`)

// InScope reports whether an LCC class is in QA1 to QA939 and not in QA75.5 to QA76.95, the computer science of QA.
func InScope(lcc string) bool {
	m := classNumber.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(lcc)))
	if m == nil || m[1] != "QA" {
		return false
	}
	n, err := strconv.ParseFloat(m[2], 64)
	if err != nil || n < 1 || n >= 940 {
		return false
	}
	return n < 75.5 || n > 76.95
}

// Math reports whether a book is a book of mathematics by its LCC or DDC class (test S3).
func Math(b Book) bool {
	if slices.ContainsFunc(b.LCC, InScope) {
		return true
	}
	return slices.ContainsFunc(b.DDC, func(d string) bool { return len(d) >= 3 && strings.HasPrefix(d, "51") && d[2] >= '0' && d[2] <= '9' })
}

// FromMARC gives the book of a MARC record.
func FromMARC(r marc.Record) Book { return marc.FromMARC21(r) }

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
			return fmt.Errorf("line %d: the book has no control number", line)
		}
		if err := fn(b); err != nil {
			return err
		}
	}
	return sc.Err()
}

func link(b Book) string {
	if b.LCCN != "" {
		if n, err := ids.Normalize("lccn", b.LCCN); err == nil {
			return "https://lccn.loc.gov/" + n
		}
	}
	return ""
}

// Read reads the books and emits one ingest record for each book.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(b Book) error {
		return emit(ingest.Record{ID: b.ID, Kind: marc.Kind(b), Title: b.Title, URL: link(b)})
	})
}

// Records reads the books and emits one source record for each book (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(b Book) error { return emit(SourceRecord(b)) })
}

// SourceRecord makes the source record of one book. A MARC record describes one edition, so its level is manifestation.
func SourceRecord(b Book) cluster.Record {
	rec := cluster.Record{NativeID: b.ID, Level: "manifestation", Kind: marc.Kind(b), Title: b.Title, Date: b.Date, Language: fetch.Lang(b.Language), Edition: b.Edition, Publisher: b.Publisher, IDs: resource.IDs{}}
	if l := link(b); l != "" {
		rec.Links = []string{l}
	}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("lccn", b.LCCN)
	put("isbn", b.ISBN...)
	put("oclc", b.OCLC...)
	for _, c := range b.Creators {
		rec.Creators = append(rec.Creators, resource.Creator{Name: c.Name, Role: marc.Role(c.Relator)})
	}
	for _, c := range b.LCC {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "lcc", Code: c})
	}
	for _, c := range b.DDC {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "ddc", Code: c})
	}
	return rec
}
