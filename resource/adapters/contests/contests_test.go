package contests

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
	"github.com/tamnd/math2-agent/resource/competitions"
)

// serve starts a server that answers each path of pages with its page, and every other path with 404. It fails the test when a request has an email or a key.
func serve(t *testing.T, pages map[string]string) (*httptest.Server, *[]string) {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := r.URL.RequestURI()
		asked = append(asked, u)
		if strings.Contains(strings.ToLower(u), "mailto") || strings.Contains(u, "@") || strings.Contains(u, "%40") || strings.Contains(r.Header.Get("User-Agent"), "@") {
			t.Errorf("the request %s has an email", u)
		}
		page, ok := pages[u]
		if !ok {
			page, ok = pages[r.URL.Path]
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, page)
	}))
	t.Cleanup(srv.Close)
	return srv, &asked
}

// get gets a URL as the Getter of Pull does. It refuses a host other than the test server, so no test sends a request to a live site.
func get(t *testing.T, srv *httptest.Server) ingest.Getter {
	return func(ctx context.Context, u string) (io.ReadCloser, error) {
		p, err := url.Parse(u)
		if err != nil || "http://"+p.Host != srv.URL {
			t.Fatalf("the request %s is not to the test server", u)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := srv.Client().Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, &ingest.StatusError{URL: u, Code: resp.StatusCode, Status: resp.Status}
		}
		return resp.Body, nil
	}
}

// fetch runs the fetcher on the server and gives the snapshot.
func fetch(t *testing.T, f ingest.Fetcher, srv *httptest.Server, path, root string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := f.Fetch(context.Background(), &ingest.Request{URL: srv.URL + path, Root: root, Get: get(t, srv)}, &buf); err != nil {
		t.Fatal(err)
	}
	r, err := ingest.Uncompressed(&buf)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// records gives the source records of a snapshot by native id.
func records(t *testing.T, s cluster.Sourcer, snap []byte) map[string]cluster.Record {
	t.Helper()
	out := map[string]cluster.Record{}
	err := s.Records(bytes.NewReader(snap), func(r cluster.Record) error {
		r.Source, r.Snapshot = "test", "h"
		if why := cluster.Check(&r); why != "" {
			t.Errorf("%s: %s", r.NativeID, why)
		}
		out[r.NativeID] = r
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func structures(t *testing.T, a interface {
	SetStructures(io.Reader) (map[string][]resource.Node, error)
}, snap []byte) map[string][]resource.Node {
	t.Helper()
	m, err := a.SetStructures(bytes.NewReader(snap))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func keys(ns []resource.Node) []string {
	var out []string
	for _, n := range ns {
		out = append(out, n.Key)
	}
	return out
}

func clock(s string) func() time.Time {
	return func() time.Time { t, _ := time.Parse(time.DateOnly, s); return t }
}

const imoEditions = `<table class="table">
<thead><tr><th>#</th><th>Year</th><th>Country</th><th>City</th><th>Date</th></tr></thead>
<tbody>
<tr><td>68</td><td><a href="/editions/2027/">2027</a></td><td>Hungary</td><td>Budapest</td><td></td></tr>
<tr><td>67</td><td><a href="/editions/2026/">2026</a></td><td>China</td><td>Shanghai</td><td>10.07.2026</td></tr>
<tr><td>65</td><td><a href="/editions/2024/">2024</a></td><td>United Kingdom</td><td>Bath</td><td>11.07.2024</td></tr>
<tr><td>64</td><td><a href="/editions/2023/">2023</a></td><td>Japan</td><td>Chiba</td><td>02.07.2023</td></tr>
<tr><td>21</td><td><a href="/editions/1979/">1979</a></td><td>United Kingdom</td><td>London</td><td>30.06.1979</td></tr>
</tbody></table>`

const imo2024 = `<h2>Day 1</h2><div><b>Problem 1</b>. <i>Proposed by Colombia</i></div><div><b>Problem 2</b></div><div><b>Problem 3</b></div>
<h2>Day 2</h2><div><b>Problem 4</b></div><div><b>Problem 5</b></div><div><b>Problem 6</b></div>
<a href="/problems/2024/eng.pdf">English</a> <a href="/problems/IMO2024SL.pdf">Shortlist</a>`

func TestIMO(t *testing.T) {
	srv, asked := serve(t, map[string]string{
		"/editions/":      imoEditions,
		"/problems/2024/": imo2024,
		"/problems/2023/": `<p>Problem 1</p><p>Problem 2</p><p>Problem 3</p><p>Problem 4</p><p>Problem 5</p><p>Problem 6</p><p>Problem 61 is not a number</p>`,
	})
	a := IMO
	a.gap, a.Now = 0, clock("2026-10-31")
	snap := fetch(t, a, srv, "/editions/", "")
	want := []string{"/editions/", "/problems/1979/", "/problems/2023/", "/problems/2024/", "/problems/2026/"}
	if !slices.Equal(*asked, want) {
		t.Errorf("the requests are %v, want %v", *asked, want)
	}
	rs := records(t, a, snap)
	if len(rs) != 5 {
		t.Errorf("got %d records, want the competition, 1979, 2023, 2024 and 2024-sl", len(rs))
	}
	if r := rs["2024"]; r.Kind != "problem-set" || r.Title != "IMO 2024" || r.Date != "2024" || len(r.Relations) != 0 || r.Links[0] != srv.URL+"/editions/2024/" {
		t.Errorf("2024 is %+v", r)
	}
	if r := rs["2024-sl"]; r.Links[0] != srv.URL+"/problems/IMO2024SL.pdf" {
		t.Errorf("the shortlist is %+v", r)
	}
	if r := rs[NativeID]; r.Kind != "competition" || r.Title != "International Mathematical Olympiad" {
		t.Errorf("the competition is %+v", r)
	}
	st := structures(t, a, snap)
	if got := keys(st["2024"]); !slices.Equal(got, []string{"1", "2", "3", "4", "5", "6"}) {
		t.Errorf("the problems of 2024 are %v", got)
	}
	if n := st["2024"][0]; n.Kind != "problem" || n.Number != "1" || !slices.Equal(n.Links, []string{srv.URL + "/problems/2024/", srv.URL + "/problems/2024/eng.pdf"}) {
		t.Errorf("problem 1 is %+v", n)
	}
	if got := keys(st["2023"]); len(got) != 6 {
		t.Errorf("the problems of 2023 are %v", got)
	}
	if _, ok := st["1979"]; ok {
		t.Error("1979 has problems, but its page is not found")
	}
	var host string
	err := a.Read(bytes.NewReader(snap), func(r ingest.Record) error {
		if r.ID == "2024" {
			host = r.Meta["host"]
		}
		if r.Kind == "problem" && r.Text != "" {
			t.Errorf("the problem %s has a text", r.ID)
		}
		return nil
	})
	if err != nil || host != "Bath, United Kingdom" {
		t.Errorf("the host of 2024 is %q (%v)", host, err)
	}
}

func TestRegister(t *testing.T) {
	srv, _ := serve(t, map[string]string{"/editions/": imoEditions, "/problems/2024/": imo2024})
	root := t.TempDir()
	if err := competitions.Write(root, []competitions.Competition{{Wikidata: "Q1", Name: "International Mathematical Olympiad"}}); err != nil {
		t.Fatal(err)
	}
	a := IMO
	a.gap, a.Now = 0, clock("2026-10-31")
	rs := records(t, a, fetch(t, a, srv, "/editions/", root))
	if r := rs[NativeID]; !slices.Equal(r.IDs["wikidata"], []string{"Q1"}) {
		t.Errorf("the competition is %+v", r)
	}
	if r := rs["2024"]; len(r.Relations) != 1 || r.Relations[0] != (resource.Relation{Kind: "part_of", To: "wikidata:Q1", Source: "imo"}) {
		t.Errorf("the relations of 2024 are %+v", r.Relations)
	}
	if r := rs["2024-sl"]; len(r.Relations) != 0 {
		t.Errorf("the shortlist has the relations %+v", r.Relations)
	}

	// A record of the register with a resource gives the resource as the target. The match by site needs no name.
	if err := competitions.Write(root, []competitions.Competition{{Wikidata: "Q1", Name: "IMO (international)", Site: "https://imo-official.org", Resource: "8F2AAAAAA3"}}); err != nil {
		t.Fatal(err)
	}
	rs = records(t, a, fetch(t, a, srv, "/editions/", root))
	if r := rs["2024"]; len(r.Relations) != 1 || r.Relations[0].To != "8F2AAAAAA3" {
		t.Errorf("the relations of 2024 are %+v", r.Relations)
	}
}

const putnamIndex = `<html><head><meta name="robots" content="nofollow"></head><body>
<p>Each exam consists of 12 problems, split into two sessions.</p>
<table>
<tr>
<th>2024</th>
<td><a href="2024.tex">TeX</a></td>
<td><a href="2024.pdf">PDF</a></td>
<td><a href="2024s.pdf">Solutions</a></td>
</tr>
<tr>
<th>2020<sup>*</sup></th>
<td><a href="2020.tex">TeX</a></td>
<td><a href="2020.pdf">PDF</a></td>
</tr>
<tr>
<th>1985</th>
<td><a href="1985.tex">TeX</a></td>
<td><a href="1985.pdf">PDF</a></td>
</tr>
<tr>
<th>1985</th>
<td><a href="1985s.pdf">Solutions</a></td>
</tr>
</table></body></html>`

func TestPutnam(t *testing.T) {
	srv, asked := serve(t, map[string]string{"/putnam-archive/": putnamIndex})
	a := Putnam
	a.gap = 0
	snap := fetch(t, a, srv, "/putnam-archive/", "")
	if len(*asked) != 1 {
		t.Errorf("the requests are %v, want the index only", *asked)
	}
	rs := records(t, a, snap)
	if len(rs) != 4 {
		t.Errorf("got %d records, want 4", len(rs))
	}
	if r := rs["1985"]; len(r.Links) != 3 {
		t.Errorf("the links of 1985 are %v", r.Links)
	}
	st := structures(t, a, snap)
	want := []string{"A1", "A2", "A3", "A4", "A5", "A6", "B1", "B2", "B3", "B4", "B5", "B6"}
	if got := keys(st["2024"]); !slices.Equal(got, want) {
		t.Errorf("the problems of 2024 are %v", got)
	}
	if n := st["2024"][11]; !slices.Equal(n.Links, []string{srv.URL + "/putnam-archive/2024.pdf"}) {
		t.Errorf("B6 is %+v", n)
	}
	if len(st["2020"]) != 12 {
		t.Errorf("2020 has %d problems", len(st["2020"]))
	}

	// With no statement of the number of problems, a year has no problems.
	srv, _ = serve(t, map[string]string{"/": strings.Replace(putnamIndex, "12 problems", "problems", 1)})
	if st := structures(t, a, fetch(t, a, srv, "/", "")); len(st) != 0 {
		t.Errorf("the structures are %v", st)
	}
}

const komalMay = `<html><head><title>KöMaL Problems in Mathematics, May 2024</title></head><body>
<h1>KöMaL Problems in Mathematics, May 2024</h1>
<p id="K814" class="feladat"><b>K.&nbsp;814.</b> <a href="https://www.komal.hu/feladat?a=feladat&amp;f=K814&amp;l=en">(more)</a></p>
<p id="KC817" class="feladat"><b>K/C.&nbsp;817.</b> <a href="https://www.komal.hu/feladat?a=feladat&amp;f=K/C817&amp;l=en">(more)</a></p>
<p id="B5390" class="feladat"><b>B.&nbsp;5390.</b> <a href="/feladat?a=feladat&amp;f=B5390&amp;l=en">(more)</a> <a href="/feladat?a=feladat&amp;f=B5390&amp;l=en">again</a></p>
<p id="A881" class="feladat"><b>A.&nbsp;881.</b> <a href="/feladat?a=feladat&amp;f=A881&amp;l=en">(more)</a></p>
<a href="/feladat?a=honap&amp;h=202404&amp;t=mat&amp;l=en">April</a>
</body></html>`

func TestKoMaL(t *testing.T) {
	srv, asked := serve(t, map[string]string{
		"/feladat?a=honap&h=202405&l=en&t=mat": komalMay,
		"/feladat?a=honap&h=202310&l=en&t=mat": `<h1>October 2023</h1><p>No problems.</p>`,
	})
	a := KoMaL
	a.gap, a.Start, a.Now = 0, 2023, clock("2024-03-15")
	snap := fetch(t, a, srv, "/feladat?a=honap&t=mat&l=en", "")
	if len(*asked) != 7 {
		t.Errorf("the requests are %v, want September 2023 to March 2024", *asked)
	}
	if st := structures(t, a, snap); len(st) != 0 {
		t.Errorf("the structures are %v, but no month up to March has a problem", st)
	}

	a.Now = clock("2024-06-01")
	snap = fetch(t, a, srv, "/feladat?a=honap&t=mat&l=en", "")
	rs := records(t, a, snap)
	if len(rs) != 2 {
		t.Errorf("got %d records, want the competition and May 2024", len(rs))
	}
	if r := rs["2024-05"]; r.Title != "KöMaL Problems in Mathematics, May 2024" || r.Date != "2024-05" || r.Links[0] != "https://www.komal.hu/feladat?a=honap&h=202405&t=mat&l=en" {
		t.Errorf("May 2024 is %+v", r)
	}
	st := structures(t, a, snap)["2024-05"]
	if got := keys(st); !slices.Equal(got, []string{"K814", "KC817", "B5390", "A881"}) {
		t.Errorf("the problems are %v", got)
	}
	if n := st[1]; n.Number != "K/C. 817" || n.Links[0] != "https://www.komal.hu/feladat?a=feladat&f=K%2FC817&l=en" {
		t.Errorf("K/C 817 is %+v", n)
	}
	if n := st[2]; n.Number != "B. 5390" || n.Links[0] != "https://www.komal.hu/feladat?a=feladat&f=B5390&l=en" {
		t.Errorf("B 5390 is %+v", n)
	}
}

const kvantFront = `<a href="//www.kvant.digital/issues/2026/8/">8</a> <a href="//www.kvant.digital/issues/2026/8/">8 again</a>
<a href="//www.kvant.digital/issues/1993/1-2/">1-2</a> <a href="//www.kvant.digital/issues/1993/3/">3</a>`

const kvantIssue = `<div class="toc--section">Задачник «Кванта»</div>
<div class="toc--title"><a target="_blank" href="//www.kvant.digital/issues/2026/8/zadachi_m2902_m2905_f2909_f2912-0af9d74b/" class=" "><span><span>Задачи М2902&#8288;` + "\u2014" + `&#8288;М2905; Ф2909&#8288;` + "\u2014" + `&#8288;Ф2912</span></span></a></div>
<div class="toc--title"><a href="//www.kvant.digital/issues/2026/8/resheniya-1/"><span>Решения задач М2890` + "\u2014" + `М2893</span></a></div>
<div class="toc--section">Kvant для младших школьников</div>
<div class="toc--title"><a href="//www.kvant.digital/issues/2026/8/zadachi-2/"><span>Задачи</span></a></div>`

const kvantOld = `<div class="toc--title"><a href="/issues/1993/1-2/zadachi-x/">Задачи M1371` + "\u2013" + `M1373, M1375, M1300-M1399</a></div>`

func TestKvant(t *testing.T) {
	srv, asked := serve(t, map[string]string{
		"/":                 kvantFront,
		"/issues/2026/8/":   kvantIssue,
		"/issues/1993/1-2/": kvantOld,
		"/issues/1993/3/":   `<div class="toc--title"><a href="/x/">Задачи</a></div>`,
	})
	a := Kvant
	a.gap = 0
	snap := fetch(t, a, srv, "/", "")
	if want := []string{"/", "/issues/2026/8/", "/issues/1993/1-2/", "/issues/1993/3/"}; !slices.Equal(*asked, want) {
		t.Errorf("the requests are %v, want %v", *asked, want)
	}
	rs := records(t, a, snap)
	if len(rs) != 3 {
		t.Errorf("got %d records, want the competition, 2026-8 and 1993-1-2", len(rs))
	}
	if r := rs["2026-8"]; r.Title != "Kvant 2026, no. 8" || r.Date != "2026" || r.Links[0] != "https://www.kvant.digital/issues/2026/8/" {
		t.Errorf("2026-8 is %+v", r)
	}
	st := structures(t, a, snap)
	if got := keys(st["2026-8"]); !slices.Equal(got, []string{"M2902", "M2903", "M2904", "M2905"}) {
		t.Errorf("the problems of 2026-8 are %v", got)
	}
	if n := st["2026-8"][0]; n.Number != "М2902" || n.Links[0] != "https://www.kvant.digital/issues/2026/8/zadachi_m2902_m2905_f2909_f2912-0af9d74b/" {
		t.Errorf("M2902 is %+v", n)
	}
	// A range larger than MaxRange gives its first number only.
	if got := keys(st["1993-1-2"]); !slices.Equal(got, []string{"M1371", "M1372", "M1373", "M1375", "M1300"}) {
		t.Errorf("the problems of 1993-1-2 are %v", got)
	}

	srv, _ = serve(t, map[string]string{"/": kvantFront})
	var buf bytes.Buffer
	if err := a.Fetch(context.Background(), &ingest.Request{URL: srv.URL + "/", Get: get(t, srv)}, &buf); err == nil {
		t.Error("a fetch with no problem gave no error")
	}
}

const bmoPage = `<ul>
<li><a href="/home/bmo1-2026.pdf">19 November
  2025</a>; <a href="/home/bmo1-2026-report.pdf">report</a></li>
<li><a href="/home/bmo2-2026.pdf">22 January 2026</a></li>
<li><a href="/home/bmo1-2006.pdf">1 December 2005</a> (first six-question paper)</li>
<li><a href="/home/bmo1-2005.pdf">2 December 2004</a> (last five-question paper)</li>
<li><a href="/home/bmo-1985.pdf">1985</a></li>
<li><a href="/home/fist-1985.pdf">FIST 1985</a></li>
</ul>`

func TestBMO(t *testing.T) {
	srv, asked := serve(t, map[string]string{"/home/bmo.shtml": bmoPage})
	a := BMO
	a.gap = 0
	snap := fetch(t, a, srv, "/home/bmo.shtml", "")
	if len(*asked) != 1 {
		t.Errorf("the requests are %v", *asked)
	}
	rs := records(t, a, snap)
	if len(rs) != 6 {
		t.Errorf("got %d records, want the competition and five papers", len(rs))
	}
	if r := rs["bmo1-2026"]; r.Title != "BMO Round 1 2026" || r.Date != "2025-11-19" || r.Links[0] != "https://bmos.ukmt.org.uk/home/bmo1-2026.pdf" {
		t.Errorf("BMO1 2026 is %+v", r)
	}
	if r := rs["bmo-1985"]; r.Date != "1985" || r.Title != "BMO 1985" {
		t.Errorf("BMO 1985 is %+v", r)
	}
	st := structures(t, a, snap)
	for id, n := range map[string]int{"bmo1-2026": 6, "bmo2-2026": 4, "bmo1-2006": 6, "bmo1-2005": 5, "bmo-1985": 0} {
		if len(st[id]) != n {
			t.Errorf("%s has %d problems, want %d", id, len(st[id]), n)
		}
	}
}

func TestSnapshotErrors(t *testing.T) {
	for _, s := range []string{
		`{"set":{"id":"2024","title":"x"}}`,
		`{"competition":{"name":"x"}}` + "\n" + `{"competition":{"name":"y"}}`,
		`{"competition":{"name":"x"}}` + "\n" + `{"set":{"id":"competition","title":"x"}}`,
		`{}`,
	} {
		if err := Putnam.Read(strings.NewReader(s), func(ingest.Record) error { return nil }); err == nil {
			t.Errorf("the snapshot %q gave no error", s)
		}
	}
}

func TestSources(t *testing.T) {
	for _, a := range []ingest.Adapter{IMO, Putnam, KoMaL, Kvant, BMO} {
		s := a.Source()
		if s.Class != ingest.RecordOnly || s.Channel != "page" || s.Terms == nil || s.Licence != "LicenseRef-facts" {
			t.Errorf("%s: the source is %+v", s.ID, s)
		}
		if ps := s.Problems(); len(ps) > 0 {
			t.Errorf("%s: %v", s.ID, ps)
		}
		if _, ok := a.(ingest.Fetcher); !ok {
			t.Errorf("%s has no Fetch", s.ID)
		}
	}
}
