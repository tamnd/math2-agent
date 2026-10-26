package propose_test

import (
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/propose"
)

func ExampleNewNode() {
	text := "---\nkind: concept\nname: Monoid\nmsc: [20M10]\n---\n\n## Summary\n\nA monoid is a set with an associative binary operation and an identity element for it.\n"
	c, err := propose.NewNode("testdata/tree", []byte(text))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(c.Method)
	fmt.Println(c.Title)
	// The node gets a new provisional id, which is random.
	for path := range c.Files {
		fmt.Println(strings.Replace(path, c.Tag, "~ID", 1))
	}
	// Output:
	// propose_node
	// Add the concept Monoid
	// nodes/~/~ID.md
}

func ExampleLimit_Cap() {
	l := propose.Limit{Multiple: 2, Floor: 10}
	// People closed 12 pull requests of a method in four weeks, 3 each week. The method can have 10 in the next seven days, because that is the floor.
	fmt.Println(l.Cap(12))
	// With 40 in four weeks, it can have 2 × 10 = 20.
	fmt.Println(l.Cap(40))
	// Output:
	// 10
	// 20
}

func ExampleMethod() {
	fmt.Println(propose.Method("m2-api/propose_binding/3C53TD-9f2c01aa"))
	fmt.Println(propose.Method("fix-a-typo") == "")
	// Output:
	// propose_binding
	// true
}
