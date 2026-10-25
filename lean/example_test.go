package lean_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/tamnd/math2-agent/lean"
)

// This example builds M2Extract for the Lean package of a math2 checkout and prints the type hash and the axioms of one Mathlib theorem.
func ExampleSession() {
	ctx := context.Background()
	project := "math2/lean"
	exe, err := lean.Install(ctx, project, filepath.Join(os.TempDir(), "m2"))
	if err != nil {
		log.Fatal(err)
	}
	s, err := lean.Start(ctx, exe, project)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if err := s.Import("Mathlib"); err != nil {
		log.Fatal(err)
	}
	rs, err := s.Extract([]string{"Subgroup.card_subgroup_dvd_card"}, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(rs[0].TypeHash, rs[0].Axioms)
}

// The canonical form "s 0" is the type Prop.
func ExampleHash() {
	fmt.Println(lean.Hash("s 0\n"))
	// Output: blake3:69efb98094d6d6e580c6b9420bf02eb6b73928ed03b0f27ab3052d520a3789d5
}
