package node_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/node"
	"github.com/tamnd/math2-agent/schema"
)

func ExampleParse() {
	data := []byte("---\ntag: 4F2M9A\nkind: topic\nname: Finite groups\nmsc: [20Dxx]\n---\n\n## Summary\n\nGroups with a finite number of elements.\n")
	n, err := node.Parse("nodes/4F/4F2M9A.md", data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(n.ID, n.Kind(), n.Name(), n.List("msc"), n.BodyLine)
	// Output: 4F2M9A topic Finite groups [20Dxx] 7
}

func ExampleParse_error() {
	data := []byte("---\ntag: 4F2M9A\nkind: topic\nname: Finite groups: a\n---\n")
	_, err := node.Parse("nodes/4F/4F2M9A.md", data)
	fmt.Println(err)
	// Output: nodes/4F/4F2M9A.md:4:1: the front matter is not valid YAML: mapping values are not allowed in this context
}

func ExampleFormat() {
	data := []byte("---\nname: Finite groups\nmsc:\n  - '20Dxx'\nkind: topic\ntag: 4F2M9A\n---\n## Summary\r\n\r\nGroups with a finite number of elements.\r\n")
	n, err := node.Parse("nodes/4F/4F2M9A.md", data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(string(node.Format(n, schema.Embedded().Order)))
	// Output:
	// ---
	// tag: 4F2M9A
	// kind: topic
	// name: Finite groups
	// msc: [20Dxx]
	// ---
	//
	// ## Summary
	//
	// Groups with a finite number of elements.
}

func ExampleHash() {
	a, _ := node.Parse("nodes/4F/4F2M9A.md", []byte("---\ntag: 4F2M9A\nkind: topic\nname: Finite groups\n---\n"))
	b, _ := node.Parse("nodes/4F/4F2M9A.md", []byte("---\nname: Finite groups # the key order and the comments do not change the hash\nkind: topic\ntag: 4F2M9A\n---\n"))
	fmt.Println(node.Hash(a) == node.Hash(b))
	// Output: true
}
