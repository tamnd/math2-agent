package openalex

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

const work1 = `{"id":"https://openalex.org/W1","doi":"https://doi.org/10.1000/ABC","title":"On  primes","publication_date":"2001-05-02","publication_year":2001,"language":"en","type":"article",` +
	`"authorships":[{"author":{"id":"https://openalex.org/A1","display_name":"Ann Smith","orcid":"https://orcid.org/0000-0002-1825-0097"}}],` +
	`"primary_location":{"landing_page_url":"https://doi.org/10.1000/abc","source":{"id":"https://openalex.org/S1","display_name":"J. Num.","issn_l":"0022-314X","host_organization_name":"Elsevier"}},` +
	`"locations":[{"landing_page_url":"https://doi.org/10.1000/abc"},{"landing_page_url":"http://arxiv.org/abs/math/0101001v2"}],"biblio":{"volume":"7","first_page":"11"}}`

const work2 = `{"id":"https://openalex.org/W2","title":"Topology","publication_year":1990,"type":"book"}`

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

func TestFetch(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		queries = append(queries, q.Get("cursor"))
		if q.Get("filter") != Filter || q.Get("per-page") != "200" || q.Get("mailto") != "" {
			t.Errorf("query %v", q)
		}
		switch q.Get("cursor") {
		case "*":
			_, _ = io.WriteString(w, `{"meta":{"next_cursor":"c2"},"results":[`+work1+`]}`)
		case "c2":
			_, _ = io.WriteString(w, `{"meta":{"next_cursor":"c3"},"results":[`+work2+`]}`)
		default:
			_, _ = io.WriteString(w, `{"meta":{"next_cursor":null},"results":[]}`)
		}
	}))
	defer srv.Close()
	var buf bytes.Buffer
	u := strings.Replace(Adapter{}.Dump(), Base, srv.URL, 1)
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: u, Get: get(srv)}, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Join(queries, " ") != "* c2 c3" {
		t.Errorf("cursors %v", queries)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var recs []ingest.Record
	if err := (Adapter{}).Read(snap, func(r ingest.Record) error { recs = append(recs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].ID != "W1" || recs[0].Title != "On primes" || recs[0].Meta["doi"] != "10.1000/abc" {
		t.Errorf("records %+v", recs)
	}
}

func TestRecords(t *testing.T) {
	var rs []cluster.Record
	if err := (Adapter{}).Records(strings.NewReader(work1+"\n\n"+work2+"\n"), func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("records %+v", rs)
	}
	a := rs[0]
	if a.NativeID != "W1" || a.Kind != "article" || a.Date != "2001-05-02" || a.Venue != "0022-314X" || a.Publisher != "Elsevier" || a.Volume != "7" || a.Page != "11" {
		t.Errorf("record %+v", a)
	}
	if a.IDs["doi"][0] != "10.1000/abc" || a.IDs["arxiv"][0] != "math/0101001" || a.IDs["openalex"][0] != "W1" {
		t.Errorf("ids %v", a.IDs)
	}
	if len(a.Creators) != 1 || a.Creators[0].IDs["orcid"][0] != "0000-0002-1825-0097" {
		t.Errorf("creators %+v", a.Creators)
	}
	if b := rs[1]; b.Kind != "monograph" || b.Date != "1990" {
		t.Errorf("record %+v", b)
	}
	if err := (Adapter{}).Records(strings.NewReader(`{"title":"x"}`), func(cluster.Record) error { return nil }); err == nil {
		t.Error("a work with no id gives no error")
	}
}
