package site

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCheckFixtures(t *testing.T) {
	out := buildFixtures(t, true)
	r, err := Check(out, "/math2")
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK() {
		t.Errorf("the fixture site misses a target: %v", r.Problems)
	}
	if r.Pages < 100 || r.Formulas == 0 || r.Figures == 0 {
		t.Errorf("the report counts %d pages, %d formulas and %d figures", r.Pages, r.Formulas, r.Figures)
	}
	if len(r.Heaviest) != 5 || r.Heaviest[0].Bytes < r.Heaviest[4].Bytes || r.Heaviest[0].Bytes >= Budget {
		t.Errorf("the heaviest pages are %v", r.Heaviest)
	}
}

func TestCheckProblems(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "assets/a.css", "body{}")
	writeFile(t, dir, "assets/f.woff2", strings.Repeat("x", 2*Budget))
	writeFile(t, dir, "ok/index.html", `<link rel="stylesheet" href="/s/assets/a.css"><script defer src="/s/assets/a.js"></script><span class="math"><span class="katex"><math></math></span></span>`)
	writeFile(t, dir, "assets/a.js", "")
	writeFile(t, dir, "bad/index.html", strings.Join([]string{
		`<link rel="stylesheet" href="https://cdn.example.org/k.css">`,
		`<link rel="stylesheet" href="../assets/none.css">`,
		`<script src="/s/assets/a.js"></script>`,
		`<span class="katex">x</span>`,
		`<span class="math">\frac{1}{2}</span>`,
		`<svg class="dag" role="img" aria-labelledby="nocaption"></svg>`,
		`<p>` + randomText(3*Budget) + `</p>`,
	}, "\n"))
	r, err := Check(dir, "/s/")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range r.Problems {
		if p.Page != "bad/index.html" {
			t.Errorf("a problem on a good page: %v", p)
		}
		got = append(got, p.Target)
	}
	want := []string{TargetWeight, TargetMathML, TargetNoJS, TargetNoJS, TargetLists, TargetLists, TargetOffline, TargetOffline}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("the targets of the problems are %v, not %v: %v", got, want, r.Problems)
	}
	for _, tg := range Targets() {
		if r.Passed(tg) {
			t.Errorf("the target %s passes", tg)
		}
	}
}

// randomText returns n bytes that gzip cannot make much smaller.
func randomText(n int) string {
	var b strings.Builder
	x := uint32(1)
	for b.Len() < n {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		b.WriteByte(byte('a' + x%26))
	}
	return b.String()
}

func TestResolve(t *testing.T) {
	for _, c := range []struct{ base, page, ref, want string }{
		{"/math2", "node/X/index.html", "/math2/assets/site.css", "assets/site.css"},
		{"/math2", "node/X/index.html", "../../assets/site.css?v=1", "assets/site.css"},
		{"", "index.html", "/assets/site.css", "assets/site.css"},
		{"/math2", "index.html", "/other/site.css", ""},
		{"/math2", "index.html", "../../etc/passwd", "etc/passwd"}, // a path cannot leave the site
	} {
		got, ok := resolve(c.base, c.page, c.ref)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("resolve(%q, %q, %q) = %q, %v", c.base, c.page, c.ref, got, ok)
		}
	}
}

func TestHandler(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "index.html", "<p>home</p>")
	writeFile(t, dir, "assets/f.woff2", "font")
	srv := httptest.NewServer(Handler(dir, "/math2"))
	defer srv.Close()
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	get := func(path, enc string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
		req.Header.Set("Accept-Encoding", enc)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	resp := get("/math2/", "gzip")
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("the home page gives %d, %q", resp.StatusCode, resp.Header.Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := io.ReadAll(zr); string(body) != "<p>home</p>" {
		t.Errorf("the home page is %q", body)
	}
	if resp := get("/math2/", ""); resp.Header.Get("Content-Encoding") != "" {
		t.Error("a client with no gzip gets gzip")
	}
	if resp := get("/math2/assets/f.woff2", "gzip"); resp.Header.Get("Content-Encoding") != "" {
		t.Error("a font gets gzip")
	}
	if resp := get("/", "gzip"); resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/math2/" {
		t.Errorf("the root gives %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp := get("/math2/none", "gzip"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("a missing file gives %d", resp.StatusCode)
	}
}
