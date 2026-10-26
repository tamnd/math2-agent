package exercise_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/exercise"
)

func ExampleAgree() {
	fmt.Println(exercise.Agree("exact", "$12$", "12."), exercise.Agree("choice", "(b)", "(c)"))
	// Output: true false
}
