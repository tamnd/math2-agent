package swmath

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

const page1 = `{"result":[{"id":825,"name":"SageMath","authors":["Stein, William"," "],"classification":["05","11","x"],"description":"zbMATH Open Web Interface contents unavailable due to conflicting licenses.","homepage":"https://www.sagemath.org/","source_code":"https://github.com/sagemath/sage","license_terms":"GPL 3 or higher","articles_count":3708,"standard_articles":[{"id":2187846,"title":"hidden"}],"related_software":[{"id":540,"name":"Magma"}]}],"status":{"last_id":825}}`

const page2 = `{"result":[{"id":900,"name":"Tiny","homepage":"not a link"}],"status":{"last_id":900}}`

const page3 = `{"result":[],"status":{"last_id":900}}`

func TestFetchAndRecords(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("start_after")
		asked = append(asked, after)
		if r.URL.Query().Get("results_per_page") != "100" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		switch after {
		case "":
			_, _ = io.WriteString(w, page1)
		case "825":
			_, _ = io.WriteString(w, page2)
		default:
			_, _ = io.WriteString(w, page3)
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
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL, Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 3 || asked[2] != "900" {
		t.Errorf("pages %q", asked)
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
	if r.NativeID != "825" || r.Kind != "software" || r.Title != "SageMath" || r.IDs["swmath"][0] != "825" || len(r.MSC) != 2 || r.MSC[0] != "05-XX" {
		t.Errorf("record %+v", r)
	}
	if len(r.Creators) != 1 || r.Creators[0].Name != "Stein, William" || len(r.Links) != 2 || len(r.Relations) != 0 {
		t.Errorf("record %+v", r)
	}
	if len(rs[1].Links) != 0 {
		t.Errorf("a homepage that is not a link: %+v", rs[1].Links)
	}
}

func TestReadEmptyID(t *testing.T) {
	err := (Adapter{}).Read(bytes.NewBufferString(`{"name":"x"}`+"\n"), func(ingest.Record) error { return nil })
	if err == nil {
		t.Error("an entry with no id must be an error")
	}
}
