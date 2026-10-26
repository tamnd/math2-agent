package curate_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/curate"
	"github.com/tamnd/math2-agent/node"
)

// The target of a curate job is a record of the form source:id, or a node.
func ExampleIsRecord() {
	fmt.Println(curate.IsRecord("arxiv_results:2312.00752#3KH2"))
	fmt.Println(curate.IsRecord("3C53TD"))
	// Output:
	// true
	// false
}

// A new node of a record names the locator of the record in its sources.
func ExampleLocates() {
	text := "---\ntag: ~1a2b3c4d\nkind: statement\nrole: theorem\nname: Groups of exponent two\nsources:\n  - arxiv: 2312.00752v1\n    locator: ax://paper/2312.00752#3KH2\n---\n\n## Statement\n\nA finite group in which each element is its own inverse is commutative.\n"
	n, err := node.Parse("nodes/~/~1a2b3c4d.md", []byte(text))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(curate.Locates(n, "ax://paper/2312.00752#3KH2"))
	// Output:
	// true
}
