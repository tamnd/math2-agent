package explain_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/explain"
)

// The grounding check labels each sentence of an explanation. A full stop in math or after e.g. does not end a sentence.
func ExampleSentences() {
	text := "A [[group|G5S1M9]] is a set with an operation, e.g. addition.\nThe order is $|G|$. Is it finite? Not always."
	for _, s := range explain.Sentences(text) {
		fmt.Println(s)
	}
	// Output:
	// A [[group|G5S1M9]] is a set with an operation, e.g. addition.
	// The order is $|G|$.
	// Is it finite?
	// Not always.
}
