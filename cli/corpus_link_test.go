package cli

import (
	"strings"
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

func TestCorpusLink(t *testing.T) {
	root := t.TempDir()
	rs := []resource.Resource{{
		ID: "8F2K1QW7ZJ", Kind: "preprint", Title: "A paper",
		IDs:       resource.IDs{"arxiv": {"2301.00001"}},
		Catalogue: []resource.CatalogueRecord{{Source: "arxiv", Snapshot: "blake3:00", NativeID: "2301.00001", Level: "work"}},
		History:   []resource.Event{{Op: "add", At: "2026-10-30", By: "m2/test"}},
	}}
	if err := resource.WriteResources(root, rs); err != nil {
		t.Fatal(err)
	}
	ms := []inventory.Mention{
		{ID: "ax://paper/2301.00001#THM1", Source: "arxiv", NativeID: "2301.00001#THM1"},
		{ID: "ax://paper/2301.00002#THM1", Source: "arxiv", NativeID: "2301.00002#THM1", Resource: "OLD0000000"},
	}
	if err := inventory.WriteMentions(root, "arxiv", ms); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := run("corpus", "link", "-root", root)
	if code != ExitOK || !strings.Contains(stdout, "arxiv: 2 mentions, 1 name their resource, 2 changed") {
		t.Fatalf("exit code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	got, err := inventory.ReadMentions(root)
	if err != nil || len(got) != 2 || got[0].Resource != "8F2K1QW7ZJ" || got[1].Resource != "" {
		t.Fatalf("the mentions are %+v, %v", got, err)
	}
	if _, stdout, _ = run("corpus", "link", "-root", root); !strings.Contains(stdout, "0 changed") {
		t.Fatalf("a second run: stdout %q", stdout)
	}
}
