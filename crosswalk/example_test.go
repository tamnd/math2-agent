package crosswalk_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/crosswalk"
)

func ExampleCheck() {
	fmt.Println(crosswalk.Check("oeis", "A000045"))
	fmt.Println(crosswalk.Check("oeis", "A45"))
	fmt.Println(crosswalk.Check("lmfdb", "EllipticCurve/Q/11.a2"))
	// Output:
	// <nil>
	// the oeis id "A45" is not an A-number such as A000045
	// <nil>
}
