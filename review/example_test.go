package review_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/review"
)

func ExampleFile() {
	fmt.Println(review.File("3C53TD"))
	// Output: reviews/3C/3C53TD.yaml
}

func ExamplePerson() {
	for _, id := range []string{"github:octocat", "orcid:0000-0002-1825-0097", "github:dependabot[bot]", "m2-agent/review@1.0.0"} {
		fmt.Println(id, review.Person(id))
	}
	// Output:
	// github:octocat true
	// orcid:0000-0002-1825-0097 true
	// github:dependabot[bot] false
	// m2-agent/review@1.0.0 false
}
