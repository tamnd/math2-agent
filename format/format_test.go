package format_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/format"
	"github.com/tamnd/math2-agent/node"
)

// The alias set is in testdata/nodes/aliases. Its nodes refer to nodes of the valid set by alias, and want/ has the output of m2 fmt for each node. testdata/nodes/README.md describes the sets.
var (
	validRoot = filepath.Join("..", "testdata", "nodes", "valid")
	aliasRoot = filepath.Join("..", "testdata", "nodes", "aliases")
)

func load(t *testing.T, root string) []*node.Node {
	t.Helper()
	nodes, err := node.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	return nodes
}

func readTree(t *testing.T, root string) []check.File {
	t.Helper()
	files, err := check.ReadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestAliasSet(t *testing.T) {
	valid := load(t, validRoot)
	aliases, err := format.ReadAliases(aliasRoot, append(load(t, aliasRoot), valid...))
	if err != nil {
		t.Fatal(err)
	}
	ix := check.Index{}
	for _, n := range valid {
		ix.Add(n)
	}
	var c check.Checker
	f := format.Formatter{Aliases: aliases}

	in := readTree(t, aliasRoot)
	for _, fd := range c.Check(in, ix).Findings {
		if fd.Rule != "S01" && fd.Rule != "B02" {
			t.Errorf("before m2 fmt, a finding is not S01 or B02: %v", fd)
		}
	}
	var out []check.File
	for _, file := range in {
		got, err := f.Source(file.Path, file.Data)
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(aliasRoot, "want", strings.TrimPrefix(file.Path, "nodes/")))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s: got:\n%s\nwant:\n%s", file.Path, got, want)
		}
		again, err := f.Source(file.Path, got)
		if err != nil || string(again) != string(got) {
			t.Errorf("%s: a second run changed the file:\n%s", file.Path, again)
		}
		out = append(out, check.File{Path: file.Path, Data: got})
	}
	rep := c.Check(out, ix)
	for _, fd := range rep.Findings {
		t.Errorf("after m2 fmt: %v", fd)
	}
	if rep.Files != 3 {
		t.Errorf("the report has %d files, want 3", rep.Files)
	}
}

func TestValidSetIsCanonical(t *testing.T) {
	aliases, err := format.ReadAliases(validRoot, load(t, validRoot))
	if err != nil {
		t.Fatal(err)
	}
	f := format.Formatter{Aliases: aliases}
	for _, file := range readTree(t, validRoot) {
		got, err := f.Source(file.Path, file.Data)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(file.Data) {
			t.Errorf("%s: m2 fmt changed a file of the valid set:\n%s", file.Path, got)
		}
	}
}

func parse(t *testing.T, path, text string) *node.Node {
	t.Helper()
	n, err := node.Parse(path, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestReadAliases(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "registers"), 0o755); err != nil {
		t.Fatal(err)
	}
	reg := "# alias tag from until\ncompact-space 1H8D2C 2026-01-02 2026-05-01\ncompact-space 7K3QX2 2026-05-01 -\n"
	if err := os.WriteFile(filepath.Join(root, format.AliasRegister), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	a := parse(t, "nodes/7K3/7K3QX2.md", "---\ntag: 7K3QX2\naliases: [compact-space, Not-An-Alias]\n---\n")
	b := parse(t, "nodes/~/~3fa94c01.md", "---\ntag: ~3fa94c01\naliases: [compact]\n---\n")
	got, err := format.ReadAliases(root, []*node.Node{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["compact-space"] != "7K3QX2" || got["compact"] != "~3fa94c01" {
		t.Errorf("got %v", got)
	}

	c := parse(t, "nodes/1H8/1H8D2C.md", "---\ntag: 1H8D2C\naliases: [compact-space]\n---\n")
	_, err = format.ReadAliases(root, []*node.Node{c})
	if err == nil || !strings.Contains(err.Error(), "the alias compact-space names 7K3QX2 in registers/aliases.reg and 1H8D2C in nodes/1H8/1H8D2C.md") {
		t.Errorf("the error is %v", err)
	}
	d := parse(t, "nodes/9TX/9TX3RB.md", "---\ntag: 9TX3RB\naliases: [compact]\n---\n")
	if _, err := format.ReadAliases(root, []*node.Node{b, d}); err == nil {
		t.Error("two nodes have the alias compact, and ReadAliases gave no error")
	}
	if got, err := format.ReadAliases(t.TempDir(), nil); len(got) != 0 || err != nil {
		t.Errorf("a tree with no register gave %v, %v", got, err)
	}
}

func TestSource(t *testing.T) {
	f := format.Formatter{Aliases: format.Aliases{"group": "DSG7KN"}}
	cases := []struct{ name, in, want string }{
		{
			"a field that is not a reference keeps its value",
			"---\ntag: X7J0XA\nkind: concept\nname: group\nuses: [group]\n---\n",
			"---\ntag: X7J0XA\nkind: concept\nname: group\nuses: [DSG7KN]\n---\n",
		},
		{
			"a link with no target and a link to an unknown alias stay",
			"---\ntag: X7J0XA\n---\n\n[[group]] [[ring|ring]] [[a|b|group]]\n",
			"---\ntag: X7J0XA\n---\n\n[[group]] [[ring|ring]] [[a|b|DSG7KN]]\n",
		},
		{
			"a link in code stays",
			"---\ntag: X7J0XA\n---\n\n``[[g|group]]`` [[g|group]]\n\n~~~\n[[g|group]]\n~~~\n[[g|group]]\n",
			"---\ntag: X7J0XA\n---\n\n``[[g|group]]`` [[g|DSG7KN]]\n\n~~~\n[[g|group]]\n~~~\n[[g|DSG7KN]]\n",
		},
	}
	for _, c := range cases {
		got, err := f.Source("nodes/X7J/X7J0XA.md", []byte(c.in))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if string(got) != c.want {
			t.Errorf("%s: got:\n%s\nwant:\n%s", c.name, got, c.want)
		}
	}

	var none format.Formatter
	in := "---\ntag: X7J0XA\nuses: [group]\n---\n"
	if got, _ := none.Source("nodes/X7J/X7J0XA.md", []byte(in)); string(got) != in {
		t.Errorf("a Formatter with no aliases changed an alias:\n%s", got)
	}
	if _, err := f.Source("nodes/X7J/X7J0XA.md", []byte("no front matter\n")); err == nil || !strings.Contains(err.Error(), "nodes/X7J/X7J0XA.md:1:1:") {
		t.Errorf("the error is %v", err)
	}
}
