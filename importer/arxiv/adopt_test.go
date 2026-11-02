package arxiv_test

import (
	"strings"
	"testing"

	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/inventory"
)

func TestAdopt(t *testing.T) {
	rec := &inventory.Outcome{Value: inventory.OutcomeRecord, Rule: "research-default"}
	rows := []inventory.Row{
		{ID: "ROWA", Kind: "statement", Name: "Widget lemma", MSC: []string{"20-XX"}, Layer: inventory.Research, Outcome: rec, Mentions: []string{"ax://paper/2402.00002#B", "ax://paper/2401.00001#A"}},
		{ID: "ROWB", Kind: "definition", Name: "Gadget", Layer: inventory.Research, Outcome: rec, Mentions: []string{"ax://paper/2401.00001#C"}},
		{ID: "ROWC", Kind: "statement", Name: "Drafted", Layer: inventory.Research, Outcome: rec, Mentions: []string{"ax://paper/2401.00001#D"}},
		{ID: "ROWD", Kind: "statement", Name: "Minted", Layer: inventory.Research, Outcome: rec, Mentions: []string{"ax://paper/2401.00001#E"}},
		{ID: "ROWE", Kind: "remark", Name: "A remark", Layer: inventory.Research, Outcome: rec, Mentions: []string{"ax://paper/2401.00001#F"}},
	}
	ms := []inventory.Mention{
		{ID: "ax://paper/2401.00001#A", Source: "arxiv", Kind: "statement", Role: "lemma", Class: "open", Origin: "own", Version: 2, Locator: "2401.00001v2 Lemma 3.2"},
		{ID: "ax://paper/2402.00002#B", Source: "arxiv", Kind: "statement", Role: "theorem", Class: "open", Origin: "cited", Version: 1, Locator: "2402.00002v1 Theorem 1"},
		{ID: "ax://paper/2401.00001#C", Source: "arxiv", Kind: "definition", Class: "record", Version: 1, Locator: "2401.00001v1 Definition 1.1"},
		{ID: "ax://paper/2401.00001#D", Source: "arxiv", Kind: "statement"},
		{ID: "ax://paper/2401.00001#E", Source: "arxiv", Kind: "statement"},
		{ID: "ax://paper/2401.00001#F", Source: "arxiv", Kind: "remark"},
	}
	nodes := []axImport.AdoptNode{
		{ID: "~draft01", Inventory: []string{"ROWC"}, Mentions: []string{"ax://paper/2401.00001#D"}},
		{ID: "3C53TD", Inventory: []string{"ROWD"}, Mentions: []string{"ax://paper/2401.00001#E"}},
	}
	ids := []string{"~new0001", "~new0002"}
	draw := func() (string, error) {
		id := ids[0]
		ids = ids[1:]
		return id, nil
	}
	res, err := axImport.Adopt(rows, ms, nodes, axImport.AdoptOptions{Rows: []string{"ROWA", "ROWB", "ROWC", "ROWD", "ROWE"}, Draw: draw, Day: "2026-11-03"})
	if err != nil {
		t.Fatal(err)
	}
	states := []string{}
	for _, it := range res.Items {
		states = append(states, it.Row+" "+it.State+" "+it.Node)
	}
	if got, want := strings.Join(states, "; "), "ROWA adopted ~new0001; ROWB adopted ~new0002; ROWC drafted ~draft01; ROWD linked 3C53TD; ROWE waiting "; got != want {
		t.Errorf("the states are %q, want %q", got, want)
	}
	if res.Linked != 1 || res.Rows[3].Outcome.Value != inventory.OutcomeNode || res.Rows[3].Outcome.Rule != axImport.AdoptRule || res.Rows[3].Outcome.Target[0] != "3C53TD" {
		t.Errorf("the minted node gives the row %s the outcome %+v", res.Rows[3].ID, res.Rows[3].Outcome)
	}
	if len(res.Drafts) != 2 {
		t.Fatalf("the drafts are %+v", res.Drafts)
	}
	want := "---\ntag: ~new0001\nkind: statement\nrole: lemma\nname: Widget lemma\nmsc: [20-XX]\nsources:\n  - arxiv: 2401.00001v2\n    locator: Lemma 3.2\n    mention: ax://paper/2401.00001#A\n    note: the paper that states the item (corpus arxiv AX9.2)\ninventory: [ROWA]\n---\n\n## Statement\n"
	if got := string(res.Drafts[0].Data); got != want {
		t.Errorf("the draft of an own item before a cited one is\n%s\nwant\n%s", got, want)
	}
	if d := string(res.Drafts[1].Data); !strings.Contains(d, "kind: concept\n") || !strings.Contains(d, "## Summary\n") || res.Drafts[1].Path != "nodes/~/~new0002.md" {
		t.Errorf("the draft of a definition is %s at %s", d, res.Drafts[1].Path)
	}
	if r := res.Items[4].Reason; !strings.Contains(r, "kind remark") {
		t.Errorf("a remark waits with the reason %q", r)
	}
	report := string(axImport.AdoptReport(res, "2026-11-03"))
	if !strings.Contains(report, "adopted: 2\n") || !strings.Contains(report, "| ROWD | statement | Minted |") {
		t.Errorf("the report is\n%s", report)
	}
}
