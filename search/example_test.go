package search_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/search"
)

// index writes the index of the example nodes in a new directory and opens it.
func index() (*search.Index, func()) {
	nodes, err := node.Load(filepath.Join("..", "testdata", "nodes", "valid"))
	if err != nil {
		log.Fatal(err)
	}
	dir, err := os.MkdirTemp("", "m2-example")
	if err != nil {
		log.Fatal(err)
	}
	path := filepath.Join(dir, "search.db")
	if err := search.Write(path, search.Input{Nodes: nodes, Records: records}); err != nil {
		log.Fatal(err)
	}
	ix, err := search.Open(path, nil)
	if err != nil {
		log.Fatal(err)
	}
	return ix, func() { _ = ix.Close(); _ = os.RemoveAll(dir) }
}

func ExampleIndex_Search() {
	ix, done := index()
	defer done()
	rs, err := ix.Search(search.Query{Text: "the size of a subgroup divides the size of a finite group", K: 3})
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range rs {
		fmt.Println(r.Tag, r.Kind, r.Name)
	}
	// Output:
	// Q529ES statement Lagrange's theorem
	// ETCDHB conjecture The converse of Lagrange's theorem
	// CM1BRG statement Cauchy's theorem for groups
}

// The identifier resolver finds a node by its tag, an alias, a decl or an external identifier, here the Wikidata item of Lagrange's theorem.
func ExampleIndex_Search_identifier() {
	ix, done := index()
	defer done()
	rs, err := ix.Search(search.Query{Text: "Q180967", K: 1})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(rs[0].Tag, rs[0].Channels[0].Channel)
	// Output:
	// Q529ES identifier
}

// The formula channel matches the structure of a formula. The names of the variables are free, thus |Y| = |H|^{q-1} finds |X| = |G|^{p-1}.
func ExampleIndex_Search_formula() {
	ix, done := index()
	defer done()
	rs, err := ix.Search(search.Query{Text: `|Y| = |H|^{q-1}`, K: 1, Channels: []string{search.Formula}})
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range rs {
		fmt.Println(r.Tag, r.Name)
	}
	// Output:
	// VTPHZM McKay's proof of Cauchy's theorem
}

// The formal channel reads a Lean type pattern and matches the heads of the conclusion and the hypotheses.
func ExampleIndex_Search_formal() {
	ix, done := index()
	defer done()
	rs, err := ix.Search(search.Query{Text: "Finite _ → Nat.card _ = _ → IsCyclic _", K: 1, Channels: []string{search.Formal}})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(rs[0].Decl)
	// Output:
	// isCyclic_of_prime_card
}

// The proof of isCyclic_of_prime_card, a declaration near the goal, uses Subgroup.card_subgroup_dvd_card. Thus the dependency channel ranks it first.
func ExampleIndex_Premises() {
	ix, done := index()
	defer done()
	ps, err := ix.Premises(search.Goal{Goal: "G : Type\ninst : Group G\n⊢ IsCyclic G", K: 2})
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range ps {
		fmt.Println(p.Decl)
	}
	// Output:
	// Subgroup.card_subgroup_dvd_card
	// isCyclic_of_prime_card
}
