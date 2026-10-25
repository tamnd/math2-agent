package coverage_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/coverage"
	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/msc"
)

func ExampleCompute() {
	m, err := msc.Parse([]byte(`
"11-XX": {label: number theory}
"11Axx": {label: elementary number theory}
"11A41": {label: prime numbers}
`))
	if err != nil {
		fmt.Println(err)
		return
	}
	rows := coverage.Compute(coverage.Input{
		Map:      m,
		Records:  map[string][]ingest.Record{"zbmath": {{ID: "1", Meta: map[string]string{"msc": "11A41"}}}},
		Expected: &coverage.Expected{Source: "zbmath", Date: "2026-10-26", Counts: map[string]int{"11-XX": 4}},
	})
	for _, r := range rows {
		c0 := "none"
		if r.C0 != nil {
			c0 = fmt.Sprintf("%.0f%%", *r.C0*100)
		}
		fmt.Println(r.Code, r.Records, c0)
	}
	// Output:
	// 11-XX 1 25%
	// 11Axx 1 none
	// 11A41 1 none
}
