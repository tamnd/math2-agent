package ndl

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

// record gives one record in the schema dcndl, as the SRU server of NDL Search gives it.
func record(id, title, ndc string) string {
	provider, _, _ := strings.Cut(id, "-")
	return fmt.Sprintf(`<record><recordSchema>info:srw/schema/1/dc-v1.1</recordSchema><recordPacking>xml</recordPacking><recordData>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns:rdfs="http://www.w3.org/2000/01/rdf-schema#" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcndl="http://ndl.go.jp/dcndl/terms/" xmlns:foaf="http://xmlns.com/foaf/0.1/">
<dcndl:BibAdminResource rdf:about="https://ndlsearch.ndl.go.jp/books/%[1]s"><dcterms:description>type : book</dcterms:description><dcndl:bibRecordCategory>%[2]s</dcndl:bibRecordCategory>
<dcndl:record rdf:resource="https://ndlsearch.ndl.go.jp/books/%[1]s#material"/></dcndl:BibAdminResource>
<dcndl:BibResource rdf:about="https://ndlsearch.ndl.go.jp/books/%[1]s#material">
<dcterms:identifier rdf:datatype="http://ndl.go.jp/dcndl/terms/ISBN">4254114591</dcterms:identifier>
<dcterms:title>%[3]s</dcterms:title>
<dc:title><rdf:Description><rdf:value>%[3]s</rdf:value><dcndl:transcription>アーベル</dcndl:transcription></rdf:Description></dc:title>
<dcndl:seriesTitle><rdf:Description><rdf:value>数学史叢書</rdf:value></rdf:Description></dcndl:seriesTitle>
<dcterms:creator><foaf:Agent><foaf:name>高瀬, 正仁</foaf:name><dcndl:transcription>タカセ, マサヒト</dcndl:transcription></foaf:Agent></dcterms:creator>
<dc:creator>高瀬正仁訳</dc:creator>
<dcterms:publisher><foaf:Agent><foaf:name>朝倉書店</foaf:name></foaf:Agent></dcterms:publisher>
<dcterms:date>1998.4</dcterms:date><dcterms:issued rdf:datatype="http://purl.org/dc/terms/W3CDTF">1998</dcterms:issued>
<dcterms:subject><rdf:Description><rdf:value>楕円関数</rdf:value></rdf:Description></dcterms:subject>
<dcterms:subject rdf:resource="http://id.ndl.go.jp/class/ndlc/MA131"/>
<dcterms:subject rdf:resource="http://id.ndl.go.jp/class/ndc9/%[4]s"/>
<dc:subject rdf:datatype="http://ndl.go.jp/dcndl/terms/NDC8">%[5]s</dc:subject>
<dcterms:language rdf:datatype="http://purl.org/dc/terms/ISO639-2">jpn</dcterms:language>
</dcndl:BibResource>
<dcndl:BibResource rdf:about="https://ndlsearch.ndl.go.jp/books/%[1]s#material"><dcndl:record rdf:resource="https://ndlsearch.ndl.go.jp/books/%[1]s#item"/></dcndl:BibResource>
</rdf:RDF></recordData><recordPosition>1</recordPosition></record>`, id, provider, title, ndc, wide(ndc))
}

// wide gives the digits of a class in full width, as some records of NDL Search give them.
func wide(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r - '0' + '０'
		}
		return r
	}, strings.Split(s, ".")[0])
}

func page(next int, records ...string) string {
	n := ""
	if next > 0 {
		n = fmt.Sprintf("<nextRecordPosition>%d</nextRecordPosition>", next)
	}
	return `<searchRetrieveResponse xmlns="http://www.loc.gov/zing/srw/"><version>1.2</version><numberOfRecords>3</numberOfRecords>` + n + `<records>` + strings.Join(records, "") + `</records></searchRetrieveResponse>`
}

func TestFetchAndRecords(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("recordSchema") != "dcndl" || q.Get("maximumRecords") != "200" || q.Get("operation") != "searchRetrieve" {
			t.Errorf("request %s", r.URL)
		}
		queries = append(queries, q.Get("query")+"@"+q.Get("startRecord"))
		switch q.Get("query") + "@" + q.Get("startRecord") {
		case `ndc="413"@1`:
			_, _ = io.WriteString(w, page(3, record("R100000002-I000002723941", "アーベル/ガロア楕円関数論", "413.57"), record("R100000136-I1970304959838111639", "楕円関数論", "413.57")))
		case `ndc="413"@3`:
			_, _ = io.WriteString(w, page(0, record("R100000002-I000000000001", "日本史", "210")))
		default:
			_, _ = io.WriteString(w, page(0))
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
	if err := (Adapter{}).Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/api/sru", Get: get}, &buf); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 11 {
		t.Errorf("queries %v", queries)
	}
	snap, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(snap)
	if strings.Contains(string(data), `"title":"楕円関数論"`) || strings.Count(string(data), "\n") != 2 {
		t.Errorf("the snapshot keeps more than the ids of a record of another provider:\n%s", data)
	}
	var recs []ingest.Record
	if err := (Adapter{}).Read(bytes.NewReader(data), func(r ingest.Record) error { recs = append(recs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[1].Title != "" || recs[1].Meta["provider"] != "R100000136" {
		t.Errorf("ingest records %+v", recs)
	}
	var rs []cluster.Record
	if err := (Adapter{}).Records(bytes.NewReader(data), func(r cluster.Record) error { rs = append(rs, r); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("records %+v", rs)
	}
	r := rs[0]
	if r.NativeID != "R100000002-I000002723941" || r.Title != "アーベル/ガロア楕円関数論" || r.Date != "1998" || r.Language != "ja" || r.Publisher != "朝倉書店" ||
		r.Links[0] != "https://ndlsearch.ndl.go.jp/books/R100000002-I000002723941" || r.IDs["isbn"][0] != "9784254114591" || r.Creators[0].Name != "高瀬, 正仁" ||
		len(r.Subjects) != 2 || r.Subjects[0].Code != "413.57" || r.Subjects[1].Code != "413" {
		t.Errorf("record %+v", r)
	}
}
