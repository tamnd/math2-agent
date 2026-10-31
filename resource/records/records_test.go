package records

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/depth"
)

func tree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	rs := []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "textbook", Title: "A", IDs: map[string]resource.Values{"isbn": {"9780000000001"}}, Scope: &resource.Scope{Value: resource.In}},
		{ID: "8F3BBBBBBT", Kind: "article", Title: "B", IDs: map[string]resource.Values{"doi": {"10.1/b"}}, Scope: &resource.Scope{Value: resource.In}},
		{ID: "8F4CCCCCCH", Kind: "course", Title: "C", Scope: &resource.Scope{Value: resource.In}},
	}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	if err := resource.WriteCites(root, []resource.Cite{{From: "8F2AAAAAA3/e1", N: 1, To: "8F3BBBBBBT"}, {From: "8F3BBBBBBT/e1", N: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := depth.Write(root, []depth.Row{{ID: "8F2AAAAAA3", Tier: depth.T2}, {ID: "8F3BBBBBBT", Tier: depth.T4}, {ID: "8F4CCCCCCH", Tier: depth.T5}}); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestExport(t *testing.T) {
	root, out := tree(t), t.TempDir()
	shards, err := Export(root, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(shards) != 5 {
		t.Fatalf("got %d shards, want 3 of resources and 2 of cites: %+v", len(shards), shards)
	}
	idx, err := ReadIndex(out)
	if err != nil || len(idx) != len(shards) {
		t.Fatalf("the index has %d shards, %v", len(idx), err)
	}
	n := 0
	for _, s := range idx {
		recs, err := ReadShard(out, s)
		if err != nil {
			t.Fatal(err)
		}
		n += len(recs)
	}
	if n != 5 {
		t.Errorf("the shards have %d records, want 5", n)
	}
	rows, err := parquet.ReadFile[Row](filepath.Join(out, ParquetFile))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Tier != depth.T2 || rows[0].ISBN != "9780000000001" || rows[1].DOI != "10.1/b" {
		t.Errorf("the rows are %+v", rows)
	}
	// A changed shard fails its hash.
	bad := idx[0]
	bad.Hash = "blake3:00"
	if _, err := ReadShard(out, bad); err == nil || !strings.Contains(err.Error(), "the hash") {
		t.Errorf("a wrong hash gives %v", err)
	}
	// A shard that the math2 tree no longer has goes from the records tree.
	if err := os.Remove(filepath.Join(root, resource.Dir, "8F4.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Export(root, out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, resource.Dir, "8F4.md"+Ext)); !os.IsNotExist(err) {
		t.Errorf("the old shard is still there: %v", err)
	}
}

func TestKeep(t *testing.T) {
	root := tree(t)
	kept, moved, err := Keep(root)
	if err != nil {
		t.Fatal(err)
	}
	if kept != 2 || moved != 1 {
		t.Errorf("kept %d and moved %d, want 2 and 1", kept, moved)
	}
	rs, _ := resource.ReadResources(root)
	cs, _ := resource.ReadCites(root)
	if len(rs) != 2 || rs[1].ID != "8F4CCCCCCH" || len(cs) != 1 || cs[0].From != "8F2AAAAAA3/e1" {
		t.Errorf("the tree has %d resources and the cites %+v", len(rs), cs)
	}
	if _, _, err := Keep(t.TempDir()); err == nil {
		t.Error("a tree with no depth rows must be an error")
	}
}

func TestPin(t *testing.T) {
	root, out := tree(t), t.TempDir()
	if _, ok, err := ReadPin(root); ok || err != nil {
		t.Fatalf("a tree with no pin gives %v, %v", ok, err)
	}
	if _, err := Export(root, out); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePin(root, out, "not-a-commit"); err == nil {
		t.Error("a bad commit must be an error")
	}
	p, err := WritePin(root, out, "0123abcd")
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadPin(root)
	if err != nil || !ok || got != p || got.Repo != Repo || got.Shards != 5 || !strings.HasPrefix(got.Index, "blake3:") {
		t.Errorf("the pin is %+v, %v, %v", got, ok, err)
	}
}

func TestUnpack(t *testing.T) {
	root, out := tree(t), t.TempDir()
	if _, err := Export(root, out); err != nil {
		t.Fatal(err)
	}
	if _, err := WritePin(root, out, "0123abcd"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Keep(root); err != nil {
		t.Fatal(err)
	}
	// A change in the math2 tree stays.
	rs, _ := resource.ReadResources(root)
	rs[0].Title = "A2"
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	n, err := Unpack(root, out)
	if err != nil || n != 1 {
		t.Fatalf("Unpack added %d, %v, want 1", n, err)
	}
	rs, _ = resource.ReadResources(root)
	cs, _ := resource.ReadCites(root)
	if len(rs) != 3 || rs[0].Title != "A2" || rs[1].ID != "8F3BBBBBBT" || len(cs) != 2 {
		t.Errorf("the tree has %+v and the cites %+v", rs, cs)
	}
	if n, err := Unpack(root, out); err != nil || n != 0 {
		t.Errorf("a second Unpack added %d, %v", n, err)
	}
	// An index that is not the index of the pin is an error.
	if err := os.WriteFile(filepath.Join(out, IndexFile), []byte("---\nrecords: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Unpack(root, out); err == nil || !strings.Contains(err.Error(), "hash") {
		t.Errorf("a changed index gives %v", err)
	}
}
