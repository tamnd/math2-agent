package format_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/format"
)

func ExampleFormatter_Source() {
	data := []byte("---\nname: Subgroup\ntag: X7J0XA\nkind: concept\nuses:\n  - group\n---\n\n## Summary\n\nA subgroup is a subset of a [[group|group]] that is a group under the same operation.\n")
	f := format.Formatter{Aliases: format.Aliases{"group": "DSG7KN"}}
	out, err := f.Source("nodes/X7/X7J0XA.md", data)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(string(out))
	// Output:
	// ---
	// tag: X7J0XA
	// kind: concept
	// name: Subgroup
	// uses: [DSG7KN]
	// ---
	//
	// ## Summary
	//
	// A subgroup is a subset of a [[group|DSG7KN]] that is a group under the same operation.
}
