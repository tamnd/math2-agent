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
	fmt.Println(len(t), tag.IsTag(t), t[0] != '0', tag.Shard(t) == t[:2])
	// Output: 10 true true true
}

func ExampleIsTag() {
	for _, s := range []string{"7K3QX2M9R2", "7K3QX2M9R3", "7K3QX2", "7k3qx2", "7K3QXI", "~3fa94c01"} {
		fmt.Println(s, tag.IsTag(s), tag.IsProvisional(s))
	}
	// Output:
	// 7K3QX2M9R2 true false
	// 7K3QX2M9R3 false false
	// 7K3QX2 true false
	// 7k3qx2 false false
	// 7K3QXI false false
	// ~3fa94c01 false true
}

func ExampleExcluded() {
	// A tag must not look like a number or an MSC code, and a long tag must have two letters or more.
	fmt.Println(tag.Excluded("123456"), tag.Excluded("20D60X"), tag.Excluded("123456789X"), tag.Excluded("7K3QX2M9R2"))
	// Output: true true true false
}

func ExampleCheck() {
	p := "7K3QX2M9R"
	fmt.Println(p + string(tag.Check(p)))
	// Output: 7K3QX2M9R2
}

func ExampleNormalize() {
	for _, s := range []string{"7k3qx-2m9r2", "7K3XQ2M9R2", "g5sim9"} {
		t, err := tag.Normalize(s)
		fmt.Println(t, err)
	}
	// Output:
	// 7K3QX2M9R2 <nil>
	//  not a valid tag: the check character does not match
	// G5S1M9 <nil>
}

func ExampleDisplay() {
	fmt.Println(tag.Display("7K3QX2M9R2"), tag.Display("3C53TD"))
	// Output: 7K3QX-2M9R2 3C53TD
}

func ExampleIsAlias() {
	for _, s := range []string{"lagrange-theorem-groups", "7K3QX2", "Compact-Space", "compact--space"} {
		fmt.Println(s, tag.IsAlias(s))
	}
	// Output:
	// lagrange-theorem-groups true
	// 7K3QX2 false
	// Compact-Space false
	// compact--space false
}

func ExampleAlias_String() {
	a := tag.Alias{Name: "compact-space", Tag: "7K3QX2", From: "2026-05-01", Until: tag.Current}
	fmt.Println(a)
	// Output: compact-space 7K3QX2 2026-05-01 -
}
