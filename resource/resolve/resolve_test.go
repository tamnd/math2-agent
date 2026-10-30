package resolve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census/fetch"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func catalogue() []resource.Resource {
	return []resource.Resource{
		{ID: "8F2K1QW7ZJ", Kind: "textbook", Title: "Principles of Mathematical Analysis", Creators: []resource.Creator{{Name: "Rudin, Walter", Role: "author"}}, Date: "1953", IDs: resource.IDs{"wikidata": {"Q1"}}, Expressions: []resource.Expression{
			{Key: "e1", Kind: "edition", Label: "1st edition", Date: "1953"},
			{Key: "e3", Kind: "edition", Label: "3rd edition", Date: "1976", Manifestations: []resource.Manifestation{{Key: "m1", Kind: "print", IDs: resource.IDs{"isbn": {"9780070542358"}}}}},
		}},
		{ID: "9A1B2C3D4E", Kind: "article", Title: "On the zeros of zeta", Creators: []resource.Creator{{Name: "Ada Lovelace", Role: "author"}}, Date: "2023", Expressions: []resource.Expression{
			{Key: "e1", Kind: "version", Label: "preprint", IDs: resource.IDs{"arxiv": {"2301.00001"}}},
			{Key: "e2", Kind: "version", Label: "published", IDs: resource.IDs{"doi": {"10.1000/z"}}},
		}},
		{ID: "7Z7Z7Z7Z7Z", Kind: "article", Title: "Gone", Scope: &resource.Scope{Value: resource.Duplicate, Target: "9A1B2C3D4E"}, IDs: resource.IDs{"wikidata": {"Q9"}}},
	}
}

type fake struct{ calls int }

func (*fake) Name() string      { return "fake" }
func (*fake) Systems() []string { return []string{"doi"} }
func (f *fake) Fetch(_ context.Context, id string) ([]string, error) {
	f.calls++
	if id == "doi:10.2000/new" {
		return []string{"arxiv:2301.00001"}, nil
	}
	return nil, nil
}

func TestID(t *testing.T) {
	f := &fake{}
	r := &Resolver{Index: Build(catalogue()), Crosswalks: Links([]cluster.Link{{A: "zbl:1500.11001", B: "doi:10.1000/Z"}}), Fetchers: []Fetcher{f}, Cache: &Cache{Day: "2026-10-30", answers: map[string]Answer{}}}
	cases := []struct{ id, status, pass, at string }{
		{"ISBN 978-0-07-054235-8", "", "", ""},
		{"isbn:978-0-07-054235-8", Found, Local, "8F2K1QW7ZJ/e3/m1"},
		{"arXiv:2301.00001v2", Found, Local, "9A1B2C3D4E/e1"},
		{"doi:https://doi.org/10.1000/Z", Found, Local, "9A1B2C3D4E/e2"},
		{"zbl:1500.11001", Found, Crosswalk, "9A1B2C3D4E/e2"},
		{"doi:10.2000/new", Found, Live, "9A1B2C3D4E/e1"},
		{"doi:10.2000/none", NotFound, "", ""},
		{"wikidata:Q9", NotFound, "", ""},
	}
	for _, c := range cases {
		res, err := r.ID(context.Background(), c.id)
		if c.status == "" {
			if err == nil {
				t.Errorf("%s: want an error", c.id)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.id, err)
		}
		at := ""
		if res.Ref != nil {
			at = res.Ref.At()
		}
		if res.Status != c.status || res.Pass != c.pass || at != c.at {
			t.Errorf("%s: got %s %s %s, want %s %s %s", c.id, res.Status, res.Pass, at, c.status, c.pass, c.at)
		}
	}
	if _, err := r.ID(context.Background(), "doi:10.2000/new"); err != nil || f.calls != 2 {
		t.Errorf("the cache did not keep the answer: %d calls", f.calls)
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		text, title, author string
		year, edition       int
		ids                 int
	}{
		{"W. Rudin, Principles of Mathematical Analysis, 3rd ed., McGraw-Hill, 1976.", "Principles of Mathematical Analysis", "W. Rudin", 1976, 3, 0},
		{"Rudin, W.: Principles of mathematical analysis. McGraw-Hill, New York (1976)", "Principles of mathematical analysis", "Rudin, W.", 1976, 0, 0},
		{`A. Lovelace and C. Babbage, "On the zeros of zeta", J. Num. 3 (2024), doi:10.1000/z, arXiv:2301.00001.`, "On the zeros of zeta", "A. Lovelace", 2024, 0, 2},
		{"S. Lang, Algebra, Graduate Texts in Mathematics 211, Springer, 2002. MR1878556", "Algebra", "S. Lang", 2002, 0, 1},
	}
	for _, c := range cases {
		got := Parse(c.text)
		if got.Title != c.title || len(got.Authors) == 0 || got.Authors[0] != c.author || got.Year != c.year || got.Edition != c.edition || len(got.IDs) != c.ids {
			t.Errorf("%s: got %+v", c.text, got)
		}
	}
}

func TestCite(t *testing.T) {
	r := &Resolver{Index: Build(catalogue())}
	cases := []struct{ text, status, at string }{
		{"W. Rudin, Principles of Mathematical Analysis, 3rd ed., McGraw-Hill, 1976.", Found, "8F2K1QW7ZJ/e3"},
		{"W. Rudin, Principles of mathematical analysis, McGraw-Hill, 1953.", Found, "8F2K1QW7ZJ"},
		{"A. Lovelace, The zeros of zeta functions, 2023.", Candidates, ""},
		{"A. Lovelace, A note on knots, 2023.", NotFound, ""},
		{"A. Weil, Basic Number Theory, 1967. doi:10.1000/z", Found, "9A1B2C3D4E/e2"},
	}
	for _, c := range cases {
		res, err := r.Cite(context.Background(), Parse(c.text))
		if err != nil {
			t.Fatal(err)
		}
		at := ""
		if res.Ref != nil {
			at = res.Ref.At()
		}
		if res.Status != c.status || at != c.at {
			t.Errorf("%s: got %s %s %v, want %s %s", c.text, res.Status, at, res.Candidates, c.status, c.at)
		}
	}
}

func TestIndexFile(t *testing.T) {
	root := t.TempDir()
	n, err := WriteIndex(root, catalogue())
	if err != nil || n != 4 {
		t.Fatalf("wrote %d rows: %v", n, err)
	}
	rows, err := ReadIndex(root)
	if err != nil || len(rows) != 4 || rows[0].ID != "arxiv:2301.00001" || rows[0].Expression != "e1" {
		t.Fatalf("got %+v, %v", rows, err)
	}
}

func TestCacheFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cache.md")
	c, err := ReadCache(p, "2026-10-30")
	if err != nil {
		t.Fatal(err)
	}
	c.Put("crossref", "doi:10.1/a", []string{"isbn:9780070542358"})
	if err := c.Write(p); err != nil {
		t.Fatal(err)
	}
	again, err := ReadCache(p, "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if ids, ok := again.Get("crossref", "doi:10.1/a"); !ok || len(ids) != 1 {
		t.Errorf("got %v %v", ids, ok)
	}
}

func TestCrossref(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/works/10.1000/z" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"message":{"ISBN":["0-07-054235-X"],"relation":{"has-preprint":[{"id-type":"doi","id":"10.48550/arXiv.2301.00001"}]}}}`))
	}))
	defer srv.Close()
	f := Crossref{Base: srv.URL, Client: fetch.NewClient(time.Millisecond)}
	got, err := f.Fetch(context.Background(), "doi:10.1000/z")
	if err != nil || len(got) != 2 || got[0] != "isbn:9780070542358" || got[1] != "arxiv:2301.00001" {
		t.Fatalf("got %v, %v", got, err)
	}
	if got, err := f.Fetch(context.Background(), "doi:10.1000/none"); err != nil || got != nil {
		t.Fatalf("got %v, %v", got, err)
	}
}
