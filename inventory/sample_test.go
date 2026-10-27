package inventory

import (
	"reflect"
	"testing"
)

func TestDrawArxiv(t *testing.T) {
	var ms []Mention
	for _, id := range []string{"2401.00001", "2401.00002", "2401.00003"} {
		ms = append(ms, Mention{ID: "ax://paper/" + id + "#T1", Source: "arxiv", NativeID: id + "#T1", Kind: "statement", Version: 1, Locator: id + "v1 Theorem 1", CitesResult: []CitedResult{{To: "bib:x"}, {To: "doi:10.1/x", Locator: "Theorem 2"}}})
	}
	ms = append(ms, Mention{ID: "ax://paper/2401.00001#P1", Source: "arxiv", NativeID: "2401.00001#P1", Kind: "statement", Head: "text"})
	rows := []Row{
		{ID: "inv:a", Layer: Research, Kind: "statement", MSC: []string{"47H10"}, Mentions: []string{"ax://paper/2401.00001#T1", "ax://paper/2401.00002#T1"}, History: []Event{{Op: "add", By: "m2/v0.3.6"}, {Op: "outcome", At: "2026-10-03", By: "p"}}, Outcome: &Outcome{Value: OutcomeNode, Target: []string{"T0001"}}},
		{ID: "inv:b", Layer: Research, Kind: "statement", MSC: []string{"05C10"}, Mentions: []string{"ax://paper/2401.00003#T1"}, History: []Event{{Op: "outcome", At: "2026-09-30", By: "p"}}, Outcome: &Outcome{Value: OutcomeNode, Target: []string{"T0002"}}},
	}
	if _, err := DrawArxiv(rows, ms, "2026-10", ""); err == nil {
		t.Error("an empty seed gave no error")
	}
	got, err := DrawArxiv(rows, ms, "2026-10", "s1")
	if err != nil {
		t.Fatal(err)
	}
	pop := map[string]int{}
	for _, s := range got {
		pop[s.Name] = s.Population
		for _, p := range s.Packets {
			if p.Check == "" || p.Stratum != s.Name {
				t.Errorf("packet %+v", p)
			}
		}
	}
	want := map[string]int{StratumRender: 0, StratumSource: 3, StratumNative: 0, StratumVision: 0, StratumProse: 1, StratumUnfold: 0, StratumEdges: 3, StratumSame: 1, StratumResearch: 2, StratumNewArea: 1}
	if !reflect.DeepEqual(pop, want) {
		t.Errorf("populations %v, want %v", pop, want)
	}
	for _, s := range got {
		if s.Name == StratumNewArea && (len(s.Packets) != 1 || s.Packets[0].ID != "T0001" || s.Packets[0].Area != "47") {
			t.Errorf("new area %+v", s.Packets)
		}
	}
	again, _ := DrawArxiv(rows, ms, "2026-10", "s1")
	if !reflect.DeepEqual(got, again) {
		t.Error("the same seed gave another sample")
	}
}
