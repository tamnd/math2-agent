package crossref

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

const work1 = `{"DOI":"10.1000/ABC","title":["On primes"],"subtitle":["a survey"],"type":"journal-article","ISSN":["0022-314X","1096-1658"],` +
	`"author":[{"given":"Ann","family":"Smith","ORCID":"http://orcid.org/0000-0002-1825-0097"},{"name":"The Group"}],"issued":{"date-parts":[[2001,5]]},` +
	`"volume":"7","page":"11-20","publisher":"Elsevier","relation":{"has-preprint":[{"id-type":"doi","id":"10.48550/arXiv.2101.00001"}]},"URL":"https://doi.org/10.1000/abc"}`

const work2 = `{"DOI":"10.1007/978-1-4612-0000-0","title":["Topology"],"type":"book","ISBN":["9781461200000","1461200008"],"editor":[{"given":"B","family":"Jones"}],"issued":{"date-parts":[[null]]}}`

func get(srv *httptest.Server) ingest.Getter {
	return func(ctx context.Context, u string) (io.ReadCloser, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
}

func records(t *testing.T, r io.Reader) []cluster.Record {
	t.Helper()
	var rs []cluster.Record
	if err := (Adapter{}).Records(r, func(x cluster.Record) error { rs = append(rs, x); return nil }); err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestRecords(t *testing.T) {
	rs := records(t, strings.NewReader(work1+"\n"+`{"items":[`+work2+`]}`+"\n"))
	if len(rs) != 2 {
		t.Fatalf("records %+v", rs)
	}
	a := rs[0]
	if a.NativeID != "10.1000/abc" || a.Title != "On primes: a survey" || a.Date != "2001-05" || a.Venue != "0022-314X" || a.Page != "11" || a.Kind != "article" {
		t.Errorf("record %+v", a)
	}
	if a.IDs["arxiv"][0] != "2101.00001" || len(a.Creators) != 2 || a.Creators[0].Name != "Ann Smith" || a.Creators[0].IDs["orcid"][0] != "0000-0002-1825-0097" || a.Creators[1].Name != "The Group" {
		t.Errorf("record %+v", a)
	}
	b := rs[1]
	if b.Kind != "monograph" || b.Date != "" || len(b.IDs["isbn"]) != 1 || b.IDs["isbn"][0] != "9781461200000" || b.Creators[0].Role != "editor" {
		t.Errorf("record %+v", b)
	}
	if err := (Adapter{}).Records(strings.NewReader(`{"title":["x"]}`), func(cluster.Record) error { return nil }); err == nil {
		t.Error("a line with no DOI gives no error")
	}
}

func TestFetchFilter(t *testing.T) {
	var cursors []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("filter") != "issn:0022-314X" || q.Get("rows") != "1000" {
			t.Errorf("query %v", q)
		}
		cursors = append(cursors, q.Get("cursor"))
		if q.Get("cursor") == "*" {
			_, _ = io.WriteString(w, `{"message":{"next-cursor":"c2","items":[`+work1+`,`+work2+`]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"message":{"next-cursor":"c3","items":[]}}`)
	}))
	defer srv.Close()
	var buf bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/works?filter=issn:0022-314X", Get: get(srv)}, &buf); err != nil {
		t.Fatal(err)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var recs []ingest.Record
	if err := (Adapter{}).Read(snap, func(r ingest.Record) error { recs = append(recs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].ID != "10.1000/abc" || strings.Join(cursors, " ") != "* c2" {
		t.Errorf("records %+v, cursors %v", recs, cursors)
	}
}

func TestFetchDOIs(t *testing.T) {
	root := t.TempDir()
	var rs []cluster.Record
	for i, d := range []string{"10.1000/abc", "10.1000/a,b", "10.1000/abc", "10.1000/xyz"} {
		rs = append(rs, cluster.Record{Source: "zbmath", NativeID: string(rune('a' + i)), Level: "work", Kind: "article", Title: "T", IDs: resource.IDs{"doi": {d}}})
	}
	if err := cluster.WriteRecords(root, "zbmath", "records", rs); err != nil {
		t.Fatal(err)
	}
	if err := cluster.WriteRecords(root, "crossref", "records", []cluster.Record{{Source: "crossref", NativeID: "10.1000/old", Level: "work", Kind: "article", Title: "T", IDs: resource.IDs{"doi": {"10.1000/old"}}}}); err != nil {
		t.Fatal(err)
	}
	var filters []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filters = append(filters, r.URL.Query().Get("filter"))
		_, _ = io.WriteString(w, `{"message":{"items":[`+work1+`]}}`)
	}))
	defer srv.Close()
	var buf bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/works", Root: root, Get: get(srv)}, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Join(filters, " ") != "doi:10.1000/abc,doi:10.1000/xyz" {
		t.Errorf("filters %v", filters)
	}
}

func TestSelectHasNoLanguage(t *testing.T) {
	if slices.Contains(strings.Split(Select, ","), "language") {
		t.Fatal("Select has language, which the works API rejects with status 400")
	}
}
