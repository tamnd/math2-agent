package resource

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/tag"
)

// draws returns the ids that tag.Mint gives from the bytes, in order, with no id used.
func draws(t *testing.T, seed []byte, n int) []string {
	t.Helper()
	r := bytes.NewReader(seed)
	var out []string
	for range n {
		id, err := tag.Mint(r, nil)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, id)
	}
	return out
}

func TestMint(t *testing.T) {
	root := t.TempDir()
	seed := bytes.Repeat([]byte{0x5a, 0x13, 0xc7, 0x91, 0x2e, 0xb4, 0x08, 0x6f, 0xd3, 0x47, 0xa1}, 64)
	ids := draws(t, seed, 4)

	// The first draw is a tag and the second draw is an inventory id, so Mint must skip them.
	if err := os.MkdirAll(filepath.Join(root, "registers"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "registers", "tags.reg"), []byte(ids[0]+" c0 concept\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := inventory.AppendRegister(root, []string{ids[1]}, "c0"); err != nil {
		t.Fatal(err)
	}

	a, b, live := fixture(t), fixture(t), fixture(t)
	a.ID, b.ID = "~0000000a", "~0000000b"
	b.Kind = "article"
	b.Relations = []Relation{{Kind: "part_of", To: "~0000000a/e3"}}
	b.Scope = &Scope{Value: Duplicate, Target: "~0000000a", By: "m2/test"}
	if err := WriteResources(root, []Resource{a, b, live}); err != nil {
		t.Fatal(err)
	}

	minted, err := Mint(root, bytes.NewReader(seed), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if len(minted) != 2 || minted[0] != (Minted{"~0000000a", ids[2], "textbook"}) || minted[1] != (Minted{"~0000000b", ids[3], "article"}) {
		t.Fatalf("got %+v, want the ids %s and %s", minted, ids[2], ids[3])
	}
	rs, err := ReadResources(root)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Resource{}
	for _, r := range rs {
		byID[r.ID] = r
	}
	if len(byID) != 3 || byID[live.ID].ID == "" {
		t.Fatalf("got the resources %v", rs)
	}
	nb := byID[ids[3]]
	if nb.Relations[0].To != ids[2]+"/e3" || nb.Scope.Target != ids[2] {
		t.Fatalf("the references were not changed: %+v %+v", nb.Relations, nb.Scope)
	}
	have, err := Registered(root)
	if err != nil || !have[ids[2]] || !have[ids[3]] || len(have) != 2 {
		t.Fatalf("got the register %v, %v", have, err)
	}

	// A second run has nothing to mint.
	if again, err := Mint(root, nil, "c2"); err != nil || len(again) != 0 {
		t.Fatalf("got %v, %v", again, err)
	}
}

func TestMintUnknownProvisional(t *testing.T) {
	root := t.TempDir()
	a := fixture(t)
	a.ID = "~0000000a"
	a.Relations = []Relation{{Kind: "part_of", To: "~0000000f"}}
	if err := WriteResources(root, []Resource{a}); err != nil {
		t.Fatal(err)
	}
	if _, err := Mint(root, nil, "c1"); err == nil {
		t.Fatal("a reference to a provisional id that no resource has must fail")
	}
	if have, _ := Registered(root); len(have) != 0 {
		t.Fatal("a failed mint wrote to the register")
	}
}
