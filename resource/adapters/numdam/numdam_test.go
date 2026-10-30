package numdam

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

const page1 = `<OAI-PMH><ListRecords><record><header><identifier>oai:numdam.org:AIF_1951__3__1_0</identifier></header><metadata>
<article xml:lang="fr"><front><journal-meta><journal-title-group><journal-title>Annales de l'institut Fourier</journal-title></journal-title-group><issn pub-type="ppub">0373-0956</issn><issn pub-type="epub">1777-5310</issn></journal-meta>
<article-meta><article-id pub-id-type="doi">10.5802/AIF.31</article-id><title-group><article-title>Sur les  espaces &eacute;tal&eacute;s G&G</article-title></title-group>
<contrib-group><contrib contrib-type="author"><name><surname>Cartan</surname><given-names>Henri</given-names></name></contrib><contrib contrib-type="editor"><name><surname>Ed</surname></name></contrib></contrib-group>
<volume>3</volume><fpage>1</fpage><ext-link ext-link-type="zbl-item-id">0045.11002</ext-link><ext-link ext-link-type="mr-item-id">49478</ext-link><ext-link ext-link-type="eudml-item-id">urn:eudml:doc:73601</ext-link>
<kwd-group kwd-group-type="msc"><kwd>32C35</kwd></kwd-group></article-meta></front></article></metadata></record>
<resumptionToken>t1</resumptionToken></ListRecords></OAI-PMH>`

const page2 = `<OAI-PMH><ListRecords><record><header status="deleted"><identifier>oai:numdam.org:GONE</identifier></header></record><resumptionToken></resumptionToken></ListRecords></OAI-PMH>`

func TestFetchAndRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("resumptionToken") == "t1" {
			_, _ = io.WriteString(w, page2)
			return
		}
		if r.URL.Query().Get("metadataPrefix") != Prefix {
			t.Errorf("query %s", r.URL.RawQuery)
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
	var rs []cluster.Record
	if err := (Adapter{}).Records(snap, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.NativeID != "AIF_1951__3__1_0" || r.Kind != "article" || r.Title != "Sur les espaces étalés G&G" || r.Language != "fr" || r.Date != "1951" ||
		r.Venue != "0373-0956" || r.Volume != "3" || r.Page != "1" || len(r.MSC) != 1 || r.MSC[0] != "32C35" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["doi"][0] != "10.5802/aif.31" || r.IDs["zbl"][0] != "0045.11002" || len(r.IDs["eudml"]) != 1 {
		t.Errorf("ids %+v", r.IDs)
	}
	if len(r.Creators) != 1 || r.Creators[0].Name != "Cartan, Henri" {
		t.Errorf("creators %+v", r.Creators)
	}
}
