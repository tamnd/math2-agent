package battery_test

import (
	"context"
	"fmt"

	"github.com/tamnd/math2-agent/battery"
	"github.com/tamnd/math2-agent/lean"
	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/replay"
)

func ExampleRun() {
	src := "---\ntag: AB12CD\nkind: statement\nname: Natural subtraction\nformal:\n  - {system: lean4, decl: Sub.self, match: exact}\n---\n\n## Statement\n\nFor each natural number n, n - n = 0.\n"
	n, err := node.Parse("nodes/AB/AB12CD.md", []byte(src))
	if err != nil {
		fmt.Println(err)
		return
	}
	in := battery.Input{
		Nodes: []*node.Node{n},
		Records: map[string]lean.Record{"Sub.self": {
			Decl:                     "Sub.self",
			Kind:                     "theorem",
			StatementPPAll:           "∀ (n : Nat), @Eq.{1} Nat (@HSub.hSub.{0, 0, 0} Nat Nat Nat (@instHSub.{0} Nat instSubNat) n n) (@OfNat.ofNat.{0} Nat 0 (instOfNatNat 0))",
			UsesConstantsInStatement: []string{"Eq", "HSub.hSub", "instHSub", "instSubNat", "OfNat.ofNat", "instOfNatNat"},
			Checks:                   &lean.Checks{},
		}},
		Replays: map[string]replay.Result{"Sub.self": {Decl: "Sub.self", Passed: true}},
	}
	reports, err := battery.Run(context.Background(), in, battery.Options{})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, c := range reports[0].Checks {
		line := fmt.Sprintf("%d %s: %s", c.Number, c.Name, c.Result)
		if c.Class != "" {
			line += " " + c.Class
		}
		fmt.Println(line)
	}
	fmt.Println("F1:", reports[0].F1)
	// Output:
	// 1 elaborates: pass
	// 2 back-translation: not-run
	// 3 constant audit: flag M-junk
	// 4 hypotheses satisfiable: pass
	// 5 conclusion not trivial: pass
	// 6 negation not provable: pass
	// 7 examples and counterexamples: skip
	// 8 neighbour consistency: skip
	// 9 critic: not-run
	// 10 pinned-statement compile: pass
	// F1: false
}

func ExampleF1() {
	checks := make([]battery.Check, 10)
	for i := range checks {
		checks[i] = battery.Check{Number: i + 1, Result: battery.Pass}
	}
	fmt.Println(battery.F1(checks))
	checks[2].Result = battery.Open
	fmt.Println(battery.F1(checks))
	// Output:
	// true
	// false
}
