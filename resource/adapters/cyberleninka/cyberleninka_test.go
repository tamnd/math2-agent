package cyberleninka

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
<record><header><identifier>oai:cyberleninka.ru:article/123</identifier></header><metadata>
<oai_dc:dc xmlns:oai_dc="http://www.openarchives.org/OAI/2.0/oai_dc/" xmlns:dc="http://purl.org/dc/elements/1.1/">
<dc:title xml:lang="ru">О  линейных формах</dc:title><dc:title xml:lang="en">On linear forms</dc:title>
<dc:creator>Иванов И. И.</dc:creator><dc:subject>Математика</dc:subject><dc:date>2020</dc:date><dc:language>rus</dc:language>
<dc:identifier>https://cyberleninka.ru/article/n/o-lineynyh-formah</dc:identifier><dc:identifier>https://doi.org/10.4213/IM8810</dc:identifier>
<dc:source>Известия РАН. Серия математическая, 1607-0046</dc:source><dc:description>Аннотация.</dc:description>
</oai_dc:dc></metadata></record>
<record><header><identifier>oai:cyberleninka.ru:article/124</identifier></header><metadata><oai_dc:dc><dc:title>Об истории</dc:title><dc:subject>История</dc:subject></oai_dc:dc></metadata></record>
<resumptionToken>t1</resumptionToken></ListRecords></OAI-PMH>`

const page2 = `<OAI-PMH><ListRecords>
<record><header status="deleted"><identifier>oai:cyberleninka.ru:article/1</identifier></header></record>
<record><header><identifier>oai:cyberleninka.ru:article/125</identifier></header><metadata><oai_dc:dc><dc:title>Algebraic geometry today</dc:title><dc:subject>Mathematics</dc:subject></oai_dc:dc></metadata></record>
<resumptionToken></resumptionToken></ListRecords></OAI-PMH>`

func TestFetchAndRecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
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
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(snap)
	if bytes.Contains(data, []byte("Аннотация")) {
		t.Error("the snapshot keeps the abstract")
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(bytes.NewReader(data), func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.NativeID != "o-lineynyh-formah" || r.Kind != "article" || r.Title != "О линейных формах" || r.TitleEn != "On linear forms" || r.Language != "ru" || r.Date != "2020" ||
		r.Venue != "1607-0046" || r.Links[0] != "https://cyberleninka.ru/article/n/o-lineynyh-formah" {
		t.Errorf("record %+v", r)
	}
	if r.IDs["cyberleninka"][0] != "o-lineynyh-formah" || r.IDs["doi"][0] != "10.4213/im8810" {
		t.Errorf("ids %+v", r.IDs)
	}
	if len(r.Creators) != 1 || r.Creators[0].Name != "Иванов И. И." {
		t.Errorf("creators %+v", r.Creators)
	}
	if r := rs[1]; r.NativeID != "article/125" || r.Title != "Algebraic geometry today" {
		t.Errorf("record %+v", r)
	}
	var ir []ingest.Record
	if err := (Adapter{}).Read(bytes.NewReader(data), func(r ingest.Record) error { ir = append(ir, r); return nil }); err != nil || len(ir) != 2 || ir[0].Kind != "article" {
		t.Errorf("ingest records %+v, %v", ir, err)
	}
}
