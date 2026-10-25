package msc_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/msc"
)

func ExampleParent() {
	for c := "11A41"; c != ""; c = msc.Parent(c) {
		fmt.Println(c, msc.Level(c))
	}
	// Output:
	// 11A41 3
	// 11Axx 2
	// 11-XX 1
}

func ExampleParse() {
	m, err := msc.Parse([]byte(`
"11-XX": {label: number theory}
"11Axx": {label: elementary number theory}
"11A41": {label: prime numbers}
`))
	if err != nil {
		fmt.Println(err)
		return
	}
	e, _ := m.Lookup("11A41")
	fmt.Println(m.Len(), e.Label, m.Children("11-XX"))
	// Output: 3 prime numbers [11Axx]
}
