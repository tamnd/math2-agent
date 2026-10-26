package reviewassist_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/reviewassist"
)

func ExampleParseFile() {
	f := &reviewassist.File{Tag: "3C53TD", By: "m2-agent/review-assist@0.1.0", Packet: "3C53TD Lagrange's theorem\n", Concerns: []reviewassist.Concern{
		{Decl: "Math2.Algebra.lagrange", Class: "M-junk", Text: "Nat.card G is 0 when G is infinite."},
	}}
	g, err := reviewassist.ParseFile(f.Markdown())
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(reviewassist.Path(g.Tag))
	for _, c := range g.Concerns {
		fmt.Printf("%s %s: %s\n", c.Decl, c.Class, c.Text)
	}
	// Output:
	// reports/review/3C53TD.md
	// Math2.Algebra.lagrange M-junk: Nat.card G is 0 when G is infinite.
}
