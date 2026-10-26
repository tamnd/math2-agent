package dataset_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/dataset"
)

func ExampleScrub() {
	fmt.Println(dataset.Scrub("the build of /home/ann/math2 failed, write to ann@example.org"))
	// Output: the build of ~/math2 failed, write to [email]
}
