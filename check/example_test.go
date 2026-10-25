package check_test

import (
	"fmt"
	"os"
	"strings"

	"github.com/tamnd/math2-agent/check"
	"github.com/tamnd/math2-agent/node"
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

func ExampleLinks() {
	n, err := node.Parse("nodes/3C/3C53TD.md", []byte("---\ntag: 3C53TD\nkind: statement\n---\n\n## Statement\n\nLet $G$ be a finite [[group|G5S1M9]]. The text `[[a|B]]` is code.\n"))
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, l := range check.Links(n) {
		fmt.Printf("%d:%d %s %q %s\n", l.Line, l.Col, l.Section, l.Text, l.Target)
	}
	// Output:
	// 8:21 Statement "group" G5S1M9
}

func ExampleSections() {
	src := "---\ntag: 3C53TD\nkind: statement\n---\n\n## Statement\n\nThe order of $H$ divides the order of $G$.\n\n## Explanations\n\n### L2 human\n\nCount the cosets.\n\n```\n## Not a heading\n```\n"
	n, err := node.Parse("nodes/3C/3C53TD.md", []byte(src))
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, s := range check.Sections(n) {
		fmt.Printf("%d %s: %d lines\n", s.Line, s.Name, strings.Count(s.Text, "\n")+1)
		for _, h := range s.Headings {
			fmt.Printf("  %d %d %s\n", h.Line, h.Level, h.Name)
		}
	}
	// Output:
	// 6 Statement: 3 lines
	// 10 Explanations: 8 lines
	//   12 3 L2 human
}
