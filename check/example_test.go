package check_test

import (
	"fmt"
	"os"

	"github.com/tamnd/math2-agent/check"
)

func ExampleChecker_Check() {
	data := []byte(`---
tag: 3EYFQZ
kind: topic
name: Elementary number theory
msc: [11Axx]
topics: [W96C20]
---

## Summary

Divisibility in $\Z$ and the fraction $\frac{a}{b$ of two [[integers|DSG7KN]].
`)
	ix := check.Index{"DSG7KN": {}}
	var c check.Checker
	rep := c.Check([]check.File{{Path: "nodes/3E/3EYFQZ.md", Data: data}}, ix)
	for _, f := range rep.Findings {
		fmt.Println(f)
	}
	_ = rep.WriteSummary(os.Stdout)
	// Output:
	// nodes/3E/3EYFQZ.md:6:10: G01: the topics target W96C20 is not a node
	// nodes/3E/3EYFQZ.md:11:48: B01: a brace is not closed
	// rule  state  checked  findings
	// S01   pass   1
	// S02   pass   1
	// S03   pass   1
	// S04   pass   1
	// B01   fail   2        1
	// B02   pass   1
	// B03   pass   1
	// G01   fail   1        1
	// 1 file, 2 findings, 2 rules failed
}

func ExampleParseMacros() {
	m, err := check.ParseMacros([]byte(`{"\\ip": "\\left\\langle #1, #2\\right\\rangle", "\\Z": "\\mathbb{Z}"}`))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(m["ip"], m["Z"])
	// Output: 2 0
}
