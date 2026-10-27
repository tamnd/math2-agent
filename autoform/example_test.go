package autoform_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/autoform"
	"github.com/tamnd/math2-agent/battery"
)

func ExampleSelect() {
	pass := &battery.Report{}
	for i, name := range battery.Names() {
		pass.Checks = append(pass.Checks, battery.Check{Number: i + 1, Name: name, Result: battery.Pass})
	}
	r := autoform.Select("7K3QX2", []autoform.Candidate{
		{Run: "r-1", Family: "claude", Decl: "Math2.A.s", Hash: "blake3:aa", Battery: pass},
		{Run: "r-2", Family: "claude", Decl: "Math2.B.s", Hash: "blake3:bb", Battery: pass},
		{Run: "r-3", Family: "claude", Decl: "Math2.C.s", Hash: "blake3:bb", Battery: pass},
	})
	fmt.Println(r.Candidates[r.Winner].Decl, len(r.Groups), r.Split())
	// Output: Math2.B.s 2 true
}
