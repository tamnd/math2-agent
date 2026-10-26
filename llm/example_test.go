package llm_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/llm"
)

func ExampleCost() {
	u := llm.Usage{Input: 2_000, Output: 8_000, CacheRead: 40_000}
	usd, _ := llm.Cost(llm.Prices, "claude-sonnet-5-5", u)
	fmt.Printf("$%.4f\n", usd)
	// Output: $0.0920
}
