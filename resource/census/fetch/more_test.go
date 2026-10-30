package fetch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/scope"
)

func TestLCCRange(t *testing.T) {
	for in, want := range map[string]string{"QA1-939": "QA1", "QA75.5-76.95": "QA75.5", "QA": "QA", "QA299.6-433": "QA299.6", " T57-57.97 ": "T57"} {
		if got := LCCRange(in); got != want {
			t.Errorf("LCCRange(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDataCite(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != dcQuery {
			t.Errorf("query = %q", r.URL.Query().Get("query"))
		}
		if r.URL.Query().Get("page[size]") == "0" {
			fmt.Fprint(w, `{"data":[],"meta":{"total":1000}}`)
			return
		}
		// The second page repeats the first, so the sample ends.
		calls.Add(1)
		fmt.Fprint(w, `{"data":[{"attributes":{"doi":"10.5281/ZENODO.1","titles":[{"title":"T"}],"publicationYear":2024,"language":"en",`+
			`"creators":[{"name":"Doe, J"}],"subjects":[{"subject":"FOS: Mathematics"},{"subject":"11A41","subjectScheme":"MSC"}],`+
			`"types":{"resourceTypeGeneral":"Preprint"},"relatedIdentifiers":[{"relatedIdentifier":"arXiv:2401.00001","relatedIdentifierType":"arXiv"}]}}],"meta":{"total":1000}}`)
	}))
	defer srv.Close()
	rs, info := collect(t, DataCite{Base: srv.URL}, 10)
	if info.Total != 1000 || len(rs) != 1 || calls.Load() != 2 {
		t.Fatalf("info = %+v, records = %d, calls = %d", info, len(rs), calls.Load())
	}
	r := rs[0]
	if r.NativeID != "10.5281/zenodo.1" || r.Kind != "preprint" || r.Date != "2024" || r.IDs["arxiv"][0] != "2401.00001" || !r.Signals.MathHeading || r.Weight != 1000 {
		t.Errorf("record = %+v", r)
	}
	if d := scope.Decide(r.Signals, scope.Env{}); d.Value != resource.In {
		t.Errorf("Decide = %+v", d)
	}
}

func TestDOAJ(t *testing.T) {
	var days atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := strings.TrimPrefix(r.URL.Path, "/search/articles/")
		if !strings.HasPrefix(q, doajQuery) {
			t.Errorf("query = %q", q)
		}
		if !strings.Contains(q, "created_date") {
			fmt.Fprint(w, `{"total":500,"results":[]}`)
			return
		}
		days.Add(1)
		fmt.Fprint(w, `{"total":1,"results":[{"id":"abc","bibjson":{"title":"T","year":"2012","identifier":[{"id":"10.1155/X","type":"doi"}],`+
			`"author":[{"name":"A"}],"subject":[{"code":"QA1-939","scheme":"LCC","term":"Mathematics"}],"journal":{"language":["EN"]}}}]}`)
	}))
	defer srv.Close()
	rs, info := collect(t, DOAJ{Base: srv.URL, Today: "2004-01-10"}, 3)
	if info.Total != 500 || len(rs) != 3 || days.Load() != 3 {
		t.Fatalf("info = %+v, records = %d, days = %d", info, len(rs), days.Load())
	}
	r := rs[0]
	if r.IDs["doi"][0] != "10.1155/x" || r.Language != "en" || r.Signals.LCC[0] != "QA1" || r.Weight != 500.0/3 {
		t.Errorf("record = %+v", r)
	}
	if d := scope.Decide(r.Signals, scope.Env{}); d.Value != resource.In {
		t.Errorf("Decide = %+v", d)
	}
}

func TestGutenberg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != GutenbergCatalogue {
			t.Errorf("path = %q", r.URL.Path)
		}
		fmt.Fprint(w, "Text#,Type,Issued,Title,Language,Authors,Subjects,LoCC,Bookshelves\n"+
			"33283,Text,2010-10-09,Calculus Made Easy,en,\"Thompson, Silvanus P.\",Calculus -- Textbooks,QA,Category: Mathematics\n"+
			"50,Dataset,1993-01-01,Pi,en,\"Hemphill, Scott\",Mathematics; Pi,QA,Mathematics\n"+
			"1019,Text,1997-08-01,Poems,fr,\"Bell, Currer\",English poetry,PR,Category: Poetry\n"+
			"7,Text,2001-01-01,Tables,de,,Tables,QA,\n")
	}))
	defer srv.Close()
	rs, info := collect(t, Gutenberg{Base: srv.URL}, 10)
	if info.Total != 2 || len(rs) != 2 {
		t.Fatalf("info = %+v, records = %+v", info, rs)
	}
	if rs[0].IDs["gutenberg"][0] != "33283" || !rs[0].Signals.MathHeading || rs[0].Authors[0] != "Thompson, Silvanus P." || rs[0].Weight != 1 {
		t.Errorf("record 0 = %+v", rs[0])
	}
	if rs[1].NativeID != "7" || rs[1].Signals.MathHeading || rs[1].Language != "de" || len(rs[1].Signals.LCC) != 0 {
		t.Errorf("record 1 = %+v", rs[1])
	}
}

func TestGutenbergColumns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "Text#,Title\n1,X\n")
	}))
	defer srv.Close()
	if _, err := (Gutenberg{Base: srv.URL}).Sample(t.Context(), client(), 10, 1, nil); err == nil || !strings.Contains(err.Error(), "no column") {
		t.Errorf("err = %v", err)
	}
}

func TestArchive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != archiveQuery {
			t.Errorf("q = %q", r.URL.Query().Get("q"))
		}
		if r.URL.Query().Get("cursor") == "" {
			fmt.Fprint(w, `{"items":[{"identifier":"arxiv-1512.05323","title":"T","year":2015,"subject":["Mathematics"]},`+
				`{"identifier":"calculus00spiv","title":["Calculus"],"year":"1967","language":"eng","creator":["Spivak, Michael"],"call_number":"QA303 .S64","isbn":["0805390219"]}],"cursor":"c1","total":3}`)
			return
		}
		fmt.Fprint(w, `{"items":[{"identifier":"x","title":"X","subject":"Education"}],"total":1}`)
	}))
	defer srv.Close()
	rs, info := collect(t, Archive{Base: srv.URL}, 10)
	if info.Total != 3 || len(rs) != 3 {
		t.Fatalf("info = %+v, records = %d", info, len(rs))
	}
	if rs[0].Kind != "preprint" || rs[0].IDs["arxiv"][0] != "1512.05323" || rs[0].Date != "2015" || !rs[0].Signals.MathHeading {
		t.Errorf("record 0 = %+v", rs[0])
	}
	b := rs[1]
	if b.Kind != "monograph" || b.Title != "Calculus" || b.Language != "en" || b.Signals.LCC[0] != "QA303" || b.Authors[0] != "Spivak, Michael" {
		t.Errorf("record 1 = %+v", b)
	}
	if d := scope.Decide(b.Signals, scope.Env{}); d.Value != resource.In {
		t.Errorf("Decide = %+v", d)
	}
}

func TestWikidata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("query"), "OFFSET 0") {
			t.Errorf("query = %q", r.URL.Query().Get("query"))
		}
		fmt.Fprint(w, `{"results":{"bindings":[{"x":{"value":"http://www.wikidata.org/entity/Q1"},"title":{"value":"On primes"},`+
			`"date":{"value":"1990-01-01T00:00:00Z"},"doi":{"value":"10.1/ABC"},"zbmath":{"value":"0845.05050"},"subjects":{"value":"Q12479 Q395"},"kinds":{"value":"Q5 Q13442814"}}]}}`)
	}))
	defer srv.Close()
	rs, info := collect(t, Wikidata{Base: srv.URL}, 10)
	if info.Total != 1 || len(rs) != 1 {
		t.Fatalf("info = %+v, records = %d", info, len(rs))
	}
	r := rs[0]
	if r.NativeID != "Q1" || r.Kind != "article" || r.Date != "1990" || r.IDs["doi"][0] != "10.1/abc" || r.IDs["zbl"][0] != "0845.05050" || len(r.Signals.MainSubjects) != 2 || !r.Signals.ZBMath {
		t.Errorf("record = %+v", r)
	}
}

const numdamPage1 = `<?xml version="1.0" encoding="UTF-8"?>
<OAI-PMH xmlns="http://www.openarchives.org/OAI/2.0/"><ListRecords>
<record><header><identifier>oai:numdam.org:MSH_1994__126__5_0</identifier></header>
<metadata><article xmlns="http://jats.nlm.nih.gov" xml:lang="fr"><front><journal-meta><issn>0995-2314</issn></journal-meta>
<article-meta><article-id pub-id-type="mathdoc-id">MSH_1994__126__5_0</article-id>
<title-group><article-title>Analyse  descriptive</article-title></title-group>
<contrib-group content-type="authors"><contrib><name><surname>Bernard</surname><given-names>Jean-Marc</given-names></name></contrib></contrib-group>
<ext-link ext-link-type="zbl-item-id">0845.05050</ext-link><ext-link ext-link-type="eudml-item-id">urn:eudml:doc:94429</ext-link>
</article-meta></front><back><ref-list><ref><ext-link ext-link-type="zbl-item-id">0271.62044</ext-link></ref></ref-list></back></article></metadata></record>
<record><header status="deleted"><identifier>oai:numdam.org:GONE</identifier></header></record>
<resumptionToken completeListSize="2">eudml-article2:100::2::</resumptionToken>
</ListRecords></OAI-PMH>`

const numdamPage2 = `<?xml version="1.0" encoding="UTF-8"?>
<OAI-PMH xmlns="http://www.openarchives.org/OAI/2.0/"><ListRecords>
<record><header><identifier>oai:numdam.org:AIF_2001__51_1_1_0</identifier></header>
<metadata><article xmlns="http://jats.nlm.nih.gov" xml:lang="en"><front><article-meta>
<article-id pub-id-type="doi">10.5802/AIF.1</article-id><title-group><article-title>Sheaves</article-title></title-group>
</article-meta></front></article></metadata></record>
<resumptionToken completeListSize="2"></resumptionToken>
</ListRecords></OAI-PMH>`

func TestNumdam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch {
		case q.Get("metadataPrefix") == "eudml-article2":
			fmt.Fprint(w, numdamPage1)
		case q.Get("resumptionToken") == "eudml-article2:100::2::":
			fmt.Fprint(w, numdamPage2)
		default:
			t.Errorf("query = %v", q)
			fmt.Fprint(w, `<OAI-PMH><error code="badArgument">bad</error></OAI-PMH>`)
		}
	}))
	defer srv.Close()
	rs, info := collect(t, Numdam{Base: srv.URL + "/oai/"}, 10)
	if info.Total != 2 || len(rs) != 2 {
		t.Fatalf("info = %+v, records = %d", info, len(rs))
	}
	r := rs[0]
	if r.NativeID != "MSH_1994__126__5_0" || r.Date != "1994" || r.Title != "Analyse descriptive" || r.Language != "fr" || r.Authors[0] != "Bernard, Jean-Marc" ||
		len(r.IDs["zbl"]) != 1 || r.IDs["eudml"][0] != "94429" || !r.Signals.ZBMath {
		t.Errorf("record 0 = %+v", r)
	}
	if rs[1].IDs["doi"][0] != "10.5802/aif.1" || rs[1].Date != "2001" || rs[1].Signals.ZBMath {
		t.Errorf("record 1 = %+v", rs[1])
	}
}

func TestNumdamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<OAI-PMH><error code="badArgument">bad argument</error></OAI-PMH>`)
	}))
	defer srv.Close()
	if _, err := (Numdam{Base: srv.URL}).Sample(t.Context(), client(), 10, 1, nil); err == nil || !strings.Contains(err.Error(), "bad argument") {
		t.Errorf("err = %v", err)
	}
}
