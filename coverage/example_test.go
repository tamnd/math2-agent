package coverage_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/coverage"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/msc"
	"github.com/tamnd/math2-agent/node"
)

func ExampleCompute() {
	m, err := msc.Parse([]byte(`
"11-XX": {label: number theory}
"11Axx": {label: elementary number theory}
"11A41": {label: prime numbers}
`))
	if err != nil {
		fmt.Println(err)
		return
	}
	rows := coverage.Compute(coverage.Input{
		Map:      m,
		Records:  map[string][]ingest.Record{"zbmath": {{ID: "1", Meta: map[string]string{"msc": "11A41"}}}},
		Expected: &coverage.Expected{Source: "zbmath", Date: "2026-10-26", Counts: map[string]int{"11-XX": 4}},
	})
	for _, r := range rows {
		c0 := "none"
		if r.C0 != nil {
			c0 = fmt.Sprintf("%.0f%%", *r.C0*100)
		}
		fmt.Println(r.Code, r.Records, c0)
	}
	// Output:
	// 11-XX 1 25%
	// 11Axx 1 none
	// 11A41 1 none
}

func ExampleCompute_skeleton() {
	m, err := msc.Parse([]byte(`
"20-XX": {label: group theory and generalizations}
`))
	if err != nil {
		fmt.Println(err)
		return
	}
	n, err := node.Parse("nodes/G5/G5S1M9.md", []byte("---\ntag: G5S1M9\nkind: concept\nname: group\nmsc: [20-XX]\nsources:\n  - {book: 'isbn:9780471433347', locator: Section 1.1}\n---\n\n## Summary\n\nText.\n"))
	if err != nil {
		fmt.Println(err)
		return
	}
	rows := coverage.Compute(coverage.Input{
		Map:   m,
		Nodes: []*node.Node{n},
		Skeletons: []coverage.Skeleton{{Code: "20-XX", Items: []coverage.Item{
			{Name: "group", Kind: "concept", Node: "G5S1M9"},
			{Name: "Sylow theorems", Kind: "statement"},
		}}},
	})
	fmt.Printf("%s %d of %d, C2 %.0f%%\n", rows[0].Code, rows[0].Mapped, rows[0].Skeleton, *rows[0].C2*100)
	// Output:
	// 20-XX 1 of 2, C2 50%
}
