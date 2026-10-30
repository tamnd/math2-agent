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

// Creator is one creator of a book, with the relator of the record.
type Creator struct {
	Name     string `json:"name"`
	Relator  string `json:"relator,omitempty"`
	Personal bool   `json:"personal,omitempty"`
	Main     bool   `json:"main,omitempty"` // the main entry (1XX)
}

// Book is one book, as one line of the snapshot.
type Book struct {
	ID        string    `json:"id"` // the control number (001)
	LCCN      string    `json:"lccn,omitempty"`
	ISBN      []string  `json:"isbn,omitempty"`
	OCLC      []string  `json:"oclc,omitempty"`
	LCC       []string  `json:"lcc,omitempty"`
	DDC       []string  `json:"ddc,omitempty"`
	Title     string    `json:"title"`
	Edition   string    `json:"edition,omitempty"`
	Publisher string    `json:"publisher,omitempty"`
	Date      string    `json:"date,omitempty"`
	Language  string    `json:"language,omitempty"`
	Creators  []Creator `json:"creators,omitempty"`
	Series    []string  `json:"series,omitempty"`
	Headings  []string  `json:"headings,omitempty"`
	Contents  []string  `json:"contents,omitempty"`
}

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
	rd := NewReader(zr)
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

// clean takes the punctuation of ISBD from the end of a value, as the / in Calculus /.
func clean(s string) string {
	return strings.TrimSpace(strings.TrimRight(strings.TrimSpace(s), " /:;,="))
}

// cleanEnd also takes a period from the end, but not the period of an initial or an abbreviation.
func cleanEnd(s string) string {
	s = clean(s)
	if strings.HasSuffix(s, ".") && !initial.MatchString(s) {
		s = strings.TrimSuffix(s, ".")
	}
	return clean(s)
}

// initial finds a value that ends in an initial or an abbreviation, as G. H. or ed.
var initial = regexp.MustCompile(`(?:\b[A-Z]|\b[a-z]{1,3})\.$`)

// FromMARC gives the book of a MARC record.
func FromMARC(r Record) Book {
	b := Book{ID: strings.TrimSpace(r.Control("001"))}
	if f := r.Control("008"); len(f) >= 38 {
		if d := f[7:11]; strings.Trim(d, "0123456789") == "" {
			b.Date = d
		}
		b.Language = strings.TrimSpace(f[35:38])
	}
	for _, f := range r.All("010") {
		// A revision mark follows a slash, as 00004426 //r842.
		if v, _, _ := strings.Cut(f.First("a"), "/"); b.LCCN == "" {
			b.LCCN = strings.TrimSpace(v)
		}
	}
	for _, f := range r.All("020") {
		for _, v := range f.Sub("a") {
			if v, _, _ = strings.Cut(strings.TrimSpace(v), " "); v != "" {
				b.ISBN = append(b.ISBN, v)
			}
		}
	}
	for _, f := range r.All("035") {
		for _, v := range f.Sub("a") {
			if n, ok := strings.CutPrefix(v, "(OCoLC)"); ok {
				b.OCLC = append(b.OCLC, strings.TrimLeft(strings.TrimSpace(n), "ocmnl"))
			}
		}
	}
	for _, f := range r.All("050") {
		if a := f.First("a"); a != "" {
			b.LCC = append(b.LCC, strings.TrimSpace(a+" "+f.First("b")))
		}
	}
	for _, f := range r.All("082") {
		for _, v := range f.Sub("a") {
			b.DDC = append(b.DDC, strings.ReplaceAll(strings.TrimSpace(v), "/", ""))
		}
	}
	if fs := r.All("245"); len(fs) > 0 {
		t := clean(fs[0].First("a"))
		if sub := clean(strings.Join(fs[0].Sub("b"), " ")); sub != "" {
			t += ": " + sub
		}
		b.Title = cleanEnd(strings.Join(strings.Fields(t), " "))
	}
	if fs := r.All("250"); len(fs) > 0 {
		b.Edition = cleanEnd(fs[0].First("a"))
	}
	for _, tag := range []string{"264", "260"} {
		for _, f := range r.All(tag) {
			if b.Publisher == "" {
				b.Publisher = cleanEnd(f.First("b"))
			}
		}
	}
	for _, tag := range []string{"100", "110", "700", "710"} {
		for _, f := range r.All(tag) {
			if n := cleanEnd(f.First("a")); n != "" {
				b.Creators = append(b.Creators, Creator{Name: n, Relator: cleanEnd(strings.Join(append(f.Sub("e"), f.Sub("4")...), " ")), Personal: tag[1:] == "00", Main: tag[0] == '1'})
			}
		}
	}
	for _, tag := range []string{"490", "830"} {
		for _, f := range r.All(tag) {
			s := cleanEnd(f.First("a"))
			if v := cleanEnd(f.First("v")); v != "" {
				s += " ; " + v
			}
			if s != "" && !slices.Contains(b.Series, s) {
				b.Series = append(b.Series, s)
			}
		}
	}
	for _, f := range r.All("650") {
		h := cleanEnd(f.First("a"))
		for _, x := range f.Sub("x") {
			h += " -- " + cleanEnd(x)
		}
		if h != "" {
			b.Headings = append(b.Headings, h)
		}
	}
	for _, f := range r.All("505") {
		if a := strings.TrimSpace(f.First("a")); a != "" {
			b.Contents = append(b.Contents, a)
		}
		for _, t := range f.Sub("t") {
			if t = cleanEnd(t); t != "" {
				b.Contents = append(b.Contents, t)
			}
		}
	}
	return b
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
			return fmt.Errorf("line %d: the book has no control number", line)
		}
		if err := fn(b); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Kind gives the kind of a book: an edited volume when the book has no main entry and an added entry is an editor, else a monograph.
func Kind(b Book) string {
	editor := false
	for _, c := range b.Creators {
		if c.Main {
			return "monograph"
		}
		editor = editor || role(c.Relator) == "editor"
	}
	if editor {
		return "edited-volume"
	}
	return "monograph"
}

// role maps a relator of MARC, as editor or edt, to a role of RS2.5.
func role(rel string) string {
	r := strings.ToLower(rel)
	switch {
	case r == "":
		return "author"
	case strings.Contains(r, "edt") || strings.Contains(r, "editor") || strings.HasPrefix(r, "ed"):
		return "editor"
	case strings.Contains(r, "trl") || strings.Contains(r, "transl"):
		return "translator"
	case strings.Contains(r, "aut") || strings.Contains(r, "author"):
		return "author"
	}
	return "contributor"
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
	return each(r, func(b Book) error { return emit(ingest.Record{ID: b.ID, Kind: Kind(b), Title: b.Title, URL: link(b)}) })
}

// Records reads the books and emits one source record for each book (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(b Book) error { return emit(SourceRecord(b)) })
}

// SourceRecord makes the source record of one book. A MARC record describes one edition, so its level is manifestation.
func SourceRecord(b Book) cluster.Record {
	rec := cluster.Record{NativeID: b.ID, Level: "manifestation", Kind: Kind(b), Title: b.Title, Date: b.Date, Language: fetch.Lang(b.Language), Edition: b.Edition, Publisher: b.Publisher, IDs: resource.IDs{}}
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
		rec.Creators = append(rec.Creators, resource.Creator{Name: c.Name, Role: role(c.Relator)})
	}
	for _, c := range b.LCC {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "lcc", Code: c})
	}
	for _, c := range b.DDC {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "ddc", Code: c})
	}
	return rec
}
