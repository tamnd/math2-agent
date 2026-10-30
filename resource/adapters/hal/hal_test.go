package hal

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

const page1 = `{"response":{"docs":[{"docid":1,"halId_s":"hal-00000001","version_i":2,"title_s":["Sur les  graphes"],"en_title_s":["On graphs"],
"authFullName_s":["Ann Smith","Bo Li"],"authORCIDIdExt_s":["Ann Smith_FacetSep_https://orcid.org/0000-0002-1825-0097"],"doiId_s":"10.1000/ABC","arxivId_s":"2101.00001",
"journalIssn_s":"0003-486X","volume_s":"12","page_s":"101-120","producedDate_s":"2021-03-04","language_s":["fr"],"docType_s":"ART",
"primaryDomain_s":"math.math-co","domain_s":["0.math","1.math.math-co","1.info.info-dm"],"uri_s":"https://hal.science/hal-00000001"}]},"nextCursorMark":"AoE1"}`

const page2 = `{"response":{"docs":[{"halId_s":"tel-00000002","title_s":["Thèse"],"docType_s":"THESE","isbn_s":["978-0-8053-9021-6"]}]},"nextCursorMark":"AoE2"}`

const page3 = `{"response":{"docs":[]},"nextCursorMark":"AoE2"}`

func TestFetchAndRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("fq") != Filter || q.Get("rows") != "1000" || q.Get("sort") != "docid asc" || q.Get("fl") != Fields {
			t.Errorf("query %v", q)
		}
		_, _ = io.WriteString(w, map[string]string{"*": page1, "AoE1": page2, "AoE2": page3}[q.Get("cursorMark")])
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
	url := strings.Replace((Adapter{}).Dump(), Base, srv.URL+"/search/", 1)
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: url, Get: get}, &buf); err != nil {
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
	if r.NativeID != "hal-00000001" || r.Kind != "article" || r.Title != "Sur les graphes" || r.TitleEn != "On graphs" || r.Date != "2021-03-04" || r.Language != "fr" ||
		r.Venue != "0003-486X" || r.Volume != "12" || r.Page != "101" || r.Links[0] != "https://hal.science/hal-00000001" {
		t.Errorf("record %+v", r)
	}
	if len(r.Creators) != 2 || r.Creators[0].IDs["orcid"][0] != "0000-0002-1825-0097" || r.Creators[1].IDs != nil {
		t.Errorf("creators %+v", r.Creators)
	}
	if r.IDs["doi"][0] != "10.1000/abc" || r.IDs["arxiv"][0] != "2101.00001" || r.IDs["hal"][0] != "hal-00000001" {
		t.Errorf("ids %v", r.IDs)
	}
	var subjects []string
	for _, s := range r.Subjects {
		subjects = append(subjects, s.System+":"+s.Code)
	}
	if strings.Join(subjects, " ") != "arxiv:math.CO arxiv:math arxiv:cs.DM" {
		t.Errorf("subjects %v", subjects)
	}
	if rs[1].Kind != "thesis" || rs[1].IDs["isbn"][0] != "9780805390216" {
		t.Errorf("record 2 %+v", rs[1])
	}
}
