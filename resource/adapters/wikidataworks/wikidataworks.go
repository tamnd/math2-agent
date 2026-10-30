// Package wikidataworks is the adapter of the works of Wikidata (spec corpus resources RS4.1 and RS6.1). The adapter ingest/wikidata reads the concepts of Wikidata. This adapter reads the works whose main subject (P921) is mathematics, a subclass of it, or an area of mathematics, and it gives an ingest record and a source record for each work.
//
// Fetch asks the SPARQL service for the works in pages of 2000, in the order of the item, and writes one JSON object for each work and each line, compressed with gzip. The object has the item, the title, the date, the DOI, the zbMATH id, the arXiv id, the ISBN, the language, the names of the authors, the main subjects and the classes. The data of Wikidata is under CC0.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/wikidataworks"
package wikidataworks

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Endpoint is the SPARQL service of Wikidata.
const Endpoint = "https://query.wikidata.org/sparql"

// MathItems is the SPARQL pattern of the items of mathematics in ?s.
const MathItems = `{ ?s wdt:P279* wd:Q395 } UNION { ?s wdt:P31 wd:Q1936384 }`

// PageSize is the number of works in one page.
const PageSize = 2000

// Query is the query of one page. The arguments are MathItems, the page size and the offset.
const Query = `SELECT ?x (SAMPLE(?t) AS ?title) (SAMPLE(?d) AS ?date) (SAMPLE(?doi) AS ?doi) (SAMPLE(?zb) AS ?zbmath) (SAMPLE(?ax) AS ?arxiv) (SAMPLE(?isbn) AS ?isbn) (SAMPLE(?lc) AS ?language)
 (GROUP_CONCAT(DISTINCT ?an; separator="|") AS ?authors) (GROUP_CONCAT(DISTINCT STRAFTER(STR(?s), "entity/"); separator=" ") AS ?subjects) (GROUP_CONCAT(DISTINCT STRAFTER(STR(?k), "entity/"); separator=" ") AS ?kinds)
WHERE {
 { SELECT DISTINCT ?x WHERE { %s ?x wdt:P921 ?s } ORDER BY ?x LIMIT %d OFFSET %d }
 ?x wdt:P921 ?s .
 OPTIONAL { ?x wdt:P1476 ?t } OPTIONAL { ?x wdt:P577 ?d } OPTIONAL { ?x wdt:P356 ?doi } OPTIONAL { ?x wdt:P894 ?zb }
 OPTIONAL { ?x wdt:P818 ?ax } OPTIONAL { ?x wdt:P212 ?isbn } OPTIONAL { ?x wdt:P31 ?k } OPTIONAL { ?x wdt:P2093 ?an }
 OPTIONAL { ?x wdt:P407 ?l . ?l wdt:P218 ?lc }
} GROUP BY ?x`

// Adapter is the adapter of the works of Wikidata.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the works of Wikidata.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "wikidata_works",
		Name:        "Wikidata, the works of mathematics",
		Home:        "https://www.wikidata.org",
		Licence:     "CC0-1.0",
		Class:       ingest.Open,
		Attribution: "Wikidata, the Wikimedia Foundation, https://www.wikidata.org, CC0 1.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "wikidata_works/1" }

// Dump returns the SPARQL service.
func (Adapter) Dump() string { return Endpoint }

// Work is one work, as one line of the snapshot.
type Work struct {
	Item     string `json:"item"`
	Title    string `json:"title,omitempty"`
	Date     string `json:"date,omitempty"`
	DOI      string `json:"doi,omitempty"`
	ZBMath   string `json:"zbmath,omitempty"`
	ArXiv    string `json:"arxiv,omitempty"`
	ISBN     string `json:"isbn,omitempty"`
	Language string `json:"language,omitempty"`
	Authors  string `json:"authors,omitempty"`  // the names, with | between them
	Subjects string `json:"subjects,omitempty"` // the main subjects, with a space between them
	Kinds    string `json:"kinds,omitempty"`    // the classes, with a space between them
}

// Fetch asks the service at req.URL for each page of the works and writes the works as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	for off := 0; ; off += PageSize {
		q := url.Values{"query": {fmt.Sprintf(Query, MathItems, PageSize, off)}, "format": {"json"}}
		body, err := req.Get(ctx, req.URL+"?"+q.Encode())
		if err != nil {
			return err
		}
		var res struct {
			Results struct {
				Bindings []map[string]struct {
					Value string `json:"value"`
				} `json:"bindings"`
			} `json:"results"`
		}
		err = json.NewDecoder(body).Decode(&res)
		_ = body.Close()
		if err != nil {
			return fmt.Errorf("the page at the offset %d: %w", off, err)
		}
		for _, b := range res.Results.Bindings {
			v := func(k string) string { return b[k].Value }
			wk := Work{Item: strings.TrimPrefix(v("x"), "http://www.wikidata.org/entity/"), Title: v("title"), Date: v("date"), DOI: v("doi"), ZBMath: v("zbmath"),
				ArXiv: v("arxiv"), ISBN: v("isbn"), Language: v("language"), Authors: v("authors"), Subjects: v("subjects"), Kinds: v("kinds")}
			if err := enc.Encode(wk); err != nil {
				return err
			}
		}
		if len(res.Results.Bindings) < PageSize {
			return zw.Close()
		}
	}
}

func each(r io.Reader, fn func(Work) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var w Work
		if err := json.Unmarshal(sc.Bytes(), &w); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if w.Item == "" {
			return fmt.Errorf("line %d: the work has no item", line)
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
		return emit(ingest.Record{ID: w.Item, Kind: SourceRecord(w).Kind, Title: w.Title, URL: "https://www.wikidata.org/wiki/" + w.Item})
	})
}

// Records reads the works and emits one source record for each work (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(w Work) error { return emit(SourceRecord(w)) })
}

// Kinds maps the classes of Wikidata to the kinds of RS2.3, in the order of preference.
var Kinds = []struct{ Class, Kind string }{
	{"Q83790", "textbook"}, {"Q580922", "preprint"}, {"Q13442814", "article"}, {"Q1266946", "thesis"}, {"Q187685", "thesis"},
	{"Q1980247", "chapter"}, {"Q23927052", "conference-paper"}, {"Q1143604", "proceedings"}, {"Q5633421", "journal"}, {"Q7397", "software"},
	{"Q571", "monograph"}, {"Q7725634", "monograph"}, {"Q47461344", "monograph"},
}

// SourceRecord makes the source record of one work. A work with no known class has the kind article. The main subjects go into the subjects with the system wikidata, for the test S7 of RS9.2.
func SourceRecord(w Work) cluster.Record {
	rec := cluster.Record{
		NativeID: w.Item,
		Level:    "work",
		Kind:     "article",
		Title:    strings.Join(strings.Fields(w.Title), " "),
		Language: w.Language,
		IDs:      resource.IDs{},
		Links:    []string{"https://www.wikidata.org/wiki/" + w.Item},
	}
	classes := strings.Fields(w.Kinds)
	for _, k := range Kinds {
		if slices.Contains(classes, k.Class) {
			rec.Kind = k.Kind
			break
		}
	}
	if len(w.Date) >= 10 {
		rec.Date = strings.TrimSuffix(strings.TrimSuffix(w.Date[:10], "-01"), "-01")
	}
	for key, v := range map[string]string{"wikidata": w.Item, "doi": w.DOI, "zbl": w.ZBMath, "arxiv": w.ArXiv, "isbn": w.ISBN} {
		if n, err := ids.Normalize(key, v); err == nil {
			rec.IDs[key] = resource.Values{n}
		}
	}
	for a := range strings.SplitSeq(w.Authors, "|") {
		if a = strings.TrimSpace(a); a != "" {
			rec.Creators = append(rec.Creators, resource.Creator{Name: a, Role: "author"})
		}
	}
	for _, s := range strings.Fields(w.Subjects) {
		rec.Subjects = append(rec.Subjects, resource.Subject{System: "wikidata", Code: s})
	}
	return rec
}
