// Package swmath is the adapter of swMATH, the catalogue of mathematical software inside zbMATH Open (spec corpus resources RS7.5 and RS8.3, phase R3). It reads each software entry, and it gives an ingest record and a source record of kind software for each entry, with its swMATH id.
//
// Fetch reads the software endpoint of the API of zbMATH Open in pages of 100 entries. The endpoint gives the id of the last entry of a page, and the next page starts after it. Fetch writes one JSON object for each entry and each line, compressed with gzip. The metadata of zbMATH Open is under CC BY-SA 4.0. The API hides the descriptions and the titles of the standard articles, and the adapter keeps neither. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/swmath"
package swmath

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

// Endpoint is the software endpoint of the API of zbMATH Open.
const Endpoint = "https://api.zbmath.org/v1/software/_all"

// PageSize is the count of entries of one page.
const PageSize = 100

// Adapter is the adapter of swMATH.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of swMATH.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "swmath",
		Name:        "swMATH, the catalogue of mathematical software of zbMATH Open",
		Home:        "https://zbmath.org/software/",
		Licence:     "CC-BY-SA-4.0",
		Class:       ingest.ClassOf("CC-BY-SA-4.0"),
		Attribution: "swMATH, zbMATH Open, FIZ Karlsruhe, https://zbmath.org/software/, CC BY-SA 4.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "swmath/1" }

// Dump returns the software endpoint.
func (Adapter) Dump() string { return Endpoint }

// Software is one entry of swMATH, as one line of the snapshot.
type Software struct {
	ID             int      `json:"id"`
	Name           string   `json:"name"`
	Authors        []string `json:"authors,omitempty"`
	Classification []string `json:"classification,omitempty"` // the MSC classes at two digits of the articles that use the software
	Homepage       string   `json:"homepage,omitempty"`
	SourceCode     string   `json:"source_code,omitempty"`
	Licence        string   `json:"license_terms,omitempty"`
	Articles       int      `json:"articles_count,omitempty"`
	Standard       []int    `json:"standard_articles,omitempty"` // the zbMATH document ids of the standard articles
	Related        []int    `json:"related_software,omitempty"`  // the swMATH ids of the related software
}

// page is one answer of the endpoint.
type page struct {
	Result []struct {
		ID             int      `json:"id"`
		Name           string   `json:"name"`
		Authors        []string `json:"authors"`
		Classification []string `json:"classification"`
		Homepage       string   `json:"homepage"`
		SourceCode     string   `json:"source_code"`
		Licence        string   `json:"license_terms"`
		Articles       int      `json:"articles_count"`
		Standard       []struct {
			ID int `json:"id"`
		} `json:"standard_articles"`
		Related []struct {
			ID int `json:"id"`
		} `json:"related_software"`
	} `json:"result"`
	Status struct {
		LastID int `json:"last_id"`
	} `json:"status"`
}

// Fetch reads each page of the endpoint at req.URL and writes the entries as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	after := 0
	for {
		q := url.Values{"results_per_page": {strconv.Itoa(PageSize)}}
		if after > 0 {
			q.Set("start_after", strconv.Itoa(after))
		}
		body, err := req.Get(ctx, req.URL+"?"+q.Encode())
		if err != nil {
			return err
		}
		var p page
		err = json.NewDecoder(body).Decode(&p)
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("swmath: the page after %d: %w", after, err)
		}
		for _, r := range p.Result {
			s := Software{ID: r.ID, Name: r.Name, Authors: r.Authors, Classification: r.Classification, Homepage: r.Homepage, SourceCode: r.SourceCode, Licence: r.Licence, Articles: r.Articles}
			for _, a := range r.Standard {
				s.Standard = append(s.Standard, a.ID)
			}
			for _, o := range r.Related {
				s.Related = append(s.Related, o.ID)
			}
			if err := enc.Encode(s); err != nil {
				return err
			}
		}
		if len(p.Result) == 0 || p.Status.LastID <= after {
			break
		}
		after = p.Status.LastID
	}
	return zw.Close()
}

func each(r io.Reader, fn func(Software) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var s Software
		if err := json.Unmarshal(sc.Bytes(), &s); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if s.ID <= 0 {
			return fmt.Errorf("line %d: the software has no id", line)
		}
		if err := fn(s); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Read reads the entries and emits one ingest record for each entry.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(s Software) error {
		id := strconv.Itoa(s.ID)
		return emit(ingest.Record{ID: id, Kind: "software", Title: s.Name, URL: "https://zbmath.org/software/" + id})
	})
}

// Records reads the entries and emits one source record for each entry (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(s Software) error { return emit(SourceRecord(s)) })
}

// SourceRecord makes the source record of one entry. The classes at two digits become MSC codes of the form 05-XX. The snapshot keeps the standard articles, but the record has no edge to them, because no relation of RS2.7 means that an article describes the software.
func SourceRecord(s Software) cluster.Record {
	id := strconv.Itoa(s.ID)
	rec := cluster.Record{NativeID: id, Level: "work", Kind: "software", Title: strings.Join(strings.Fields(s.Name), " "), IDs: resource.IDs{"swmath": {id}}}
	for _, a := range s.Authors {
		if a = strings.TrimSpace(a); a != "" {
			rec.Creators = append(rec.Creators, resource.Creator{Name: a, Role: "author"})
		}
	}
	for _, c := range s.Classification {
		if len(c) == 2 && c[0] >= '0' && c[0] <= '9' && c[1] >= '0' && c[1] <= '9' {
			rec.MSC = append(rec.MSC, c+"-XX")
		}
	}
	for _, l := range []string{s.Homepage, s.SourceCode} {
		if strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") {
			rec.Links = append(rec.Links, l)
		}
	}
	return rec
}
