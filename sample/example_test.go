package sample_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/sample"
)

func ExampleUpper() {
	// With no errors in 300 samples, the rate is under 1%. With 1 error in 100, the upper end is above 5%.
	fmt.Printf("%.4f %.4f\n", sample.Upper(0, 300), sample.Upper(1, 100))
	// Output: 0.0100 0.0545
}
