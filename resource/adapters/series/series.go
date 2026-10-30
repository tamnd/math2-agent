// Package series is the adapter of the lists of the book series of mathematics (spec corpus resources RS4.1, RS6.5 and RS8.3, phase R2). It reads each book of each series from Crossref by the ISSN of the series, and it gives a source record for each series and for each book.
//
// One adapter serves each publisher: springer-series for Springer and ams-series for the American Mathematical Society. The list of the series of each publisher is in the code. Crossref gives no volume number for most books of a series, so the volume of the part_of edge is empty when Crossref has none. Crossref has the books of Cambridge University Press with no series, so cambridge-series needs the pages of the publisher and is not in this package yet. The metadata of Crossref is facts with no licence. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/series"
package series

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/crossref"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Series is one book series. The first ISSN names the series.
type Series struct {
	Name      string   `json:"name"`
	Publisher string   `json:"publisher"`
	ISSN      []string `json:"issn"`
}

// Adapter is the adapter of the series of one publisher.
type Adapter struct {
	ID     string
	Name   string
	Series []Series
}

// Springer is the adapter of the series of Springer.
var Springer = Adapter{ID: "springer-series", Name: "The book series of mathematics of Springer, through Crossref", Series: []Series{
	{"Graduate Texts in Mathematics", "Springer", []string{"0072-5285", "2197-5612"}},
	{"Lecture Notes in Mathematics", "Springer", []string{"0075-8434", "1617-9692"}},
	{"Grundlehren der mathematischen Wissenschaften", "Springer", []string{"0072-7830", "2196-9701"}},
	{"Undergraduate Texts in Mathematics", "Springer", []string{"0172-6056", "2197-5604"}},
	{"Springer Undergraduate Mathematics Series", "Springer", []string{"1615-2085", "2197-4144"}},
	{"Universitext", "Springer", []string{"0172-5939", "2191-6675"}},
	{"Ergebnisse der Mathematik und ihrer Grenzgebiete", "Springer", []string{"0071-1136", "2197-5655"}},
	{"Progress in Mathematics", "Springer", []string{"0743-1643", "2296-505X"}},
	{"Springer Monographs in Mathematics", "Springer", []string{"1439-7382", "2196-9922"}},
	{"Texts in Applied Mathematics", "Springer", []string{"0939-2475", "2196-9949"}},
	{"Applied Mathematical Sciences", "Springer", []string{"0066-5452", "2196-968X"}},
	{"Problem Books in Mathematics", "Springer", []string{"0941-3502", "2197-8506"}},
	{"CMS Books in Mathematics", "Springer", []string{"1613-5237", "2197-4152"}},
	{"Encyclopaedia of Mathematical Sciences", "Springer", []string{"0938-0396"}},
}}

// AMS is the adapter of the series of the American Mathematical Society.
var AMS = Adapter{ID: "ams-series", Name: "The book series of the American Mathematical Society, through Crossref", Series: []Series{
	{"Graduate Studies in Mathematics", "American Mathematical Society", []string{"1065-7339"}},
	{"Colloquium Publications", "American Mathematical Society", []string{"0065-9258"}},
	{"Mathematical Surveys and Monographs", "American Mathematical Society", []string{"0076-5376"}},
	{"Proceedings of Symposia in Pure Mathematics", "American Mathematical Society", []string{"0082-0717"}},
	{"Contemporary Mathematics", "American Mathematical Society", []string{"0271-4132", "1098-3627"}},
	{"The Student Mathematical Library", "American Mathematical Society", []string{"1520-9121"}},
	{"Translations of Mathematical Monographs", "American Mathematical Society", []string{"0065-9282"}},
	{"University Lecture Series", "American Mathematical Society", []string{"1047-3998"}},
	{"CBMS Regional Conference Series in Mathematics", "American Mathematical Society", []string{"0160-7642"}},
}}

func init() {
	ingest.Register(Springer)
	ingest.Register(AMS)
}

// Source returns the catalogue entry of the series of the publisher.
func (a Adapter) Source() ingest.Source {
	return ingest.Source{ID: a.ID, Name: a.Name, Home: "https://www.crossref.org", Licence: "LicenseRef-facts", Class: ingest.RecordOnly, Attribution: "Crossref, https://www.crossref.org"}
}

// Version returns the version of the adapter.
func (a Adapter) Version() string { return a.ID + "/1" }

// Dump returns the works route of the REST API of Crossref.
func (a Adapter) Dump() string { return crossref.Base + "/works" }

// Types are the types of Crossref that Fetch asks for: the books, not the chapters.
var Types = []string{"book", "monograph", "edited-book", "reference-book"}

// Line is one line of the snapshot: a series or a work of Crossref.
type Line struct {
	Series *Series          `json:"series,omitempty"`
	Work   *json.RawMessage `json:"work,omitempty"`
}

type page struct {
	Message struct {
		NextCursor string            `json:"next-cursor"`
		Items      []json.RawMessage `json:"items"`
	} `json:"message"`
}

// Fetch writes each series of the list, then reads the books of each ISSN of each series from the works route at req.URL with a cursor. A book with two ISSNs of a series comes once.
func (a Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	u, err := url.Parse(req.URL)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	for i := range a.Series {
		if err := enc.Encode(Line{Series: &a.Series[i]}); err != nil {
			return err
		}
	}
	seen := map[string]bool{}
	for _, s := range a.Series {
		for _, issn := range s.ISSN {
			filter := []string{"issn:" + issn}
			for _, t := range Types {
				filter = append(filter, "type:"+t)
			}
			q := url.Values{"filter": {strings.Join(filter, ",")}, "rows": {strconv.Itoa(crossref.Rows)}, "select": {crossref.Select}}
			for cursor := "*"; cursor != ""; {
				q.Set("cursor", cursor)
				u.RawQuery = q.Encode()
				var p page
				if err := getJSON(ctx, req.Get, u.String(), &p); err != nil {
					return err
				}
				for _, it := range p.Message.Items {
					var head struct {
						DOI string `json:"DOI"`
					}
					if err := json.Unmarshal(it, &head); err != nil || head.DOI == "" {
						return fmt.Errorf("%s: a work with no DOI", u)
					}
					if d := strings.ToLower(head.DOI); !seen[d] {
						seen[d] = true
						if err := enc.Encode(Line{Work: &it}); err != nil {
							return err
						}
					}
				}
				if len(p.Message.Items) == 0 {
					break
				}
				cursor = p.Message.NextCursor
			}
		}
	}
	return zw.Close()
}

func getJSON(ctx context.Context, get ingest.Getter, u string, v any) error {
	body, err := get(ctx, u)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		return fmt.Errorf("%s: %w", u, err)
	}
	return nil
}

// each reads the snapshot and calls the function of each line. The series come before the works.
func each(r io.Reader, series func(Series) error, work func(crossref.Work) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 64<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var l Line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		switch {
		case l.Series != nil:
			if len(l.Series.ISSN) == 0 {
				return fmt.Errorf("line %d: the series has no ISSN", line)
			}
			if err := series(*l.Series); err != nil {
				return err
			}
		case l.Work != nil:
			var w crossref.Work
			if err := json.Unmarshal(*l.Work, &w); err != nil {
				return fmt.Errorf("line %d: %w", line, err)
			}
			if err := work(w); err != nil {
				return err
			}
		default:
			return fmt.Errorf("line %d: the line has no series and no work", line)
		}
	}
	return sc.Err()
}

// Read reads the snapshot and emits one ingest record for each series and each book.
func (a Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(s Series) error {
		return emit(ingest.Record{ID: Record(s).NativeID, Kind: "series", Title: s.Name})
	}, func(w crossref.Work) error {
		rec := crossref.SourceRecord(w)
		return emit(ingest.Record{ID: rec.NativeID, Kind: rec.Kind, Title: rec.Title, URL: "https://doi.org/" + rec.NativeID})
	})
}

// Records reads the snapshot and emits one source record for each series and each book (RS13.1). A book has a part_of edge to each series of the list whose ISSN it has.
func (a Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	byISSN := map[string]Series{}
	return each(r, func(s Series) error {
		rec := Record(s)
		for _, v := range rec.IDs["issn"] {
			byISSN[v] = s
		}
		return emit(rec)
	}, func(w crossref.Work) error {
		rec := crossref.SourceRecord(w)
		var done []string
		for _, raw := range w.ISSN {
			v, err := ids.Normalize("issn", raw)
			if err != nil {
				continue
			}
			s, ok := byISSN[v]
			if !ok || slices.Contains(done, s.Name) {
				continue
			}
			done = append(done, s.Name)
			rec.Relations = append(rec.Relations, resource.Relation{Kind: "part_of", To: "issn:" + Record(s).NativeID, Volume: rec.Volume, Source: a.ID})
		}
		return emit(rec)
	})
}

// Record makes the source record of one series. Its native id is the first ISSN.
func Record(s Series) cluster.Record {
	rec := cluster.Record{Level: "work", Kind: "series", Title: s.Name, Publisher: s.Publisher, IDs: resource.IDs{}}
	for _, v := range s.ISSN {
		if n, err := ids.Normalize("issn", v); err == nil {
			rec.IDs["issn"] = append(rec.IDs["issn"], n)
		}
	}
	if len(rec.IDs["issn"]) > 0 {
		rec.NativeID = rec.IDs["issn"][0]
	}
	return rec
}
