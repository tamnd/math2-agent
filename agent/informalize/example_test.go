package informalize_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/agent/informalize"
)

func ExampleDecl_LeanName() {
	d := &informalize.Decl{Name: "Subgroup.card_subgroup_dvd_card"}
	fmt.Println(d.LeanName("card_subgroup_dvd_card"), d.LeanName("Lagrange's theorem"))
	// Output: true false
}
