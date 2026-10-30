// Package eudml is the adapter of EuDML, the European Digital Mathematics Library (spec corpus resources RS6.1 and RS8.3, phase R3). It reads each document of EuDML, and it gives an ingest record and a source record for each document.
//
// Fetch harvests the OAI-PMH endpoint of EuDML in the format oai_dc, which each OAI-PMH endpoint must give, and writes one JSON object for each document and each line, compressed with gzip. Each document of EuDML is a work of mathematics, so Fetch keeps each record. The format oai_dc gives the ids of zbMATH Open and MathSciNet as links in the identifiers and the relations. The licence of the metadata of EuDML is not read (RS8.3), so the adapter keeps the class record only. The adapter sends no email and no key.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/eudml"
package eudml

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
	"github.com/tamnd/math2-agent/resource/adapters/oai"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Endpoint is the OAI-PMH endpoint of EuDML.
const Endpoint = "https://eudml.org/oai/OAIHandler"

// Prefix is the metadata format that Fetch asks for.
const Prefix = "oai_dc"

// Adapter is the adapter of EuDML.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of EuDML.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "eudml",
		Name:        "EuDML, the European Digital Mathematics Library",
		Home:        "https://eudml.org",
		Licence:     "LicenseRef-unknown",
		Class:       ingest.RecordOnly,
		Attribution: "EuDML, https://eudml.org",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "eudml/1" }

// Dump returns the OAI-PMH endpoint.
func (Adapter) Dump() string { return Endpoint }

// Document is one document, as one line of the snapshot.
type Document struct {
	ID        string   `json:"id"` // the number of the document, as 94429 in urn:eudml:doc:94429
	Type      string   `json:"type,omitempty"`
	Title     string   `json:"title,omitempty"`
	Language  string   `json:"language,omitempty"`
	Date      string   `json:"date,omitempty"`
	Publisher string   `json:"publisher,omitempty"`
	DOI       []string `json:"doi,omitempty"`
	ZBL       []string `json:"zbl,omitempty"`
	MR        []string `json:"mr,omitempty"`
	ISSN      []string `json:"issn,omitempty"`
	ISBN      []string `json:"isbn,omitempty"`
	Authors   []string `json:"authors,omitempty"`
}

var (
	// docID finds the number of a document of EuDML, as 94429 in urn:eudml:doc:94429, in https://eudml.org/doc/94429 or in doc/94429.
	docID = regexp.MustCompile(`(?:urn:eudml:doc:|eudml\.org/doc/|^doc/|^doc:)([1-9][0-9]*)$`)
	doi   = regexp.MustCompile(`(?i)(?:^|doi\.org/|^doi:\s*)(10\.[0-9]{4,9}/\S+)`)
	zbl   = regexp.MustCompile(`(?i)(?:zbmath\.org/\?q=an:|zentralblatt-math\.org/zmath/en/search/\?q=an:|[?&]an=|^zbl:?\s*)([0-9]{4}\.[0-9]{5})`)
	mr    = regexp.MustCompile(`(?i)(?:mathscinet-getitem\?mr=|mathscinet/search/publdoc\.html\?.*?mr=|^mr:?\s*)([0-9]+)`)
	issn  = regexp.MustCompile(`(?i)^(?:issn:?\s*)([0-9]{4}-?[0-9]{3}[0-9x])$`)
	isbn  = regexp.MustCompile(`(?i)^(?:isbn:?\s*)([0-9][0-9 -]{8,16}[0-9x])$`)
	year  = regexp.MustCompile(`^[0-9]{4}`)
)

// Fetch harvests the endpoint at req.URL and writes the documents as JSON Lines, compressed with gzip.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	enc.SetEscapeHTML(false)
	err := oai.Harvest(ctx, req.Get, req.URL, Prefix, "", func(r oai.Record) error {
		if r.Deleted {
			return nil
		}
		var m oai.DC
		if err := oai.Unmarshal(r.Metadata, &m); err != nil {
			return fmt.Errorf("%s: %w", r.ID, err)
		}
		d := FromDC(r.ID, m)
		if d.ID == "" {
			return nil
		}
		return enc.Encode(d)
	})
	if err != nil {
		return err
	}
	return zw.Close()
}

// FromDC makes a document from the OAI identifier and the metadata of a record. The id is empty when the record gives no number of a document of EuDML.
func FromDC(oaiID string, m oai.DC) Document {
	var d Document
	add := func(list *[]string, v string) {
		if !slices.Contains(*list, v) {
			*list = append(*list, v)
		}
	}
	local := oaiID
	if _, after, ok := strings.Cut(strings.TrimPrefix(oaiID, "oai:"), ":"); ok {
		local = after
	}
	for _, v := range append([]string{local}, append(oai.Texts(m.Identifier), oai.Texts(m.Relation)...)...) {
		if x := docID.FindStringSubmatch(v); x != nil && d.ID == "" {
			d.ID = x[1]
		}
		if x := doi.FindStringSubmatch(v); x != nil {
			add(&d.DOI, x[1])
		}
		if x := zbl.FindStringSubmatch(v); x != nil {
			add(&d.ZBL, x[1])
		}
		if x := mr.FindStringSubmatch(v); x != nil {
			add(&d.MR, x[1])
		}
		if x := issn.FindStringSubmatch(v); x != nil {
			add(&d.ISSN, x[1])
		}
		if x := isbn.FindStringSubmatch(v); x != nil {
			add(&d.ISBN, x[1])
		}
	}
	if t := oai.Texts(m.Title); len(t) > 0 {
		d.Title = t[0]
	}
	if t := oai.Texts(m.Type); len(t) > 0 {
		d.Type = t[0]
	}
	if t := oai.Texts(m.Language); len(t) > 0 {
		d.Language = t[0]
	}
	if t := oai.Texts(m.Publisher); len(t) > 0 {
		d.Publisher = t[0]
	}
	for _, v := range oai.Texts(m.Date) {
		if year.MatchString(v) {
			d.Date, _, _ = strings.Cut(v, "T")
			break
		}
	}
	d.Authors = oai.Texts(m.Creator)
	return d
}

func each(r io.Reader, fn func(Document) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 16<<20)
	for line := 1; sc.Scan(); line++ {
		if len(strings.TrimSpace(sc.Text())) == 0 {
			continue
		}
		var d Document
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if d.ID == "" {
			return fmt.Errorf("line %d: the document has no id", line)
		}
		if err := fn(d); err != nil {
			return err
		}
	}
	return sc.Err()
}

func link(d Document) string { return "https://eudml.org/doc/" + d.ID }

// kinds maps the types of oai_dc to the kinds of resources.
var kinds = map[string]string{
	"article": "article", "text": "article", "journal article": "article", "book": "monograph", "monograph": "monograph", "chapter": "chapter",
	"book chapter": "chapter", "thesis": "thesis", "proceedings": "proceedings", "conference paper": "conference-paper", "conference object": "conference-paper",
}

// Kind gives the kind of a document by its type. A document with no known type is an article, because most documents of EuDML are articles.
func Kind(d Document) string {
	if k, ok := kinds[strings.ToLower(d.Type)]; ok {
		return k
	}
	return "article"
}

// Read reads the documents and emits one ingest record for each document.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(d Document) error {
		return emit(ingest.Record{ID: d.ID, Kind: Kind(d), Title: d.Title, URL: link(d)})
	})
}

// Records reads the documents and emits one source record for each document (RS13.1).
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(d Document) error { return emit(SourceRecord(d)) })
}

// SourceRecord makes the source record of one document.
func SourceRecord(d Document) cluster.Record {
	rec := cluster.Record{NativeID: d.ID, Level: "work", Kind: Kind(d), Title: d.Title, Language: fetch.Lang(d.Language), Date: d.Date, IDs: resource.IDs{}, Links: []string{link(d)}}
	put := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	put("eudml", d.ID)
	put("doi", d.DOI...)
	put("zbl", d.ZBL...)
	put("mr", d.MR...)
	put("isbn", d.ISBN...)
	for _, s := range d.ISSN {
		if n, err := ids.Normalize("issn", s); err == nil {
			rec.Venue = n
			break
		}
	}
	if rec.Kind == "monograph" || rec.Kind == "proceedings" {
		rec.Publisher = d.Publisher
	}
	for _, name := range d.Authors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: name, Role: "author"})
	}
	return rec
}
