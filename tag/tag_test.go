package tag

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMintMillion(t *testing.T) {
	n := 1_000_000
	if testing.Short() {
		n = 10_000
	}
	seen := make(map[string]bool, n)
	used := func(s string) bool { return seen[s] }
	for range n {
		got, err := Mint(nil, used)
		if err != nil {
			t.Fatal(err)
		}
		if !IsTag(got) || got[0] == '0' || Excluded(got) || strings.ContainsAny(got, "ILOU") {
			t.Fatalf("Mint gave %q", got)
		}
		seen[got] = true
	}
	if len(seen) != n {
		t.Fatalf("got %d different tags, want %d", len(seen), n)
	}
}

func TestMintSeed(t *testing.T) {
	for range 1000 {
		got, err := MintSeed(nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !IsSeed(got) {
			t.Fatalf("MintSeed gave %q", got)
		}
	}
}

func TestMintExhausted(t *testing.T) {
	_, err := Mint(nil, func(string) bool { return true })
	if !errors.Is(err, ErrExhausted) {
		t.Fatalf("got %v, want ErrExhausted", err)
	}
}

func TestEncodeUsesEachCharacter(t *testing.T) {
	for v := range uint32(32) {
		if got, want := encode(v), "00000"+string(Alphabet[v]); got != want {
			t.Fatalf("encode(%d) = %q, want %q", v, got, want)
		}
	}
	if got := encode(1<<30 - 1); got != "ZZZZZZ" {
		t.Fatalf("encode(max) = %q", got)
	}
}

func TestExcluded(t *testing.T) {
	for _, s := range []string{"123456", "12E345", "20D60X", "05A15Z"} {
		if !Excluded(s) {
			t.Errorf("Excluded(%q) = false", s)
		}
	}
	for _, s := range []string{"7K3QX2", "1H8D2C", "2E5ABC", "9TX3RB"} {
		if Excluded(s) {
			t.Errorf("Excluded(%q) = true", s)
		}
	}
}

func TestIDs(t *testing.T) {
	cases := []struct {
		s           string
		tag, provis bool
	}{
		{"7K3QX2", true, false},
		{"7K3QXI", false, false},
		{"7k3qx2", false, false},
		{"7K3QX", false, false},
		{"~3fa94c01", false, true},
		{"~3FA94C01", false, false},
		{"~3fa94c0", false, false},
	}
	for _, c := range cases {
		if IsTag(c.s) != c.tag || IsProvisional(c.s) != c.provis {
			t.Errorf("%q: IsTag=%v IsProvisional=%v", c.s, IsTag(c.s), IsProvisional(c.s))
		}
	}
	p, err := Provisional(nil)
	if err != nil || !IsProvisional(p) {
		t.Fatalf("Provisional() = %q, %v", p, err)
	}
	if Shard("7K3QX2") != "7K" || Shard(p) != "~" {
		t.Fatal("Shard gave the wrong directory")
	}
}

func TestAppendKeepsLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tags.reg")
	old := "# tag commit kind\n7K3QX2 aaaaaaa statement\nZZZZZ1 bbbbbbb proof\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Append(path, []Entry{{"1H8D2C", "ccccccc", "concept"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	want := "# tag commit kind\n1H8D2C ccccccc concept\n7K3QX2 aaaaaaa statement\nZZZZZ1 bbbbbbb proof\n"
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if err := Append(path, []Entry{{"7K3QX2", "d", "proof"}}); err == nil {
		t.Fatal("Append accepted a tag that is registered")
	}
	used, err := ReadUsed(path, filepath.Join(t.TempDir(), "missing.reg"))
	if err != nil || len(used) != 3 || !used["1H8D2C"] {
		t.Fatalf("ReadUsed = %v, %v", used, err)
	}
}

func TestReadAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases.reg")
	text := "# alias tag from until\ncompact-space 1H8D2C 2026-01-02 2026-05-01\ncompact-space 7K3QX2 2026-05-01 -\n\nlagrange 9TX3RB 2026-03-04 -\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAliases(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []Alias{{"compact-space", "1H8D2C", "2026-01-02", "2026-05-01"}, {"compact-space", "7K3QX2", "2026-05-01", Current}, {"lagrange", "9TX3RB", "2026-03-04", Current}}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, err := ReadAliases(filepath.Join(dir, "missing.reg")); got != nil || err != nil {
		t.Fatalf("a missing register gave %v, %v", got, err)
	}
	for _, line := range []string{
		"compact-space 1H8D2C 2026-01-02",
		"Compact 1H8D2C 2026-01-02 -",
		"compact-space 1h8d2c 2026-01-02 -",
		"compact-space 1H8D2C 2 Jan 2026",
		"compact-space 1H8D2C 2026-01-02 now",
	} {
		if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadAliases(path); err == nil || !strings.Contains(err.Error(), "aliases.reg:1:") {
			t.Errorf("%q: the error is %v", line, err)
		}
	}
}

func write(t *testing.T, root, rel, text string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func node(id, kind, body string) string {
	return "---\ntag: " + id + "\nkind: " + kind + "\nname: A node\nuses: [" + strings.Fields(body)[0] + "]\n---\n\n## Statement\n\nSee " + body + ".\n"
}

func TestRewriteThreeLinkedNodes(t *testing.T) {
	root := t.TempDir()
	ids := []string{"~00000001", "~00000002", "~00000003"}
	for i, id := range ids {
		next := ids[(i+1)%3]
		write(t, root, "nodes/~/"+id+".md", node(id, "statement", next+" [[the next node|"+next+"]]"))
	}
	write(t, root, "nodes/7K/7K3QX2.md", node("7K3QX2", "concept", "~00000001 [[a new node|~00000001]]"))
	write(t, root, "registers/tags.reg", "7K3QX2 aaaaaaa concept\n")

	minted, err := Rewrite(root, nil, "ccccccc")
	if err != nil {
		t.Fatal(err)
	}
	if len(minted) != 3 {
		t.Fatalf("minted %d tags, want 3", len(minted))
	}
	var all bytes.Buffer
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, _ error) error {
		if !d.IsDir() {
			data, _ := os.ReadFile(path)
			all.Write(data)
			all.WriteString(path)
		}
		return nil
	})
	if ProvisionalInText.Match(all.Bytes()) {
		t.Fatalf("a provisional id is left after the rewrite:\n%s", all.String())
	}
	reg, _ := os.ReadFile(filepath.Join(root, "registers", "tags.reg"))
	for _, m := range minted {
		path := filepath.Join(root, "nodes", Shard(m.Tag), m.Tag+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "tag: "+m.Tag+"\n") {
			t.Errorf("%s does not have its new tag", path)
		}
		if !strings.Contains(string(reg), m.Tag+" ccccccc statement\n") {
			t.Errorf("tags.reg does not have %s", m.Tag)
		}
	}
}

func TestRewriteUnknownProvisional(t *testing.T) {
	root := t.TempDir()
	write(t, root, "nodes/~/~00000001.md", node("~00000001", "statement", "~0000000f [[a node in another PR|~0000000f]]"))
	if _, err := Rewrite(root, nil, "c"); err == nil || !strings.Contains(err.Error(), "T03") {
		t.Fatalf("got %v, want a T03 error", err)
	}
	if _, err := os.Stat(filepath.Join(root, "nodes", "~", "~00000001.md")); err != nil {
		t.Fatal("Rewrite moved a file after an error")
	}
}
