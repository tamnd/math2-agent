package fetch

import (
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/scope"
)

func TestThemaMath(t *testing.T) {
	for in, want := range map[string]bool{
		"thema EDItEUR::P Mathematics and Science::PB Mathematics::PBT Probability and statistics": true,
		"bic Book Industry Communication::P Mathematics & science::PB Mathematics":                 true,
		"thema EDItEUR::P Mathematics and Science::PH Physics":                                     false,
		"thema EDItEUR::J Society and Social Sciences::JN Education":                               false,
	} {
		if got := ThemaMath(in); got != want {
			t.Errorf("ThemaMath(%q) = %v", in, got)
		}
	}
}

// oaiServer serves the pages of an OAI-PMH endpoint: page 1 for the first request and page 2 for the resumption token t2.
func oaiServer(t *testing.T, set, page1, page2 string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("verb") != "ListRecords":
			t.Errorf("query = %v", q)
		case q.Get("resumptionToken") == "t2":
			fmt.Fprint(w, page2)
		case q.Get("metadataPrefix") == "oai_dc" && q.Get("set") == set:
			fmt.Fprint(w, page1)
		default:
			t.Errorf("query = %v", q)
			fmt.Fprint(w, `<OAI-PMH><error code="badArgument">bad</error></OAI-PMH>`)
		}
	}))
}

func oaiPage(token string, records ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?><OAI-PMH xmlns="http://www.openarchives.org/OAI/2.0/"><ListRecords>` + strings.Join(records, "") +
		`<resumptionToken completeListSize="3">` + token + `</resumptionToken></ListRecords></OAI-PMH>`
}

func dcRec(id, body string) string {
	return `<record><header><identifier>` + id + `</identifier></header><metadata><oai_dc:dc xmlns:oai_dc="http://www.openarchives.org/OAI/2.0/oai_dc/" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:oaire="https://raw.githubusercontent.com/rcic/openaire4/master/schemas/4.0/oaire.xsd">` + body + `</oai_dc:dc></metadata></record>`
}

func TestOAIBooks(t *testing.T) {
	book := dcRec("oai:library.oapen.org:20.500.12657/1", `<dc:title>Numerical  analysis</dc:title><dc:creator>Doe, Jane</dc:creator>`+
		`<dc:subject>splines</dc:subject><dc:subject type="classification">thema EDItEUR::P Mathematics and Science::PB Mathematics::PBK Calculus and mathematical analysis</dc:subject>`+
		`<dc:date>2022-12-22T16:07:01Z</dc:date><dc:date type="Issued">2021</dc:date><oaire:resourceType>book</oaire:resourceType>`+
		`<dc:identifier>https://doi.org/10.1515/ABC</dc:identifier><dc:alternateIdentifier type="ISBN">9783110000009</dc:alternateIdentifier><dc:language>ger</dc:language>`)
	history := dcRec("oai:library.oapen.org:20.500.12657/2", `<dc:title>History</dc:title><dc:subject type="classification">thema EDItEUR::N History and Archaeology</dc:subject>`)
	chapter := dcRec("oai:library.oapen.org:20.500.12657/3", `<dc:title>Chapter</dc:title><dc:subject type="classification">bic Book Industry Communication::P Mathematics &amp; science::PB Mathematics</dc:subject><oaire:resourceType>chapter</oaire:resourceType>`)
	gone := `<record><header status="deleted"><identifier>oai:library.oapen.org:20.500.12657/4</identifier></header></record>`
	srv := oaiServer(t, "books", oaiPage("t2", book, history, gone), oaiPage("", chapter))
	defer srv.Close()
	rs, info := collect(t, OAIBooks{Catalogue: "oapen", Base: srv.URL, Set: "books"}, 10)
	if info.Total != 2 || len(rs) != 2 {
		t.Fatalf("info = %+v, records = %+v", info, rs)
	}
	r := rs[0]
	if r.NativeID != "20.500.12657/1" || r.Kind != "monograph" || r.Title != "Numerical analysis" || r.Date != "2021" || r.Language != "de" ||
		r.IDs["doi"][0] != "10.1515/abc" || r.IDs["isbn"][0] != "9783110000009" || r.IDs["handle"][0] != "20.500.12657/1" || !r.Signals.MathHeading || r.Weight != 1 {
		t.Errorf("record 0 = %+v", r)
	}
	if rs[1].Kind != "chapter" || rs[1].Date != "" {
		t.Errorf("record 1 = %+v", rs[1])
	}
}

func TestEuDML(t *testing.T) {
	a := dcRec("oai:eudml.org:doc/94429", `<dc:title>Sur les espaces</dc:title><dc:creator>Bernard, Jean</dc:creator><dc:date>1994</dc:date><dc:type>text</dc:type>`+
		`<dc:identifier>urn:eudml:doc:94429</dc:identifier><dc:identifier>doi:10.5802/AIF.1</dc:identifier><dc:language>fre</dc:language>`)
	b := dcRec("oai:eudml.org:doc/7", `<dc:title>B</dc:title>`)
	srv := oaiServer(t, "", oaiPage("t2", a), oaiPage("", b))
	defer srv.Close()
	rs, info := collect(t, EuDML{Base: srv.URL}, 10)
	if info.Total != 2 || len(rs) != 2 {
		t.Fatalf("info = %+v, records = %+v", info, rs)
	}
	r := rs[0]
	if r.NativeID != "doc/94429" || r.Kind != "article" || r.Date != "1994" || r.Language != "fr" || r.IDs["eudml"][0] != "94429" || len(r.IDs["eudml"]) != 1 || r.IDs["doi"][0] != "10.5802/aif.1" {
		t.Errorf("record 0 = %+v", r)
	}
	if rs[1].IDs["eudml"][0] != "7" {
		t.Errorf("record 1 = %+v", rs[1])
	}
}

func TestCyberLeninka(t *testing.T) {
	math := dcRec("oai:cyberleninka.ru:article/1", `<dc:title>О числах</dc:title><dc:creator>Иванов И. И.</dc:creator><dc:subject>Математика</dc:subject>`+
		`<dc:date>2015</dc:date><dc:identifier>https://cyberleninka.ru/article/n/o-chislah</dc:identifier><dc:language>rus</dc:language>`)
	other := dcRec("oai:cyberleninka.ru:article/2", `<dc:title>История</dc:title><dc:subject>История</dc:subject>`)
	srv := oaiServer(t, "", oaiPage("", math, other), "")
	defer srv.Close()
	rs, info := collect(t, CyberLeninka{Base: srv.URL}, 10)
	if info.Total != 1 || len(rs) != 1 {
		t.Fatalf("info = %+v, records = %+v", info, rs)
	}
	r := rs[0]
	if r.NativeID != "o-chislah" || r.IDs["cyberleninka"][0] != "o-chislah" || r.Language != "ru" || r.Date != "2015" || !r.Signals.MathHeading || r.Authors[0] != "Иванов И. И." {
		t.Errorf("record = %+v", r)
	}
}

const locRecord = `<zs:record><zs:recordData><record xmlns="http://www.loc.gov/MARC21/slim"><leader>06155cam a2200469 i 4500</leader>
<controlfield tag="001">22679267</controlfield><controlfield tag="008">220705t20202020maua     b    001 0 eng d</controlfield>
<datafield tag="010" ind1=" " ind2=" "><subfield code="a">  2022303398</subfield></datafield>
<datafield tag="020" ind1=" " ind2=" "><subfield code="a">9781642656930</subfield><subfield code="q">(hardcover)</subfield></datafield>
<datafield tag="035" ind1=" " ind2=" "><subfield code="a">(OCoLC)1199015254</subfield></datafield>
<datafield tag="050" ind1="0" ind2="0"><subfield code="a">QA36</subfield><subfield code="b">.P75 2020</subfield></datafield>
<datafield tag="082" ind1="0" ind2="4"><subfield code="a">510</subfield></datafield>
<datafield tag="100" ind1="1" ind2=" "><subfield code="a">Boslaugh, Sarah,</subfield></datafield>
<datafield tag="245" ind1="0" ind2="0"><subfield code="a">Principles of mathematics /</subfield></datafield>
<datafield tag="650" ind1=" " ind2="0"><subfield code="a">Mathematics.</subfield></datafield>
</record></zs:recordData></zs:record>`

func TestLoC(t *testing.T) {
	var pages atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("operation") != "searchRetrieve" || q.Get("recordSchema") != "marcxml" || !strings.HasPrefix(q.Get("query"), locQuery+" and dc.date=") {
			t.Errorf("query = %v", q)
		}
		date := strings.TrimPrefix(q.Get("query"), locQuery+" and dc.date=")
		total := 0
		switch date {
		case "202*":
			total = 20000 // more than the window, so the sample reads each year
		case "2020":
			total = 30
		case "199*":
			total = 10
		}
		head := fmt.Sprintf(`<zs:searchRetrieveResponse xmlns:zs="http://www.loc.gov/zing/srw/"><zs:numberOfRecords>%d</zs:numberOfRecords><zs:records>`, total)
		if q.Get("maximumRecords") == "0" || total == 0 {
			fmt.Fprint(w, head+`</zs:records></zs:searchRetrieveResponse>`)
			return
		}
		pages.Add(1)
		fmt.Fprint(w, head+locRecord+`</zs:records></zs:searchRetrieveResponse>`)
	}))
	defer srv.Close()
	rs, info := collect(t, LoC{Base: srv.URL, LastYear: 2021}, 4)
	if info.Total != 40 || len(rs) != 2 || pages.Load() != 2 {
		t.Fatalf("info = %+v, records = %d, pages = %d", info, len(rs), pages.Load())
	}
	r := rs[0]
	if r.NativeID != "22679267" || r.Kind != "monograph" || r.Date != "2020" || r.Language != "en" || r.Title != "Principles of mathematics" || r.Authors[0] != "Boslaugh, Sarah" ||
		r.IDs["lccn"][0] != "2022303398" || r.IDs["isbn"][0] != "9781642656930" || r.IDs["oclc"][0] != "1199015254" || r.Signals.LCC[0] != "QA36" || r.Signals.DDC[0] != "510" || !r.Signals.MathHeading || r.Weight != 10 {
		t.Errorf("record = %+v", r)
	}
	if d := scope.Decide(r.Signals, scope.Env{}); d.Value != resource.In {
		t.Errorf("Decide = %+v", d)
	}
}

func TestLoCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<zs:searchRetrieveResponse xmlns:zs="http://www.loc.gov/zing/srw/"><zs:diagnostics><diag:diagnostic xmlns:diag="x"><diag:details>offset too large</diag:details></diag:diagnostic></zs:diagnostics></zs:searchRetrieveResponse>`)
	}))
	defer srv.Close()
	if _, err := (LoC{Base: srv.URL}).Sample(t.Context(), client(), 10, 1, nil); err == nil || !strings.Contains(err.Error(), "offset too large") {
		t.Errorf("err = %v", err)
	}
}

func TestSemanticScholar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/paper/search/bulk" || q.Get("fieldsOfStudy") != "Mathematics" || r.URL.Query().Has("key") {
			t.Errorf("request = %v", r.URL)
		}
		if q.Get("year") != "2020-2021" {
			fmt.Fprint(w, `{"total":0,"data":[]}`)
			return
		}
		if q.Get("fields") == "paperId" {
			fmt.Fprint(w, `{"total":300,"token":"x","data":[{"paperId":"a"}]}`)
			return
		}
		p := `{"paperId":"%s","title":"T","year":2020,"externalIds":{"DOI":"10.1/X","ArXiv":"2001.00001","CorpusId":%d},"publicationTypes":["JournalArticle"],"authors":[{"name":"A B"}]}`
		if q.Get("token") == "" {
			fmt.Fprintf(w, `{"total":300,"token":"t2","data":[`+p+`]}`, "a1", 11)
			return
		}
		fmt.Fprintf(w, `{"total":300,"data":[`+p+`,`+p+`]}`, "a2", 12, "a3", 13)
	}))
	defer srv.Close()
	rs, info := collect(t, SemanticScholar{Base: srv.URL, LastYear: 2021}, 2)
	if info.Total != 300 || len(rs) != 2 {
		t.Fatalf("info = %+v, records = %+v", info, rs)
	}
	r := rs[0]
	if r.NativeID != "a1" || r.Kind != "article" || r.Date != "2020" || r.IDs["s2"][0] != "11" || r.IDs["doi"][0] != "10.1/x" || r.IDs["arxiv"][0] != "2001.00001" || r.Weight != 150 {
		t.Errorf("record 0 = %+v", r)
	}
	if rs[1].NativeID != "a2" {
		t.Errorf("record 1 = %+v", rs[1])
	}
}

const mnIssuePage = `<a class="ry" href="/eng/im/authornotes">Guidelines</a>
<td colspan=2 width=90% valign=top align=left>
<a class=SLink href="/eng/im8810">Linear forms of a given Diophantine type</a><br>O.&nbsp;N.&nbsp;German
<div style='font-size: small'><i>Izvestiya: Mathematics, 2020, Pages 3&ndash;22<br/>DOI:&nbsp;<a class=SLink href='https://doi.org/10.1070/IM8810'>https://doi.org/10.1070/IM8810</a></i></div>
<div><i>Izv. RAN, DOI:&nbsp;<a class=SLink href='https://doi.org/10.4213/im8810'>x</a></i></div></td>
<td colspan=2 width=90% valign=top align=left>
<a class=SLink href="/eng/im8786">On mixed dynamics of&nbsp;<i>two</i>-dimensional maps</a><br>S.&nbsp;V.&nbsp;Gonchenko, M.&nbsp;S.&nbsp;Gonchenko
<div style='font-size: small'></div></td>`

func TestMathNetRu(t *testing.T) {
	var issues atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case r.URL.Path == "/ej.phtml":
			fmt.Fprint(w, `<A href=/php/journal.phtml?jrnid=im&option_lang=eng>x</a><A href=/php/journal.phtml?jrnid=im&option_lang=eng>y</a>`)
		case q.Get("wshow") == "contents" && q.Get("jrnid") == "im":
			for i := 1; i <= 4; i++ {
				fmt.Fprintf(w, `<a class=SLink href=/php/archive.phtml?jrnid=im&wshow=issue&year=2020&volume=84&issue=%d&option_lang=eng>%d</a>`+"\n", i, i)
			}
			fmt.Fprint(w, `<a href=/php/archive.phtml?jrnid=sm&wshow=issue&year=2020&volume=1&issue=1&option_lang=eng>other</a>`)
		case q.Get("wshow") == "issue" && q.Get("jrnid") == "im" && q.Get("year") == "2020":
			issues.Add(1)
			fmt.Fprint(w, mnIssuePage)
		default:
			t.Errorf("request = %v", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	rs, info := collect(t, MathNetRu{Base: srv.URL}, 3)
	if issues.Load() != 2 || len(rs) != 4 || info.Total != 8 {
		t.Fatalf("info = %+v, records = %d, issues = %d", info, len(rs), issues.Load())
	}
	r := rs[0]
	if r.NativeID != "im8810" || r.IDs["mathnetru"][0] != "im8810" || r.Date != "2020" || r.Title != "Linear forms of a given Diophantine type" || r.Authors[0] != "O. N. German" ||
		len(r.IDs["doi"]) != 2 || r.IDs["doi"][1] != "10.4213/im8810" || r.Weight != 2 {
		t.Errorf("record 0 = %+v", r)
	}
	if rs[1].Title != "On mixed dynamics of two-dimensional maps" || len(rs[1].Authors) != 2 || len(rs[1].IDs["doi"]) != 0 {
		t.Errorf("record 1 = %+v", rs[1])
	}
}

const dblpDump = `<?xml version="1.0" encoding="ISO-8859-1"?>
<!DOCTYPE dblp SYSTEM "dblp.dtd">
<dblp>
<article mdate="2020-01-01" key="journals/dm/Erdos90"><author>Paul Erd&ouml;s</author><title>On <i>sums</i>.</title><year>1990</year><ee>https://doi.org/10.1016/0012-365X(90)90001-A</ee></article>
<www key="homepages/e/PaulErdos"><author>Paul Erd&ouml;s</author><title>Home Page</title></www>
<article mdate="2020-01-01" key="journals/corr/abs-2001-00001" publtype="informal"><author>Ann B</author><title>T.</title><year>2020</year><ee>https://arxiv.org/abs/2001.00001</ee></article>
<book key="books/x/Knuth97"><editor>Don Knuth</editor><title>The Art</title><year>1997</year><isbn>0-201-89683-4</isbn></book>
</dblp>`

func TestDBLP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dblp.xml.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := gzip.NewWriter(f)
	// The dump is in ISO-8859-1, so the byte 0xE9 is é.
	if _, err := z.Write([]byte(strings.Replace(dblpDump, "Ann B", "Ann B\xe9", 1))); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	rs, info := collect(t, DBLP{File: path}, 10)
	if info.Total != 3 || len(rs) != 3 {
		t.Fatalf("info = %+v, records = %+v", info, rs)
	}
	r := rs[0]
	if r.NativeID != "journals/dm/Erdos90" || r.Kind != "article" || r.Title != "On sums" || r.Authors[0] != "Paul Erdös" || r.Date != "1990" || r.IDs["doi"][0] != "10.1016/0012-365x(90)90001-a" || r.Weight != 1 {
		t.Errorf("record 0 = %+v", r)
	}
	if rs[1].Kind != "preprint" || rs[1].IDs["arxiv"][0] != "2001.00001" || rs[1].Authors[0] != "Ann Bé" {
		t.Errorf("record 1 = %+v", rs[1])
	}
	if rs[2].Kind != "monograph" || rs[2].Authors[0] != "Don Knuth" || rs[2].IDs["isbn"][0] != "0-201-89683-4" {
		t.Errorf("record 2 = %+v", rs[2])
	}
	rs, info = collect(t, DBLP{File: path}, 2)
	if info.Total != 3 || len(rs) != 2 || rs[0].Weight != 1.5 {
		t.Errorf("info = %+v, records = %+v", info, rs)
	}
	if _, err := (DBLP{}).Sample(t.Context(), nil, 10, 1, nil); err == nil {
		t.Error("no error with no file")
	}
}

func TestHathi(t *testing.T) {
	row := func(htid, key, date, format string) string {
		cols := make([]string, hfColumns)
		cols[hfHTID], cols[hfBibKey], cols[hfDate], cols[hfFormat] = htid, key, date, format
		cols[hfOCLC], cols[hfISBN], cols[hfLCCN] = "2779601", "8081281584,9788081281587", "70518371"
		cols[hfTitle], cols[hfLang], cols[hfAuthor] = "Slovenské vysťahovalectvo / Zost. František Bielik.", "slo", "Bielik, František,"
		return strings.Join(cols, "\t")
	}
	data := strings.Join([]string{
		row("mdp.39015018415946", "000000001", "1969", "BK"),
		row("mdp.39015066356547", "000000001", "1969", "BK"), // a second volume of the same record
		row("uc1.b000000002", "000000002", "9999", "SE"),
		"short\tline",
	}, "\n") + "\n"
	path := filepath.Join(t.TempDir(), "hathi_full_20261001.txt")
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	rs, info := collect(t, Hathi{File: path}, 10)
	if info.Total != 2 || len(rs) != 2 {
		t.Fatalf("info = %+v, records = %+v", info, rs)
	}
	r := rs[0]
	if r.NativeID != "1" || r.Kind != "monograph" || r.Date != "1969" || r.Language != "sk" || r.Title != "Slovenské vysťahovalectvo / Zost. František Bielik" ||
		r.Authors[0] != "Bielik, František" || r.IDs["hathi"][0] != "mdp.39015018415946" || r.IDs["hathi_record"][0] != "1" || len(r.IDs["isbn"]) != 2 || r.IDs["lccn"][0] != "70518371" {
		t.Errorf("record 0 = %+v", r)
	}
	if rs[1].Kind != "journal" || rs[1].Date != "" {
		t.Errorf("record 1 = %+v", rs[1])
	}
}
