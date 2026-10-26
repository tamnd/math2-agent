package node_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
)

// testOrder returns the key order of the embedded schema.
func testOrder(testing.TB) node.Order { return schema.Embedded().Order }

func TestParseOrder(t *testing.T) {
	data, err := os.ReadFile("../schema/node.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	o, err := node.ParseOrder(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(o[""][:4], " "); got != "tag kind role name" {
		t.Errorf("top order starts with %q", got)
	}
	if got := strings.Join(o["formal"][:4], " "); got != "system library decl match" {
		t.Errorf("formal order starts with %q", got)
	}
	if got := strings.Join(o["engine.resources"], " "); got != "cpu mem wall" {
		t.Errorf("engine.resources order is %q", got)
	}
	for _, bad := range []string{"", "[]", `{"properties": {}}`} {
		if _, err := node.ParseOrder([]byte(bad)); err == nil {
			t.Errorf("ParseOrder(%q) gave no error", bad)
		}
	}
}

// TestCanonicalFixtures checks that parse and then write gives the same bytes for each canonical fixture.
func TestCanonicalFixtures(t *testing.T) {
	o := testOrder(t)
	files, err := filepath.Glob("testdata/canonical/*.md")
	if err != nil || len(files) < 5 {
		t.Fatalf("found %d fixtures, %v", len(files), err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := node.Canonical("nodes/xx/"+filepath.Base(f), data, o)
		if err != nil || !ok {
			n, _ := node.Parse("nodes/xx/"+filepath.Base(f), data)
			t.Errorf("%s is not canonical (%v). Format gives:\n%s", f, err, node.Format(n, o))
		}
	}
}

func TestFormat(t *testing.T) {
	o := testOrder(t)
	in := "---\r\nzeta: 1\r\nname: 'A node'\r\nalpha: True\r\nkind: concept\r\ntag: 7K3QX2\r\nsame_as:\r\n  nlab: ~\r\n  wikidata: Q1\r\nmsc:\r\n  - \"20D60\"\r\n  - '20E05'\r\n---\r\n## Summary\r\n\r\nText.\r\n\r\n\r\n"
	want := "---\ntag: 7K3QX2\nkind: concept\nname: A node\nmsc: [20D60, \"20E05\"]\nsame_as:\n  wikidata: Q1\n  nlab: null\nalpha: true\nzeta: 1\n---\n\n## Summary\n\nText.\n"
	n, err := node.Parse("nodes/7K3/7K3QX2.md", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(node.Format(n, o)); got != want {
		t.Errorf("Format gives\n%s\nwant\n%s", got, want)
	}
	if ok, _ := node.Canonical("nodes/7K3/7K3QX2.md", []byte(in), o); ok {
		t.Error("Canonical is true for a file in CRLF form")
	}
}

func TestFormatKeepsComments(t *testing.T) {
	in := "---\ntag: 7K3QX2\nkind: statement\nname: A node\nrole: lemma\nuses:\n  - 1H8D2C # subgroup\n  - 6PQ0WZ\n---\n"
	n, err := node.Parse("nodes/7K3/7K3QX2.md", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	got := string(node.Format(n, testOrder(t)))
	if !strings.Contains(got, "role: lemma\nname: A node\n") || !strings.Contains(got, "uses:\n  - 1H8D2C # subgroup\n  - 6PQ0WZ\n") {
		t.Errorf("Format gives\n%s", got)
	}
}

func TestHash(t *testing.T) {
	data, err := os.ReadFile("testdata/canonical/7K3QX2.md")
	if err != nil {
		t.Fatal(err)
	}
	n, err := node.Parse("nodes/7K3/7K3QX2.md", data)
	if err != nil {
		t.Fatal(err)
	}
	h := node.Hash(n)
	if !strings.HasPrefix(h, node.HashPrefix) || len(h) != len(node.HashPrefix)+64 {
		t.Fatalf("Hash = %q", h)
	}
	text := string(data)
	same := map[string]string{
		"key order":   strings.Replace(text, "kind: statement\nrole: theorem\n", "role: theorem\nkind: statement\n", 1),
		"comments":    strings.ReplaceAll(text, " # proof nodes, in the order shown to readers", ""),
		"list style":  strings.Replace(text, "msc: [20D60]", "msc:\n  - \"20D60\"", 1),
		"quotes":      strings.Replace(text, "locator: Thm 3.1", "locator: \"Thm 3.1\"", 1),
		"whitespace":  strings.Replace(text, "of $G$.\n", "of $G$.  \n", 1) + "\n\n",
		"line ending": strings.ReplaceAll(text, "\n", "\r\n"),
		"NFC":         strings.Replace(text, "## Remarks\n", "## Remarks\n\nCafé.\n", 1),
	}
	nfc := strings.Replace(text, "## Remarks\n", "## Remarks\n\nCafé.\n", 1)
	for name, variant := range same {
		v, err := node.Parse("nodes/7K3/7K3QX2.md", []byte(variant))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := h
		if name == "NFC" {
			w, _ := node.Parse("nodes/7K3/7K3QX2.md", []byte(nfc))
			want = node.Hash(w)
		}
		if got := node.Hash(v); got != want {
			t.Errorf("a change of %s changes the hash", name)
		}
	}
	different := map[string]string{
		"value": strings.Replace(text, "role: theorem", "role: lemma", 1),
		"body":  strings.Replace(text, "divides", "is a multiple of", 1),
		"null":  strings.Replace(text, "nlab: null", "nlab: \"null\"", 1),
	}
	for name, variant := range different {
		v, err := node.Parse("nodes/7K3/7K3QX2.md", []byte(variant))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if node.Hash(v) == h {
			t.Errorf("a change of %s does not change the hash", name)
		}
	}
}

func TestErrorPositions(t *testing.T) {
	cases := map[string]string{
		"---\ntag: 7K3QX2\nname: a: b\nkind: concept\n---\n":         "nodes/7K3/7K3QX2.md:3:",
		"---\ntag: 7K3QX2\nkind: concept\nkind: topic\n---\n":        "nodes/7K3/7K3QX2.md:4:1: the front matter is not valid YAML: mapping key \"kind\" already defined at line 3",
		"---\ntag: 7K3QX2\n---\n\nab\xffc\n":                         "nodes/7K3/7K3QX2.md:5:3: the file is not valid UTF-8",
		"---\ntag: 7K3QX2\nx: &a 1\ny: *a\n---\n":                    "nodes/7K3/7K3QX2.md:3:4: the front matter must not use YAML anchors",
		"---\ntag: 7K3QX2\nbase: {a: 1}\nmore:\n  <<: {b: 2}\n---\n": "nodes/7K3/7K3QX2.md:5:3: the front matter must not use YAML anchors",
	}
	for in, want := range cases {
		_, err := node.Parse("nodes/7K3/7K3QX2.md", []byte(in))
		if err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("Parse(%q) = %v, want an error that starts with %q", in, err, want)
		}
	}
}

// FuzzParse checks that the parser does not crash, and that node.Format is stable: a second node.Format gives the same bytes and the hash does not change.
func FuzzParse(f *testing.F) {
	files, _ := filepath.Glob("testdata/canonical/*.md")
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add([]byte("---\n---\n"))
	f.Add([]byte("---\na: [1, {b: c}]\n---\n\n## A\n"))
	o := testOrder(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		n, err := node.Parse("nodes/7K3/7K3QX2.md", data)
		if err != nil {
			return
		}
		once := node.Format(n, o)
		m, err := node.Parse("nodes/7K3/7K3QX2.md", once)
		if err != nil {
			t.Fatalf("Format gives a file that does not parse: %v\n%s", err, once)
		}
		if twice := node.Format(m, o); !bytes.Equal(once, twice) {
			t.Fatalf("Format is not stable:\n%s\nthen\n%s", once, twice)
		}
		if node.Hash(n) != node.Hash(m) {
			t.Fatalf("Format changes the hash:\n%s\nthen\n%s", data, once)
		}
	})
}
