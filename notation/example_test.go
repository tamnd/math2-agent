package notation_test

import (
	"fmt"
	"log"

	"github.com/tamnd/math2-agent/notation"
)

func ExampleLoad() {
	r, err := notation.Load("testdata/tree")
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range r.Entries {
		fmt.Println(e.ID(), e.Denotes, e.Scopes())
	}
	// Output:
	// global.R 08J2M1JYAG [global]
	// global.ip 0NB0AWGJBW [global]
	// geometry.goldenratio 3C53TD7K3Q [geometry number-theory]
	// number-theory.totient 2HQ7XKM4CP [number-theory]
}

// The seed leaves denotes and reads empty for a curator to fill in.
func ExampleSeed() {
	entries, err := notation.Seed([]byte("\\newcommand{\\abs}[1]{\\left|#1\\right|}\n"))
	if err != nil {
		log.Fatal(err)
	}
	data, err := notation.Marshal(entries)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(string(data))
	// Output:
	// - macro: \abs
	//   denotes: ""
	//   args: 1
	//   latex: \left|#1\right|
	//   reads: ""
}
