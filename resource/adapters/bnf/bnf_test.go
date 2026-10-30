package bnf

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

// record gives one record in UNIMARC in the form MarcXchange, as the SRU server of the BnF gives it.
func record(ark, title, dewey string) string {
	return fmt.Sprintf(`<srw:record><srw:recordSchema>marcxchange</srw:recordSchema><srw:recordPacking>xml</srw:recordPacking><srw:recordData>
<mxc:record xmlns:mxc="info:lc/xmlns/marcxchange-v2" format="UNIMARC" id="ark:/12148/%[1]s" type="Bibliographic"><mxc:leader>     cam  22     3  450 </mxc:leader>
<mxc:controlfield tag="001">FRBN</mxc:controlfield><mxc:controlfield tag="003">http://catalogue.bnf.fr/ark:/12148/%[1]s</mxc:controlfield>
<mxc:datafield tag="010" ind1=" " ind2=" "><mxc:subfield code="a">1-58488-210-7</mxc:subfield></mxc:datafield>
<mxc:datafield tag="100" ind1=" " ind2=" "><mxc:subfield code="a">20040920d2001    m  y0frey50      ba</mxc:subfield></mxc:datafield>
<mxc:datafield tag="101" ind1="0" ind2=" "><mxc:subfield code="a">fre</mxc:subfield></mxc:datafield>
<mxc:datafield tag="200" ind1="1" ind2=" "><mxc:subfield code="a">%[2]s</mxc:subfield><mxc:subfield code="b">Texte imprimé</mxc:subfield></mxc:datafield>
<mxc:datafield tag="210" ind1=" " ind2=" "><mxc:subfield code="c">Hermann</mxc:subfield><mxc:subfield code="d">2001</mxc:subfield></mxc:datafield>
<mxc:datafield tag="676" ind1=" " ind2=" "><mxc:subfield code="a">%[3]s</mxc:subfield></mxc:datafield>
<mxc:datafield tag="700" ind1=" " ind2="|"><mxc:subfield code="a">Cartan</mxc:subfield><mxc:subfield code="b">Henri</mxc:subfield><mxc:subfield code="4">070</mxc:subfield></mxc:datafield>
</mxc:record></srw:recordData><srw:recordIdentifier>ark:/12148/%[1]s</srw:recordIdentifier></srw:record>`, ark, title, dewey)
}

func page(next int, records ...string) string {
	n := ""
	if next > 0 {
		n = fmt.Sprintf("<srw:nextRecordPosition>%d</srw:nextRecordPosition>", next)
	}
	return `<?xml version="1.0" encoding="UTF-8"?><srw:searchRetrieveResponse xmlns:srw="http://www.loc.gov/zing/srw/"><srw:version>1.2</srw:version>
<srw:numberOfRecords>3</srw:numberOfRecords><srw:records>` + strings.Join(records, "") + `</srw:records>` + n + `</srw:searchRetrieveResponse>`
}

func getter(srv *httptest.Server) ingest.Getter {
	return func(ctx context.Context, u string) (io.ReadCloser, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
}

func TestFetchAndRecords(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/api/SRU" || q.Get("operation") != "searchRetrieve" || q.Get("recordSchema") != "unimarcxchange" || q.Get("maximumRecords") != "100" {
			t.Errorf("request %s", r.URL)
		}
		if strings.Contains(strings.ToLower(r.URL.RawQuery), "mail") || strings.Contains(strings.ToLower(r.URL.RawQuery), "key") {
			t.Errorf("the request %s has an email or a key", r.URL)
		}
		queries = append(queries, q.Get("query")+"@"+q.Get("startRecord"))
		switch {
		case q.Get("query") == `bib.dewey all "515"` && q.Get("startRecord") == "1":
			_, _ = io.WriteString(w, page(2, record("cb1234567x", "Calcul différentiel", "515.33"), record("cb7654321p", "Histoire de France", "944")))
		case q.Get("query") == `bib.dewey all "515"`:
			_, _ = io.WriteString(w, page(0, record("cb1234567x", "Calcul différentiel", "515.33"), record("cb2222222q", "Formes différentielles", "515/.37")))
		default:
			_, _ = io.WriteString(w, page(0))
		}
	}))
	defer srv.Close()
	var buf bytes.Buffer
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/api/SRU", Get: getter(srv)}, &buf); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 11 || queries[0] != `bib.dewey all "510"@1` || queries[6] != `bib.dewey all "515"@2` {
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
	if len(rs) != 2 {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.NativeID != "cb1234567x" || r.Title != "Calcul différentiel" || r.Date != "2001" || r.Language != "fr" || r.Publisher != "Hermann" || r.Links[0] != "https://catalogue.bnf.fr/ark:/12148/cb1234567x" ||
		r.IDs["isbn"][0] != "9781584882107" || r.Creators[0].Name != "Cartan, Henri" || r.Creators[0].Role != "author" || r.Subjects[0].Code != "515.33" {
		t.Errorf("record %+v", r)
	}
	if rs[1].NativeID != "cb2222222q" || rs[1].Subjects[0].Code != "515.37" {
		t.Errorf("record %+v", rs[1])
	}
}

func TestOneQuery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if q := r.URL.Query().Get("query"); q != `bib.dewey all "512"` {
			t.Errorf("query %q", q)
		}
		_, _ = io.WriteString(w, `<srw:searchRetrieveResponse xmlns:srw="http://www.loc.gov/zing/srw/"><srw:diagnostics><diag:diagnostic xmlns:diag="http://www.loc.gov/zing/srw/diagnostic/"><diag:uri>info:srw/diagnostic/1/10</diag:uri><diag:details>bad query</diag:details></diag:diagnostic></srw:diagnostics></srw:searchRetrieveResponse>`)
	}))
	defer srv.Close()
	var buf bytes.Buffer
	err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + `/api/SRU?query=bib.dewey+all+%22512%22`, Get: getter(srv)}, &buf)
	if err == nil || !strings.Contains(err.Error(), "bad query") {
		t.Errorf("the diagnostic gives the error %v", err)
	}
}
