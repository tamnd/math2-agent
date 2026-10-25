package schema_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
	"github.com/tamnd/math2-agent/tag"
)

// The fixture sets of spec 19 M0 are in testdata/nodes at the module root. The valid set is a tree of nodes with no error. Each node in the broken set has one planted error, and expect.yaml records it.
var (
	validRoot  = filepath.Join("..", "testdata", "nodes", "valid")
	brokenRoot = filepath.Join("..", "testdata", "nodes", "broken")
)

// Expect is one entry of expect.yaml.
type expect struct {
	File    string `yaml:"file"`
	Rule    string `yaml:"rule"`
	Line    int    `yaml:"line"`
	From    string `yaml:"from"`
	Planted string `yaml:"planted"`
}

// rules are the rules that m2 check runs in M0 (spec 15.3).
var rules = []string{"S01", "S02", "S03", "S04", "B01", "B02", "B03", "G01"}

// edgeFields are the front matter fields whose values are node references.
var edgeFields = []string{"topics", "uses", "proofs", "defines", "proves", "specializes", "about", "instance_of", "methods", "resolved_by", "successor", "target", "via"}

func TestValidFixtures(t *testing.T) {
	nodes, err := node.Load(validRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(nodes) < 100 {
		t.Errorf("the valid set has %d nodes, and it must have at least 100", len(nodes))
	}
	s := schema.Embedded()
	kinds := map[string]int{}
	ids := map[string]bool{}
	for _, n := range nodes {
		ids[n.ID] = true
	}
	for _, n := range nodes {
		kinds[n.Kind()]++
		if got := n.Get("tag"); got != n.ID {
			t.Errorf("%s: the tag field is %q", n.Path, got)
		}
		for _, f := range s.Validate(n) {
			t.Error(f)
		}
		for _, r := range refs(n.Meta, "") {
			if !ids[r] {
				t.Errorf("%s: the reference %s is not a node in the valid set", n.Path, r)
			}
		}
		data, err := os.ReadFile(filepath.Join(validRoot, n.Path))
		if err != nil {
			t.Fatal(err)
		}
		if got := node.Format(n, s.Order); string(got) != string(data) {
			t.Errorf("%s is not in canonical form", n.Path)
		}
	}
	for _, k := range s.Kinds() {
		if kinds[k] < 5 {
			t.Errorf("the valid set has %d nodes of kind %s, and it must have at least 5", kinds[k], k)
		}
	}
}

// refs returns the node references in the front matter value v. The key is the field that holds v.
func refs(v any, key string) []string {
	var out []string
	switch v := v.(type) {
	case string:
		if slices.Contains(edgeFields, key) && (tag.IsTag(v) || tag.IsProvisional(v)) {
			out = append(out, v)
		}
	case []any:
		for _, item := range v {
			out = append(out, refs(item, key)...)
		}
	case map[string]any:
		for k, item := range v {
			out = append(out, refs(item, k)...)
		}
	}
	return out
}

func readExpect(t *testing.T) []expect {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(brokenRoot, "expect.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var es []expect
	if err := yaml.Unmarshal(data, &es); err != nil {
		t.Fatal(err)
	}
	return es
}

func TestBrokenFixturesComplete(t *testing.T) {
	es := readExpect(t)
	if len(es) < 200 {
		t.Errorf("expect.yaml has %d entries, and it must have at least 200", len(es))
	}
	files := map[string]bool{}
	count := map[string]int{}
	for _, e := range es {
		if files[e.File] {
			t.Errorf("%s: expect.yaml has two entries for this file", e.File)
		}
		files[e.File] = true
		count[e.Rule]++
		if !slices.Contains(rules, e.Rule) {
			t.Errorf("%s: the rule %s is not a rule of m2 check", e.File, e.Rule)
		}
		if e.Line < 1 || e.Planted == "" {
			t.Errorf("%s: the entry needs a line and a planted text", e.File)
		}
		if _, err := os.Stat(filepath.Join(validRoot, "nodes", tag.Shard(e.From), e.From+".md")); err != nil {
			t.Errorf("%s: the node %s that it copies is not in the valid set", e.File, e.From)
		}
	}
	for _, r := range rules {
		if count[r] < 3 {
			t.Errorf("the rule %s has %d broken fixtures, and it must have at least 3", r, count[r])
		}
	}
	err := filepath.WalkDir(filepath.Join(brokenRoot, "nodes"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(brokenRoot, path)
		if !files[filepath.ToSlash(rel)] {
			t.Errorf("%s has no entry in expect.yaml", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBrokenFixturesSchema checks the entries for S01, S02 and S03. Each gives exactly one finding, with the expected rule and line. The other entries must give no schema finding. The tests of the check package check the other rules.
func TestBrokenFixturesSchema(t *testing.T) {
	s := schema.Embedded()
	for _, e := range readExpect(t) {
		data, err := os.ReadFile(filepath.Join(brokenRoot, e.File))
		if err != nil {
			t.Fatal(err)
		}
		var got []schema.Finding
		n, err := node.Parse(e.File, data)
		var pe *node.Error
		switch {
		case errors.As(err, &pe):
			got = append(got, schema.Finding{Rule: "S01", Path: pe.Path, Line: pe.Line, Col: pe.Col, Msg: pe.Msg})
		case err != nil:
			t.Fatal(err)
		default:
			got = s.Validate(n)
		}
		if !strings.HasPrefix(e.Rule, "S") || e.Rule == "S04" || strings.Contains(e.Planted, "file name") {
			for _, f := range got {
				t.Errorf("%s: planted %s (%s), but the schema gives %s", e.File, e.Rule, e.Planted, f)
			}
			continue
		}
		if len(got) != 1 || got[0].Rule != e.Rule || got[0].Line != e.Line {
			t.Errorf("%s: want one finding %s at line %d (%s), got %v", e.File, e.Rule, e.Line, e.Planted, got)
		}
	}
}
