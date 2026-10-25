package audit

import (
	"github.com/tamnd/math2-agent/check"
)

// checkRules are the rules of m2 check that the audit takes from one run of a check.Checker.
var checkRules = []string{"S01", "S02", "S03", "S04"}

// fromCheck returns the function of the rule id. The function takes the result of id from a run of m2 check on the node files in scope (rules S01 to S04). The links resolve against every node in the tree.
func fromCheck(id string) func(*auditor) outcome {
	return func(a *auditor) outcome {
		if a.checkRun == nil {
			c := check.Checker{Schema: a.schema, Only: checkRules}
			a.checkRun = c.Check(a.scope, a.ix)
		}
		var o outcome
		for _, res := range a.checkRun.Results {
			if res.Rule == id {
				o.checked = res.Checked
			}
		}
		for _, f := range a.checkRun.Findings {
			if f.Rule == id {
				o.findings = append(o.findings, f)
			}
		}
		return o
	}
}
