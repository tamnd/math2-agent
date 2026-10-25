package arxiv_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/ingest/arxiv"
)

func ExampleMSC() {
	fmt.Println(arxiv.MSC("11N05, 11N36 (Primary) 11P32; 11-02"))
	// Output: [11N05 11N36 11P32 11-02]
}

func ExampleLicence() {
	fmt.Println(arxiv.Licence("cc-by-sa"), arxiv.Licence("arxiv-1.0"))
	// Output: CC-BY-SA-4.0 LicenseRef-arXiv
}
