package zbmath_test

import (
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/ingest/zbmath"
)

func ExampleCount() {
	fmt.Println(zbmath.CountURL("11A"))
	n, err := zbmath.Count(strings.NewReader(`{"result": [], "status": {"nr_total_results": 4213, "status_code": 200}}`))
	fmt.Println(n, err)
	// Output:
	// https://api.zbmath.org/v1/document/_search?page=0&results_per_page=1&search_string=cc%3A11A
	// 4213 <nil>
}
