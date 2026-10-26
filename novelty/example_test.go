package novelty_test

import (
	"context"
	"fmt"

	"github.com/tamnd/math2-agent/novelty"
)

// shelf is a search of a small list of results.
type shelf []novelty.Match

func (s shelf) Name() string { return "shelf" }

func (s shelf) Search(context.Context, novelty.Claim) (string, []novelty.Match, error) {
	return "the query", s, nil
}

func ExampleCheck() {
	claim := novelty.Claim{Name: "Infinitude of the primes", Statement: "For each natural number $N$ there is a prime $p > N$."}
	found := shelf{{ID: "euclid", Title: "There are more primes than any given number of primes"}}
	// A judge compares the claim with each match. Here it says that the first match states the claim.
	judge := func(_ context.Context, _ novelty.Claim, _ novelty.Match) (string, string, error) {
		return novelty.Same, "the match states the claim in other words", nil
	}
	r, err := novelty.Check(context.Background(), claim, judge, found)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(r.Verdict, r.Match.Source, r.Match.ID)
	// Output: known shelf euclid
}
