package tag

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestMoveShardsCode(t *testing.T) {
	root := t.TempDir()
	write(t, root, "compute/AAYEQE/main.g", "a\n")
	write(t, root, "compute/~1a2b3c4d/main.py", "b\n")
	write(t, root, "compute/ZN/ZNAZT5/main.py", "c\n")
	write(t, root, "compute/ZNF/ZNF0V2/main.g", "d\n")
	write(t, root, "compute/README.md", "e\n")
	moves, err := MoveShards(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Move{
		{"compute/AAYEQE", "compute/AAY/AAYEQE"},
		{"compute/ZN/ZNAZT5", "compute/ZNA/ZNAZT5"},
		{"compute/~1a2b3c4d", "compute/~/~1a2b3c4d"},
	}
	if !slices.Equal(moves, want) {
		t.Errorf("the moves are %v, and they must be %v", moves, want)
	}
	for _, p := range []string{"compute/AAY/AAYEQE/main.g", "compute/~/~1a2b3c4d/main.py", "compute/ZNA/ZNAZT5/main.py", "compute/ZNF/ZNF0V2/main.g", "compute/README.md"} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Error(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "compute", "ZN")); err == nil {
		t.Error("the empty shard compute/ZN is still there")
	}
	if moves, err := MoveShards(root); err != nil || len(moves) != 0 {
		t.Errorf("a second run gives %v, %v, and it must give no moves", moves, err)
	}
}

func TestMoveShardsCodeClash(t *testing.T) {
	root := t.TempDir()
	write(t, root, "compute/AAYEQE/main.g", "a\n")
	write(t, root, "compute/AAY/AAYEQE/main.g", "b\n")
	if _, err := MoveShards(root); err == nil {
		t.Error("two copies of the code of AAYEQE give no error")
	}
}

func TestCodeID(t *testing.T) {
	tests := []struct {
		path, want string
	}{
		{"compute/AAY/AAYEQE/main.g", "AAYEQE"},
		{"compute/~/~1a2b3c4d/main.py", "~1a2b3c4d"},
		{"compute/7K3/7K3QX2ABCE/sub/main.py", "7K3QX2ABCE"},
		{"compute/AAYEQE/main.g", ""},
		{"compute/AA/AAYEQE/main.g", ""},
		{"compute/AAY/AAYEQE", ""},
		{"compute/7K3/7K3QX2ABCF/main.py", ""},
		{"nodes/AAY/AAYEQE.md", ""},
	}
	for _, tt := range tests {
		got, ok := CodeID(tt.path)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("CodeID(%q) gives %q, %v, and it must give %q", tt.path, got, ok, tt.want)
		}
	}
}
