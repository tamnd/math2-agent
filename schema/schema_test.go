package schema

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
)

const image = "m2-sage@sha256:0000000000000000000000000000000000000000000000000000000000000000"

// valid has a valid front matter for each of the 11 kinds. The test adds the tag line.
var valid = map[string]string{
	"concept":        "kind: concept\nname: A node\n",
	"definition":     "kind: definition\nname: A node\ndefines: 1H8D2C\n",
	"statement":      "kind: statement\nname: A node\nrole: theorem\n",
	"conjecture":     "kind: conjecture\nname: A node\n",
	"problem":        "kind: problem\nname: A node\n",
	"example":        "kind: example\nname: A node\ninstance_of: [1H8D2C]\n",
	"counterexample": "kind: counterexample\nname: A node\n",
	"object":         "kind: object\nname: A node\n",
	"proof":          "kind: proof\nname: A node\nproves: 1H8D2C\n",
	"computation":    "kind: computation\nname: A node\nclaim: x\nengine:\n  image: " + image + "\n  entry: run.py\n",
	"topic":          "kind: topic\nname: A node\n",
}

func parse(t *testing.T, front string) *node.Node {
	t.Helper()
	n, err := node.Parse("nodes/7K/7K3QX2.md", []byte("---\ntag: 7K3QX2\n"+front+"---\n"))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEmbedded(t *testing.T) {
	s := Embedded()
	if got := len(s.Kinds()); got != 11 {
		t.Fatalf("the schema has sections for %d kinds, want 11", got)
	}
	sec, ok := s.Sections("statement")
	if !ok || sec.Required[0] != "Statement" {
		t.Errorf("statement sections = %+v", sec)
	}
	if len(s.Forbidden) == 0 || s.Order[""][0] != "tag" {
		t.Errorf("forbidden = %v, order = %v", s.Forbidden, s.Order[""])
	}
	want := []string{"about", "defines", "instance_of", "methods", "proofs", "proves", "resolved_by", "specializes", "successor", "target", "topics", "uses", "via"}
	if !slices.Equal(s.Refs, want) {
		t.Errorf("refs = %v, want %v", s.Refs, want)
	}
}

// TestEmbeddedIsCurrent compares the embedded copy with the schema in a math2 working tree. Set M2_CONTENT to the tree to run it.
func TestEmbeddedIsCurrent(t *testing.T) {
	root := os.Getenv("M2_CONTENT")
	if root == "" {
		t.Skip("M2_CONTENT is not set")
	}
	data, err := os.ReadFile(filepath.Join(root, File))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, embedded) {
		t.Errorf("schema/node.v1.json in this module is not the same as %s. Copy the file again.", filepath.Join(root, File))
	}
}

func TestValidKinds(t *testing.T) {
	s := Embedded()
	for k, front := range valid {
		if f := s.Validate(parse(t, front)); len(f) != 0 {
			t.Errorf("%s: %v", k, f)
		}
	}
}

func TestFindings(t *testing.T) {
	cases := []struct {
		name, front, rule string
		line              int
		msg               string
	}{
		{"no name", "kind: concept\n", "S01", 2, `the field "name" is missing`},
		{"no role", "kind: statement\nname: A node\n", "S02", 2, `the field "role" is missing, and a node of kind statement needs it`},
		{"no defines", "kind: definition\nname: A node\n", "S02", 2, `"defines"`},
		{"status", "kind: concept\nname: A node\nstatus: E5\n", "S03", 5, `the field "status" states a status`},
		{"field of another kind", "kind: statement\nrole: lemma\nname: A node\nproves: 1H8D2C\n", "S01", 6, `the field "proves" is not allowed in a node of kind statement`},
		{"bad msc", "kind: concept\nname: A node\nmsc:\n  - 20D60\n  - 20E05\n", "S01", 7, "msc.1:"},
		{"bad ref", "kind: concept\nname: A node\nuses: [7K3QXI]\n", "S01", 5, "uses.0: the value is not a tag or a provisional id"},
		{"bad nested field", "kind: statement\nrole: lemma\nname: A node\nformal:\n  - system: lean4\n    decl: X\n    match: exact\n    level: E5\n", "S01", 10, `the field "level" is not allowed here`},
		{"bad enum", "kind: statement\nrole: claim\nname: A node\n", "S01", 4, "role:"},
		{"two source ids", "kind: concept\nname: A node\nsources:\n  - doi: \"10.1/x\"\n    arxiv: \"2106.09685v2\"\n    locator: p. 3\n", "S01", 6, "sources.0: a source must have exactly one identifier"},
	}
	s := Embedded()
	for _, c := range cases {
		got := s.Validate(parse(t, c.front))
		found := false
		for _, f := range got {
			if f.Rule == c.rule && f.Line == c.line && strings.Contains(f.Msg, c.msg) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: want %s on line %d with %q, got %v", c.name, c.rule, c.line, c.msg, got)
		}
	}
}

func TestAllErrors(t *testing.T) {
	got := Embedded().Validate(parse(t, "kind: statement\nstatus: E5\nmsc: [\"2D60\"]\n"))
	rules := map[string]bool{}
	for _, f := range got {
		rules[f.Rule] = true
		if !strings.HasPrefix(f.String(), "nodes/7K/7K3QX2.md:") {
			t.Errorf("finding %q has no file", f)
		}
	}
	if !rules["S01"] || !rules["S02"] || !rules["S03"] || len(got) != 4 {
		t.Errorf("Validate gives %v", got)
	}
}

func TestDateStaysString(t *testing.T) {
	front := "kind: statement\nrole: lemma\nname: A node\nretired:\n  date: 2026-10-25\n  reason: duplicate\n  successor: [1H8D2C]\n"
	if f := Embedded().Validate(parse(t, front)); len(f) != 0 {
		t.Errorf("an unquoted date gives %v", f)
	}
}

func TestLoad(t *testing.T) {
	root := t.TempDir()
	s, err := Load(root)
	if err != nil || s != Embedded() {
		t.Fatalf("Load on a tree with no schema = %v, %v", s, err)
	}
	if err := os.MkdirAll(filepath.Join(root, "schema"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, File), embedded, 0o644); err != nil {
		t.Fatal(err)
	}
	if s, err = Load(root); err != nil || !strings.HasSuffix(s.Source, File) {
		t.Fatalf("Load = %v, %v", s, err)
	}
	if err := os.WriteFile(filepath.Join(root, File), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Error("Load of a bad schema gives no error")
	}
}

func TestMigrate(t *testing.T) {
	var front yaml.Node
	if path, err := Migrate(&front, Version); err != nil || len(path) != 0 {
		t.Errorf("Migrate from the current version = %v, %v", path, err)
	}
	if _, err := Migrate(&front, "node.v0"); err == nil {
		t.Error("Migrate from node.v0 gives no error")
	}
	saved := Migrations
	defer func() { Migrations = saved }()
	called := false
	Migrations = []Migration{{From: "node.v0", To: Version, Apply: func(*yaml.Node) error { called = true; return nil }}}
	if path, err := Migrate(&front, "node.v0"); err != nil || !called || len(path) != 1 {
		t.Errorf("Migrate = %v, %v, called %v", path, err, called)
	}
}

// TestNodeFixtures checks that the canonical fixtures of the node package are valid nodes.
func TestNodeFixtures(t *testing.T) {
	files, err := filepath.Glob("../node/testdata/canonical/*.md")
	if err != nil || len(files) == 0 {
		t.Fatalf("found %d fixtures, %v", len(files), err)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		n, err := node.Parse("nodes/xx/"+filepath.Base(f), data)
		if err != nil {
			t.Fatal(err)
		}
		if got := Embedded().Validate(n); len(got) != 0 {
			t.Errorf("%s: %v", f, got)
		}
	}
}

func TestTemplate(t *testing.T) {
	s := Embedded()
	for _, k := range s.Kinds() {
		data, err := s.Template(k, "~0123abcd")
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		path := "nodes/~/~0123abcd.md"
		if ok, err := node.Canonical(path, data, s.Order); !ok || err != nil {
			t.Errorf("%s: the template is not canonical (%v):\n%s", k, err, data)
		}
		n, err := node.Parse(path, data)
		if err != nil {
			t.Fatalf("%s: %v", k, err)
		}
		req, _ := s.Required(k)
		var want []string
		for _, f := range req {
			if f != "tag" && f != "kind" {
				want = append(want, fmt.Sprintf("S01: the field %q has no value", f))
			}
		}
		var got []string
		for _, f := range s.Validate(n) {
			got = append(got, f.Rule+": "+f.Msg)
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: the findings are %q, want %q", k, got, want)
		}
		if !strings.Contains(string(data), "\n## ") {
			t.Errorf("%s: the template has no section:\n%s", k, data)
		}
	}
	if _, err := s.Template("lemma", "~0123abcd"); err == nil || !strings.Contains(err.Error(), `"lemma" is not a kind`) {
		t.Errorf("the error is %v", err)
	}
}

func TestRequired(t *testing.T) {
	s := Embedded()
	cases := map[string][]string{
		"concept":     {"tag", "kind", "name"},
		"statement":   {"tag", "kind", "role", "name"},
		"computation": {"tag", "kind", "name", "claim", "engine"},
	}
	for k, want := range cases {
		if got, ok := s.Required(k); !ok || !slices.Equal(got, want) {
			t.Errorf("%s: got %v, want %v", k, got, want)
		}
	}
	if _, ok := s.Required("lemma"); ok {
		t.Error("Required knows the kind lemma")
	}
}
