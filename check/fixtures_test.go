package check_test

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
)

// The fixture sets of spec 19 M0 are in testdata/nodes at the module root. testdata/nodes/README.md describes them.
var (
	validRoot  = filepath.Join("..", "testdata", "nodes", "valid")
	brokenRoot = filepath.Join("..", "testdata", "nodes", "broken")
)

type expect struct {
	File    string `yaml:"file"`
	Rule    string `yaml:"rule"`
	Line    int    `yaml:"line"`
	Planted string `yaml:"planted"`
}

func readTree(t testing.TB, root string) []check.File {
	t.Helper()
	files, err := check.ReadTree(root)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func index(t testing.TB, root string) check.Index {
	t.Helper()
	nodes, err := node.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	ix := check.Index{}
	for _, n := range nodes {
		ix.Add(n)
	}
	return ix
}

func TestValidSet(t *testing.T) {
	files := readTree(t, validRoot)
	var c check.Checker
	rep := c.Check(files, nil)
	for _, f := range rep.Findings {
		t.Error(f)
	}
	if rep.Files != len(files) || rep.Files < 100 {
		t.Errorf("the report has %d files, and the tree has %d", rep.Files, len(files))
	}
	for _, res := range rep.Results {
		if res.State != check.Pass {
			t.Errorf("%s: the state is %s, and it must be pass", res.Rule, res.State)
		}
	}
}

func TestBrokenSet(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(brokenRoot, "expect.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var es []expect
	if err := yaml.Unmarshal(data, &es); err != nil {
		t.Fatal(err)
	}
	ix := index(t, validRoot)
	var c check.Checker
	caught := 0
	for _, e := range es {
		data, err := os.ReadFile(filepath.Join(brokenRoot, e.File))
		if err != nil {
			t.Fatal(err)
		}
		rep := c.Check([]check.File{{Path: e.File, Data: data}}, ix)
		if len(rep.Findings) != 1 || rep.Findings[0].Rule != e.Rule || rep.Findings[0].Line != e.Line {
			t.Errorf("%s: want one finding %s at line %d (%s), got %v", e.File, e.Rule, e.Line, e.Planted, rep.Findings)
			continue
		}
		if !rep.Failed() {
			t.Errorf("%s: the report has a finding of a hard rule, but it did not fail", e.File)
		}
		caught++
	}
	t.Logf("m2 check caught %d of %d planted errors", caught, len(es))
}
