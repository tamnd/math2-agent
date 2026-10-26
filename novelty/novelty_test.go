package novelty_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/novelty"
	"github.com/tamnd/math2-agent/search"
)

// fixed is a search with fixed matches, or an error.
type fixed struct {
	name string
	ms   []novelty.Match
	err  error
}

func (f fixed) Name() string { return f.name }

func (f fixed) Search(context.Context, novelty.Claim) (string, []novelty.Match, error) {
	return "q", f.ms, f.err
}

// by gives the relation of each match by its id, and Different for the others.
func by(rel map[string]string, calls *[]string) novelty.Judge {
	return func(_ context.Context, _ novelty.Claim, m novelty.Match) (string, string, error) {
		*calls = append(*calls, m.ID)
		if r, ok := rel[m.ID]; ok {
			return r, "the scripted reason", nil
		}
		return novelty.Different, "", nil
	}
}

var primes = novelty.Claim{Name: "Infinitude of the primes", Statement: "For each natural number $N$ there is a prime $p > N$."}

func matches(ids ...string) []novelty.Match {
	var out []novelty.Match
	for _, id := range ids {
		out = append(out, novelty.Match{ID: id})
	}
	return out
}

func TestQuery(t *testing.T) {
	for _, c := range []struct {
		claim novelty.Claim
		want  string
	}{
		{primes, "Infinitude of the primes"},
		{novelty.Claim{Statement: "Each [[group|G5S1M9]] of order $p^2$ is abelian, (for a prime $p$)."}, "Each group of order is abelian for a prime"},
		{novelty.Claim{Statement: "one two three four five six seven eight nine ten eleven twelve thirteen"}, "one two three four five six seven eight nine ten eleven twelve"},
	} {
		if got := c.claim.Query(); got != c.want {
			t.Errorf("Query of %+v gives %q, want %q", c.claim, got, c.want)
		}
	}
}

func TestCheck(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name    string
		rel     map[string]string
		nojudge bool
		ss      []novelty.Searcher
		verdict string
		match   string
		judged  []string
		err     string
	}{
		{name: "same", rel: map[string]string{"b": novelty.Same}, ss: []novelty.Searcher{fixed{name: "x", ms: matches("a", "b")}}, verdict: novelty.Known, match: "b", judged: []string{"a", "b"}},
		{name: "specializes", rel: map[string]string{"a": novelty.Related, "c": novelty.Specializes}, ss: []novelty.Searcher{fixed{name: "x", ms: matches("a")}, fixed{name: "y", ms: matches("c")}}, verdict: novelty.Known, match: "c", judged: []string{"a", "c"}},
		{name: "related", rel: map[string]string{"a": novelty.Related}, ss: []novelty.Searcher{fixed{name: "x", ms: matches("a")}}, verdict: novelty.LikelyKnown, judged: []string{"a"}},
		{name: "generalizes", rel: map[string]string{"a": novelty.Generalizes}, ss: []novelty.Searcher{fixed{name: "x", ms: matches("a")}}, verdict: novelty.LikelyKnown, judged: []string{"a"}},
		{name: "different", ss: []novelty.Searcher{fixed{name: "x", ms: matches("a", "b")}}, verdict: novelty.NoMatch, judged: []string{"a", "b"}},
		{name: "the best matches", rel: map[string]string{"d": novelty.Same}, ss: []novelty.Searcher{fixed{name: "x", ms: matches("a", "b", "c", "d")}}, verdict: novelty.NoMatch, judged: []string{"a", "b", "c"}},
		{name: "no judge", nojudge: true, ss: []novelty.Searcher{fixed{name: "x", ms: matches("a")}}, verdict: novelty.LikelyKnown},
		{name: "no judge and no match", nojudge: true, ss: []novelty.Searcher{fixed{name: "x"}}, verdict: novelty.NoMatch},
		{name: "a failed search", ss: []novelty.Searcher{fixed{name: "x", err: errors.New("down")}, fixed{name: "y", ms: matches("a")}}, verdict: novelty.NoMatch, judged: []string{"a"}, err: "the searches x failed"},
		{name: "a failed search and a match", rel: map[string]string{"a": novelty.Same}, ss: []novelty.Searcher{fixed{name: "x", err: errors.New("down")}, fixed{name: "y", ms: matches("a")}}, verdict: novelty.Known, match: "a", judged: []string{"a"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var judged []string
			judge := by(c.rel, &judged)
			if c.nojudge {
				judge = nil
			}
			r, err := novelty.Check(ctx, primes, judge, c.ss...)
			if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
				t.Fatalf("Check gives the error %v, want %q", err, c.err)
			}
			if r.Verdict != c.verdict {
				t.Errorf("the verdict is %s, want %s", r.Verdict, c.verdict)
			}
			if got := ""; r.Match != nil || c.match != "" {
				if r.Match != nil {
					got = r.Match.ID
				}
				if got != c.match {
					t.Errorf("the match is %q, want %q", got, c.match)
				}
			}
			if strings.Join(judged, ",") != strings.Join(c.judged, ",") {
				t.Errorf("the judge compared %v, want %v", judged, c.judged)
			}
			if len(r.Searches) != len(c.ss) {
				t.Errorf("the result has the searches %+v", r.Searches)
			}
		})
	}
	if _, err := novelty.Check(ctx, novelty.Claim{}, nil); err == nil {
		t.Error("a claim with no statement gives no error")
	}
	bad := func(context.Context, novelty.Claim, novelty.Match) (string, string, error) { return "new", "", nil }
	if _, err := novelty.Check(ctx, primes, bad, fixed{name: "x", ms: matches("a")}); err == nil || !strings.Contains(err.Error(), `the relation "new"`) {
		t.Errorf("a relation that is not known gives %v", err)
	}
	fail := func(context.Context, novelty.Claim, novelty.Match) (string, string, error) {
		return "", "", errors.New("no model")
	}
	if _, err := novelty.Check(ctx, primes, fail, fixed{name: "x", ms: matches("a")}); err == nil || !strings.Contains(err.Error(), "no model") {
		t.Errorf("an error of the judge gives %v", err)
	}
}

func TestResultJSON(t *testing.T) {
	var calls []string
	r, err := novelty.Check(context.Background(), primes, by(map[string]string{"a": novelty.Same}, &calls), fixed{name: "zbmath", ms: []novelty.Match{{ID: "a", URL: "https://zbmath.org/a", Title: "A title", Text: "A text"}}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"verdict":"known","match":{"source":"zbmath","id":"a","url":"https://zbmath.org/a","verdict":"same","why":"the scripted reason"},"matches":[{"source":"zbmath","id":"a","url":"https://zbmath.org/a","verdict":"same","why":"the scripted reason"}],"searches":[{"source":"zbmath","query":"q","matches":1}]}`
	if string(data) != want {
		t.Errorf("the result is\n%s\nwant\n%s", data, want)
	}
}

var recs = []ingest.Record{
	{ID: "1", Kind: "problem", Title: "Infinitude of the primes", Text: "Show that the primes never stop.", Meta: map[string]string{"status": "proved"}},
	{ID: "2", Kind: "problem", Title: "Squares of odd numbers", Text: "Each odd square is one more than a multiple of eight."},
}

func TestRecords(t *testing.T) {
	rs, err := novelty.NewRecords("erdosproblems", recs, nil)
	if err != nil {
		t.Fatal(err)
	}
	q, ms, err := rs.Search(context.Background(), primes)
	if err != nil || q == "" || len(ms) == 0 {
		t.Fatalf("Search gives %q, %+v, %v", q, ms, err)
	}
	if m := ms[0]; m.ID != "1" || m.Source != "records:erdosproblems" || m.Status != "proved" || m.Title != "Infinitude of the primes" {
		t.Errorf("the first match is %+v", m)
	}
	for _, m := range ms {
		if m.Score < novelty.MinScore {
			t.Errorf("the match %+v has a score under MinScore", m)
		}
	}
	if _, _, err := (&novelty.Records{Source: "x"}).Search(context.Background(), primes); err == nil {
		t.Error("a Records that NewRecords did not make gives no error")
	}
}

func TestRecordPlane(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(ingest.CatalogueFile, "sources:\n  - id: erdosproblems\n    name: Erdős problems\n    licence: Apache-2.0\n    class: open\n  - id: oeis\n    name: OEIS\n    licence: CC-BY-SA-4.0\n    class: open\n")
	var b strings.Builder
	for _, r := range recs {
		data, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	write("build/records/erdosproblems.jsonl", b.String())
	ss, err := novelty.RecordPlane(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) != 1 || ss[0].Name() != "records:erdosproblems" {
		t.Errorf("RecordPlane gives %d searches", len(ss))
	}
	write("build/records/oeis.jsonl", "{")
	if _, err := novelty.RecordPlane(dir, nil); err == nil || !strings.Contains(err.Error(), "oeis.jsonl") {
		t.Errorf("a file of records that is not valid gives %v", err)
	}
}

func TestGraph(t *testing.T) {
	var nodes []*node.Node
	for _, n := range []struct{ tag, text string }{
		{"Q4E7NW", "---\ntag: Q4E7NW\nkind: statement\nrole: theorem\nname: Infinitude of the primes\n---\n\n## Statement\n\nThere are infinitely many prime numbers.\n"},
		{"AA0001", "---\ntag: AA0001\nkind: statement\nrole: conjecture\nname: Infinitude of the twin primes\n---\n\n## Statement\n\nThere are infinitely many primes $p$ such that $p + 2$ is prime.\n"},
	} {
		nd, err := node.Parse("nodes/"+n.tag[:2]+"/"+n.tag+".md", []byte(n.text))
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, nd)
	}
	path := filepath.Join(t.TempDir(), "search.db")
	if err := search.Write(path, search.Input{Nodes: nodes}); err != nil {
		t.Fatal(err)
	}
	ix, err := search.Open(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ix.Close() }()
	g := &novelty.Graph{Index: ix, Nodes: nodes}
	_, ms, err := g.Search(context.Background(), novelty.Claim{Tag: "AA0001", Name: "Infinitude of the primes", Statement: "There are infinitely many primes."})
	if err != nil || len(ms) == 0 {
		t.Fatalf("Search gives %+v, %v", ms, err)
	}
	if m := ms[0]; m.ID != "Q4E7NW" || m.Source != "graph" || !strings.Contains(m.Text, "infinitely many prime numbers") {
		t.Errorf("the first match is %+v", m)
	}
	for _, m := range ms {
		if m.ID == "AA0001" {
			t.Error("the node of the claim is a match")
		}
	}
}

// serve answers each request with the body, and records the request.
func serve(t *testing.T, body string, got **http.Request) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = r
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestWeb(t *testing.T) {
	ctx := context.Background()
	var req *http.Request
	web := func(body string) novelty.Web {
		return novelty.Web{URL: serve(t, body, &req).URL + "/", UserAgent: "m2/test (+https://github.com/tamnd/math2-agent)", K: 2}
	}

	z := &novelty.ZbMATH{Web: web(`{"result": [{"id": 1234567, "title": {"title": "A note on primes"}, "zbmath_url": "https://zbmath.org/1234567"}], "status": {"nr_total_results": 1}}`)}
	_, ms, err := z.Search(ctx, primes)
	if err != nil || len(ms) != 1 || ms[0] != (novelty.Match{Source: "zbmath", ID: "1234567", URL: "https://zbmath.org/1234567", Title: "A note on primes"}) {
		t.Errorf("zbmath gives %+v, %v", ms, err)
	}
	if req.URL.Path != "/document/_search" || req.URL.Query().Get("search_string") != "Infinitude of the primes" || req.URL.Query().Get("results_per_page") != "2" || req.UserAgent() != "m2/test (+https://github.com/tamnd/math2-agent)" {
		t.Errorf("zbmath sends %s with %q", req.URL, req.UserAgent())
	}

	a := &novelty.ArXiv{Web: web(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"><entry><id>http://arxiv.org/abs/2401.00001v2</id><title>Primes
  without end</title><summary>We give one more proof.</summary></entry></feed>`)}
	_, ms, err = a.Search(ctx, primes)
	if err != nil || len(ms) != 1 || ms[0] != (novelty.Match{Source: "arxiv", ID: "2401.00001", URL: "https://arxiv.org/abs/2401.00001", Title: "Primes without end", Text: "We give one more proof."}) {
		t.Errorf("arxiv gives %+v, %v", ms, err)
	}
	if got := req.URL.Query().Get("search_query"); got != "all:Infinitude AND all:of AND all:the AND all:primes" {
		t.Errorf("arxiv sends the query %q", got)
	}

	s := &novelty.StackExchange{Web: web(`{"items": [{"question_id": 42, "title": "Why are there infinitely many primes &amp; why?", "link": "https://mathoverflow.net/questions/42", "is_answered": true}], "quota_remaining": 299}`)}
	_, ms, err = s.Search(ctx, primes)
	if err != nil || len(ms) != 1 || ms[0] != (novelty.Match{Source: "mathoverflow", ID: "42", URL: "https://mathoverflow.net/questions/42", Status: "answered", Title: "Why are there infinitely many primes & why?"}) {
		t.Errorf("mathoverflow gives %+v, %v", ms, err)
	}
	if req.URL.Path != "/search/advanced" || req.URL.Query().Get("site") != "mathoverflow" || req.URL.Query().Get("pagesize") != "2" {
		t.Errorf("mathoverflow sends %s", req.URL)
	}
	if (&novelty.StackExchange{Site: "math"}).Name() != "math" {
		t.Error("the name of a site is not the site")
	}

	seq := novelty.Claim{Name: "Sums of two earlier terms", Statement: "The terms.", Terms: []string{"1", "1", "2", "3", "5"}}
	for _, body := range []string{
		`[{"number": 45, "name": "The sum of the two terms before.", "data": "0,1,1,2,3,5"}, {"number": 7, "name": "b", "data": "1"}, {"number": 8, "name": "c", "data": "1"}]`,
		`{"results": [{"number": 45, "name": "The sum of the two terms before.", "data": "0,1,1,2,3,5"}]}`,
	} {
		o := &novelty.OEIS{Web: web(body)}
		q, ms, err := o.Search(ctx, seq)
		if err != nil || q != "1,1,2,3,5" || len(ms) == 0 || len(ms) > 2 || ms[0].ID != "A000045" || ms[0].URL != "https://oeis.org/A000045" || !strings.Contains(ms[0].Text, "0,1,1,2,3,5") {
			t.Errorf("oeis gives %q, %+v, %v", q, ms, err)
		}
		if req.URL.Query().Get("q") != "1,1,2,3,5" || req.URL.Query().Get("fmt") != "json" {
			t.Errorf("oeis sends %s", req.URL)
		}
	}
	o := &novelty.OEIS{Web: web(`null`)}
	if _, ms, err := o.Search(ctx, seq); err != nil || len(ms) != 0 {
		t.Errorf("an answer with no entry gives %+v, %v", ms, err)
	}
	if q, ms, err := o.Search(ctx, primes); q != "" || ms != nil || err != nil {
		t.Errorf("a claim with no terms gives %q, %+v, %v", q, ms, err)
	}

	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "busy", http.StatusServiceUnavailable) }))
	defer down.Close()
	if _, _, err := (&novelty.ZbMATH{Web: novelty.Web{URL: down.URL + "/"}}).Search(ctx, primes); err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("a service that is down gives %v", err)
	}
	if _, _, err := (&novelty.ZbMATH{Web: web(`<html>`)}).Search(ctx, primes); err == nil {
		t.Error("an answer that is not JSON gives no error")
	}
}
