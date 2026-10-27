package audit

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBlob checks that blob gives the same data through the batch reader and through git show.
func TestBlob(t *testing.T) {
	root := t.TempDir()
	run(t, root, "init", "-q")
	if err := os.MkdirAll(filepath.Join(root, "d"), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"a.txt": "one\ntwo\n", "empty.txt": "", "d/b.txt": "no newline"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	commit(t, root, "first")
	cases := []struct {
		path  string
		want  string
		none  bool
		batch bool // only the batch reader: git show lists a tree
	}{
		{path: "a.txt", want: "one\ntwo\n"},
		{path: "empty.txt", want: ""},
		{path: "d/b.txt", want: "no newline"},
		{path: "missing.txt", none: true},
		{path: "d", none: true, batch: true},
	}
	for _, batch := range []bool{true, false} {
		a := &auditor{root: root, noCat: !batch}
		for _, c := range cases {
			if c.batch && !batch {
				continue
			}
			got := a.blob("HEAD", c.path)
			if c.none {
				if got != nil {
					t.Errorf("batch %v: blob(%s) = %q, want no data", batch, c.path, got)
				}
				continue
			}
			if got == nil || string(got) != c.want {
				t.Errorf("batch %v: blob(%s) = %q, want %q", batch, c.path, got, c.want)
			}
		}
		if batch && a.cat == nil {
			t.Error("the batch reader did not start")
		}
		if a.cat != nil {
			a.cat.close()
		}
	}
}
