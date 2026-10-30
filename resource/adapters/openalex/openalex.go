// Package openalex is the adapter of OpenAlex (spec corpus resources RS4.1 and RS6.1). It reads works of OpenAlex, one JSON object for each line, as the S3 snapshot and the API give them, and it gives an ingest record and a source record for each work.
//
// The bulk channel of OpenAlex is the S3 snapshot, of about 400 GB compressed. The channel of this adapter is the API: Fetch reads each work that the filter of the URL selects, in pages of 200 with a cursor, and writes the works as JSON Lines compressed with gzip. The default filter is the works whose primary topic is in the field of mathematics. A part file of the S3 snapshot is a valid snapshot too. The metadata of OpenAlex is under CC0. The adapter sends no mailto parameter and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/openalex"
package openalex

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

// Base is the API of OpenAlex.
const Base = "https://api.openalex.org"

// Filter selects the works whose primary topic is in the field of mathematics, field 26 of the topics of OpenAlex.
const Filter = "primary_topic.field.id:26"

// Select names the fields of a work that Fetch asks for.
const Select = "id,doi,title,publication_date,publication_year,language,type,authorships,ids,primary_location,locations,biblio,primary_topic"

// PageSize is the number of works in one page of the API.
const PageSize = 200

// Adapter is the adapter of OpenAlex.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of OpenAlex.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "openalex",
		Name:        "OpenAlex",
		Home:        "https://openalex.org",
		Licence:     "CC0-1.0",
		Class:       ingest.Open,
		Attribution: "OpenAlex, OurResearch, https://openalex.org, CC0 1.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "openalex/1" }

// Dump returns the URL of the works of mathematics in the API.
func (Adapter) Dump() string { return Base + "/works?filter=" + url.QueryEscape(Filter) }

// Fetch reads each page of the works at req.URL with a cursor and writes the works as JSON Lines, compressed with gzip. The URL keeps its filter, and Fetch sets the page size, the fields and the cursor.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	u, err := url.Parse(req.URL)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("per-page", strconv.Itoa(PageSize))
	q.Set("select", Select)
	zw := gzip.NewWriter(w)
	for cursor := "*"; cursor != ""; {
		q.Set("cursor", cursor)
		u.RawQuery = q.Encode()
		var page struct {
			Meta struct {
				NextCursor string `json:"next_cursor"`
			} `json:"meta"`
			Results []json.RawMessage `json:"results"`
		}
		if err := getJSON(ctx, req.Get, u.String(), &page); err != nil {
			return err
		}
		for _, r := range page.Results {
			if _, err := zw.Write(append(r, '\n')); err != nil {
				return err
			}
		}
		cursor = page.Meta.NextCursor
		if len(page.Results) == 0 {
			cursor = ""
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

// Work is one work of OpenAlex, with the fields that the adapter reads.
type Work struct {
	ID              string `json:"id"`
	DOI             string `json:"doi"`
	Title           string `json:"title"`
	PublicationDate string `json:"publication_date"`
	PublicationYear int    `json:"publication_year"`
	Language        string `json:"language"`
	Type            string `json:"type"`
	Authorships     []struct {
		Author struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			ORCID       string `json:"orcid"`
		} `json:"author"`
	} `json:"authorships"`
	IDs struct {
		PMID string `json:"pmid"`
	} `json:"ids"`
	PrimaryLocation *Location  `json:"primary_location"`
	Locations       []Location `json:"locations"`
	Biblio          struct {
		Volume    string `json:"volume"`
		FirstPage string `json:"first_page"`
	} `json:"biblio"`
	PrimaryTopic *struct {
		Field struct {
			ID string `json:"id"`
		} `json:"field"`
	} `json:"primary_topic"`
}

// Location is one place of a work: a journal, a repository or a home page.
type Location struct {
	LandingPage string `json:"landing_page_url"`
	Source      *struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
		ISSNL       string `json:"issn_l"`
		Publisher   string `json:"host_organization_name"`
	} `json:"source"`
}

// Kinds maps the types of OpenAlex to the kinds of RS2.3. A type that is not in the map gives the kind article.
var Kinds = map[string]string{
	"article": "article", "book": "monograph", "book-chapter": "chapter", "dissertation": "thesis", "preprint": "preprint",
	"review": "survey", "report": "report", "dataset": "dataset", "editorial": "article", "letter": "article",
	"paratext": "article", "reference-entry": "chapter", "standard": "report", "libguides": "website", "other": "article",
}

// each calls fn for each work of the snapshot.
func each(r io.Reader, fn func(Work) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 64<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var w Work
		if err := json.Unmarshal(sc.Bytes(), &w); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if w.ID == "" {
			return fmt.Errorf("line %d: the work has no id", line)
		}
		if err := fn(w); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Read reads the works and emits one ingest record for each work.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(w Work) error {
		rec := ingest.Record{ID: short(w.ID), Kind: w.Type, Title: strings.Join(strings.Fields(w.Title), " "), URL: w.ID, Meta: map[string]string{}}
		if d, err := ids.Normalize("doi", w.DOI); err == nil {
			rec.Meta["doi"] = d
		}
		if len(rec.Meta) == 0 {
			rec.Meta = nil
		}
		return emit(rec)
	})
}

// Records reads the works and emits one source record for each work (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(w Work) error { return emit(SourceRecord(w)) })
}

// short gives the id of a work without the URL prefix, as W2741809807.
func short(id string) string { return strings.TrimPrefix(id, "https://openalex.org/") }

// SourceRecord makes the source record of one work. The arXiv id comes from a location on arxiv.org. The venue is the ISSN-L of the primary location.
func SourceRecord(w Work) cluster.Record {
	kind := Kinds[w.Type]
	if kind == "" {
		kind = "article"
	}
	rec := cluster.Record{
		NativeID: short(w.ID),
		Level:    "work",
		Kind:     kind,
		Title:    strings.Join(strings.Fields(w.Title), " "),
		Date:     w.PublicationDate,
		Language: w.Language,
		IDs:      resource.IDs{},
		Volume:   w.Biblio.Volume,
		Page:     w.Biblio.FirstPage,
	}
	if rec.Date == "" && w.PublicationYear > 0 {
		rec.Date = strconv.Itoa(w.PublicationYear)
	}
	add := func(key, v string) {
		if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
			rec.IDs[key] = append(rec.IDs[key], n)
		}
	}
	add("openalex", short(w.ID))
	add("doi", w.DOI)
	for _, a := range w.Authorships {
		if a.Author.DisplayName == "" {
			continue
		}
		c := resource.Creator{Name: a.Author.DisplayName, Role: "author"}
		if o, err := ids.Normalize("orcid", a.Author.ORCID); err == nil {
			c.IDs = resource.IDs{"orcid": {o}}
		}
		rec.Creators = append(rec.Creators, c)
	}
	if p := w.PrimaryLocation; p != nil && p.Source != nil {
		if v, err := ids.Normalize("issn", p.Source.ISSNL); err == nil {
			rec.Venue = v
		}
		rec.Publisher = p.Source.Publisher
	}
	for _, l := range w.Locations {
		if a, ok := arxivID(l.LandingPage); ok {
			add("arxiv", a)
		}
	}
	if p := w.PrimaryLocation; p != nil && p.LandingPage != "" {
		rec.Links = []string{p.LandingPage}
	}
	return rec
}

// arxivID gives the arXiv id of a landing page on arxiv.org.
func arxivID(page string) (string, bool) {
	for _, p := range []string{"https://arxiv.org/abs/", "http://arxiv.org/abs/", "https://export.arxiv.org/abs/"} {
		if a, ok := strings.CutPrefix(page, p); ok {
			if n, err := ids.Normalize("arxiv", a); err == nil {
				return n, true
			}
		}
	}
	return "", false
}
