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
		if !IsTag(got) || len(got) != Len || got[0] == '0' || Excluded(got) || strings.ContainsAny(got, "ILOU") {
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
	for v := range uint64(32) {
		if got, want := encode(v), "00000000"+string(Alphabet[v]); got != want {
			t.Fatalf("encode(%d) = %q, want %q", v, got, want)
		}
	}
	if got := encode(1<<45 - 1); got != "ZZZZZZZZZ" {
		t.Fatalf("encode(max) = %q", got)
	}
}

func TestCheck(t *testing.T) {
	if got := Check("7K3QX2M9R"); got != '2' {
		t.Fatalf("Check(7K3QX2M9R) = %q, want '2'", got)
	}
	if got := Check("7K3QX2M9U"); got != 0 {
		t.Fatalf("Check of a payload with U = %q, want 0", got)
	}
	// Each change of one character and each swap of two neighbours that differ must fail the check.
	good := "7K3QX2M9R2"
	for i := range len(good) {
		for _, c := range []byte(Alphabet) {
			if c == good[i] {
				continue
			}
			b := []byte(good)
			b[i] = c
			if IsTag(string(b)) {
				t.Fatalf("IsTag(%s) = true after a change of character %d", b, i)
			}
		}
		if i+1 < len(good) && good[i] != good[i+1] {
			b := []byte(good)
			b[i], b[i+1] = b[i+1], b[i]
			if IsTag(string(b)) {
				t.Fatalf("IsTag(%s) = true after a swap at %d", b, i)
			}
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want, err string }{
		{"7K3QX2M9R2", "7K3QX2M9R2", ""},
		{"7k3qx-2m9r2", "7K3QX2M9R2", ""},
		{" 7K3QX 2M9R2 ", "7K3QX2M9R2", ""},
		{"3c53td", "3C53TD", ""},
		{"G5SIM9", "G5S1M9", ""},
		{"7K3XQ2M9R2", "", "check character"},
		{"7KZQX2M9R2", "", "check character"},
		{"7K3QX2M9", "", "not a valid tag"},
		{"7K3QX2M9RU", "", "not a valid tag"},
	}
	for _, c := range cases {
		got, err := Normalize(c.in)
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("Normalize(%q) = %q, %v; want an error with %q", c.in, got, err, c.err)
			}
			continue
		}
		if got != c.want || err != nil {
			t.Errorf("Normalize(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	if _, err := Normalize("7K3XQ2M9R2"); !errors.Is(err, ErrCheck) {
		t.Errorf("a wrong check character gave %v, want ErrCheck", err)
	}
}

func TestDisplay(t *testing.T) {
	if got := Display("7K3QX2M9R2"); got != "7K3QX-2M9R2" {
		t.Errorf("Display = %q", got)
	}
	if got := Display("3C53TD"); got != "3C53TD" {
		t.Errorf("Display of a legacy tag = %q", got)
	}
}

func TestExcluded(t *testing.T) {
	for _, s := range []string{"123456", "12E345", "20D60X", "05A15Z"} {
		if !Excluded(s) {
			t.Errorf("Excluded(%q) = false", s)
		}
	}
	for _, s := range []string{"1234567890", "12E3456789", "20D60XXXXX", "123456789X", "A123456789"} {
		if !Excluded(s) {
			t.Errorf("Excluded(%q) = false", s)
		}
	}
	for _, s := range []string{"7K3QX2", "1H8D2C", "2E5ABC", "9TX3RB", "7K3QX2M9R2", "AB12345678"} {
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
		{"7K3QX2M9R2", true, false},
		{"7K3QX2M9R3", false, false},
		{"7K3QX2M9", false, false},
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
	write(t, root, "compute/~00000003/main.g", "# The code of ~00000003.\n")
	write(t, root, "prov/~/~00000002.jsonl", `{"field":"*","op":"add","agent":null,"human":"github:writer","at":"2026-09-01T10:00:00Z"}`+"\n")

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
		_, err = os.Stat(filepath.Join(root, "prov", Shard(m.Tag), m.Tag+".jsonl"))
		if hasLog := err == nil; hasLog != (m.Provisional == "~00000002") {
			t.Errorf("%s has a provenance log: %v", m.Tag, hasLog)
		}
		_, err = os.Stat(filepath.Join(root, "compute", m.Tag, "main.g"))
		if hasCode := err == nil; hasCode != (m.Provisional == "~00000003") {
			t.Errorf("%s has code in compute/: %v", m.Tag, hasCode)
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

func TestRewriteSeed(t *testing.T) {
	root := t.TempDir()
	write(t, root, "nodes/~/~00000001.md", node("~00000001", "concept", "A concept."))
	write(t, root, "nodes/~/~00000002.md", node("~00000002", "statement", "It uses [[the concept|~00000001]]."))
	minted, err := RewriteSeed(root, nil, "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(minted) != 2 {
		t.Fatalf("minted %d tags, want 2", len(minted))
	}
	for _, m := range minted {
		if !IsSeed(m.Tag) {
			t.Errorf("%s gets %s, and that is not a seed tag", m.Provisional, m.Tag)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "nodes", Shard(minted[1].Tag), minted[1].Tag+".md"))
	if err != nil || !strings.Contains(string(data), "|"+minted[0].Tag+"]]") {
		t.Errorf("the link to the first node is not rewritten: %v\n%s", err, data)
	}
}

func TestRewriteContentDirs(t *testing.T) {
	root := t.TempDir()
	write(t, root, "nodes/~/~00000001.md", node("~00000001", "statement", "~00000001 [[this node|~00000001]]"))
	write(t, root, "lean/Math2/Algebra/A.lean", "@[m2 \"~00000001\"]\ntheorem a : True := trivial\n")
	readme := "A new node has a provisional id, for example `~3fa94c01`.\n"
	write(t, root, "README.md", readme)
	write(t, root, ".github/workflows/check.yml", "# ~3fa94c01\n")

	minted, err := Rewrite(root, nil, "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(minted) != 1 {
		t.Fatalf("minted %d tags, want 1", len(minted))
	}
	lean, _ := os.ReadFile(filepath.Join(root, "lean", "Math2", "Algebra", "A.lean"))
	if !strings.Contains(string(lean), `@[m2 "`+minted[0].Tag+`"]`) {
		t.Errorf("the Lean attribute does not have the new tag:\n%s", lean)
	}
	got, _ := os.ReadFile(filepath.Join(root, "README.md"))
	if string(got) != readme {
		t.Errorf("Rewrite changed the README:\n%s", got)
	}
}
