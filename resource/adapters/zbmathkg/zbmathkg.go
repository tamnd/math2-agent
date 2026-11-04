// Package zbmathkg is the adapter of the zbMATH Open Knowledge Graph (spec corpus resources RS4.1), the RDF dump of zbMATH Open on Zenodo. The dump is one tar file, compressed with gzip, with files in Turtle. It gives the same documents as the OAI-PMH interface of zbMATH Open, but in one download, and a harvest of the interface takes days.
//
// Fetch reads the dump as a stream and writes one line of JSON for each document, with the names of its authors. The full dump has more than ten gigabytes of Turtle, and the snapshot keeps only the fields that Records reads. Each file of the dump gives the people of its documents after the documents, so Fetch keeps the documents of one file until the end of the file.
//
// The source has the lineage zbmath, because the dump is made from the OAI-PMH interface of zbMATH Open. The data of zbMATH Open is under CC BY-SA 4.0.
//
// The m2 command imports this package for its side effect:
//
//	import _ "github.com/tamnd/math2-agent/resource/adapters/zbmathkg"
package zbmathkg

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/ingest/zbmath"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/ids"
)

// Dump is the dump of the zbMATH Open Knowledge Graph of 2026 on Zenodo.
const Dump = "https://zenodo.org/api/records/21497976/files/zbmath-kg-ttl-2026.tar.gz/content"

// The IRIs that the adapter reads.
const (
	rdfType     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#type"
	dcterms     = "http://purl.org/dc/terms/"
	schemaOrg   = "https://schema.org/"
	rdfsLabel   = "http://www.w3.org/2000/01/rdf-schema#label"
	docTypes    = "https://zbmath.org/doctype/"
	mscPrefix   = "http://msc2020.org/"
	doiPrefix   = "https://doi.org/"
	articleType = schemaOrg + "ScholarlyArticle"
	personType  = schemaOrg + "Person"
)

// docIRI matches the IRI of a document, with the document id.
var docIRI = regexp.MustCompile(`^https://zbmath\.org/([0-9]+)$`)

// kinds maps the document types of the dump to the kinds of RS2.3.
var kinds = map[string]string{"journal-article": "article", "book-series": "monograph", "collection-article": "chapter", "preprints": "preprint"}

// Adapter is the adapter of the zbMATH Open Knowledge Graph.
type Adapter struct{}

func init() { ingest.Register(Adapter{}) }

// Source returns the catalogue entry of the zbMATH Open Knowledge Graph.
func (Adapter) Source() ingest.Source {
	return ingest.Source{
		ID:          "zbmath-kg",
		Name:        "zbMATH Open Knowledge Graph",
		Home:        "https://github.com/zbMATHOpen/zbmath-open-kg",
		Licence:     "CC-BY-SA-4.0",
		Class:       ingest.Open,
		Lineage:     []string{"zbmath"},
		Attribution: "zbMATH Open Knowledge Graph, FIZ Karlsruhe, https://doi.org/10.5281/zenodo.21497975, CC BY-SA 4.0",
	}
}

// Version returns the version of the adapter.
func (Adapter) Version() string { return "zbmath-kg/1" }

// Dump returns the URL of the dump.
func (Adapter) Dump() string { return Dump }

// Doc is one document, as one line of the snapshot.
type Doc struct {
	ID       string   `json:"id"`
	Zbl      string   `json:"zbl,omitempty"`
	Kind     string   `json:"kind,omitempty"`
	Title    string   `json:"title"`
	Year     string   `json:"year,omitempty"`
	Language string   `json:"language,omitempty"`
	MSC      []string `json:"msc,omitempty"`
	DOI      []string `json:"doi,omitempty"`
	Authors  []string `json:"authors,omitempty"`
	Pages    string   `json:"pages,omitempty"`
	creators []string
}

// Fetch reads the dump at req.URL as a stream and writes one line of JSON for each document, compressed with gzip. It gets the dump into the cache first (ingest.Spool), because the server can reset a connection that is read slowly.
func (Adapter) Fetch(ctx context.Context, req *ingest.Request, w io.Writer) error {
	body, err := ingest.Spool(ctx, req, req.URL)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	zr, err := gzip.NewReader(body)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(w)
	if err := Convert(ctx, zr, zw); err != nil {
		return err
	}
	return zw.Close()
}

// Convert reads a tar file of Turtle files and writes one line of JSON for each document.
func Convert(ctx context.Context, r io.Reader, w io.Writer) error {
	tr := tar.NewReader(r)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Typeflag != tar.TypeReg || !strings.HasSuffix(h.Name, ".ttl") {
			continue
		}
		if err := file(ctx, tr, enc); err != nil {
			return fmt.Errorf("%s: %w", h.Name, err)
		}
	}
}

// file reads one Turtle file and writes its documents, in the order of the file.
func file(ctx context.Context, r io.Reader, enc *json.Encoder) error {
	p := newParser(r)
	var docs []*Doc
	names := map[string]string{}
	for n := 0; ; n++ {
		if n%100000 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		s, err := p.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		types := s.props[rdfType]
		switch {
		case slices.ContainsFunc(types, func(t term) bool { return t.iri == personType }):
			if name := s.first(schemaOrg + "name"); name != "" {
				names[s.subject] = name
			}
		case slices.ContainsFunc(types, func(t term) bool { return t.iri == articleType }):
			if d := document(s); d != nil {
				docs = append(docs, d)
			}
		}
	}
	for _, d := range docs {
		for _, c := range d.creators {
			if n := names[c]; n != "" && !slices.Contains(d.Authors, n) {
				d.Authors = append(d.Authors, n)
			}
		}
		if err := enc.Encode(d); err != nil {
			return err
		}
	}
	return nil
}

// document makes the document of a statement, or nil when the subject is not the IRI of a document.
func document(s statement) *Doc {
	m := docIRI.FindStringSubmatch(s.subject)
	if m == nil {
		return nil
	}
	d := &Doc{ID: m[1], Zbl: s.first(dcterms + "identifier"), Title: s.first(dcterms + "title"), Year: s.first(dcterms + "issued"), Pages: s.first(schemaOrg + "pagination")}
	if d.Title == "" {
		d.Title = s.first(rdfsLabel)
	}
	if d.Year == "" {
		d.Year = s.first(schemaOrg + "datePublished")
	}
	if f := strings.Fields(s.first(dcterms + "language")); len(f) > 0 {
		d.Language = f[0]
	}
	d.Kind = kinds[strings.TrimPrefix(s.first(dcterms+"type"), docTypes)]
	for _, t := range s.props[dcterms+"subject"] {
		if c, ok := strings.CutPrefix(t.iri, mscPrefix); ok && !slices.Contains(d.MSC, c) {
			d.MSC = append(d.MSC, c)
		}
	}
	for _, t := range slices.Concat(s.props[schemaOrg+"sameAs"], s.props[schemaOrg+"identifier"]) {
		if v, ok := strings.CutPrefix(t.iri, doiPrefix); ok && !slices.Contains(d.DOI, v) {
			d.DOI = append(d.DOI, v)
		}
		if t.blank != nil && d.Zbl == "" && (statement{props: t.blank}).first(schemaOrg+"propertyID") == "zbl_id" {
			d.Zbl = (statement{props: t.blank}).first(schemaOrg + "value")
		}
	}
	for _, t := range slices.Concat(s.props[dcterms+"creator"], s.props[schemaOrg+"author"]) {
		if t.iri != "" && !slices.Contains(d.creators, t.iri) {
			d.creators = append(d.creators, t.iri)
		}
	}
	return d
}

func each(r io.Reader, fn func(d *Doc) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(nil, 64<<20)
	for line := 1; sc.Scan(); line++ {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var d Doc
		if err := json.Unmarshal(sc.Bytes(), &d); err != nil {
			return fmt.Errorf("line %d: %w", line, err)
		}
		if d.ID == "" {
			return fmt.Errorf("line %d: a document has no id", line)
		}
		if err := fn(&d); err != nil {
			return err
		}
	}
	return sc.Err()
}

// Read reads the snapshot and emits one ingest record for each document.
func (Adapter) Read(r io.Reader, emit func(ingest.Record) error) error {
	return each(r, func(d *Doc) error {
		return emit(ingest.Record{ID: d.ID, Kind: kind(d), Title: d.Title, URL: "https://zbmath.org/?q=an:" + d.ID})
	})
}

// Records reads the snapshot and emits one source record for each document (RS13.1). The ids are the same as the ids of the adapter zbmath: the document id as zbl_id and the Zbl number as zbl.
func (Adapter) Records(r io.Reader, emit func(cluster.Record) error) error {
	return each(r, func(d *Doc) error { return emit(SourceRecord(d)) })
}

func kind(d *Doc) string {
	if d.Kind == "" {
		return "article"
	}
	return d.Kind
}

// SourceRecord makes the source record of a document. A document of an unknown type has the kind article. The first page comes from the pages of an article or a chapter, as 206 from 206-209.
func SourceRecord(d *Doc) cluster.Record {
	rec := cluster.Record{
		NativeID: d.ID,
		Level:    "work",
		Kind:     kind(d),
		Title:    strings.Join(strings.Fields(d.Title), " "),
		Date:     d.Year,
		Language: zbmath.Language(d.Language),
		MSC:      d.MSC,
		IDs:      resource.IDs{},
		Links:    []string{"https://zbmath.org/?q=an:" + d.ID},
	}
	for _, n := range d.Authors {
		rec.Creators = append(rec.Creators, resource.Creator{Name: n, Role: "author"})
	}
	add := func(key string, vs ...string) {
		for _, v := range vs {
			if n, err := ids.Normalize(key, v); err == nil && !slices.Contains(rec.IDs[key], n) {
				rec.IDs[key] = append(rec.IDs[key], n)
			}
		}
	}
	add("zbl_id", d.ID)
	add("zbl", d.Zbl)
	add("doi", d.DOI...)
	if rec.Kind == "article" || rec.Kind == "chapter" {
		if p, _, _ := strings.Cut(d.Pages, "-"); p != "" && !strings.ContainsAny(p, " ,") {
			rec.Page = p
		}
	}
	return rec
}
