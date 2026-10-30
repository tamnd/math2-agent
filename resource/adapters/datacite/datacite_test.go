package datacite

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

const page1 = `{"data":[{"attributes":{"doi":"10.5281/ZENODO.1","titles":[{"title":"Graphes  et réseaux","lang":"fr"},{"title":"Graphs and networks","titleType":"TranslatedTitle","lang":"en"}],
"creators":[{"name":"Smith, Ann","nameIdentifiers":[{"nameIdentifier":"https://orcid.org/0000-0002-1825-0097","nameIdentifierScheme":"ORCID"}]}],
"publisher":{"name":"Zenodo"},"publicationYear":2021,"language":"fr","subjects":[{"subject":"05C10","subjectScheme":"MSC"},{"subject":"510 Mathematics","subjectScheme":"DDC"},{"subject":"FOS: Mathematics"}],
"types":{"resourceTypeGeneral":"Preprint"},"relatedIdentifiers":[{"relatedIdentifier":"arXiv:2101.00001","relatedIdentifierType":"arXiv","relationType":"IsIdenticalTo"},
{"relatedIdentifier":"10.1000/journal.5","relatedIdentifierType":"DOI","relationType":"IsPartOf"}],"url":"https://zenodo.org/record/1"}}],
"links":{"next":"NEXT"}}`

const page2 = `{"data":[{"attributes":{"doi":"10.1/x","titles":[{"title":"Data"}],"publisher":"Lab","publicationYear":"2019","types":{"resourceTypeGeneral":"Image"}}}],"links":{}}`

func TestFetchAndRecords(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page[cursor]") {
		case "1":
			if r.URL.Query().Get("query") != Query || r.URL.Query().Get("page[size]") != "1000" {
				t.Errorf("query %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, strings.Replace(page1, "NEXT", srv.URL+"/dois?page%5Bcursor%5D=abc", 1))
		case "abc":
			_, _ = io.WriteString(w, page2)
		default:
			t.Errorf("query %v", r.URL.Query())
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
	url := strings.Replace((Adapter{}).Dump(), Base, srv.URL, 1)
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
	if r.NativeID != "10.5281/zenodo.1" || r.Kind != "preprint" || r.Title != "Graphes et réseaux" || r.TitleEn != "Graphs and networks" || r.Date != "2021" || r.Language != "fr" || r.Publisher != "Zenodo" {
		t.Errorf("record %+v", r)
	}
	if len(r.Creators) != 1 || r.Creators[0].IDs["orcid"][0] != "0000-0002-1825-0097" {
		t.Errorf("creators %+v", r.Creators)
	}
	if len(r.MSC) != 1 || r.MSC[0] != "05C10" || len(r.Subjects) != 1 || r.Subjects[0].Code != "510" || r.IDs["arxiv"][0] != "2101.00001" {
		t.Errorf("msc %v, subjects %v, ids %v", r.MSC, r.Subjects, r.IDs)
	}
	if len(r.Relations) != 1 || r.Relations[0].Kind != "part_of" || r.Relations[0].To != "doi:10.1000/journal.5" {
		t.Errorf("relations %+v", r.Relations)
	}
	if rs[1].Kind != "dataset" || rs[1].Date != "2019" || rs[1].Publisher != "Lab" {
		t.Errorf("record 2 %+v", rs[1])
	}
}
