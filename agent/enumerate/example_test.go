package enumerate_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/enumerate"
)

// Two names of one item have the same key, thus the tool list_items refuses the second one.
func ExampleKey() {
	fmt.Println(enumerate.Key("Heine–Borel theorem"))
	fmt.Println(enumerate.Key("Heine-Borel Theorem"))
	// Output:
	// heineboreltheorem
	// heineboreltheorem
}

// The tool list_items keeps an ISBN only when its check digit is correct.
func ExampleISBN() {
	fmt.Println(enumerate.ISBN("978-0-07-054235-8"))
	fmt.Println(enumerate.ISBN("978-0-07-054235-7"))
	// Output:
	// true
	// false
}
