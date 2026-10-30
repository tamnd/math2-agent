package resource_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/resource"
)

func ExampleCheck() {
	r := resource.Resource{ID: "8F2K1QW7ZB", Kind: "book", Title: "Principles of Mathematical Analysis"}
	for _, p := range resource.Check(&r) {
		fmt.Println(p)
	}
	// Output:
	// kind: "book" is not a kind of RS2.3
	// catalogue: a resource needs at least one record of a catalogue
	// history: a resource needs at least the event that added it
}
