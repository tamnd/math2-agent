package zbmathkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

const ttl = `@prefix dcterms: <http://purl.org/dc/terms/> .
@prefix msc: <http://msc2020.org/> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix schema: <https://schema.org/> .
@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .
@prefix zbmath: <https://zbmath.org/> .

<https://zbmath.org/doctype/journal-article> a rdfs:Class ;
    rdfs:label "Journal Article" .

# a comment
zbmath:3851025 a schema:ScholarlyArticle ;
    rdfs:label "List of works of Béla Gyires" ;
    dcterms:creator <https://zbmath.org/author/Gyires_B>,
        <https://zbmath.org/author/Varga_T> ;
    dcterms:identifier "0536.01021" ;
    dcterms:issued "1982"^^xsd:gYear ;
    dcterms:language "Hungarian" ;
    dcterms:subject msc:01A70,
        msc:01A60 ;
    dcterms:title "List of works of Béla Gyires" ;
    dcterms:type <https://zbmath.org/doctype/journal-article> ;
    schema:identifier [ a schema:PropertyValue ;
            schema:propertyID "zbl_id" ;
            schema:value "0536.01021" ],
        <https://doi.org/10.1121/1.390395> ;
    schema:pagination "1-3" ;
    schema:reviewBody """A long review with "quotes"
on two lines.""" .

zbmath:5017555 a schema:ScholarlyArticle ;
    rdfs:label """Proceedings of the 2nd Summer School (M\\&MKT 2004), Porto Ercole,
June 6-12, 2004""" ;
    dcterms:type <https://zbmath.org/doctype/book-series> ;
    schema:pagination "104 S." .

<https://zbmath.org/author/Gyires_B> a schema:Person ;
    schema:authorOf zbmath:3851025 ;
    schema:name "Gyires, Béla" .
`

func dump(t *testing.T) []byte {
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	tw := tar.NewWriter(zw)
	for _, f := range []struct{ name, body string }{{"out/", ""}, {"out/out-1.ttl", ttl}, {"out/README", "x"}} {
		h := &tar.Header{Name: f.name, Mode: 0o644, Size: int64(len(f.body)), Typeflag: tar.TypeReg}
		if strings.HasSuffix(f.name, "/") {
			h.Typeflag = tar.TypeDir
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestFetch(t *testing.T) {
	data := dump(t)
	get := func(_ context.Context, u string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	var out bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: Dump, Get: get}, &out); err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(&out)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(bytes.NewReader(snap), func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("records %+v", rs)
	}
	a := rs[0]
	if a.NativeID != "3851025" || a.Kind != "article" || a.Title != "List of works of Béla Gyires" || a.Date != "1982" || a.Language != "hu" || a.Page != "1" {
		t.Errorf("article %+v", a)
	}
	if a.IDs["zbl_id"][0] != "3851025" || a.IDs["zbl"][0] != "0536.01021" || a.IDs["doi"][0] != "10.1121/1.390395" {
		t.Errorf("ids %+v", a.IDs)
	}
	if strings.Join(a.MSC, " ") != "01A70 01A60" || len(a.Creators) != 1 || a.Creators[0].Name != "Gyires, Béla" {
		t.Errorf("article %+v", a)
	}
	b := rs[1]
	if b.Kind != "monograph" || b.Title != "Proceedings of the 2nd Summer School (M\\&MKT 2004), Porto Ercole, June 6-12, 2004" || b.Page != "" {
		t.Errorf("book %+v", b)
	}
	var recs []ingest.Record
	if err := (Adapter{}).Read(bytes.NewReader(snap), func(r ingest.Record) error { recs = append(recs, r); return nil }); err != nil || len(recs) != 2 {
		t.Errorf("read %+v, %v", recs, err)
	}
}

func TestHexRune(t *testing.T) {
	for _, c := range []struct {
		hex  string
		want rune
		ok   bool
	}{{"00e9", 'é', true}, {"0001F600", '😀', true}, {"00g9", 0, false}, {"0000D800", 0, false}, {"FFFFFFFF", 0, false}} {
		if r, ok := hexRune([]byte(c.hex)); r != c.want || ok != c.ok {
			t.Errorf("%s: %q, %v", c.hex, r, ok)
		}
	}
}

func TestParserErrors(t *testing.T) {
	for _, s := range []string{
		`zbmath:1 a schema:ScholarlyArticle .`,
		`<https://zbmath.org/1> <https://schema.org/name> "no end .`,
		`<https://zbmath.org/1> <https://schema.org/name> "x" <https://schema.org/y> .`,
	} {
		if err := file(context.Background(), strings.NewReader(s), nil); err == nil {
			t.Errorf("%q gives no error", s)
		}
	}
}
