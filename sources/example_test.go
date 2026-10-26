package sources_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/sources"
)

func ExampleConfirm() {
	// The record is made up for the example.
	p := &sources.Plane{}
	p.Add("zbmath", ingest.Record{ID: "9999.00001", Title: "On the orders of the subgroups of a finite group", Meta: map[string]string{"authors": "Tester, Ada", "year": "1901"}})
	for _, c := range []sources.Claim{
		{System: "zbmath", ID: "9999.00001", Title: "On the orders of the subgroups of a finite group", Authors: []string{"A. Tester"}, Year: "1901"},
		{System: "zbmath", ID: "9999.00001", Year: "1910"},
		{System: "zbmath", ID: "9999.00002"},
	} {
		r, _ := sources.Confirm(p, c)
		fmt.Println(c.ID, r.Verdict, r.Problems)
	}
	// Output:
	// 9999.00001 confirmed []
	// 9999.00001 mismatch [the year is 1901 in the record]
	// 9999.00002 fabricated [no record of zbmath 9999.00002]
}
