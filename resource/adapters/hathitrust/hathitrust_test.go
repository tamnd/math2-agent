package hathitrust

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func row(vs ...string) string {
	r := make([]string, len(Fields))
	for i := 0; i+1 < len(vs); i += 2 {
		for j, f := range Fields {
			if f == vs[i] {
				r[j] = vs[i+1]
			}
		}
	}
	return strings.Join(r, "\t") + "\n"
}

func TestFetchAndRecords(t *testing.T) {
	root := t.TempDir()
	rs := []resource.Resource{
		{ID: "r-hardy", Kind: "monograph", Title: "A course of pure mathematics", IDs: resource.IDs{"oclc": {"2779601"}}},
		{ID: "r-dup", Kind: "monograph", Title: "Gone", IDs: resource.IDs{"isbn": {"9788081281587"}}, Scope: &resource.Scope{Value: resource.Duplicate, Target: "r-hardy"}},
		{ID: "r-lccn", Kind: "monograph", Title: "Tables", Expressions: []resource.Expression{{Key: "e1", Manifestations: []resource.Manifestation{{IDs: resource.IDs{"lccn": {"70518371"}}}}}}},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	_, _ = io.WriteString(zw, row("htid", "mdp.39015018415946", "access", "allow", "rights", "pd", "ht_bib_key", "000000001", "description", "v.1", "oclc_num", "2779601,99", "title", "A course of pure mathematics / by G. H. Hardy.",
		"imprint", "Cambridge University Press, 1908.", "rights_date_used", "1908", "lang", "eng", "bib_fmt", "BK", "author", "Hardy, G. H. 1877-1947."))
	_, _ = io.WriteString(zw, row("htid", "mdp.39015018415947", "access", "deny", "rights", "ic", "ht_bib_key", "000000001", "description", "v.2", "oclc_num", "2779601", "title", "A course of pure mathematics / by G. H. Hardy."))
	_, _ = io.WriteString(zw, row("htid", "uc1.b1", "ht_bib_key", "000000002", "isbn", "9788081281587", "title", "Only the duplicate has this ISBN"))
	_, _ = io.WriteString(zw, row("htid", "uc1.b2", "ht_bib_key", "000000003", "lccn", "70518371", "title", "Tables", "rights_date_used", "9999", "bib_fmt", "SE"))
	_ = zw.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/list.json":
			_, _ = io.WriteString(w, `[{"filename":"hathi_full_20260901.txt.gz","full":true,"url":"`+"http://"+r.Host+`/old.txt.gz"},{"filename":"hathi_upd_20261002.txt.gz","full":false,"url":"x"},{"filename":"hathi_full_20261001.txt.gz","full":true,"url":"`+"http://"+r.Host+`/full.txt.gz"}]`)
		case "/full.txt.gz":
			_, _ = w.Write(gz.Bytes())
		default:
			t.Errorf("path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	get := func(ctx context.Context, u string) (io.ReadCloser, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	var buf bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/list.json", Root: root, Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var out []cluster.Record
	if err := (Adapter{}).Records(snap, func(r cluster.Record) error { out = append(out, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("records %+v", out)
	}
	r := out[0]
	if r.NativeID != "000000001" || r.Kind != "monograph" || r.Level != "manifestation" || r.Title != "A course of pure mathematics" || r.Date != "1908" || r.Language != "en" ||
		r.Publisher != "Cambridge University Press" || r.Links[0] != "https://catalog.hathitrust.org/Record/000000001" {
		t.Errorf("record %+v", r)
	}
	if len(r.IDs["hathi"]) != 2 || r.IDs["hathi_record"][0] != "1" || len(r.IDs["oclc"]) != 2 || len(r.Creators) != 1 || r.Creators[0].Name != "Hardy, G. H." {
		t.Errorf("ids %+v creators %+v", r.IDs, r.Creators)
	}
	if r := out[1]; r.NativeID != "000000003" || r.Kind != "journal" || r.Date != "" || r.IDs["lccn"][0] != "70518371" {
		t.Errorf("record %+v", r)
	}
}

func TestEmptyTree(t *testing.T) {
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: "x", Root: t.TempDir()}, io.Discard); err == nil {
		t.Error("a tree with no resource gives no error")
	}
}
