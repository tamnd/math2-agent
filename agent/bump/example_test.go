package bump_test

import (
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/agent/bump"
	"github.com/tamnd/math2-agent/workspace"
)

func ExampleRebinds() {
	old := "---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nformal:\n  - system: lean4\n    library: mathlib\n    decl: Subgroup.card_subgroup_dvd_card\n    match: exact\n---\n\n## Statement\n\nThe order of a subgroup of a finite group divides the order of the group.\n"
	cur := strings.Replace(old, "card_subgroup_dvd_card", "card_dvd_card", 1)
	rbs, err := bump.Rebinds("3C53TD", []workspace.Change{{Path: "nodes/3C/3C53TD.md", Old: []byte(old), New: []byte(cur)}})
	if err != nil {
		panic(err)
	}
	for _, rb := range rbs {
		fmt.Printf("formal[%d] %s -> %s\n", rb.Index, rb.Old, rb.New)
	}
	// Output: formal[0] Subgroup.card_subgroup_dvd_card -> Subgroup.card_dvd_card
}
