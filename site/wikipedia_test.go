package site

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWikipedia(t *testing.T) {
	opt := Options{Content: fixtures, Out: filepath.Join(t.TempDir(), "site"), URL: "https://example.org/math2", Repo: "o/math2", AgentRepo: "o/math2-agent", Version: "test", Wikipedia: "testdata/wikipedia"}
	if err := Build(opt); err != nil {
		t.Fatal(err)
	}
	read := func(p string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(opt.Out, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	page := read("wikipedia/index.html")
	for _, want := range []string{
		`<tr><th scope="row">Candidates</th><td class="num">5</td></tr>`,
		`<tr><th scope="row">In scope</th><td class="num">4</td></tr>`,
		`<tr><th scope="row">Not decided yet</th><td class="num">1</td></tr>`,
		`<a href="/math2/wikipedia/a/">A</a> <span class="count">1</span>`,
		`<a href="/math2/wikipedia/other/">Other</a>`,
		`<tr><td>strong</td><td class="num">3</td><td class="num">3</td></tr>`,
		`<a href="/math2/wikipedia/">Wikipedia scope</a>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("wikipedia/index.html has no %s", want)
		}
	}
	list := read("wikipedia/g/index.html")
	if !strings.Contains(list, `<a href="https://en.wikipedia.org/wiki/Group_%28mathematics%29">Group (mathematics)</a>`) {
		t.Errorf("the list of G has no link to Group (mathematics):\n%s", list)
	}
	if strings.Contains(read("wikipedia/a/index.html"), "Anarchism") {
		t.Error("the list of A has Anarchism, which is not in scope")
	}
	var scope struct {
		InScope  int `json:"in_scope"`
		Articles []struct {
			Title string `json:"title"`
		} `json:"articles"`
	}
	if err := json.Unmarshal([]byte(read("api/wikipedia/scope.json")), &scope); err != nil {
		t.Fatal(err)
	}
	if scope.InScope != 4 || len(scope.Articles) != 4 || scope.Articles[0].Title != "2-group" {
		t.Errorf("scope.json: %+v", scope)
	}

	// With no corpus, the site has no Wikipedia pages and no link to them.
	out := buildFixtures(t, false)
	if _, err := os.Stat(filepath.Join(out, "wikipedia")); err == nil {
		t.Error("a site with no corpus has the directory wikipedia")
	}
	home, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(home), "Wikipedia scope") {
		t.Error("a site with no corpus links to the Wikipedia scope")
	}
}

func TestWikipediaPages(t *testing.T) {
	defer func(n int) { wpPageSize = n }(wpPageSize)
	wpPageSize = 1
	opt := Options{Content: fixtures, Out: filepath.Join(t.TempDir(), "site"), URL: "https://example.org/math2", Repo: "o/math2", AgentRepo: "o/math2-agent", Version: "test", Wikipedia: "testdata/wikipedia"}
	if err := Build(opt); err != nil {
		t.Fatal(err)
	}
	// The fixture has two articles in scope under G, so G has two pages. A has one article, so it has one page with no page links.
	read := func(p string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(opt.Out, filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	g1, g2 := read("wikipedia/g/index.html"), read("wikipedia/g/2/index.html")
	if !strings.Contains(g1, "Group (mathematics)") || strings.Contains(g1, "Group action") || !strings.Contains(g2, "Group action") {
		t.Error("the two pages of G do not split the articles")
	}
	if !strings.Contains(g1, `<a href="/math2/wikipedia/g/2/">Page 2</a>`) || !strings.Contains(g1, "2 articles, on 2 pages.") {
		t.Errorf("page 1 of G has no link to page 2:\n%s", g1)
	}
	if strings.Contains(read("wikipedia/a/index.html"), "Page 1") {
		t.Error("the list of A has one page, and it links to its pages")
	}
}
