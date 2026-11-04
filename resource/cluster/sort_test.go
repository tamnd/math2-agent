package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The Sorter writes the same files as WriteEncoded, with records that share a native id in the order that they came.
func TestSorter(t *testing.T) {
	for _, n := range []int{3, ShardSize, ShardSize + 1, 2*ShardSize + 7} {
		var es []Encoded
		for i := range n {
			id := fmt.Sprintf("%d", (i*7919)%(n/2+1))
			es = append(es, Encoded{NativeID: id, JSON: fmt.Appendf(nil, `{"native_id":%q,"n":%d}`, id, i)})
		}
		want, got := t.TempDir(), t.TempDir()
		if err := WriteEncoded(want, "s", "records", append([]Encoded(nil), es...)); err != nil {
			t.Fatal(err)
		}
		// An earlier write with more files, which the Sorter removes.
		old := make([]Encoded, 3*ShardSize+1)
		for i := range old {
			old[i].JSON = []byte(`{}`)
		}
		if err := WriteEncoded(got, "s", "records", old); err != nil {
			t.Fatal(err)
		}
		s, err := NewSorter(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			if err := s.Add(e); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.Write(got, "s", "records"); err != nil || s.Len() != n {
			t.Fatalf("n %d: %d records, %v", n, s.Len(), err)
		}
		if _, err := os.Stat(s.dir); !os.IsNotExist(err) {
			t.Fatalf("n %d: the spill directory is still there", n)
		}
		files := func(root string) map[string]string {
			out := map[string]string{}
			ms, _ := filepath.Glob(filepath.Join(root, RecordsDir, "s", "*"))
			for _, m := range ms {
				b, err := os.ReadFile(m)
				if err != nil {
					t.Fatal(err)
				}
				out[filepath.Base(m)] = string(b)
			}
			return out
		}
		w, g := files(want), files(got)
		if len(w) != len(g) {
			t.Fatalf("n %d: %d files, want %d", n, len(g), len(w))
		}
		for f, b := range w {
			if g[f] != b {
				t.Fatalf("n %d: the file %s differs", n, f)
			}
		}
	}
}
