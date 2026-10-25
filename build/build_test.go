package build_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/build"
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
	if len(m.Steps) != 4 || m.Mathlib == "" || m.Checkers["leanchecker"] == "" {
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

type logger struct{ t *testing.T }

func (l logger) Write(p []byte) (int, error) {
	l.t.Log(strings.TrimSpace(string(p)))
	return len(p), nil
}
