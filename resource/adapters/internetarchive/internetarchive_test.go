package internetarchive

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

const page1 = `{"items":[
{"identifier":"courseofpuremath00hard","title":"A course of pure mathematics","year":"1908","language":"eng","creator":["Hardy, G. H."],"publisher":"Cambridge University Press",
 "isbn":["0521092272"],"oclc-id":"2779601","call_number":"QA303 .H3 1908","description":"not asked for"},
{"identifier":"arxiv-math0211159","title":"The entropy formula","date":"2002-11-11T00:00:00Z","creator":"Perelman, Grisha"}
],"count":2,"cursor":"c1","total":3}`

const page2 = `{"items":[{"identifier":"","title":"No id"},{"identifier":"tables1920","title":["Tables of functions"],"year":1920,"call_number":"510.8"}],"count":2,"total":3}`

func TestFetchAndRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("q") != Query || q.Get("count") != "10000" || q.Get("fields") != Fields {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		for k := range q {
			if k != "q" && k != "fields" && k != "count" && k != "cursor" {
				t.Errorf("the request has the parameter %s", k)
			}
		}
		if q.Get("cursor") == "c1" {
			_, _ = io.WriteString(w, page2)
			return
		}
		_, _ = io.WriteString(w, page1)
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
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(snap)
	if bytes.Contains(data, []byte("not asked for")) {
		t.Error("the snapshot keeps the description")
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(bytes.NewReader(data), func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.NativeID != "courseofpuremath00hard" || r.Kind != "monograph" || r.Level != "manifestation" || r.Title != "A course of pure mathematics" || r.Date != "1908" || r.Language != "en" ||
		r.Publisher != "Cambridge University Press" || r.Links[0] != "https://archive.org/details/courseofpuremath00hard" || len(r.Subjects) != 1 || r.Subjects[0].Code != "QA303" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["ia"][0] != "courseofpuremath00hard" || r.IDs["isbn"][0] != "9780521092272" || r.IDs["oclc"][0] != "2779601" || len(r.Creators) != 1 {
		t.Errorf("ids %+v creators %+v", r.IDs, r.Creators)
	}
	if r := rs[1]; r.Kind != "preprint" || r.Date != "2002-11-11" || len(r.IDs["arxiv"]) != 1 || r.IDs["arxiv"][0] != "math/0211159" {
		t.Errorf("record %+v", r)
	}
	if r := rs[2]; r.NativeID != "tables1920" || r.Title != "Tables of functions" || r.Date != "1920" || len(r.Subjects) != 0 {
		t.Errorf("record %+v", r)
	}
	var ir []ingest.Record
	if err := (Adapter{}).Read(bytes.NewReader(data), func(r ingest.Record) error { ir = append(ir, r); return nil }); err != nil || len(ir) != 3 || ir[1].Kind != "preprint" {
		t.Errorf("ingest records %+v, %v", ir, err)
	}
}
