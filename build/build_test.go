package build_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

const fixture = "../testdata/nodes/valid"

func TestGraph(t *testing.T) {
	ctx := context.Background()
	out := t.TempDir()
	m, err := build.Run(ctx, build.Options{Root: fixture, Out: out, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Steps) != 2 || m.Steps[0].Name != build.Status || m.Steps[1].Name != build.Graph || m.Steps[0].Cached || m.Steps[1].Cached {
		t.Fatalf("the steps are %+v, and only the status and graph steps must run", m.Steps)
	}
	if m.Counts.Nodes == 0 || m.Counts.Edges == 0 {
		t.Errorf("the counts are %+v", m.Counts)
	}
	if _, err := os.Stat(filepath.Join(out, build.GraphDir, "edges", "logical.parquet")); err != nil {
		t.Error(err)
	}
	again, err := build.Run(ctx, build.Options{Root: fixture, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if !again.Steps[0].Cached || !again.Steps[1].Cached || again.Output != m.Output {
		t.Errorf("the second build is %+v, and it must be cached with the output %s", again.Steps, m.Output)
	}
	other, err := build.Run(ctx, build.Options{Root: fixture, Out: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if other.Output != m.Output {
		t.Errorf("a clean build gives %s, and the first build gave %s", other.Output, m.Output)
	}
	read, err := build.ReadManifest(out)
	if err != nil || read.Output != m.Output {
		t.Errorf("ReadManifest gives %+v, %v", read, err)
	}
}

// TestStatus checks that the status step reads the reviews, and that a change to the reviews runs the status step again.
func TestStatus(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "build")
	m, err := build.Run(ctx, build.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	st, err := build.ReadStatus(out)
	if err != nil || len(st) == 0 {
		t.Fatalf("ReadStatus gives %d statuses, %v", len(st), err)
	}
	if err := os.MkdirAll(filepath.Join(root, "reviews"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reviews", "x.yaml"), []byte("reviews: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, err := build.Run(ctx, build.Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if again.Steps[0].Name != build.Status || again.Steps[0].Cached || !again.Steps[1].Cached || again.Output != m.Output {
		t.Errorf("after a change to the reviews, the steps are %+v", again.Steps)
	}
	if err := os.WriteFile(filepath.Join(root, "reviews", "x.yaml"), []byte("reviews:\n  - id: r-1A0000-01\n    verdict: good\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := build.Run(ctx, build.Options{Root: root}); err == nil || !strings.Contains(err.Error(), "verdict") {
		t.Errorf("a bad review gives %v", err)
	}
}

func TestOnly(t *testing.T) {
	if _, err := build.Run(context.Background(), build.Options{Root: fixture, Out: t.TempDir(), Only: "search"}); err == nil {
		t.Error("an unknown step works")
	}
	m, err := build.Run(context.Background(), build.Options{Root: fixture, Out: t.TempDir(), Only: build.Lean})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Steps) != 0 {
		t.Errorf("a tree without lean/ has the steps %+v", m.Steps)
	}
}

// TestMath2 builds the tree of tamnd/math2 two times from a clean output directory, and checks that the two builds give the same output (rule R01). Set M2_MATH2_LEAN to the lean/ directory of the tree.
func TestMath2(t *testing.T) {
	project := os.Getenv("M2_MATH2_LEAN")
	if project == "" {
		t.Skip("M2_MATH2_LEAN is not set")
	}
	ctx := context.Background()
	opt := build.Options{Root: filepath.Dir(project), Cache: t.TempDir(), Out: t.TempDir(), Log: logger{t}}
	m, err := build.Run(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Steps) != 5 || m.Mathlib == "" || m.Checkers["leanchecker"] == "" {
		t.Errorf("the manifest is %+v", m)
	}
	x, err := build.ReadExtraction(opt.Out)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range x.Records {
		if r.Decl == "Math2.Match.lagrange" && r.Error == "" && len(r.UsesBound) == 1 && r.UsesBound[0] == "Subgroup.card_subgroup_dvd_card" {
			found = true
		}
	}
	if !found || len(x.Bindings) == 0 {
		t.Errorf("the extraction has no record of the adapter of 3C53TD, or no bindings: %+v", x.Bindings)
	}

	st, err := build.ReadStatus(opt.Out)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range st {
		if s.Tag == "3C53TD" && (len(s.Bindings) != 1 || s.Bindings[0].TypeHash == "" || s.Bindings[0].Checked != "Math2.Match.lagrange") {
			t.Errorf("the status of 3C53TD is %+v", s)
		}
	}

	opt.Out = t.TempDir()
	again, err := build.Run(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	if again.Output != m.Output {
		t.Errorf("the second build gives %s, and the first build gave %s", again.Output, m.Output)
	}
}

// TestReplayMath2 adds a review of the adapter of 3C53TD to a copy of the math2 tree. The binding then needs a replay, and the replay step checks it in each kernel. On Linux the checkers run in the sandbox, and the binding is at E5. On other systems the checkers run without the sandbox, and the binding is not at E5.
func TestReplayMath2(t *testing.T) {
	project := os.Getenv("M2_MATH2_LEAN")
	if project == "" {
		t.Skip("M2_MATH2_LEAN is not set")
	}
	ctx := context.Background()
	src := filepath.Dir(project)
	root := t.TempDir()
	for _, dir := range []string{"nodes", "registers"} {
		if err := os.CopyFS(filepath.Join(root, dir), os.DirFS(filepath.Join(src, dir))); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(project, filepath.Join(root, "lean")); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "nodes", "3C", "3C53TD.md")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	const line = "    adapter: Math2.Match.lagrange\n"
	if !strings.Contains(string(data), line) {
		t.Fatalf("%s has no line %q", file, line)
	}
	data = []byte(strings.Replace(string(data), line, line+"    fidelity_reviews: [r-3C53TD-01]\n", 1))
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}

	opt := build.Options{Root: root, Cache: t.TempDir(), Out: t.TempDir(), NoSandbox: runtime.GOOS != "linux", Log: logger{t}}
	if _, err := build.Run(ctx, opt); err != nil {
		t.Fatal(err)
	}
	x, err := build.ReadExtraction(opt.Out)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(x.Records, func(r lean.Record) bool { return r.Decl == "Math2.Match.lagrange" })
	if i < 0 {
		t.Fatal("the extraction has no record of Math2.Match.lagrange")
	}
	nodes, err := node.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	n := nodes[slices.IndexFunc(nodes, func(n *node.Node) bool { return n.ID == "3C53TD" })]
	review := fmt.Sprintf("reviews:\n  - id: r-3C53TD-01\n    reviewer: github:octocat\n    date: 2026-10-25\n    decl: Math2.Match.lagrange\n    statement_hash: %s\n    type_hash: %s\n    verdict: faithful\n", status.StatementHash(n), x.Records[i].TypeHash)
	reg, err := os.OpenFile(filepath.Join(root, "registers", "reviewers.reg"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reg.WriteString("github:octocat 2026-10-25 20D -\n"); err != nil {
		t.Fatal(err)
	}
	if err := reg.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "reviews", "3C"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "reviews", "3C", "3C53TD.yaml"), []byte(review), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := build.Run(ctx, opt)
	if err != nil {
		t.Fatal(err)
	}
	if m.Counts.Replays != 1 {
		t.Errorf("%d declarations need a replay, and one must", m.Counts.Replays)
	}
	rs, err := build.ReadReplays(opt.Out)
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || !rs[0].Passed || len(rs[0].Kernels) < 2 {
		t.Fatalf("the replays are %+v", rs)
	}
	st, err := build.ReadStatus(opt.Out)
	if err != nil {
		t.Fatal(err)
	}
	s := st[slices.IndexFunc(st, func(s status.Status) bool { return s.Tag == "3C53TD" })]
	b := s.Bindings[0]
	if b.Fidelity != status.F2 || b.Replay == nil {
		t.Fatalf("the binding of 3C53TD is %+v", b)
	}
	if runtime.GOOS == "linux" {
		if b.Level != status.Kernel || len(b.Replay.Checkers) < 2 {
			t.Errorf("the binding of 3C53TD is %+v, and it must be at E5", b)
		}
	} else if b.Level != status.None || !strings.Contains(b.Reason, "sandbox") {
		t.Errorf("the binding of 3C53TD is %+v, and without the sandbox it must not be at E5", b)
	}
}

type logger struct{ t *testing.T }

func (l logger) Write(p []byte) (int, error) {
	l.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}
