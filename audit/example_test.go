package audit_test

import (
	"fmt"
	"os"

	"github.com/tamnd/math2-agent/audit"
)

func ExampleRun() {
	// In this tree, registers/tags.reg has no record of a node and gives the wrong kind for another node.
	rep, err := audit.Run("testdata/T05/bad", audit.Options{Rules: []string{"T05"}})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, f := range rep.Findings {
		fmt.Println(f)
	}
	_ = rep.WriteSummary(os.Stdout)
	fmt.Println("failed:", rep.Failed())
	// Output:
	// nodes/3C5/3C53TD.md:1:1: T05: the tag 3C53TD is not in registers/tags.reg. Only the mint workflow gives tags
	// nodes/3C5/3C53TD.md:6:1: T05: the alias lagrange-theorem names G5S1M9 in registers/aliases.reg, and an alias names one node
	// nodes/G5S/G5S1M9.md:1:1: T05: the kind is concept, and registers/tags.reg gives definition
	// nodes/G5S/G5S1M9.md:5:1: T05: the alias abstract-group names 3C53TD in nodes/3C5/3C53TD.md, and an alias names one node
	// rule  state  checked  findings
	// T05   fail   6        4
	// 2 files, 4 findings, 1 rule failed
	// failed: true
}

func ExampleRules() {
	for _, r := range audit.Rules() {
		if r.Plane == audit.Release {
			fmt.Println(r.ID, r.Hard, r.Text)
		}
	}
	// Output:
	// R01 true the derived graph rebuilds to the same hash
	// R02 true the .olean rebuild matches, or the list of differences explains each one
	// R03 true the release notes give the cause of each count change
	// R04 true the last quarterly restore drill passed
	// R05 true the release pins one Lean toolchain and one Mathlib commit, and the bump branch passed
}
