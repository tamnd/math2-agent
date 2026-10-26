package bump_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/bump"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

func write(t *testing.T, root, path, data string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const lakefile = `name = "Math2"

[[require]]
name = "batteries"
git = "https://github.com/leanprover-community/batteries"
rev = "main"

# Pin Mathlib to a release tag.
[[require]]
name = "mathlib"
git = "https://github.com/leanprover-community/mathlib4"
rev = "v4.35.0"

[[lean_lib]]
name = "Math2"
`

func project(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "lakefile.toml", lakefile)
	write(t, dir, "lean-toolchain", "leanprover/lean4:v4.35.0\n")
	write(t, dir, "lake-manifest.json", `{"version": "1.1.0", "packages": [{"name": "batteries", "rev": "b1"}, {"name": "mathlib", "rev": "0123456789abcdef0123456789abcdef01234567"}]}`)
	return dir
}

func TestPins(t *testing.T) {
	dir := project(t)
	p, err := bump.ReadPins(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := bump.Pins{Toolchain: "leanprover/lean4:v4.35.0", Git: "https://github.com/leanprover-community/mathlib4", Tag: "v4.35.0", Commit: "0123456789abcdef0123456789abcdef01234567"}
	if p != want {
		t.Errorf("the pins are %+v, want %+v", p, want)
	}
	if err := bump.SetPins(dir, "v4.36.0", "leanprover/lean4:v4.36.0"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, dir, "lakefile.toml"); got != strings.Replace(lakefile, `rev = "v4.35.0"`, `rev = "v4.36.0"`, 1) {
		t.Errorf("the lakefile is\n%s", got)
	}
	if p, _ := bump.ReadPins(dir); p.Tag != "v4.36.0" || p.Toolchain != "leanprover/lean4:v4.36.0" {
		t.Errorf("the new pins are %+v", p)
	}
	if err := bump.SetPins(dir, `v4"`, "leanprover/lean4:v4.36.0"); err == nil {
		t.Error("SetPins with a quote in the tag gives no error")
	}
	write(t, dir, "lakefile.toml", "name = \"Math2\"\n")
	if _, err := bump.ReadPins(dir); err == nil {
		t.Error("ReadPins with no mathlib gives no error")
	}
}

func TestLatest(t *testing.T) {
	for _, tc := range []struct {
		tags []string
		want string
	}{
		{[]string{"v4.9.0", "v4.35.0", "v4.10.1", "v4.36.0-rc1", "master", "nightly-testing"}, "v4.35.0"},
		{[]string{"v4.35.0", "v4.35.1"}, "v4.35.1"},
		{[]string{"v4.36.0-rc1"}, ""},
	} {
		if got, ok := bump.Latest(tc.tags); got != tc.want || ok != (tc.want != "") {
			t.Errorf("Latest(%v) is %q, %v, want %q", tc.tags, got, ok, tc.want)
		}
	}
}

// transport answers each request with the body, and records the request.
type transport struct {
	body string
	code int
	req  *http.Request
}

func (f *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.req = r
	return &http.Response{StatusCode: f.code, Status: http.StatusText(f.code), Body: io.NopCloser(strings.NewReader(f.body)), Request: r}, nil
}

func TestToolchainOf(t *testing.T) {
	f := &transport{body: "leanprover/lean4:v4.36.0\n", code: http.StatusOK}
	c := &http.Client{Transport: f}
	got, err := bump.ToolchainOf(context.Background(), c, "https://github.com/leanprover-community/mathlib4.git", "v4.36.0", "m2/test")
	if err != nil || got != "leanprover/lean4:v4.36.0" {
		t.Fatalf("ToolchainOf is %q, %v", got, err)
	}
	if u := f.req.URL.String(); u != "https://raw.githubusercontent.com/leanprover-community/mathlib4/v4.36.0/lean-toolchain" || f.req.UserAgent() != "m2/test" {
		t.Errorf("the request is %s with the User-Agent %q", u, f.req.UserAgent())
	}
	f.body = "<html>"
	if _, err := bump.ToolchainOf(context.Background(), c, "https://github.com/leanprover-community/mathlib4", "v4.36.0", ""); err == nil {
		t.Error("a file that is not a toolchain gives no error")
	}
	f.code = http.StatusNotFound
	if _, err := bump.ToolchainOf(context.Background(), c, "https://github.com/leanprover-community/mathlib4", "v9", ""); err == nil {
		t.Error("a missing tag gives no error")
	}
	if _, err := bump.ToolchainOf(context.Background(), c, "https://example.org/mathlib4", "v4.36.0", ""); err == nil {
		t.Error("a repository that is not on GitHub gives no error")
	}
}

// nodeFile returns a node file with a Lean binding of each declaration.
func nodeFile(tag string, decls ...string) string {
	s := "---\ntag: " + tag + "\nkind: statement\nrole: theorem\nname: A statement of the test\nformal:\n"
	for _, d := range decls {
		s += "  - system: lean4\n    library: mathlib\n    decl: " + d + "\n    match: exact\n"
	}
	return s + "---\n\n## Statement\n\nA statement of the test.\n"
}

func parse(t *testing.T, tag string, decls ...string) *node.Node {
	t.Helper()
	n, err := node.Parse("nodes/"+tag[:3]+"/"+tag+".md", []byte(nodeFile(tag, decls...)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func rec(decl, hash string) lean.Record { return lean.Record{Decl: decl, TypeHash: hash} }

func gone(decl string) lean.Record { return lean.Record{Decl: decl, Error: "unknown declaration"} }

func TestClassify(t *testing.T) {
	nodes := []*node.Node{
		parse(t, "AAAAAA", "Old.alias"),
		parse(t, "BBBBBB", "Old.moved", "Same.thing"),
		parse(t, "CCCCCC", "Old.changed"),
		parse(t, "DDDDDD", "Old.lost", "Old.twice"),
		parse(t, "EEEEEE", "New.binding"),
	}
	old := &build.Extraction{Records: []lean.Record{
		rec("Old.alias", "h1"), rec("Old.moved", "h2"), rec("Same.thing", "h3"), rec("Old.changed", "h4"), rec("Old.lost", "h5"), rec("Old.twice", "h6"),
	}}
	dep := rec("Old.alias", "h1")
	dep.Deprecated = &lean.Deprecation{NewName: "New.alias", Since: "2026-11-01"}
	cur := &build.Extraction{Records: []lean.Record{
		dep, gone("Old.moved"), rec("Same.thing", "h3"), rec("Old.changed", "h9"), gone("Old.lost"), gone("Old.twice"), rec("New.binding", "h7"),
	}}
	byHash := bump.ByHash([]lean.Decl{
		{Decl: "New.moved", TypeHash: "h2"}, {Decl: "Same.thing", TypeHash: "h3"},
		{Decl: "Twice.a", TypeHash: "h6"}, {Decl: "Twice.b", TypeHash: "h6"},
	})
	got := bump.Classify(nodes, old, cur, byHash)
	want := []bump.Break{
		{Kind: bump.Deprecated, Tag: "AAAAAA", Old: "Old.alias", New: "New.alias"},
		{Kind: bump.Renamed, Tag: "BBBBBB", Old: "Old.moved", New: "New.moved"},
		{Kind: bump.Refactored, Tag: "CCCCCC", Old: "Old.changed"},
		{Kind: bump.Lost, Tag: "DDDDDD", Old: "Old.lost", Text: "unknown declaration"},
		{Kind: bump.Lost, Tag: "DDDDDD", Old: "Old.twice", Text: "the declarations with the old type hash are Twice.a, Twice.b"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the breaks are\n%+v\nwant\n%+v", got, want)
	}
	for _, b := range bump.Classify(nodes, old, cur, nil) {
		if b.Old == "Old.moved" && b.Kind != bump.Lost {
			t.Errorf("without the hashes, a rename is %+v, want lost", b)
		}
	}
}

func TestFailures(t *testing.T) {
	out := `✖ [1203/1210] Building Math2.Algebra.Units
trace: .> LEAN_PATH=...
error: ././././Math2/Algebra/Units.lean:12:4: unknown identifier 'Subgroup.card_subgroup_dvd_card'
error: ././././Math2/Algebra/Units.lean:12:4: unknown identifier 'Subgroup.card_subgroup_dvd_card'
error: ./Math2/Match.lean:3:0: unknown module prefix 'Mathlib.Old'
error: Lean exited with code 1`
	got := bump.Failures(out)
	want := []bump.Break{
		{Kind: bump.Broken, File: "Math2/Algebra/Units.lean", Line: 12, Text: "unknown identifier 'Subgroup.card_subgroup_dvd_card'"},
		{Kind: bump.Broken, File: "Math2/Match.lean", Line: 3, Text: "unknown module prefix 'Mathlib.Old'"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the failures are\n%+v\nwant\n%+v", got, want)
	}
}

func TestUpstream(t *testing.T) {
	x := &build.Extraction{
		Bindings: []lean.Binding{{Decl: "Math2.cyclic", Tag: "AAAAAA"}, {Decl: "Math2.own", Tag: "BBBBBB"}},
		Records:  []lean.Record{rec("Math2.cyclic", "h1"), rec("Math2.own", "h2")},
	}
	byHash := bump.ByHash([]lean.Decl{{Decl: "IsCyclic.of_finite", TypeHash: "h1"}, {Decl: "Math2.own", TypeHash: "h2"}})
	got := bump.Upstream(x, byHash)
	if len(got) != 1 || got[0].Tag != "AAAAAA" || !slices.Equal(got[0].Mathlib, []string{"IsCyclic.of_finite"}) {
		t.Errorf("the catch-ups are %+v", got)
	}
}

func TestStale(t *testing.T) {
	old := []status.Status{{Tag: "AAAAAA", Fidelity: status.F2}, {Tag: "BBBBBB", Fidelity: status.F1}, {Tag: "CCCCCC", Fidelity: status.F0}}
	cur := []status.Status{{Tag: "AAAAAA", Fidelity: status.F0}, {Tag: "BBBBBB", Fidelity: status.F1}, {Tag: "CCCCCC", Fidelity: status.F2}, {Tag: "DDDDDD", Fidelity: status.F0}}
	if got := bump.Stale(old, cur); !slices.Equal(got, []string{"AAAAAA"}) {
		t.Errorf("the stale nodes are %v", got)
	}
}

func TestRebind(t *testing.T) {
	root := t.TempDir()
	write(t, root, "nodes/AAA/AAAAAA.md", nodeFile("AAAAAA", "Old.alias", "Same.thing"))
	write(t, root, "nodes/CCC/CCCCCC.md", nodeFile("CCCCCC", "Old.changed"))
	write(t, root, bump.Register, "# tag system old new cause mathlib author\nAAAAAA lean4 - Old.alias bind c0 github:octocat\nZZZZZZ lean4 - Last.one bind c0 github:octocat\n")
	bs := []bump.Break{
		{Kind: bump.Deprecated, Tag: "AAAAAA", Old: "Old.alias", New: "New.alias"},
		{Kind: bump.Refactored, Tag: "CCCCCC", Old: "Old.changed"},
		{Kind: bump.Lost, Tag: "DDDDDD", Old: "Old.lost"},
	}
	n, err := bump.Rebind(root, bs, "c1")
	if err != nil || n != 1 {
		t.Fatalf("Rebind changed %d files, %v", n, err)
	}
	if got := read(t, root, "nodes/AAA/AAAAAA.md"); got != nodeFile("AAAAAA", "New.alias", "Same.thing") {
		t.Errorf("the node file is\n%s", got)
	}
	want := "# tag system old new cause mathlib author\nAAAAAA lean4 - Old.alias bind c0 github:octocat\nAAAAAA lean4 Old.alias New.alias rename c1 -\nCCCCCC lean4 Old.changed Old.changed refactor c1 -\nZZZZZZ lean4 - Last.one bind c0 github:octocat\n"
	if got := read(t, root, bump.Register); got != want {
		t.Errorf("the register is\n%s\nwant\n%s", got, want)
	}
	authors, err := status.LoadAuthors(root)
	if err != nil || authors[status.Key{Tag: "AAAAAA", Decl: "New.alias"}] != "github:octocat" {
		t.Errorf("after the rename, the authors are %v, %v", authors, err)
	}
	if _, err := bump.Rebind(root, bs, "c 1"); err == nil {
		t.Error("a commit with a space gives no error")
	}
}

func TestReport(t *testing.T) {
	r := &bump.Report{
		Date: "2026-11-02",
		From: bump.Pins{Toolchain: "leanprover/lean4:v4.35.0", Tag: "v4.35.0", Commit: "0123456789abcdef0123"},
		To:   bump.Pins{Toolchain: "leanprover/lean4:v4.36.0", Tag: "v4.36.0", Commit: "fedcba9876543210fedc"},
		Breaks: []bump.Break{
			{Kind: bump.Deprecated, Tag: "AAAAAA", Old: "Old.alias", New: "New.alias"},
			{Kind: bump.Broken, File: "Math2/Match.lean", Line: 3, Text: "unknown | module"},
		},
		Stale: []string{"CCCCCC"},
		Took:  90 * time.Second,
	}
	md := string(r.Markdown())
	for _, s := range []string{
		"# Bump of 2026-11-02",
		"Mathlib moves from v4.35.0 (0123456789ab) to v4.36.0 (fedcba987654).",
		"The build fails.",
		"| deprecated | 1 |",
		"| renamed | 0 |",
		"| deprecated | AAAAAA | `Old.alias` | `New.alias` |  |",
		"| broken | Math2/Match.lean:3 |  |  | unknown \\| module |",
		"1 node has a lower fidelity level",
		"No Math2 declaration has the type hash",
	} {
		if !strings.Contains(md, s) {
			t.Errorf("the report has no %q:\n%s", s, md)
		}
	}
	if r.Path() != "reports/bump/2026-11-02.md" || r.Count(bump.Broken) != 1 {
		t.Errorf("the path is %s and the count is %d", r.Path(), r.Count(bump.Broken))
	}
	if !bump.Deprecated.Mechanical() || bump.Refactored.Mechanical() || len(bump.Kinds()) != 5 {
		t.Error("the kinds are wrong")
	}
}
