package bump_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/bump"
)

func ExampleLatest() {
	tag, ok := bump.Latest([]string{"v4.34.0", "v4.35.0", "v4.36.0-rc1", "master"})
	fmt.Println(tag, ok)
	// Output: v4.35.0 true
}

func ExampleFailures() {
	out := "error: ././././Math2/Match.lean:3:0: unknown identifier 'Subgroup.card_dvd'\nerror: Lean exited with code 1"
	for _, b := range bump.Failures(out) {
		fmt.Println(b.Kind, b.File, b.Line, b.Text)
	}
	// Output: broken Math2/Match.lean 3 unknown identifier 'Subgroup.card_dvd'
}
