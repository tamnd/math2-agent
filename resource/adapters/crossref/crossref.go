// Package crossref is the adapter of Crossref (spec corpus resources RS4.1 and RS6.1). It reads works of Crossref, one JSON object for each line, and it gives an ingest record and a source record for each work. A line can also hold a page of the public data file, an object with a list items, and each item of the list is a work.
//
// Crossref has no subject, so the adapter selects the works in two ways. With a URL that has a filter, as https://api.crossref.org/works?filter=issn:0003-486X, Fetch reads each page of the works with a cursor. With the URL of Dump, which has no query, Fetch reads the DOIs of the source records of the other sources in the tree and asks for them in groups of 50. The bibliographic metadata of Crossref is facts with no licence, and the abstracts are not free, so the adapter keeps no abstract and the class is record.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/crossref"
package crossref

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
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Base is the REST API of Crossref.
const Base = "https://api.crossref.org"

// Rows is the number of works in one page of the API.
const Rows = 1000

// Group is the number of DOIs in one request of the DOI mode.
const Group = 50

// Select names the fields of a work that Fetch asks for. The works API does not accept language in select (it answers 400), thus Fetch does not ask for it, and Language is empty when the API does not send it.
const Select = "DOI,title,subtitle,type,ISSN,ISBN,author,editor,issued,volume,page,publisher,container-title,relation,URL"

// Adapter is the adapter of Crossref.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of Crossref.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "crossref",
		Name:        "Crossref",
		Home:        "https://www.crossref.org",
		Licence:     "LicenseRef-facts",
		Class:       ingest.RecordOnly,
		Attribution: "Crossref metadata, https://www.crossref.org",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "crossref/1" }

// Dump returns the works API with no query, which selects the DOI mode of Fetch.
func (Adapter) Dump() string { return Base + "/works" }

type page struct {
	Message struct {
		NextCursor string            `json:"next-cursor"`
		Items      []json.RawMessage `json:"items"`
	} `json:"message"`
}

// Fetch writes the works as JSON Lines, compressed with gzip. A URL with a query gives the works of its filter, page by page with a cursor. A URL with no query gives the works of the DOIs of the source records in req.Root.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	u, err := url.Parse(req.URL)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(w)
	write := func(p *page) error {
		for _, it := range p.Message.Items {
			if _, err := zw.Write(append(it, '\n')); err != nil {
				return err
			}
		}
		return nil
	}
	if u.RawQuery == "" {
		dois, err := DOIs(req.Root)
		if err != nil {
			return err
		}
		for g := range slices.Chunk(dois, Group) {
			filter := make([]string, len(g))
			for i, d := range g {
				filter[i] = "doi:" + d
			}
			q := url.Values{"filter": {strings.Join(filter, ",")}, "rows": {strconv.Itoa(Group)}, "select": {Select}}
			u.RawQuery = q.Encode()
			var p page
			if err := getJSON(ctx, req.Get, u.String(), &p); err != nil {
				return err
			}
			if err := write(&p); err != nil {
				return err
			}
		}
		return zw.Close()
	}
	q := u.Query()
	q.Set("rows", strconv.Itoa(Rows))
	q.Set("select", Select)
	for cursor := "*"; cursor != ""; {
		q.Set("cursor", cursor)
		u.RawQuery = q.Encode()
		var p page
		if err := getJSON(ctx, req.Get, u.String(), &p); err != nil {
			return err
		}
		if err := write(&p); err != nil {
			return err
		}
		cursor = p.Message.NextCursor
		if len(p.Message.Items) == 0 {
			cursor = ""
		}
	}
	return zw.Close()
}

// DOIs gives the DOIs of the source records of the other sources in the tree at root, sorted and without repeats. A DOI with a comma is left out, because the filter of Crossref splits on commas.
func DOIs(root string) ([]string, error) {
	rs, err := cluster.ReadRecords(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range rs {
		if r.Source == "crossref" {
			continue
		}
		for _, d := range r.IDs["doi"] {
			if !strings.Contains(d, ",") {
				out = append(out, d)
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
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

type person struct {
	Given  string `json:"given"`
	Family string `json:"family"`
	Name   string `json:"name"`
	ORCID  string `json:"ORCID"`
}

// Work is one work of Crossref, with the fields that the adapter reads.
type Work struct {
	DOI      string   `json:"DOI"`
	Title    []string `json:"title"`
	Subtitle []string `json:"subtitle"`
	Type     string   `json:"type"`
	ISSN     []string `json:"ISSN"`
	ISBN     []string `json:"ISBN"`
	Author   []person `json:"author"`
	Editor   []person `json:"editor"`
	Issued   struct {
		DateParts [][]*int `json:"date-parts"`
	} `json:"issued"`
	Language  string   `json:"language"`
	Volume    string   `json:"volume"`
	Page      string   `json:"page"`
	Publisher string   `json:"publisher"`
	Container []string `json:"container-title"`
	Relation  map[string][]struct {
		IDType string `json:"id-type"`
		ID     string `json:"id"`
	} `json:"relation"`
	URL string `json:"URL"`
}

// Kinds maps the types of Crossref to the kinds of RS2.3. A type that is not in the map gives the kind article.
var Kinds = map[string]string{
	"journal-article": "article", "book": "monograph", "monograph": "monograph", "edited-book": "edited-volume", "book-chapter": "chapter",
	"book-part": "chapter", "book-section": "chapter", "reference-entry": "chapter", "proceedings-article": "conference-paper",
	"proceedings": "proceedings", "posted-content": "preprint", "dissertation": "thesis", "report": "report", "dataset": "dataset",
	"journal": "journal", "book-series": "series", "book-set": "series", "reference-book": "handbook", "standard": "report",
}

// each calls fn for each work of the snapshot.
func each(r io.Reader, fn func(Work) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 256<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var x struct {
			Work
			Items []Work `json:"items"`
		}
		if err := json.Unmarshal(sc.Bytes(), &x); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		works := x.Items
		if x.DOI != "" {
			works = []Work{x.Work}
		}
		if len(works) == 0 {
			return fmt.Errorf("line %d: the line has no DOI and no items", line)
		}
		for _, w := range works {
			if w.DOI == "" {
				return fmt.Errorf("line %d: a work has no DOI", line)
			}
			if err := fn(w); err != nil {
				return err
			}
		}
	}
	return sc.Err()
}

// Read reads the works and emits one ingest record for each work. The id is the DOI in its normal form.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(w Work) error {
		d := doi(w.DOI)
		return emit(ingest.Record{ID: d, Kind: w.Type, Title: title(w), URL: "https://doi.org/" + d})
	})
}

// Records reads the works and emits one source record for each work (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(w Work) error { return emit(SourceRecord(w)) })
}

func doi(s string) string {
	if d, err := ids.Normalize("doi", s); err == nil {
		return d
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func title(w Work) string {
	t := ""
	if len(w.Title) > 0 {
		t = w.Title[0]
	}
	if len(w.Subtitle) > 0 && w.Subtitle[0] != "" && t != "" {
		t += ": " + w.Subtitle[0]
	}
	return strings.Join(strings.Fields(t), " ")
}

func name(p person) string {
	switch {
	case p.Family != "" && p.Given != "":
		return p.Given + " " + p.Family
	case p.Family != "":
		return p.Family
	}
	return p.Name
}

// SourceRecord makes the source record of one work. The venue is the first ISSN. A relation to a preprint on arXiv gives the arXiv id.
func SourceRecord(w Work) cluster.Record {
	kind := Kinds[w.Type]
	if kind == "" {
		kind = "article"
	}
	d := doi(w.DOI)
	rec := cluster.Record{
		NativeID:  d,
		Level:     "work",
		Kind:      kind,
		Title:     title(w),
		Language:  w.Language,
		IDs:       resource.IDs{"doi": {d}},
		Volume:    w.Volume,
		Publisher: w.Publisher,
	}
	if p, _, _ := strings.Cut(w.Page, "-"); p != "" {
		rec.Page = p
	}
	if len(w.Issued.DateParts) > 0 {
		var parts []string
		for i, p := range w.Issued.DateParts[0] {
			if p == nil || *p <= 0 {
				break
			}
			if i == 0 {
				parts = append(parts, fmt.Sprintf("%04d", *p))
			} else {
				parts = append(parts, fmt.Sprintf("%02d", *p))
			}
		}
		rec.Date = strings.Join(parts, "-")
	}
	for _, s := range w.ISSN {
		if v, err := ids.Normalize("issn", s); err == nil {
			rec.Venue = v
			break
		}
	}
	for _, s := range w.ISBN {
		if v, err := ids.Normalize("isbn", s); err == nil && !slices.Contains(rec.IDs["isbn"], v) {
			rec.IDs["isbn"] = append(rec.IDs["isbn"], v)
		}
	}
	for _, g := range []struct {
		role string
		ps   []person
	}{{"author", w.Author}, {"editor", w.Editor}} {
		for _, p := range g.ps {
			n := name(p)
			if n == "" {
				continue
			}
			c := resource.Creator{Name: n, Role: g.role}
			if o, err := ids.Normalize("orcid", p.ORCID); err == nil {
				c.IDs = resource.IDs{"orcid": {o}}
			}
			rec.Creators = append(rec.Creators, c)
		}
	}
	for _, kind := range []string{"has-preprint", "is-preprint-of"} {
		for _, r := range w.Relation[kind] {
			a := ""
			switch {
			case r.IDType == "arxiv":
				a = r.ID
			case r.IDType == "doi" && strings.HasPrefix(strings.ToLower(r.ID), "10.48550/arxiv."):
				a = r.ID[len("10.48550/arxiv."):]
			}
			if v, err := ids.Normalize("arxiv", a); err == nil && !slices.Contains(rec.IDs["arxiv"], v) {
				rec.IDs["arxiv"] = append(rec.IDs["arxiv"], v)
			}
		}
	}
	if w.URL != "" {
		rec.Links = []string{w.URL}
	}
	return rec
}
