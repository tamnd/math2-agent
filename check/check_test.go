package check_test

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/check"
)

// topic returns a topic node file with the given body. The body starts on line 7, so the text under the first heading is on line 10.
func topic(body string) check.File {
	return check.File{Path: "nodes/3EY/3EYFQZ.md", Data: []byte("---\ntag: 3EYFQZ\nkind: topic\nname: Elementary number theory\nmsc: [11Axx]\n---\n" + body)}
}

func run(files ...check.File) *check.Report {
	var c check.Checker
	return c.Check(files, check.Index{"DSG7KN": {}, "5FK5AA": {Retired: true}, "DTT8HP": {Retired: true, Successors: []string{"ENS73X"}}})
}

func TestBody(t *testing.T) {
	tests := []struct {
		body string
		want string // the first finding, as rule:line:col, or ""
	}{
		{"\n## Summary\n\nThe integers $\\Z$ and $\\abs{x}$, $\\ip{x}{y}$, $\\left\\{ x \\right\\}$ and $\\frac{1}{2}$.\n", ""},
		{"\n## Summary\n\n$$\n\\begin{aligned}\na &= b \\\\\nc &= d\n\\end{aligned}\n$$\n", ""},
		{"\n## Summary\n\nThe price is \\$5, and `$x` is code.\n\n```\n$ not math\n## Not a heading\n```\n", ""},
		{"\n## Summary\n\nSee [[the group|DSG7KN]] and [[an old proof|DTT8HP]].\n", ""},
		{"\n## Summary\n\n### A subheading\n\nText.\n\n## Remarks\n\nMore.\n", ""},
		{"\n## Summary\n\nThe sum $\\ip x y$ has two arguments.\n", ""},
		{"\n## Summary\n\nThe map $x \\mapsto \\foo$.\n", "B01:10:20"},
		{"\n## Summary\n\nThe set $\\{x$ is open.\n", ""},
		{"\n## Summary\n\nThe set ${x$ is open.\n", "B01:10:10"},
		{"\n## Summary\n\n$$\n\\begin{matrix} a \\end{pmatrix}\n$$\n", "B01:11:18"},
		{"\n## Summary\n\n$$\n\\frac{1}{2}\n", "B01:10:1"},
		{"\n## Summary\n\nThe math $x\\$ is open.\n", "B01:10:10"},
		{"\n## Summary\n\nSee [[the old claim|5FK5AA]].\n", "B02:10:5"},
		{"\n## Summary\n\nSee [[a dead tag|W96C20]].\n", "B02:10:5"},
		{"\n## Summary\n\nSee https://example.org, which is fine outside a statement section.\n", ""},
		{"\n## Remarks\n\nText.\n\n## Summary\n\nText.\n", "B03:12:1"},
		{"\n## Summary\n\nText.\n\n## Summary\n\nText.\n", "B03:12:1"},
	}
	for _, tt := range tests {
		rep := run(topic(tt.body))
		got := ""
		if len(rep.Findings) > 0 {
			f := rep.Findings[0]
			got = fmt.Sprintf("%s:%d:%d", f.Rule, f.Line, f.Col)
		}
		if got != tt.want {
			t.Errorf("body %q: got %s %v, want %s", tt.body, got, rep.Findings, tt.want)
		}
	}
}

func TestStates(t *testing.T) {
	rep := run(topic("\n## Summary\n\nNo math and no links.\n"))
	want := map[string]check.State{"S01": check.Pass, "S04": check.Pass, "B01": check.NA, "B02": check.NA, "B03": check.Pass, "G01": check.NA}
	for _, res := range rep.Results {
		if w, ok := want[res.Rule]; ok && res.State != w {
			t.Errorf("%s: the state is %s, and it must be %s", res.Rule, res.State, w)
		}
	}
	if rep.Failed() {
		t.Error("the report failed, and no rule found anything")
	}

	empty := run()
	for _, res := range empty.Results {
		if res.State != check.NA {
			t.Errorf("%s: with no files the state is %s, and it must be n/a", res.Rule, res.State)
		}
	}

	c := check.Checker{Only: []string{"B01"}}
	rep = c.Check([]check.File{topic("\n## Remarks\n\nThe value $\\foo$.\n")}, nil)
	for _, res := range rep.Results {
		switch {
		case res.Rule == "B01" && res.State != check.Fail:
			t.Errorf("B01: the state is %s, and it must be fail", res.State)
		case res.Rule != "B01" && res.State != check.Skip:
			t.Errorf("%s: the state is %s, and it must be skip", res.Rule, res.State)
		}
	}
	if len(rep.Findings) != 1 {
		t.Errorf("with only B01, got %v", rep.Findings)
	}
}

func TestLinksResolveInRun(t *testing.T) {
	a := check.File{Path: "nodes/~5/~5c1e8a20.md", Data: []byte("---\ntag: ~5c1e8a20\nkind: topic\nname: A new topic\nmsc: [11Axx]\n---\n\n## Summary\n\nSee [[the other|3EYFQZ]].\n")}
	rep := run(a, topic("\n## Summary\n\nSee [[the new topic|~5c1e8a20]].\n"))
	for _, f := range rep.Findings {
		if f.Rule == "B02" {
			t.Error(f)
		}
	}
}

func TestSize(t *testing.T) {
	big := topic("\n## Summary\n\n" + strings.Repeat("Some text.\n", check.MaxSize/11+1))
	rep := run(big)
	if len(rep.Findings) != 1 || rep.Findings[0].Rule != "S04" {
		t.Errorf("got %v, want one S04 finding", rep.Findings)
	}
}

func TestParseMacros(t *testing.T) {
	m, err := check.ParseMacros([]byte(`{"\\ip": "\\langle #1, #2\\rangle", "\\R": "\\mathbb{R}"}`))
	if err != nil {
		t.Fatal(err)
	}
	if m["ip"] != 2 || m["R"] != 0 || len(m) != 2 {
		t.Errorf("got %v", m)
	}
	for _, bad := range []string{`{"ip": "x"}`, `{"\\i2": "x"}`, `[]`} {
		if _, err := check.ParseMacros([]byte(bad)); err == nil {
			t.Errorf("%s: want an error", bad)
		}
	}
	if m := check.EmbeddedMacros(); m["ip"] != 2 || m["Z"] != 0 {
		t.Errorf("the embedded table is %v", m)
	}
}

// TestEmbeddedMacrosAreCurrent compares the embedded macro table with the table in a math2 working tree. Set M2_CONTENT to the tree to run it.
func TestEmbeddedMacrosAreCurrent(t *testing.T) {
	root := os.Getenv("M2_CONTENT")
	if root == "" {
		t.Skip("M2_CONTENT is not set")
	}
	m, err := check.LoadMacros(root)
	if err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(m, check.EmbeddedMacros()) {
		t.Errorf("check/m2.katex.json is not the same as %s. Copy the file again.", filepath.Join(root, check.MacroFile))
	}
}
