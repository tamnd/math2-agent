package tag_test

import (
	"bytes"
	"fmt"

	"github.com/tamnd/math2-agent/tag"
)

func ExampleMint() {
	// A fixed reader gives the same tag each time. Use a nil reader to draw from crypto/rand.
	r := bytes.NewReader(bytes.Repeat([]byte{0x5a, 0x17, 0x3c, 0x99}, 16))
	t, err := tag.Mint(r, func(string) bool { return false })
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(tag.IsTag(t), t[0] != '0', tag.Shard(t) == t[:2])
	// Output: true true true
}

func ExampleIsTag() {
	for _, s := range []string{"7K3QX2", "7k3qx2", "7K3QXI", "~3fa94c01"} {
		fmt.Println(s, tag.IsTag(s), tag.IsProvisional(s))
	}
	// Output:
	// 7K3QX2 true false
	// 7k3qx2 false false
	// 7K3QXI false false
	// ~3fa94c01 false true
}

func ExampleExcluded() {
	// A tag must not look like a number or an MSC code.
	fmt.Println(tag.Excluded("123456"), tag.Excluded("20D60X"), tag.Excluded("7K3QX2"))
	// Output: true true false
}
