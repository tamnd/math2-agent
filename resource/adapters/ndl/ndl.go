// Package ndl is the adapter of NDL Search, the catalogue of the National Diet Library of Japan (spec corpus resources RS4.1 and RS8.3, phase R3). It reads the books of mathematics of NDL Search, and it gives an ingest record and a source record for each book that the NDL made.
//
// Fetch reads the SRU server of NDL Search in the schema dcndl, with one search for each NDC class from 410 to 419, in pages of 200 records. NDL Search can block heavy access, so the adapter waits Gap between two requests. The adapter keeps the records with an NDC class that starts with 41, and it writes one JSON object for each record and each line, compressed with gzip. NDL Search also holds the records of other libraries. The NDL lets everyone use the records that it made, with credit and the date of retrieval, and the records of other providers follow the terms of each provider (RQ06). Thus the snapshot keeps the full record only when the provider is the NDL, and it keeps only the ids of a record of another provider. A record with ids only has no title, so it gives an ingest record and no source record. The attribution gives the credit, and the catalogue keeps the date of each snapshot, which is the date of retrieval. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/ndl"
package ndl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/adapters/marc"
	"github.com/tamnd/math2-agent/resource/adapters/oai"
	"github.com/tamnd/math2-agent/resource/adapters/sru"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Base is the SRU server of NDL Search.
const Base = "https://ndlsearch.ndl.go.jp/api/sru"

// PageSize is the most records of one request to the SRU server of NDL Search.
const PageSize = 200

// Gap is the time between two requests. It is long, because NDL Search can block heavy access.
const Gap = 5 * time.Second

// Classes are the NDC classes of the searches.
var Classes = []string{"410", "411", "412", "413", "414", "415", "416", "417", "418", "419"}

// Providers are the providers of NDL Search whose records the NDL made: R100000002 is the catalogue of the NDL.
var Providers = []string{"R100000002"}

// Adapter is the adapter of NDL Search. Gap is the time between two requests.
type Adapter struct{ Gap time.Duration }

func init() { ingest.Register(Adapter{Gap: Gap}) }

// Source returns the catalogue entry of the NDL.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "ndl",
		Name:        "National Diet Library, the SRU server of NDL Search",
		Home:        "https://ndlsearch.ndl.go.jp",
		Licence:     "LicenseRef-ndl-terms",
		Class:       ingest.RecordOnly,
		Attribution: "National Diet Library, NDL Search, https://ndlsearch.ndl.go.jp",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "ndl/1" }

// Dump returns the SRU server.
func (Adapter) Dump() string { return Base }

// Item is one record of NDL Search, as one line of the snapshot. A record of another provider has only the id, the provider and the ids ISBN and ISSN.
type Item struct {
	ID        string   `json:"id"`       // the id of NDL Search, as R100000002-I000010980401
	Provider  string   `json:"provider"` // the provider of the record, as R100000002
	ISBN      []string `json:"isbn,omitempty"`
	ISSN      []string `json:"issn,omitempty"`
	Title     string   `json:"title,omitempty"`
	Creators  []string `json:"creators,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	Date      string   `json:"date,omitempty"`
	Language  string   `json:"language,omitempty"`
	Edition   string   `json:"edition,omitempty"`
	Series    []string `json:"series,omitempty"`
	NDC       []string `json:"ndc,omitempty"`
}

// typed is a literal with a datatype, as an ISBN or an NDC class.
type typed struct {
	Type  string `xml:"datatype,attr"`
	Value string `xml:",chardata"`
}

type agent struct {
	Name string `xml:"Agent>name"`
}

type bibResource struct {
	About      string   `xml:"about,attr"`
	IDs        []typed  `xml:"http://purl.org/dc/terms/ identifier"`
	Title      string   `xml:"http://purl.org/dc/terms/ title"`
	Creators   []agent  `xml:"http://purl.org/dc/terms/ creator"`
	Publishers []agent  `xml:"http://purl.org/dc/terms/ publisher"`
	Issued     string   `xml:"http://purl.org/dc/terms/ issued"`
	Dates      []string `xml:"http://purl.org/dc/terms/ date"`
	Languages  []string `xml:"http://purl.org/dc/terms/ language"`
	Edition    string   `xml:"http://ndl.go.jp/dcndl/terms/ edition"`
	Series     []struct {
		Value string `xml:"Description>value"`
	} `xml:"http://ndl.go.jp/dcndl/terms/ seriesTitle"`
	Subjects []struct {
		Resource string `xml:"resource,attr"`
	} `xml:"http://purl.org/dc/terms/ subject"`
	Classes []typed `xml:"http://purl.org/dc/elements/1.1/ subject"`
}

type rdfDoc struct {
	Admin []struct {
		About    string `xml:"about,attr"`
		Category string `xml:"bibRecordCategory"`
	} `xml:"BibAdminResource"`
	Bib []bibResource `xml:"BibResource"`
}

// ndcResource finds the NDC class in the URI of a class, as 413.57 in http://id.ndl.go.jp/class/ndc9/413.57.
var ndcResource = regexp.MustCompile(`/class/ndc[0-9]*/([^/]+)$`)

// halfWidth changes the digits of full width, as ４１３, to ASCII digits.
var halfWidth = strings.NewReplacer("０", "0", "１", "1", "２", "2", "３", "3", "４", "4", "５", "5", "６", "6", "７", "7", "８", "8", "９", "9", "．", ".")

// Parse decodes one record of the schema dcndl. The second value is false when the record has no id.
func Parse(data []byte) (Item, bool, error) {
	var doc rdfDoc
	if err := oai.Unmarshal(data, &doc); err != nil {
		return Item{}, false, err
	}
	var it Item
	for _, a := range doc.Admin {
		if i := strings.LastIndex(a.About, "/books/"); i >= 0 && it.ID == "" {
			it.ID = strings.TrimSpace(a.About[i+len("/books/"):])
			it.Provider = strings.TrimSpace(a.Category)
		}
	}
	if it.ID == "" {
		return it, false, nil
	}
	if it.Provider == "" {
		it.Provider, _, _ = strings.Cut(it.ID, "-")
	}
	for _, b := range doc.Bib {
		for _, id := range b.IDs {
			switch strings.TrimPrefix(id.Type, "http://ndl.go.jp/dcndl/terms/") {
			case "ISBN":
				it.ISBN = add(it.ISBN, id.Value)
			case "ISSN", "ISSNL":
				it.ISSN = add(it.ISSN, id.Value)
			}
		}
		for _, s := range b.Subjects {
			if m := ndcResource.FindStringSubmatch(strings.TrimSpace(s.Resource)); m != nil {
				it.NDC = add(it.NDC, halfWidth.Replace(m[1]))
			}
		}
		for _, c := range b.Classes {
			if t := strings.TrimPrefix(c.Type, "http://ndl.go.jp/dcndl/terms/"); strings.HasPrefix(t, "NDC") {
				it.NDC = add(it.NDC, halfWidth.Replace(c.Value))
			}
		}
		if it.Title != "" || strings.TrimSpace(b.Title) == "" {
			continue
		}
		it.Title = strings.Join(strings.Fields(b.Title), " ")
		for _, c := range b.Creators {
			it.Creators = add(it.Creators, c.Name)
		}
		for _, p := range b.Publishers {
			if n := strings.TrimSpace(p.Name); n != "" && it.Publisher == "" {
				it.Publisher = n
			}
		}
		it.Date = strings.TrimSpace(b.Issued)
		for _, d := range b.Dates {
			if it.Date == "" && len(strings.TrimSpace(d)) >= 4 {
				it.Date = strings.TrimSpace(d)[:4]
			}
		}
		if len(b.Languages) > 0 {
			it.Language = strings.TrimSpace(b.Languages[0])
		}
		it.Edition = strings.TrimSpace(b.Edition)
		for _, s := range b.Series {
			it.Series = add(it.Series, s.Value)
		}
	}
	return it, true, nil
}

func add(list []string, v string) []string {
	if v = strings.TrimSpace(v); v != "" && !slices.Contains(list, v) {
		list = append(list, v)
	}
	return list
}

// Math reports whether a record has an NDC class that starts with 41, the class of mathematics.
func Math(it Item) bool {
	return slices.ContainsFunc(it.NDC, func(c string) bool { return strings.HasPrefix(c, "41") })
}

// ByNDL reports whether the NDL made the record.
func ByNDL(it Item) bool { return slices.Contains(Providers, it.Provider) }

// IDsOnly gives the record with only its id, its provider and its ids.
func IDsOnly(it Item) Item {
	return Item{ID: it.ID, Provider: it.Provider, ISBN: it.ISBN, ISSN: it.ISSN}
}

// Query gives the search of one NDC class, as ndc="413".
func Query(class string) string { return `ndc="` + class + `"` }

// Fetch reads the SRU server at req.URL and writes the records of mathematics as JSON Lines, compressed with gzip. When req.URL has the parameter query, Fetch makes that one search in place of the searches of Classes.
func (a Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	base, query, err := sru.Split(req.URL)
	if err != nil {
		return err
	}
	queries := []string{query}
	if query == "" {
		queries = queries[:0]
		for _, c := range Classes {
			queries = append(queries, Query(c))
		}
	}
	out := marc.NewLines(w)
	seen := map[string]bool{}
	for i, q := range queries {
		if i > 0 {
			if err := sru.Sleep(ctx, a.Gap); err != nil {
				return err
			}
		}
		params := url.Values{"version": {"1.2"}, "recordSchema": {"dcndl"}, "recordPacking": {"xml"}, "query": {q}}
		err := sru.Harvest(ctx, req.Get, sru.Query{Base: base, Params: params, Size: PageSize, Gap: a.Gap}, func(r sru.Record) error {
			it, ok, err := Parse(r.Data)
			if err != nil {
				return err
			}
			if !ok || seen[it.ID] || !Math(it) {
				return nil
			}
			seen[it.ID] = true
			if !ByNDL(it) {
				it = IDsOnly(it)
			}
			return out.Add(it)
		})
		if err != nil {
			return fmt.Errorf("ndl: %s: %w", q, err)
		}
	}
	return out.Close()
}

func each(r io.Reader, fn func(Item) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 64<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var it Item
		if err := json.Unmarshal(sc.Bytes(), &it); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if it.ID == "" {
			return fmt.Errorf("line %d: the record has no id", line)
		}
		if err := fn(it); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Link gives the page of a record in NDL Search.
func Link(it Item) string { return "https://ndlsearch.ndl.go.jp/books/" + it.ID }

// Read reads the records and emits one ingest record for each record. A record of another provider has no title.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(it Item) error {
		return emit(ingest.Record{ID: it.ID, Kind: "monograph", Title: it.Title, URL: Link(it), Meta: map[string]string{"provider": it.Provider}})
	})
}

// Records reads the records and emits one source record for each record that the NDL made (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(it Item) error {
		if !ByNDL(it) || it.Title == "" {
			return nil
		}
		return emit(SourceRecord(it))
	})
}

// SourceRecord makes the source record of one record that the NDL made. The record describes one edition, so its level is manifestation. The record names no role, so each creator has the role author.
func SourceRecord(it Item) cluster.Record {
	rec := cluster.Record{NativeID: it.ID, Level: "manifestation", Kind: "monograph", Title: it.Title, Date: it.Date, Language: fetch.Lang(it.Language), Edition: it.Edition, Publisher: it.Publisher,
		IDs: resource.IDs{}, Links: []string{Link(it)}}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("isbn", it.ISBN...)
	put("issn", it.ISSN...)
	for _, c := range it.Creators {
		rec.Creators = append(rec.Creators, resource.Creator{Name: c, Role: "author"})
	}
	for _, c := range it.NDC {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "ndc", Code: c})
	}
	return rec
}
