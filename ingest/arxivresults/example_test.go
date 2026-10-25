package arxivresults_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/ingest/arxivresults"
)

func ExampleKind() {
	for _, o := range [][2]string{{"lemma", "statement"}, {"conjecture", "statement"}, {"satz", "statement"}, {"remark", "remark"}} {
		kind, role := arxivresults.Kind(o[0], o[1])
		fmt.Printf("%s: %q %q\n", o[0], kind, role)
	}
	// Output:
	// lemma: "statement" "lemma"
	// conjecture: "conjecture" ""
	// satz: "statement" ""
	// remark: "" ""
}
