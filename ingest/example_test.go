package ingest_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/ingest"
)

func ExampleClassOf() {
	for _, licence := range []string{"CC-BY-SA-4.0", "CC-BY-NC-SA-4.0", "GFDL-1.3-or-later", "LicenseRef-arXiv"} {
		c := ingest.ClassOf(licence)
		fmt.Printf("%s: %s, node prose %v\n", licence, c, c.Prose())
	}
	// Output:
	// CC-BY-SA-4.0: open, node prose true
	// CC-BY-NC-SA-4.0: share-alike-nc, node prose false
	// GFDL-1.3-or-later: gfdl, node prose false
	// LicenseRef-arXiv: record, node prose false
}

func ExampleGate() {
	stacks := &ingest.Source{ID: "stacks", Name: "The Stacks Project", Licence: "GFDL-1.3-or-later", Class: ingest.GFDL}
	r := ingest.Gate(stacks, ingest.Record{ID: "01H8", Title: "Lemma 26.4.2", Text: "Let X be a scheme. The underlying topological space of X is sober."})
	fmt.Printf("%s %s: text %q, %d fingerprints\n", r.ID, r.Class, r.Text, len(r.Fingerprints))
	// Output:
	// 01H8 gfdl: text "", 3 fingerprints
}

func ExampleShared() {
	book := ingest.Fingerprints("We prove that every finite group of prime order is cyclic, and each element other than the identity generates it.")
	node := ingest.Fingerprints("Every finite group of prime order is cyclic and each element other than the identity generates it.")
	other := ingest.Fingerprints("A ring is a set with two operations that satisfy the usual laws of arithmetic.")
	fmt.Println(ingest.Shared(node, book) > 0, ingest.Shared(node, other))
	// Output:
	// true 0
}
