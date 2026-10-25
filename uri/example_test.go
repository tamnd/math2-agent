package uri_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/uri"
)

func ExampleParse() {
	u, err := uri.Parse("m2://7K3QX2@2026.11#proof")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(u.Tag, u.Release, u.Section)
	fmt.Println(u)
	// Output:
	// 7K3QX2 2026.11 proof
	// m2://7K3QX2@2026.11#proof
}

func ExampleParse_alias() {
	u, err := uri.Parse("m2://lean/Subgroup.card_subgroup_dvd_card")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(u.IsAlias(), u.Namespace, u.Name)
	// Output: true lean Subgroup.card_subgroup_dvd_card
}
