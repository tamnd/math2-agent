package wikidataworks

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func TestFetchAndRecords(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !strings.Contains(r.URL.Query().Get("query"), fmt.Sprintf("LIMIT %d OFFSET 0", PageSize)) {
			t.Errorf("query %q", r.URL.Query().Get("query"))
		}
		_, _ = io.WriteString(w, `{"results":{"bindings":[{"x":{"value":"http://www.wikidata.org/entity/Q1"},"title":{"value":"Primes"},"date":{"value":"2001-01-01T00:00:00Z"},`+
			`"doi":{"value":"10.1000/ABC"},"zbmath":{"value":"1001.11001"},"isbn":{"value":"978-0-8053-9021-6"},"language":{"value":"en"},"authors":{"value":"Ann Smith|Bo Li"},`+
			`"subjects":{"value":"Q395 Q12479"},"kinds":{"value":"Q571 Q83790"}}]}}`)
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
	if calls != 1 {
		t.Errorf("%d calls for a short page", calls)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(snap, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.NativeID != "Q1" || r.Kind != "textbook" || r.Date != "2001" || r.Language != "en" || len(r.Creators) != 2 || len(r.Subjects) != 2 || r.Subjects[0].System != "wikidata" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["doi"][0] != "10.1000/abc" || r.IDs["zbl"][0] != "1001.11001" || r.IDs["isbn"][0] != "9780805390216" || r.IDs["wikidata"][0] != "Q1" {
		t.Errorf("ids %v", r.IDs)
	}
	if err := (Adapter{}).Read(strings.NewReader(`{"title":"x"}`), func(ingest.Record) error { return nil }); err == nil {
		t.Error("a work with no item gives no error")
	}
}
