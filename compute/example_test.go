package compute_test

import (
	"fmt"

	"github.com/tamnd/math2-agent/compute"
)

func ExampleCheckerFor() {
	c, _ := compute.CheckerFor("lrat")
	fmt.Println(c.Images, c.Text)
	// Output: [m2-sat] cake_lpr, a verified checker in CakeML
}

func ExampleParseBytes() {
	n, _ := compute.ParseBytes("4G")
	fmt.Println(n)
	// Output: 4294967296
}

func ExampleCommand() {
	fmt.Println(compute.Command("m2-pari", "/work/main.gp"))
	// Output: [gp -q -f /work/main.gp]
}

func ExampleSplitRef() {
	name, digest, ok := compute.SplitRef("m2-pari@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	fmt.Println(name, digest[:15], ok)
	// Output: m2-pari sha256:01234567 true
}
