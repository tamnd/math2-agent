package release_test

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/release"
)

var update = flag.Bool("update", false, "write the golden files")

// repo copies the tree src to a new git repository and commits it. It returns the directory and the commit.
func repo(t *testing.T, src string) (dir, commit string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on the PATH")
	}
	dir = t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "m2")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "m2@example.com")
	}
	for _, k := range []string{"GIT_AUTHOR_DATE", "GIT_COMMITTER_DATE"} {
		t.Setenv(k, "2026-10-01T12:00:00Z")
	}
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "seed")
	return dir, git(t, dir, "rev-parse", "HEAD")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestRebuildGraph rebuilds the graph of the seed and compares the hash with testdata/seed.graph. The golden file comes from another machine, thus a pass in CI shows that the graph rebuilds bit for bit on a second machine (rule R01).
func TestRebuildGraph(t *testing.T) {
	dir, commit := repo(t, filepath.Join("testdata", "seed"))
	got, err := release.RebuildGraph(context.Background(), dir, commit)
	if err != nil {
		t.Fatal(err)
	}
	m, err := build.Run(context.Background(), build.Options{Root: dir, Only: build.Graph})
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != m.Steps[0].Output {
		t.Errorf("RebuildGraph gives %s, and the graph step of m2 build gives %s", got.Hash, m.Steps[0].Output)
	}
	golden := filepath.Join("testdata", "seed.graph")
	if *update {
		if err := os.WriteFile(golden, []byte(got.Hash+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != strings.TrimSpace(string(want)) {
		t.Errorf("the graph of the seed has the hash %s, and %s has %s", got.Hash, golden, want)
	}
	if _, err := release.RebuildGraph(context.Background(), dir, "-x"); err == nil {
		t.Error("RebuildGraph accepts the commit -x")
	}
	if _, err := release.RebuildGraph(context.Background(), dir, strings.Repeat("0", 40)); err == nil {
		t.Error("RebuildGraph accepts a commit that is not in the repository")
	}
}

func TestMake(t *testing.T) {
	dir, commit := repo(t, filepath.Join("testdata", "seed"))
	out := t.TempDir()
	if _, err := build.Run(context.Background(), build.Options{Root: dir, Out: out, Version: "v0.1.4"}); err != nil {
		t.Fatal(err)
	}
	m, err := release.Make(dir, out, "2027.03")
	if err != nil {
		t.Fatal(err)
	}
	if m.Release != "2027.03" || m.Commit != commit || m.Toolchain.M2 != "v0.1.4" {
		t.Errorf("Make gives the release %s, the commit %s and m2 %s", m.Release, m.Commit, m.Toolchain.M2)
	}
	graph, err := release.RebuildGraph(context.Background(), dir, commit)
	if err != nil {
		t.Fatal(err)
	}
	if m.Artefacts[release.Graph] != graph {
		t.Errorf("the manifest has the graph %+v, and the rebuild gives %+v", m.Artefacts[release.Graph], graph)
	}
	if _, ok := m.Artefacts[release.Oleans]; ok {
		t.Error("the manifest has .olean files, and the tree has no lean/ directory")
	}
	want := release.Counts{NodesLive: 2, ByKind: map[string]int{"concept": 1, "statement": 1}, ByEvidence: map[string]int{"E1": 1}}
	if !reflect.DeepEqual(m.Counts, want) {
		t.Errorf("the counts are %+v, and they must be %+v", m.Counts, want)
	}

	data, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "2027.03.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	back, err := release.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, m) {
		t.Errorf("Read gives %+v after Marshal of %+v", back, m)
	}

	if err := os.WriteFile(filepath.Join(dir, "nodes", "3C", "3C53TD.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := release.Make(dir, out, "2027.03"); err == nil {
		t.Error("Make accepts a working tree with changes")
	}
}

func TestCompare(t *testing.T) {
	write := func(dir string, files map[string]string) {
		for p, c := range files {
			p = filepath.Join(dir, p)
			if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	want, got := t.TempDir(), t.TempDir()
	write(want, map[string]string{"Math2.olean": "a", "Math2/Basic.olean": "b", "Math2/Old.ilean": "c", "Math2.trace": "1"})
	write(got, map[string]string{"Math2.olean": "a", "Math2/Basic.olean": "x", "Math2/New.ir": "d", "Math2.trace": "2"})
	ds, err := release.Compare(want, got)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, d := range ds {
		paths = append(paths, d.Path)
		if d.Path == "Math2/Old.ilean" && d.Got != "" || d.Path == "Math2/New.ir" && d.Want != "" {
			t.Errorf("the difference %+v has a hash for a file that one tree does not have", d)
		}
	}
	if strings.Join(paths, " ") != "Math2/Basic.olean Math2/New.ir Math2/Old.ilean" {
		t.Errorf("Compare gives %v", paths)
	}
	if ds, err := release.Compare(want, want); err != nil || len(ds) != 0 {
		t.Errorf("Compare of a tree with itself gives %v, %v", ds, err)
	}
}

func TestIsOlean(t *testing.T) {
	for rel, want := range map[string]bool{
		"Math2.olean": true, "Math2.olean.private": true, "Math2.olean.server": true, "Math2.ilean": true, "Math2.ir": true, "Math2.ir.sig": true,
		"Math2.olean.hash": false, "Math2.trace": false, "Math2/Basic.log": false,
	} {
		if got := release.IsOlean(rel); got != want {
			t.Errorf("IsOlean(%q) = %v", rel, got)
		}
	}
}

func TestReadSpecExample(t *testing.T) {
	src := `release: 2026.11
commit: 8b1e4c0f2d
toolchain:
  m2: v0.9.0
  lean: leanprover/lean4:v4.35.0
  mathlib: 0123abcd
  images:
    sage: ghcr.io/tamnd/m2-sage@sha256:00
artefacts:
  graph: {hash: "blake3:af01", size: 3100000000}
counts:
  nodes_live: 0
  by_kind: {}
  by_evidence: {E5: 0}
rebuild_check:
  runner: ubuntu-24.04/x86_64/32c/128G
  graph_hash_reproduced: true
  oleans_hash_reproduced: false
  olean_differences:
    - {path: Math2/Basic.olean, reason: the toolchain was built again}
`
	var m release.Manifest
	if err := yaml.Unmarshal([]byte(src), &m); err != nil {
		t.Fatal(err)
	}
	if m.Toolchain.Images["sage"] == "" || m.Artefacts[release.Graph].Size != 3100000000 || len(m.RebuildCheck.OleanDifferences) != 1 {
		t.Errorf("the manifest is %+v", m)
	}
}
