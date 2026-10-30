package eudml

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

const page1 = `<OAI-PMH xmlns="http://www.openarchives.org/OAI/2.0/"><ListRecords>
<record><header><identifier>oai:eudml.org:doc/94429</identifier></header><metadata>
<oai_dc:dc xmlns:oai_dc="http://www.openarchives.org/OAI/2.0/oai_dc/" xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:title>On  the number of G&G graphs</dc:title><dc:creator>Erdős, Paul</dc:creator><dc:creator>Rényi, Alfréd</dc:creator>
<dc:type>article</dc:type><dc:date>1995-01-01T00:00:00Z</dc:date><dc:language>eng</dc:language><dc:publisher>Kluwer</dc:publisher>
<dc:identifier>https://eudml.org/doc/94429</dc:identifier><dc:identifier>ISSN 0373-0956</dc:identifier><dc:identifier>doi:10.5802/AIF.31</dc:identifier>
<dc:relation>http://www.zentralblatt-math.org/zmath/en/search/?q=an:0845.05050</dc:relation><dc:relation>http://www.ams.org/mathscinet-getitem?mr=1234567</dc:relation>
<dc:description>An abstract that the adapter does not keep.</dc:description>
</oai_dc:dc></metadata></record>
<resumptionToken>t1</resumptionToken></ListRecords></OAI-PMH>`

const page2 = `<OAI-PMH><ListRecords>
<record><header status="deleted"><identifier>oai:eudml.org:doc/1</identifier></header></record>
<record><header><identifier>oai:eudml.org:doc/270001</identifier></header><metadata><oai_dc:dc><dc:title>Lectures on groups</dc:title><dc:type>book</dc:type>
<dc:publisher>Springer</dc:publisher><dc:identifier>ISBN 978-3-540-20000-0</dc:identifier></oai_dc:dc></metadata></record>
<record><header><identifier>oai:eudml.org:other</identifier></header><metadata><oai_dc:dc><dc:title>No number</dc:title></oai_dc:dc></metadata></record>
<resumptionToken></resumptionToken></ListRecords></OAI-PMH>`

func TestFetchAndRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("verb") != "ListRecords" {
			t.Errorf("query %s", r.URL.RawQuery)
		}
		if q.Get("resumptionToken") == "t1" {
			_, _ = io.WriteString(w, page2)
			return
		}
		if q.Get("metadataPrefix") != Prefix {
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
	if bytes.Contains(buf.Bytes(), []byte("abstract")) {
		t.Error("the snapshot keeps the abstract")
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
	if r.NativeID != "94429" || r.Kind != "article" || r.Level != "work" || r.Title != "On the number of G&G graphs" || r.Language != "en" || r.Date != "1995-01-01" ||
		r.Venue != "0373-0956" || r.Publisher != "" || r.Links[0] != "https://eudml.org/doc/94429" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["eudml"][0] != "94429" || r.IDs["doi"][0] != "10.5802/aif.31" || r.IDs["zbl"][0] != "0845.05050" || r.IDs["mr"][0] != "1234567" {
		t.Errorf("ids %+v", r.IDs)
	}
	if len(r.Creators) != 2 || r.Creators[0].Name != "Erdős, Paul" {
		t.Errorf("creators %+v", r.Creators)
	}
	if r := rs[1]; r.NativeID != "270001" || r.Kind != "monograph" || r.Publisher != "Springer" || r.IDs["isbn"][0] != "9783540200000" {
		t.Errorf("record %+v", r)
	}
	var n int
	err = (Adapter{}).Read(bytes.NewReader([]byte(`{"id":"5","title":"T"}`+"\n")), func(r ingest.Record) error {
		n++
		if r.ID != "5" || r.Kind != "article" || r.URL != "https://eudml.org/doc/5" {
			t.Errorf("ingest record %+v", r)
		}
		return nil
	})
	if err != nil || n != 1 {
		t.Errorf("read %d records, %v", n, err)
	}
}
