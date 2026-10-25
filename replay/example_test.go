package replay_test

import (
	"context"
	"fmt"
	"log"

	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/replay"
)

// TargetOf takes the declaration, the module, the universe parameters and the pp.all statement of a record.
func ExampleTargetOf() {
	r := lean.Record{Decl: "Fixture.refl", Module: "Fixture.Basic", LevelParams: []string{"u"}, StatementPPAll: "∀ (α : Sort u) (a : α), @Eq.{u} α a a"}
	t := replay.TargetOf(r)
	fmt.Println(t.Decl, t.Module, t.LevelParams)
	fmt.Println(t.Statement)
	// Output:
	// Fixture.refl Fixture.Basic [u]
	// ∀ (α : Sort u) (a : α), @Eq.{u} α a a
}

// The checker runs in a Lake project that has built the module of the declaration. The example does not run, because it needs lake.
func ExampleChecker_Check() {
	ctx := context.Background()
	c, err := replay.New(ctx, "testdata/fixture", replay.Options{Paranoid: true})
	if err != nil {
		log.Fatal(err)
	}
	res, err := c.Check(ctx, replay.Target{Decl: "Fixture.add_comm", Module: "Fixture.Basic", Statement: "∀ (a b : Nat), a + b = b + a"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.Passed, res.Kernels)
}
