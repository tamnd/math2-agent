package copycheck_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/copycheck"
	"github.com/tamnd/math2-agent/ingest"
)

func ExampleCorpus_Check() {
	ref := "A field is a commutative ring in which every nonzero element has a multiplicative inverse, and in which the zero element differs from the unit element."
	c := copycheck.New([]copycheck.Text{{Source: "book", URI: "book:1", Class: ingest.RecordOnly, Text: ref}}, nil)
	body := "Recall that a field is a commutative ring in which every nonzero element has a multiplicative inverse, and in which $0 \\ne 1$."
	r := c.Check(copycheck.Section{Field: "body.statement", Text: body})
	for _, m := range r.Matches() {
		fmt.Printf("%s %d-%d stage %d: %s (%s)\n", m.Field, m.Start, m.End, m.Stage, m.URI, m.Class)
	}
	fmt.Println("blocks:", r.Blocks())
	// Output:
	// body.statement 12-116 stage 1: book:1 (record)
	// blocks: true
}
