package models_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/models"
)

func ExampleRegister_Family() {
	r := models.Default()
	fmt.Println(r.Family("claude-opus-5-5"))
	fmt.Println(r.Check("gpt-0"))
	// Output:
	// claude
	// models: the model gpt-0 has no entry in the model register models/models.yaml (corpus K7.7)
}
