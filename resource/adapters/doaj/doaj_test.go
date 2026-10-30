package doaj

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

const record = `<record><header><identifier>oai:doaj.org/article:c1d6</identifier><setSpec>TENDOk1hdGhlbWF0aWNz</setSpec><setSpec>TENDOlByb2JhYmlsaXRpZXMuIE1hdGhlbWF0aWNhbCBzdGF0aXN0aWNz</setSpec></header>
<metadata><oai_doaj:doajArticle xmlns:oai_doaj="http://doaj.org/features/oai_doaj/1.0/"><oai_doaj:language>por</oai_doaj:language><oai_doaj:publisher>PUC-SP</oai_doaj:publisher>
<oai_doaj:journalTitle>Revista</oai_doaj:journalTitle><oai_doaj:eissn>2237-9657</oai_doaj:eissn><oai_doaj:publicationDate>2018-06-01</oai_doaj:publicationDate><oai_doaj:volume>7</oai_doaj:volume>
<oai_doaj:startPage>12</oai_doaj:startPage><oai_doaj:doi>10.23925/ABC</oai_doaj:doi><oai_doaj:title>Resolução  gráfica &amp; G&G</oai_doaj:title>
<oai_doaj:authors><oai_doaj:author><oai_doaj:name>Robert Sousa</oai_doaj:name><oai_doaj:orcid_id>https://orcid.org/0000-0002-1825-0097</oai_doaj:orcid_id></oai_doaj:author><oai_doaj:author><oai_doaj:name>Ana Lima</oai_doaj:name></oai_doaj:author></oai_doaj:authors>
<oai_doaj:fullTextUrl format="HTML">https://revistas.pucsp.br/x/34643</oai_doaj:fullTextUrl></oai_doaj:doajArticle></metadata></record>`

func TestFetchAndRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("set") {
		case SetSpec(Sets[0]), SetSpec(Sets[2]):
			// The same article is in two sets, and Fetch writes it once.
			_, _ = io.WriteString(w, `<OAI-PMH><ListRecords>`+record+`<record><header status="deleted"><identifier>oai:doaj.org/article:gone</identifier></header></record></ListRecords></OAI-PMH>`)
		default:
			_, _ = io.WriteString(w, `<OAI-PMH><error code="noRecordsMatch">none</error></OAI-PMH>`)
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
	if r.NativeID != "c1d6" || r.Kind != "article" || r.Title != "Resolução gráfica & G&G" || r.Language != "pt" || r.Date != "2018-06" || r.Venue != "2237-9657" ||
		r.Volume != "7" || r.Page != "12" || r.Publisher != "PUC-SP" || r.IDs["doi"][0] != "10.23925/abc" || r.Links[0] != "https://revistas.pucsp.br/x/34643" {
		t.Errorf("record %+v", r)
	}
	if len(r.Creators) != 2 || r.Creators[0].IDs["orcid"][0] != "0000-0002-1825-0097" || r.Creators[1].IDs != nil {
		t.Errorf("creators %+v", r.Creators)
	}
	if len(r.Subjects) != 2 || r.Subjects[0].Code != "QA1" || r.Subjects[1].Code != "QA273" {
		t.Errorf("subjects %+v", r.Subjects)
	}
}

func TestSetSpec(t *testing.T) {
	if got := SetSpec("LCC:Statistics"); got != "TENDOlN0YXRpc3RpY3M~" {
		t.Errorf("SetSpec = %s", got)
	}
}
