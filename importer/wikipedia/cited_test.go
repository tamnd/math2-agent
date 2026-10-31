package wikipedia

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCited(t *testing.T) {
	root := t.TempDir()
	write := func(p, s string) {
		p = filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("nodes/a.md", "---\nid: a\nsame_as:\n  wikipedia: \"Braess's paradox\"\nclose_to:\n  - system: wikipedia\n    id: Lagrange's theorem (group theory)\n---\nSee [the page](https://en.wikipedia.org/wiki/Itô%27s_lemma). Also https://en.wikipedia.org/wiki/Bayes'_theorem and https://en.wikipedia.org/wiki/0.999...\n")
	write("topics/b.yaml", "wikipedia:\n  - Picard&ndash;Lindelöf theorem\n  - Category:Theorems\nsource: https://en.wikipedia.org/wiki/Group_(mathematics)#Notes\n")
	write("records/c.jsonl", "{\"same\":{\"wikipedia\":[\"cauchy sequence\"]}}\n\n")
	write("registers/d.json", "{not json")
	write("manifests/wikipedia/e.yaml", "wikipedia: Skipped\n")
	write("other/f.yaml", "wikipedia: Not read\n")
	got, bad, err := Cited(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"0.999...", "Bayes' theorem", "Braess's paradox", "Cauchy sequence", "Group (mathematics)", "Itô's lemma", "Lagrange's theorem (group theory)", "Picard–Lindelöf theorem"}
	if !slices.Equal(got, want) {
		t.Errorf("titles:\n got %q\nwant %q", got, want)
	}
	if bad != 1 {
		t.Errorf("files that did not parse: got %d, want 1", bad)
	}
}
