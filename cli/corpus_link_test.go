package cli

import (
	"testing"

	"github.com/tamnd/math2-agent/inventory"
	"github.com/tamnd/math2-agent/resource"
)

func TestLinkPapers(t *testing.T) {
	root := t.TempDir()
	ms := []inventory.Mention{{ID: "ax://paper/2301.00001#THM1", NativeID: "2301.00001#THM1"}}
	if n, err := linkPapers(root, ms); err != nil || n != 0 || ms[0].Resource != "" {
		t.Fatalf("a tree with no resources: %d, %v, %q", n, err, ms[0].Resource)
	}
	rs := []resource.Resource{{
		ID: "8F2K1QW7ZJ", Kind: "preprint", Title: "A paper",
		Expressions: []resource.Expression{{Key: "e1", Kind: "version", IDs: resource.IDs{"arxiv": {"2301.00001"}}}},
		Catalogue:   []resource.CatalogueRecord{{Source: "arxiv", Snapshot: "blake3:00", NativeID: "2301.00001", Level: "expression"}},
		History:     []resource.Event{{Op: "add", At: "2026-10-30", By: "m2/test"}},
	}}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	ms = append(ms, inventory.Mention{ID: "ax://paper/2301.00002#THM1", NativeID: "2301.00002#THM1"})
	n, err := linkPapers(root, ms)
	if err != nil || n != 1 || ms[0].Resource != "8F2K1QW7ZJ/e1" || ms[1].Resource != "" {
		t.Fatalf("linked %d, %v, resources %q and %q", n, err, ms[0].Resource, ms[1].Resource)
	}
}
