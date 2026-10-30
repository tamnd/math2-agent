package gutenberg

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

const catalogue = "Text#,Type,Issued,Title,Language,Authors,Subjects,LoCC,Bookshelves\n" +
	"33283,Text,2010-10-09,\"Calculus Made\n Easy\",en,\"Thompson, Silvanus P. (Silvanus Phillips), 1851-1916; Gardner, Martin, 1914-2010 [Editor]\",Calculus -- Textbooks,QA,Category: Mathematics\n" +
	"50,Dataset,1993-01-01,Pi,en,\"Hemphill, Scott\",Mathematics; Pi,QA,Mathematics\n" +
	"1019,Text,1997-08-01,Poems,fr,\"Bell, Currer\",English poetry,PR,Category: Poetry\n" +
	"7,Text,2001-01-01,Elemente,de,\"Euclid, 325? BCE-265? BCE; Lorenz, J. F. [Translator]\",Geometry,,\n"

func TestFetchAndRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, catalogue) }))
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
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(snap, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.NativeID != "33283" || r.Kind != "monograph" || r.Level != "manifestation" || r.Title != "Calculus Made Easy" || r.Language != "en" || r.IDs["gutenberg"][0] != "33283" ||
		len(r.Subjects) != 1 || r.Subjects[0].Code != "QA" || r.Date != "" {
		t.Errorf("record %+v", r)
	}
	if len(r.Creators) != 2 || r.Creators[0].Name != "Thompson, Silvanus P." || r.Creators[0].Role != "author" || r.Creators[1].Name != "Gardner, Martin" || r.Creators[1].Role != "editor" {
		t.Errorf("creators %+v", r.Creators)
	}
	r = rs[1]
	if r.NativeID != "7" || r.Language != "de" || len(r.Creators) != 2 || r.Creators[0].Name != "Euclid" || r.Creators[1].Role != "translator" || r.Creators[1].Name != "Lorenz, J. F." {
		t.Errorf("record %+v", r)
	}
}

func TestColumns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "Text#,Title\n1,A\n") }))
	defer srv.Close()
	get := func(ctx context.Context, u string) (io.ReadCloser, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get}, io.Discard); err == nil {
		t.Error("a catalogue with no column Type gives no error")
	}
}
