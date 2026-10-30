package resource

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTexts(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, filepath.FromSlash(CurriculaDir))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadTexts(root); err != nil || len(got) != 0 {
		t.Fatalf("ReadTexts of a tree with no curricula = %v, %v", got, err)
	}
	data := "curriculum: a\ntexts:\n  - {name: \"Rudin, Principles\", resource: 8F2K1QW7ZJ, kind: book}\ncourses:\n  - course: a-1\n    texts:\n      - {name: Notes, kind: notes}\n"
	if err := os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadTexts(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d texts, want 2", len(got))
	}
	if g := got[0]; g.Name != "Rudin, Principles" || g.Resource != "8F2K1QW7ZJ" || g.Line != 3 || g.Course != "" || g.File != CurriculaDir+"/a.yaml" {
		t.Errorf("text 1 = %+v", g)
	}
	if g := got[1]; g.Name != "Notes" || g.Resource != "" || g.Course != "a-1" || g.Line != 7 {
		t.Errorf("text 2 = %+v", g)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("texts:\n  - {name: X, kind: film}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadTexts(root); err == nil {
		t.Error("ReadTexts took the kind film")
	}
}
