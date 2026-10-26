package workspace_test

import (
	"context"
	"fmt"
	"log"

	"github.com/tamnd/math2-agent/workspace"
)

// This example makes a work tree of a math2 checkout for one run, writes a Lean file in it, and prints the changes of the run.
func ExampleAdd() {
	ctx := context.Background()
	w, err := workspace.Add(ctx, "math2", "/tmp/m2-run", "origin/main")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = w.Remove(ctx) }()
	if err := w.Write("lean/Math2/Algebra/Lagrange.lean", []byte("module\n")); err != nil {
		log.Fatal(err)
	}
	changes, err := w.Changes(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, c := range changes {
		fmt.Println(c.Path, c.Old == nil)
	}
}
