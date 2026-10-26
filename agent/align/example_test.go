package align_test

import (
	"fmt"
	"strings"

	"github.com/tamnd/math2-agent/agent/align"
	"github.com/tamnd/math2-agent/workspace"
)

func ExampleBindings() {
	old := "---\ntag: 3C53TD\nkind: statement\nrole: theorem\nname: Lagrange's theorem\nformal:\n  - system: lean4\n    library: math2\n    decl: Math2.Algebra.lagrange\n    match: exact\n---\n\n## Statement\n\nThe order of a subgroup of a finite group divides the order of the group.\n"
	cur := strings.Replace(old, "    match: exact\n", "    match: exact\n  - system: lean4\n    library: mathlib\n    decl: Subgroup.card_subgroup_dvd_card\n    match: exact\n", 1)
	bs, err := align.Bindings(workspace.Change{Path: "nodes/3C5/3C53TD.md", Old: []byte(old), New: []byte(cur)})
	if err != nil {
		panic(err)
	}
	for _, b := range bs {
		fmt.Println(b["decl"], b["match"])
	}
	// Output: Subgroup.card_subgroup_dvd_card exact
}
