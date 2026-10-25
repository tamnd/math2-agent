package store_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/tamnd/math2-agent/build"
	"github.com/tamnd/math2-agent/graph"
	"github.com/tamnd/math2-agent/store"
)

// openExample builds the tree in testdata/tree and opens its store. The caller removes the directory.
func openExample() (*store.Store, string) {
	out, err := os.MkdirTemp("", "m2-store")
	if err != nil {
		log.Fatal(err)
	}
	if _, err := build.Run(context.Background(), build.Options{Root: "testdata/tree", Out: out}); err != nil {
		log.Fatal(err)
	}
	s, err := store.Open(filepath.Join(out, build.StoreFile))
	if err != nil {
		log.Fatal(err)
	}
	return s, out
}

func ExampleStore_Resolve() {
	s, dir := openExample()
	defer func() { _ = os.RemoveAll(dir) }()
	defer func() { _ = s.Close() }()
	for _, ref := range []string{"group", "m2://lean/Subgroup.card_subgroup_dvd_card", "m2://ext/wikidata/Q83478"} {
		tags, err := s.Resolve(ref)
		fmt.Println(ref, tags, err)
	}
	// Output:
	// group [G5S1M9] <nil>
	// m2://lean/Subgroup.card_subgroup_dvd_card [3C53TD] <nil>
	// m2://ext/wikidata/Q83478 [G5S1M9] <nil>
}

func ExampleStore_Neighbourhood() {
	s, dir := openExample()
	defer func() { _ = os.RemoveAll(dir) }()
	defer func() { _ = s.Close() }()
	nb, err := s.Neighbourhood("D2SB01", 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(nb.Nodes)
	for _, e := range nb.Edges {
		fmt.Println(e.From, e.Type, e.To)
	}
	// Output:
	// [3C53TD C05ET1 D1F0A1 D2SB01 G5SB02]
	// D2SB01 defines G5SB02
	// 3C53TD uses D2SB01
	// C05ET1 uses D2SB01
	// D2SB01 uses D1F0A1
}

func ExampleStore_Closure() {
	s, dir := openExample()
	defer func() { _ = os.RemoveAll(dir) }()
	defer func() { _ = s.Close() }()
	c, err := s.Closure("S1RH01", graph.MinDepth)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(c.Nodes, c.Evidence, c.Weakest, c.Hypotheses)
	// Output:
	// [H1RH01 S1RH01] E1 E0 [H1RH01]
}

func ExampleStore_Frontier() {
	s, dir := openExample()
	defer func() { _ = os.RemoveAll(dir) }()
	defer func() { _ = s.Close() }()
	es, err := s.Frontier(store.Filter{Open: true, MSC: "11P"})
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range es {
		fmt.Println(e.Tag, e.Kind, e.Evidence)
		for _, b := range e.Bounds {
			fmt.Println(" ", b.Side, b.Value, "by", b.Tag, "via", b.Via)
		}
	}
	// Output:
	// K7GD01 conjecture E0
	//   lower 4e18 by B1GD01 via P3GD01
}
