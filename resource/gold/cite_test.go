package gold

import (
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func TestCitations(t *testing.T) {
	var edges []resource.Cite
	langs := map[string]string{}
	for i, l := range []string{"en", "en", "de", "fr", "ru", "ja", "it"} {
		from := "8F2AAAAAA" + string(rune('0'+i)) + "/e1"
		langs[from] = l
		edges = append(edges, resource.Cite{From: from, N: 1, To: "8F2K1QW7ZJ/e3"}, resource.Cite{From: from, N: 2})
	}
	lang := func(from string) string { return langs[from] }
	cs := SampleCitations(edges, lang, 100, "s", nil)
	if len(cs) != 12 {
		t.Fatalf("%d references, want 12 from five languages", len(cs))
	}
	for _, c := range cs {
		if c.Stratum == "ru matched" || c.Stratum == "ru not matched" {
			t.Errorf("the sample takes the sixth language: %+v", c)
		}
	}
	if again := SampleCitations(edges, lang, 100, "s", cs); len(again) != 0 {
		t.Errorf("the sample draws a reference again: %+v", again)
	}
	root := t.TempDir()
	if err := AddCiteLabel(cs, "8F2AAAAAA0/e1#1", Label{Value: "8F2K1QW7ZJ", By: "a"}); err != nil {
		t.Fatal(err)
	}
	for _, x := range []struct{ ref, by, v string }{{"8F2AAAAAA0/e1#1", "b", "8F2K1QW7ZJ"}, {"8F2AAAAAA0/e1#2", "a", "8F2K1QW7ZJ"}, {"8F2AAAAAA0/e1#2", "b", "8F2K1QW7ZJ"}, {"8F2AAAAAA2/e1#1", "a", NoResource}, {"8F2AAAAAA2/e1#1", "b", NoResource}} {
		if err := AddCiteLabel(cs, x.ref, Label{Value: x.v, By: x.by}); err != nil {
			t.Fatal(err)
		}
	}
	if err := AddCiteLabel(cs, "8F2AAAAAA0/e1#1", Label{Value: "8F2K1QW7ZJ", By: "a"}); err == nil {
		t.Error("a person labels a reference twice")
	}
	if err := AddCiteLabel(cs, "8F2AAAAAA0/e1#1", Label{Value: "8F2K1QW7ZJ/e3", By: "c"}); err == nil {
		t.Error("a label names an expression")
	}
	if err := WriteCitations(root, cs); err != nil {
		t.Fatal(err)
	}
	back, err := ReadCitations(root)
	if err != nil || len(back) != len(cs) {
		t.Fatalf("read %d, %v", len(back), err)
	}
	s := EvalCitations(edges, back)
	want := CiteScore{Labelled: 3, Matched: 2, Wrong: 1, Cited: 2, Found: 1, Precision: 0.5, Recall: 0.5}
	if s != want {
		t.Errorf("score %+v, want %+v", s, want)
	}
}
