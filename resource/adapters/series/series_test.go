package series

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource/cluster"
)

const book1 = `{"DOI":"10.1007/978-0-387-00001-1","title":["Algebra"],"type":"monograph","ISSN":["0072-5285","2197-5612"],"ISBN":["9780387000011"],"volume":"73","author":[{"given":"Ann","family":"Smith"}],"issued":{"date-parts":[[1974]]}}`

const book2 = `{"DOI":"10.1007/978-0-387-00002-8","title":["Topology"],"type":"book","ISSN":["2197-5612"],"issued":{"date-parts":[[1980]]}}`

var test = Adapter{ID: "test-series", Series: []Series{{Name: "Graduate Texts in Mathematics", Publisher: "Springer", ISSN: []string{"0072-5285", "2197-5612"}}}}

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

func TestFetchRecords(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		queries = append(queries, q.Get("filter")+" "+q.Get("cursor"))
		if q.Get("mailto") != "" {
			t.Error("the request has a mailto")
		}
		switch {
		case q.Get("cursor") != "*":
			_, _ = io.WriteString(w, `{"message":{"next-cursor":"","items":[]}}`)
		case strings.HasPrefix(q.Get("filter"), "issn:0072-5285"):
			_, _ = io.WriteString(w, `{"message":{"next-cursor":"c2","items":[`+book1+`]}}`)
		default:
			_, _ = io.WriteString(w, `{"message":{"next-cursor":"c3","items":[`+book1+`,`+book2+`]}}`)
		}
	}))
	defer srv.Close()
	var buf bytes.Buffer
	if err := test.Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/works", Get: get(srv)}, &buf); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 4 || !strings.Contains(queries[0], "type:edited-book") {
		t.Errorf("queries %q", queries)
	}
	r, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := test.Records(r, func(x cluster.Record) error { rs = append(rs, x); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 {
		t.Fatalf("records %+v", rs)
	}
	if s := rs[0]; s.Kind != "series" || s.NativeID != "0072-5285" || len(s.IDs["issn"]) != 2 || s.Publisher != "Springer" {
		t.Errorf("series %+v", s)
	}
	a := rs[1]
	if a.NativeID != "10.1007/978-0-387-00001-1" || a.Kind != "monograph" || len(a.Relations) != 1 {
		t.Fatalf("book %+v", a)
	}
	if e := a.Relations[0]; e.Kind != "part_of" || e.To != "issn:0072-5285" || e.Volume != "73" || e.Source != "test-series" {
		t.Errorf("edge %+v", e)
	}
	if b := rs[2]; len(b.Relations) != 1 || b.Relations[0].Volume != "" {
		t.Errorf("book %+v", b)
	}
}

func TestRead(t *testing.T) {
	in := `{"series":{"name":"S","publisher":"P","issn":["1065-7339"]}}` + "\n" + `{"work":` + book2 + `}` + "\n"
	var got []ingest.Record
	if err := test.Read(strings.NewReader(in), func(r ingest.Record) error { got = append(got, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "1065-7339" || got[0].Kind != "series" || got[1].URL != "https://doi.org/10.1007/978-0-387-00002-8" {
		t.Errorf("records %+v", got)
	}
	for _, bad := range []string{`{"series":{"name":"S"}}`, `{}`, `{"work":`} {
		if err := test.Read(strings.NewReader(bad), func(ingest.Record) error { return nil }); err == nil {
			t.Errorf("%s gives no error", bad)
		}
	}
}

func TestLists(t *testing.T) {
	for _, a := range []Adapter{Springer, AMS} {
		for _, s := range a.Series {
			if r := Record(s); len(r.IDs["issn"]) != len(s.ISSN) {
				t.Errorf("%s: an ISSN of %s is not valid: %v", a.ID, s.Name, s.ISSN)
			}
		}
	}
}
