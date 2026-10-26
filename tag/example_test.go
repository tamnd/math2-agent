package tag_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

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
	fmt.Println(len(t), tag.IsTag(t), t[0] != '0', tag.Shard(t) == t[:3])
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

func ExampleMoveShards() {
	root, err := os.MkdirTemp("", "shard")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(root) }()
	// A node in the old shard of two characters.
	dir := filepath.Join(root, "nodes", "7K")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fmt.Println(err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "7K3QX2.md"), []byte("---\n"), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	moves, err := tag.MoveShards(root)
	fmt.Println(moves, err)
	// Output: [{nodes/7K/7K3QX2.md nodes/7K3/7K3QX2.md}] <nil>
}

func ExampleCodeDir() {
	fmt.Println(tag.CodeDir("AAYEQE"))
	fmt.Println(tag.CodeDir("~1a2b3c4d"))
	// Output:
	// compute/AAY/AAYEQE
	// compute/~/~1a2b3c4d
}

func ExampleCodeID() {
	fmt.Println(tag.CodeID("compute/AAY/AAYEQE/main.g"))
	fmt.Println(tag.CodeID("compute/AAYEQE/main.g"))
	// Output:
	// AAYEQE true
	//  false
}

func ExampleIsLegacy() {
	fmt.Println(tag.IsLegacy("3C53TD"), tag.IsLegacy("7K3QX2M9R2"))
	// Output: true false
}

func ExampleIsProvisional() {
	fmt.Println(tag.IsProvisional("~3fa94c01"), tag.IsProvisional("~3FA94C01"), tag.IsProvisional("3C53TD"))
	// Output: true false false
}

func ExampleIsSeed() {
	seed := "0K3QX2M9R" + string(tag.Check("0K3QX2M9R"))
	fmt.Println(tag.IsSeed(seed), tag.IsSeed("7K3QX2M9R2"))
	// Output: true false
}

func ExampleMintSeed() {
	r := bytes.NewReader(bytes.Repeat([]byte{0x5a, 0x17, 0x3c, 0x99}, 16))
	t, err := tag.MintSeed(r, func(string) bool { return false })
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(t), t[0], tag.IsSeed(t))
	// Output: 10 48 true
}

func ExampleProvisional() {
	// A fixed reader gives the same id each time. Use a nil reader to draw from crypto/rand.
	id, err := tag.Provisional(bytes.NewReader([]byte{0x3f, 0xa9, 0x4c, 0x01}))
	fmt.Println(id, err)
	// Output: ~3fa94c01 <nil>
}

func ExampleShard() {
	fmt.Println(tag.Shard("7K3QX2M9R2"), tag.Shard("3C53TD"), tag.Shard("~3fa94c01"))
	// Output: 7K3 3C5 ~
}

func ExampleContentDirs() {
	fmt.Println(tag.ContentDirs())
	// Output: [nodes topics reviews compute lean]
}

func ExampleAppend() {
	root, err := os.MkdirTemp("", "register")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(root) }()
	path := filepath.Join(root, "registers", "tags.reg")
	if err := tag.Append(path, []tag.Entry{{Tag: "G5S1M9", Commit: "aaaaaaa", Kind: "concept"}, {Tag: "3C53TD", Commit: "aaaaaaa", Kind: "statement"}}); err != nil {
		fmt.Println(err)
		return
	}
	data, _ := os.ReadFile(path)
	fmt.Print(string(data))
	// Output:
	// 3C53TD aaaaaaa statement
	// G5S1M9 aaaaaaa concept
}

func ExampleReadUsed() {
	root, err := os.MkdirTemp("", "register")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(root) }()
	path := filepath.Join(root, "tags.reg")
	if err := os.WriteFile(path, []byte("# The tags.\n3C53TD aaaaaaa statement\n\nG5S1M9 aaaaaaa concept\n"), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	used, err := tag.ReadUsed(path, filepath.Join(root, "retired.reg"))
	fmt.Println(used, err)
	// Output: map[3C53TD:true G5S1M9:true] <nil>
}

func ExampleReadAliases() {
	root, err := os.MkdirTemp("", "register")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(root) }()
	path := filepath.Join(root, "aliases.reg")
	if err := os.WriteFile(path, []byte("# The aliases.\ngroup G5S1M9 2026-05-01 -\n"), 0o600); err != nil {
		fmt.Println(err)
		return
	}
	aliases, err := tag.ReadAliases(path)
	fmt.Println(aliases, err)
	// Output: [group G5S1M9 2026-05-01 -] <nil>
}

// rewriteTree makes a working tree with one new node, ~3fa94c01, and one node that links to it.
func rewriteTree() (string, error) {
	root, err := os.MkdirTemp("", "rewrite")
	if err != nil {
		return "", err
	}
	files := map[string]string{
		"nodes/~/~3fa94c01.md": "---\ntag: ~3fa94c01\nkind: statement\nname: A new node\n---\n",
		"nodes/G5S/G5S1M9.md":  "---\ntag: G5S1M9\nkind: concept\nname: Group\n---\n\nSee [[a new node|~3fa94c01]].\n",
		"registers/tags.reg":   "G5S1M9 aaaaaaa concept\n",
	}
	for p, text := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o750); err != nil {
			return root, err
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte(text), 0o600); err != nil {
			return root, err
		}
	}
	return root, nil
}

func ExampleRewrite() {
	root, err := rewriteTree()
	defer func() { _ = os.RemoveAll(root) }()
	if err != nil {
		fmt.Println(err)
		return
	}
	// Use a nil reader to draw the tags from crypto/rand.
	minted, err := tag.Rewrite(root, nil, "bbbbbbb")
	if err != nil {
		fmt.Println(err)
		return
	}
	m := minted[0]
	data, _ := os.ReadFile(filepath.Join(root, "nodes", "G5S", "G5S1M9.md"))
	_, err = os.Stat(filepath.Join(root, "nodes", tag.Shard(m.Tag), m.Tag+".md"))
	fmt.Println(m.Provisional, tag.IsTag(m.Tag), m.Kind, bytes.Contains(data, []byte("|"+m.Tag+"]]")), err)
	// Output: ~3fa94c01 true statement true <nil>
}

func ExampleRewriteSeed() {
	root, err := rewriteTree()
	defer func() { _ = os.RemoveAll(root) }()
	if err != nil {
		fmt.Println(err)
		return
	}
	minted, err := tag.RewriteSeed(root, nil, "bbbbbbb")
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(minted), tag.IsSeed(minted[0].Tag))
	// Output: 1 true
}
