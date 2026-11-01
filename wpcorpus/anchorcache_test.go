package wpcorpus

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestAnchorCache(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anchors.tsv.gz")
	c, err := CreateAnchorCache(path, "enwiki-20261001")
	if err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{
		"Group": "A [[Ring#Definition|ring]] and [[Field#Def\tinition]].",
		"Empty": "No links with an anchor: [[Ring]].",
		"Lemma": "See [[Group#Axioms]].",
	}
	for _, from := range []string{"Group", "Empty", "Lemma"} {
		if err := c.Add(from, AnchorLinks(pages[from])); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	got := map[string][]AnchorLink{}
	dump, n, err := ReadAnchorCache(path, func(from string, links []AnchorLink) error {
		got[from] = links
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if dump != "enwiki-20261001" || n != 3 {
		t.Errorf("header: got %q and %d pages", dump, n)
	}
	want := map[string][]AnchorLink{
		"Group": {{"Ring", "Definition"}, {"Field", "Def inition"}},
		"Lemma": {{"Group", "Axioms"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links:\n got %v\nwant %v", got, want)
	}
	// The pointers from the cache are the pointers from the wikitext.
	target := func(t string) string { return normTitle(t) }
	for from, text := range pages {
		if a, b := LinkPointers(from, text, target), AnchorPointers(from, got[from], target); len(a)+len(b) > 0 && !reflect.DeepEqual(a, b) {
			t.Errorf("%s: the pointers differ:\n%v\n%v", from, a, b)
		}
	}
}
