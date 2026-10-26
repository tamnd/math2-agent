package write_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/write"
)

// A proof of math2 is a list of numbered steps (corpus K8.5).
func ExampleSteps() {
	fmt.Println(write.Steps("1. The cosets of $H$ partition $G$.\n2. Thus $|H|$ divides $|G|$, by step 1.\n"))
	fmt.Println(write.Steps("The cosets of $H$ partition $G$, and the claim follows.\n"))
	// Output:
	// true
	// false
}
