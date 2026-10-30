package dnb

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

// record gives one record in MARC21-xml, as the SRU server of the DNB gives it. The class is a field 084 or 082.
func record(idn, title, class string) string {
	return fmt.Sprintf(`<record><recordSchema>MARC21-xml</recordSchema><recordPacking>xml</recordPacking><recordData><record xmlns="http://www.loc.gov/MARC21/slim" type="Bibliographic">
<leader>00000nam a2200000 c 4500</leader><controlfield tag="001">%s</controlfield><controlfield tag="008">200101s2020    gw |||||||||||||| ||ger  </controlfield>
<datafield tag="020" ind1=" " ind2=" "><subfield code="a">9783662000007</subfield></datafield>
<datafield tag="024" ind1="7" ind2=" "><subfield code="a">urn:nbn:de:101:1-2020010101</subfield><subfield code="2">urn</subfield></datafield>
%s
<datafield tag="100" ind1="1" ind2=" "><subfield code="a">Forster, Otto</subfield><subfield code="4">aut</subfield></datafield>
<datafield tag="245" ind1="1" ind2="0"><subfield code="a">%s</subfield><subfield code="b">Differential- und Integralrechnung</subfield></datafield>
<datafield tag="264" ind1=" " ind2="1"><subfield code="b">Springer Spektrum</subfield></datafield>
</record></recordData><recordPosition>1</recordPosition></record>`, idn, class, title)
}

func page(next int, records ...string) string {
	n := ""
	if next > 0 {
		n = fmt.Sprintf("<nextRecordPosition>%d</nextRecordPosition>", next)
	}
	return `<?xml version="1.0" encoding="UTF-8"?><searchRetrieveResponse xmlns="http://www.loc.gov/zing/srw/"><version>1.1</version><numberOfRecords>3</numberOfRecords><records>` +
		strings.Join(records, "") + `</records>` + n + `</searchRetrieveResponse>`
}

func TestFetchAndRecords(t *testing.T) {
	const sg = `<datafield tag="084" ind1=" " ind2=" "><subfield code="a">510</subfield><subfield code="q">DE-101</subfield><subfield code="2">sdnb</subfield></datafield>`
	const ddc = `<datafield tag="082" ind1="0" ind2="4"><subfield code="a">515.7</subfield></datafield>`
	const other = `<datafield tag="084" ind1=" " ind2=" "><subfield code="a">943</subfield><subfield code="2">sdnb</subfield></datafield>`
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("recordSchema") != "MARC21-xml" || q.Get("version") != "1.1" || q.Get("accessToken") != "" {
			t.Errorf("request %s", r.URL)
		}
		queries = append(queries, q.Get("query")+"@"+q.Get("startRecord"))
		switch q.Get("query") + "@" + q.Get("startRecord") {
		case "dnb.sgt=510@1":
			_, _ = io.WriteString(w, page(3, record("1200000001", "Analysis 1", sg), record("1200000002", "Geschichte", other)))
		case "dnb.sgt=510@3":
			_, _ = io.WriteString(w, page(0, record("1200000003", "Analysis 2", sg)))
		case "dnb.ddc=51*@1":
			_, _ = io.WriteString(w, page(0, record("1200000001", "Analysis 1", sg), record("1200000004", "Funktionalanalysis", ddc)))
		default:
			t.Errorf("query %s", r.URL)
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
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/sru/dnb", Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 3 {
		t.Errorf("queries %v", queries)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(snap, func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 3 || rs[0].NativeID != "1200000001" || rs[1].NativeID != "1200000003" || rs[2].NativeID != "1200000004" {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.Title != "Analysis 1: Differential- und Integralrechnung" || r.Date != "2020" || r.Language != "de" || r.Publisher != "Springer Spektrum" || r.Links[0] != "https://d-nb.info/1200000001" ||
		r.IDs["isbn"][0] != "9783662000007" || r.IDs["urn"][0] != "urn:nbn:de:101:1-2020010101" || r.Creators[0].Role != "author" || r.Subjects[0].System != "sdnb" {
		t.Errorf("record %+v", r)
	}
	if rs[2].Subjects[0].System != "ddc" || rs[2].Subjects[0].Code != "515.7" {
		t.Errorf("record %+v", rs[2])
	}
}
