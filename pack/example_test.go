package pack_test

import (
	"fmt"
	"log"

	"github.com/tamnd/math2-agent/pack"
)

// A volume of notes on Lagrange's theorem. Pack takes the nodes that the theorem needs, puts each after its prerequisites, and numbers them.
func Example() {
	v, err := pack.Parse("volumes/lagrange.yaml", []byte(`id: lagrange
title: Lagrange's theorem
editors: [github:ada]
kind: book
targets: [Q529ES]
choose: {DSG7KN: GY1XDB}
`))
	if err != nil {
		log.Fatal(err)
	}
	d, err := pack.Pack("../testdata/nodes/valid", v, pack.Options{})
	if err != nil {
		log.Fatal(err)
	}
	for _, c := range d.Chapters {
		fmt.Printf("%d. %s\n", c.Number, c.Title)
		for _, s := range c.Sections {
			for _, it := range s.Items {
				fmt.Printf("   %s %s (%s)\n", it.Label, it.Number, it.Name)
			}
		}
	}
	fmt.Println("failed:", d.Check().Failed())
	// Output:
	// 1. Group theory
	//    Definition 1.1 (Group (definition))
	//    Definition 1.2 (Subgroup (definition))
	//    Definition 1.3 (Coset (definition))
	//    Claim 1.4 (Cosets partition a group)
	// 2. Finite groups
	//    Definition 2.1 (Order of a group (definition))
	//    Claim 2.2 (Lagrange's theorem)
	// failed: false
}
