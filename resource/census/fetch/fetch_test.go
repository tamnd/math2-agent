package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
	"github.com/tamnd/math2-agent/resource/scope"
)

func TestLCC(t *testing.T) {
	cases := map[string]string{
		"QA-0043.00000000":     "QA43",
		"QA-0076.73000000.P98": "QA76.73.P98",
		"QA-0009.58000000":     "QA9.58",
		"QA-0000.00000000":     "QA0",
		"QA-0939.00000000.B6":  "QA939.B6",
		"QA300":                "QA300",
	}
	for in, want := range cases {
		if got := LCC(in); got != want {
			t.Errorf("LCC(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHALDomain(t *testing.T) {
	cases := map[string]string{
		"math.math-co":   "math.CO",
		"1.math.math-ag": "math.AG",
		"0.math":         "math",
		"phys.mphy":      "math-ph",
		"info.info-lo":   "cs.LO",
		"stat.stat-th":   "stat.TH",
		"phys":           "phys",
		"phys.hthe":      "",
	}
	for in, want := range cases {
		if got := HALDomain(in); got != want {
			t.Errorf("HALDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLang(t *testing.T) {
	for in, want := range map[string]string{"English": "en", "ger": "de", "fre": "fr", "en": "en", " Russian ": "ru"} {
		if got := Lang(in); got != want {
			t.Errorf("Lang(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestArxivYear(t *testing.T) {
	for in, want := range map[string]string{"0704.0002": "2007", "2310.12345": "2023", "math/9912001": "1999", "hep-th/0101001": "2001", "x": ""} {
		if got := ArxivYear(in); got != want {
			t.Errorf("ArxivYear(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBatches(t *testing.T) {
	got := batches([]string{"a", "b", "c", "d", "e"}, 2)
	if len(got) != 3 || len(got[2]) != 1 {
		t.Errorf("batches = %v", got)
	}
}

func TestWeight(t *testing.T) {
	if w := weight(1000, 100); w != 10 {
		t.Errorf("weight(1000, 100) = %v", w)
	}
	if w := weight(50, 100); w != 1 {
		t.Errorf("weight(50, 100) = %v", w)
	}
}

func client() *Client { return NewClient(0) }

func TestClientRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/missing":
			http.NotFound(w, r)
		case r.URL.Path == "/bad":
			http.Error(w, "bad", http.StatusBadRequest)
		case calls.Add(1) == 1:
			http.Error(w, "busy", http.StatusServiceUnavailable)
		default:
			if r.Header.Get("User-Agent") != UserAgent {
				t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
			}
			fmt.Fprint(w, `{"ok":true}`)
		}
	}))
	defer srv.Close()
	c := client()
	c.Retries = 2
	var v struct{ OK bool }
	if err := c.JSON(context.Background(), srv.URL+"/x", &v); err != nil || !v.OK {
		t.Fatalf("JSON = %v, %+v", err, v)
	}
	if _, err := c.Get(context.Background(), srv.URL+"/missing"); err != ErrNotFound {
		t.Errorf("Get missing = %v", err)
	}
	if _, err := c.Get(context.Background(), srv.URL+"/bad"); err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("Get bad = %v", err)
	}
}

func collect(t *testing.T, s Sampler, n int) ([]census.Record, Info) {
	t.Helper()
	var out []census.Record
	info, err := s.Sample(context.Background(), client(), n, 1, func(r census.Record) error {
		out = append(out, r)
		return nil
	})
	if err != nil {
		t.Fatalf("%s: %v", s.Name(), err)
	}
	return out, info
}

func TestZBMath(t *testing.T) {
	doc := `{"id":7,"identifier":"0346.26002","year":"1977","document_type":{"code":"j"},"title":{"title":"On sums"},"language":{"languages":["English"]},` +
		`"contributors":{"authors":[{"name":"Erdős, Paul"}]},"links":[{"type":"doi","identifier":"10.1000/A"},{"type":"arxiv","identifier":"math/9912001"}],` +
		`"msc":[{"code":"26A42"},{"code":"26A42"}],"source":{"series":[{"issn":[{"number":"0001-8708"}]}]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("search_string")
		switch {
		case q == "py:1970-1979" || q == "py:1977":
			fmt.Fprintf(w, `{"result":[%s],"status":{"nr_total_results":250}}`, doc)
		case strings.HasPrefix(q, "py:"):
			http.NotFound(w, r)
		default:
			if q != "doi:(10.1000/a | 10.1000/b)" {
				t.Errorf("search_string = %q", q)
			}
			fmt.Fprintf(w, `{"result":[%s],"status":{"nr_total_results":1}}`, doc)
		}
	}))
	defer srv.Close()
	z := ZBMath{Base: srv.URL, LastYear: 1979}
	rs, info := collect(t, z, 100)
	if info.Total != 250 || len(rs) != 1 {
		t.Fatalf("info = %+v, records = %d", info, len(rs))
	}
	r := rs[0]
	if r.Kind != "article" || r.Language != "en" || r.Weight != 250 || r.IDs["doi"][0] != "10.1000/A" || len(r.Signals.MSC) != 1 || !r.Signals.ZBMath || r.Signals.Venues[0] != "0001-8708" {
		t.Errorf("record = %+v", r)
	}
	found, err := z.Lookup(context.Background(), client(), []string{"10.1000/a", "10.1000/b", "10.1000/(c)"})
	if err != nil || !found["10.1000/a"] || found["10.1000/b"] {
		t.Errorf("Lookup = %v, %v", found, err)
	}
}

func TestHAL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("rows") == "0" {
			fmt.Fprint(w, `{"response":{"numFound":120,"docs":[]}}`)
			return
		}
		fmt.Fprint(w, `{"response":{"numFound":120,"docs":[{"halId_s":"hal-1","title_s":["T"],"authFullName_s":["A B"],"doiId_s":"10.2000/X",`+
			`"producedDateY_i":2010,"language_s":["fr"],"docType_s":"ART","primaryDomain_s":"math.math-co","domain_s":["0.math","1.math.math-pr"]}]}}`)
	}))
	defer srv.Close()
	h := HAL{Base: srv.URL}
	rs, info := collect(t, h, 100)
	if info.Total != 120 || len(rs) != 1 {
		t.Fatalf("info = %+v, records = %d", info, len(rs))
	}
	r := rs[0]
	if r.Kind != "article" || r.Date != "2010" || strings.Join(r.Signals.Arxiv, ",") != "math.CO,math.PR" {
		t.Errorf("record = %+v", r)
	}
	found, err := h.Lookup(context.Background(), client(), []string{"10.2000/x"})
	if err != nil || !found["10.2000/x"] {
		t.Errorf("Lookup = %v, %v", found, err)
	}
}

func TestOpenLibrary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("subject") != "mathematics" {
			t.Errorf("subject = %q", r.URL.Query().Get("subject"))
		}
		fmt.Fprint(w, `{"numFound":50,"docs":[{"key":"/works/OL1W","title":"Calculus","author_name":["S"],"first_publish_year":1960,`+
			`"language":["eng"],"isbn":["1","2","3","4","5","6"],"lcc":["QA-0303.00000000"],"ddc":["515"]}]}`)
	}))
	defer srv.Close()
	rs, _ := collect(t, OpenLibrary{Base: srv.URL, Subject: "mathematics"}, 100)
	if len(rs) == 0 {
		t.Fatal("no records")
	}
	r := rs[0]
	if r.NativeID != "OL1W" || r.Language != "en" || len(r.IDs["isbn"]) != 5 || r.Signals.LCC[0] != "QA303" || r.Weight != 1 {
		t.Errorf("record = %+v", r)
	}
	if d := scope.Decide(r.Signals, scope.Env{}); d.Value != resource.In {
		t.Errorf("Decide = %+v", d)
	}
}

func TestArxiv(t *testing.T) {
	dir := t.TempDir()
	rec := "---\nrecords:\n  - id: \"2310.00001\"\n    title: A\n    authors:\n      - surname: Noether\n        forename: Emmy\n    categories: [math.AC]\n    doi: 10.3000/Y\n    msc_class: 13A02\n" +
		"  - id: \"2310.00002\"\n    title: B\n    categories: [hep-th]\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "2310.md"), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}
	rs, info := collect(t, Arxiv{Dir: dir}, 0)
	if info.Total != 1 || len(rs) != 1 {
		t.Fatalf("info = %+v, records = %d", info, len(rs))
	}
	r := rs[0]
	if r.Date != "2023" || r.Authors[0] != "Noether, Emmy" || r.IDs["doi"][0] != "10.3000/Y" || r.Weight != 0 {
		t.Errorf("record = %+v", r)
	}
}

type fakeLooker struct {
	name  string
	has   map[string]bool
	calls int
}

func (f *fakeLooker) Name() string { return f.name }
func (f *fakeLooker) Batch() int   { return 2 }
func (f *fakeLooker) Lookup(_ context.Context, _ *Client, dois []string) (map[string]bool, error) {
	f.calls++
	out := map[string]bool{}
	for _, d := range dois {
		if f.has[d] {
			out[d] = true
		}
	}
	return out, nil
}

func TestMark(t *testing.T) {
	in := scope.Record{MSC: []string{"11A41"}}
	rs := []census.Record{
		{NativeID: "1", IDs: resource.IDs{"doi": {"10.1000/A"}}, Signals: in},
		{NativeID: "2", IDs: resource.IDs{"doi": {"10.1000/B"}}, Signals: in},
		{NativeID: "3", IDs: resource.IDs{"doi": {"10.1000/C"}}, Signals: in},
		{NativeID: "4", Signals: in},
		{NativeID: "5", IDs: resource.IDs{"doi": {"10.1000/E"}}, Signals: scope.Record{MSC: []string{"95A10"}}},
	}
	self := &fakeLooker{name: "a"}
	other := &fakeLooker{name: "b", has: map[string]bool{"10.1000/a": true}}
	n, err := Mark(context.Background(), client(), rs, scope.Env{}, "a", []Looker{self, other}, 10, 1)
	if err != nil || n != 3 {
		t.Fatalf("Mark = %d, %v", n, err)
	}
	if self.calls != 0 || other.calls != 2 {
		t.Errorf("calls = %d, %d", self.calls, other.calls)
	}
	if !rs[0].Found["b"] || rs[1].Found["b"] || rs[1].Found == nil || rs[3].Found != nil || rs[4].Found != nil {
		t.Errorf("found = %v %v %v %v", rs[0].Found, rs[1].Found, rs[3].Found, rs[4].Found)
	}
}
