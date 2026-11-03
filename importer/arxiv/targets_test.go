package arxiv_test

import (
	"testing"

	axImport "github.com/tamnd/math2-agent/importer/arxiv"
	"github.com/tamnd/math2-agent/inventory"
)

func TestADecisionAttachesAnOrphanProof(t *testing.T) {
	ms := []inventory.Mention{
		{ID: "ax://paper/2401.00001#R1", Kind: "statement"},
		{ID: "ax://paper/2401.00001#P1", Kind: "proof", AttachedTo: "unknown", Hints: []string{"target-not-an-item"}},
		{ID: "ax://paper/2401.00001#P2", Kind: "proof", AttachedTo: "unknown"},
		{ID: "ax://paper/2401.00001#P3", Kind: "proof", AttachedTo: "ax://paper/2401.00001#R1"},
		{ID: "ax://paper/2401.00002#R1", Kind: "statement"},
	}
	n := axImport.ApplyTargets(ms, map[string]string{
		"ax://paper/2401.00001#P1": "ax://paper/2401.00001#R1",
		"ax://paper/2401.00001#P2": "ax://paper/2401.00002#R1", // another paper
		"ax://paper/2401.00001#P3": "ax://paper/2401.00001#R9", // attached already
	})
	if n != 1 || ms[1].AttachedTo != "ax://paper/2401.00001#R1" || len(ms[1].Hints) != 0 || ms[2].AttachedTo != "unknown" || ms[3].AttachedTo != "ax://paper/2401.00001#R1" {
		t.Errorf("ApplyTargets gives %d and %+v", n, ms)
	}
}
