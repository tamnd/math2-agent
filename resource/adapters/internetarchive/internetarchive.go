// Package internetarchive is the adapter of the texts of mathematics of the Internet Archive (spec corpus resources RS4.1 and RS8.3, phase R3). It reads the items of the Internet Archive with the subject mathematics and the media type texts, and it gives an ingest record and a source record for each item.
//
// Fetch reads the scrape API of the search of the Internet Archive with the query Query. The API gives pages of up to 10000 items and pages by a cursor, and it needs no key. Fetch writes one JSON object for each item and each line, compressed with gzip. The licence of the metadata of the Internet Archive is not read (RS8.3), so the adapter keeps the class record only. The snapshot keeps no description and no text. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/internetarchive"
package internetarchive

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Endpoint is the scrape API of the search of the Internet Archive.
const Endpoint = "https://archive.org/services/search/v1/scrape"

// Query is the query of the texts with the subject mathematics.
const Query = "subject:mathematics AND mediatype:texts"

// Fields are the fields of an item that Fetch asks for.
const Fields = "identifier,title,year,date,language,creator,publisher,isbn,oclc-id,lccn,call_number,volume"

// PageSize is the number of items of one page. The API accepts 100 to 10000.
const PageSize = 10000

// MaxPages is the most pages of one fetch. It stops a cursor with no end.
const MaxPages = 1000

// Adapter is the adapter of the Internet Archive.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the Internet Archive.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "internetarchive",
		Name:        "Internet Archive, the texts of mathematics",
		Home:        "https://archive.org",
		Licence:     "LicenseRef-unknown",
		Class:       ingest.RecordOnly,
		Attribution: "Internet Archive, https://archive.org",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "internetarchive/1" }

// Dump returns the scrape API.
func (Adapter) Dump() string { return Endpoint }

// Item is one item, as one line of the snapshot.
type Item struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Date      string   `json:"date,omitempty"`
	Language  string   `json:"language,omitempty"`
	Creators  []string `json:"creators,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	Volume    string   `json:"volume,omitempty"`
	ISBN      []string `json:"isbn,omitempty"`
	OCLC      []string `json:"oclc,omitempty"`
	LCCN      []string `json:"lccn,omitempty"`
	Call      []string `json:"call_number,omitempty"`
}

// item is one item of a page. Each field is a string, a number or a list.
type item struct {
	ID        any `json:"identifier"`
	Title     any `json:"title"`
	Year      any `json:"year"`
	Date      any `json:"date"`
	Language  any `json:"language"`
	Creator   any `json:"creator"`
	Publisher any `json:"publisher"`
	ISBN      any `json:"isbn"`
	OCLC      any `json:"oclc-id"`
	LCCN      any `json:"lccn"`
	Call      any `json:"call_number"`
	Volume    any `json:"volume"`
}

type page struct {
	Items  []item `json:"items"`
	Cursor string `json:"cursor"`
}

// strs reads a field of the Internet Archive, which is a string, a number or a list of them. It gives the values that are not empty.
func strs(v any) []string {
	switch x := v.(type) {
	case string:
		if s := strings.Join(strings.Fields(x), " "); s != "" {
			return []string{s}
		}
	case float64:
		return []string{strconv.FormatFloat(x, 'f', -1, 64)}
	case []any:
		var out []string
		for _, e := range x {
			out = append(out, strs(e)...)
		}
		return out
	}
	return nil
}

func first(v any) string {
	if s := strs(v); len(s) > 0 {
		return s[0]
	}
	return ""
}

// Fetch reads each page of the scrape API at req.URL until the cursor ends, and writes the items as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	q := url.Values{"q": {Query}, "fields": {Fields}, "count": {strconv.Itoa(PageSize)}}
	for n := 0; ; n++ {
		if n == MaxPages {
			return fmt.Errorf("internetarchive: more than %d pages", MaxPages)
		}
		u := req.URL + "?" + q.Encode()
		body, err := req.Get(ctx, u)
		if err != nil {
			return err
		}
		var p page
		err = json.NewDecoder(body).Decode(&p)
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("internetarchive: page %d: %w", n+1, err)
		}
		for _, it := range p.Items {
			if x := fromItem(it); x.ID != "" {
				if err := enc.Encode(x); err != nil {
					return err
				}
			}
		}
		if p.Cursor == "" || len(p.Items) == 0 {
			break
		}
		q.Set("cursor", p.Cursor)
	}
	return zw.Close()
}

// fromItem makes an item of the snapshot from an item of a page. The date is the year, or else the date of the item.
func fromItem(it item) Item {
	x := Item{ID: first(it.ID), Title: first(it.Title), Date: first(it.Year), Language: first(it.Language), Creators: strs(it.Creator), Publisher: first(it.Publisher),
		Volume: first(it.Volume), ISBN: strs(it.ISBN), OCLC: strs(it.OCLC), LCCN: strs(it.LCCN), Call: strs(it.Call)}
	if x.Date == "" {
		x.Date, _, _ = strings.Cut(first(it.Date), "T")
	}
	return x
}

func each(r io.Reader, fn func(Item) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var x Item
		if err := json.Unmarshal(sc.Bytes(), &x); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if x.ID == "" {
			return fmt.Errorf("line %d: the item has no id", line)
		}
		if err := fn(x); err != nil {
			return err
		}
	}
	return sc.Err()
}

// lcc finds an LCC class at the start of a call number, as QA303.2 in QA303.2 .S7 1990. A call number of the DDC has no letters, so it does not match.
var lcc = regexp.MustCompile(`^[A-Z]{1,3}[0-9]+(?:\.[0-9]+)?`)

// oldArXiv finds an old arXiv id with no slash, as math0211159 in the id arxiv-math0211159.
var oldArXiv = regexp.MustCompile(`^([a-z-]+(?:\.[A-Z]{2})?)([0-9]{7})$`)

func link(x Item) string { return "https://archive.org/details/" + x.ID }

// Kind gives the kind of an item. An item whose id starts with arxiv- is a copy of a preprint of arXiv. Each other text is a monograph, because most texts of mathematics of the Internet Archive are scans of books.
func Kind(x Item) string {
	if strings.HasPrefix(x.ID, "arxiv-") {
		return "preprint"
	}
	return "monograph"
}

// Read reads the items and emits one ingest record for each item.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(x Item) error {
		return emit(ingest.Record{ID: x.ID, Kind: Kind(x), Title: x.Title, URL: link(x)})
	})
}

// Records reads the items and emits one source record for each item (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(x Item) error { return emit(SourceRecord(x)) })
}

// SourceRecord makes the source record of one item. An item is one scan of one edition, so its level is manifestation.
func SourceRecord(x Item) cluster.Record {
	rec := cluster.Record{NativeID: x.ID, Level: "manifestation", Kind: Kind(x), Title: x.Title, Date: x.Date, Language: fetch.Lang(x.Language), Publisher: x.Publisher,
		Volume: x.Volume, IDs: resource.IDs{}, Links: []string{link(x)}}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("ia", x.ID)
	if id, ok := strings.CutPrefix(x.ID, "arxiv-"); ok {
		put("arxiv", oldArXiv.ReplaceAllString(id, "$1/$2"))
	}
	put("isbn", x.ISBN...)
	put("oclc", x.OCLC...)
	put("lccn", x.LCCN...)
	for _, c := range x.Call {
		if m := lcc.FindString(strings.ToUpper(c)); m != "" {
			rec.Subjects = append(rec.Subjects, resource.Subject{System: "lcc", Code: m})
		}
	}
	for _, name := range x.Creators {
		rec.Creators = append(rec.Creators, resource.Creator{Name: name, Role: "author"})
	}
	return rec
}
