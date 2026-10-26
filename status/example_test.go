package status_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/status"
)

func ExampleCompute() {
	src := `---
tag: 1A0000
kind: statement
name: An example
sources:
  - book: isbn:9780471433347
    locator: Section 3.2
  - book: isbn:9780387953854
    locator: Theorem 1.2
---

## Statement

Each finite group has a finite number of subgroups.
`
	n, err := node.Parse("nodes/1A0/1A0000.md", []byte(src))
	if err != nil {
		panic(err)
	}
	for _, s := range status.Compute(status.Input{Nodes: []*node.Node{n}}) {
		fmt.Println(s.Tag, s.Evidence, s.Evidence.Word(), s.Flags)
		fmt.Println(s.Reasons[len(s.Reasons)-1])
	}
	// Output:
	// 1A0000 E2 standard [orphan]
	// 2 textbook sources have locators, so the level is E2.
}
