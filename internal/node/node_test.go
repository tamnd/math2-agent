package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lagrange = `---
tag: 7K3QX2
kind: statement
role: theorem
name: Lagrange's theorem
msc: [20D60]
uses: [1H8D2C, 6PQ0WZ]
---

## Statement

Let $G$ be a finite [[group|1A2B3C]].
`

func TestParse(t *testing.T) {
	n, err := Parse("nodes/7K/7K3QX2.md", []byte(lagrange))
	if err != nil {
		t.Fatal(err)
	}
	if n.ID != "7K3QX2" || n.Kind() != "statement" || n.Name() != "Lagrange's theorem" {
		t.Fatalf("got %q %q %q", n.ID, n.Kind(), n.Name())
	}
	if got := n.Strings("uses"); strings.Join(got, ",") != "1H8D2C,6PQ0WZ" {
		t.Fatalf("uses = %v", got)
	}
	if !strings.HasPrefix(n.Body, "\n## Statement") || n.BodyLine != 9 {
		t.Fatalf("body starts at line %d: %q", n.BodyLine, n.Body)
	}
	if line := n.Front.Content[0].Content[0].Line; line != 2 {
		t.Fatalf("the tag key is on line %d, want 2", line)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"nodes/7K/7K3QXI.md": lagrange,
		"nodes/7K/7K3QX2.md": "## Statement\n",
		"nodes/7K/7K3QX3.md": "---\ntag: 7K3QX3\n",
		"nodes/7K/7K3QX4.md": "---\n- a\n- b\n---\n",
		"nodes/7K/7K3QX5.md": "---\ntag: [\n---\n",
	}
	for path, text := range cases {
		if _, err := Parse(path, []byte(text)); err == nil {
			t.Errorf("%s: no error", path)
		}
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	if nodes, err := Load(root); err != nil || len(nodes) != 0 {
		t.Fatalf("Load on a tree with no nodes = %v, %v", nodes, err)
	}
	dir := filepath.Join(root, "nodes", "7K")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "7K3QX2.md"), []byte(lagrange), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "7K3QX3.md"), []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	nodes, err := Load(root)
	if len(nodes) != 1 || err == nil || !strings.Contains(err.Error(), "7K3QX3.md:1:1") {
		t.Fatalf("Load = %d nodes, %v", len(nodes), err)
	}
}
