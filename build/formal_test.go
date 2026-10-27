package build_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/status"
)

// TestFormal checks that each node of a formal kind gets a formal outcome, and that a record of registers/formal.reg makes the step run again.
func TestFormal(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(fixture)); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "build")
	if _, err := build.Run(ctx, build.Options{Root: root}); err != nil {
		t.Fatal(err)
	}
	f, err := build.ReadFormal(out)
	if err != nil {
		t.Fatal(err)
	}
	var open string
	for _, n := range f.Nodes {
		if n.Outcome == status.FormalNone && n.Kind == "statement" {
			open = n.Tag
			break
		}
	}
	if open == "" {
		t.Fatalf("the fixture has no statement with the outcome none: %+v", f.Outcomes)
	}

	reg := "# tag outcome date by author reason\n" + open + " out_of_reach 2026-10-27 - github:tamnd needs-a-large-library\n"
	if err := os.MkdirAll(filepath.Join(root, "registers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "registers", "formal.reg"), []byte(reg), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := build.Run(ctx, build.Options{Root: root, Only: build.Formal})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range m.Steps {
		if s.Name == build.Formal && s.Cached {
			t.Error("the formal step is cached after a change of registers/formal.reg")
		}
	}
	if f, err = build.ReadFormal(out); err != nil {
		t.Fatal(err)
	}
	for _, n := range f.Nodes {
		if n.Tag == open && (n.Outcome != status.FormalOutOfReach || n.Reason != "needs-a-large-library") {
			t.Errorf("%s has %+v after the decision", open, n)
		}
	}
}
