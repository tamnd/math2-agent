package arxivtext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func TestDetect(t *testing.T) {
	en := strings.Repeat("Let \\(x\\) be a point of the space. Then we have that the map is open, and this is the claim. ", 5)
	fr := strings.Repeat("Soit \\(x\\) un point de l'espace. Alors nous avons que les applications sont ouvertes, et c'est une preuve pour des cas dans le plan. ", 5)
	formulas := strings.Repeat("\\[ a + e = o \\] $o \\cdot e$ ", 50)
	cases := map[string]string{en: "en", fr: "fr", "The map is open.": "", formulas: "", en + fr: ""}
	for text, want := range cases {
		if got := Detect(text); got != want {
			t.Errorf("Detect(%.40q) = %q, want %q", text, got, want)
		}
	}
}

func TestBuild(t *testing.T) {
	root := t.TempDir()
	text := "---\nrecords:\n    - tag: X\n---\n" + strings.Repeat("Let the map be open. Then we have that this is the claim of the theorem, and it is true for each point. ", 5)
	for _, f := range []string{"analysis/objects/2101/2101.00001.md", "analysis/objects/9506/alg-geom-9506022.md"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	in := &resource.Scope{Value: resource.In}
	cat := []resource.CatalogueRecord{{Source: "arxiv", NativeID: "x"}}
	rs := []resource.Resource{
		{ID: "A", Kind: "preprint", Title: "A", Scope: in, Catalogue: cat, IDs: resource.IDs{"arxiv": {"2101.00001"}}},
		{ID: "B", Kind: "preprint", Title: "B", Scope: in, Catalogue: cat, IDs: resource.IDs{"arxiv": {"alg-geom/9506022"}}},
		{ID: "C", Kind: "preprint", Title: "C", Scope: in, Catalogue: cat, IDs: resource.IDs{"arxiv": {"2101.00002"}}},
		{ID: "D", Kind: "preprint", Title: "D", Scope: in, Catalogue: cat, IDs: resource.IDs{"arxiv": {"2101.00001"}}, Expressions: []resource.Expression{{Key: "e1", Language: "en"}}},
		{ID: "E", Kind: "monograph", Title: "E", Scope: in, Catalogue: cat, IDs: resource.IDs{"doi": {"10.1/e"}}},
	}
	recs, n, err := Build(root, rs)
	if err != nil {
		t.Fatal(err)
	}
	if n.Asked != 2 || n.Found != 2 || len(recs) != 2 {
		t.Fatalf("counts %+v, records %+v", n, recs)
	}
	if r := recs[1]; r.Source != Source || r.NativeID != "alg-geom/9506022" || r.Language != "en" || r.IDs["arxiv"][0] != "alg-geom/9506022" || r.Date != "" {
		t.Errorf("record %+v", r)
	}
}
