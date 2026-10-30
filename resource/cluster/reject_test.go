package cluster

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheck(t *testing.T) {
	for _, c := range []struct {
		r    Record
		want bool
	}{
		{Record{NativeID: "1", Title: "T", Kind: "monograph"}, false},
		{Record{Title: "T", Kind: "monograph"}, true},
		{Record{NativeID: "1", Kind: "monograph"}, true},
		{Record{NativeID: "1", Title: "T", Kind: "film"}, true},
		{Record{NativeID: "1", Title: "T", Kind: "thesis", Thesis: "doctoral"}, false},
		{Record{NativeID: "1", Title: "T", Kind: "thesis", Thesis: "phd"}, true},
		{Record{NativeID: "1", Title: "T", Kind: "article", Thesis: "master"}, true},
	} {
		if got := Check(&c.r) != ""; got != c.want {
			t.Errorf("Check(%+v) = %q", c.r, Check(&c.r))
		}
	}
}

func TestRejects(t *testing.T) {
	root := t.TempDir()
	in := []Reject{{NativeID: "1", Snapshot: "s", Reason: "no title"}}
	if err := WriteRejects(root, "zbmath", in); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRejects(root, "zbmath")
	if err != nil || len(got) != 1 || got[0] != in[0] {
		t.Fatalf("ReadRejects = %+v, %v", got, err)
	}
	if err := WriteRejects(root, "zbmath", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, RejectedDir, "zbmath.md")); !os.IsNotExist(err) {
		t.Errorf("WriteRejects with no rejects left the file: %v", err)
	}
	if got, err := ReadRejects(root, "zbmath"); err != nil || len(got) != 0 {
		t.Errorf("ReadRejects with no file = %+v, %v", got, err)
	}
}
