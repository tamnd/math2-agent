package wpcorpus

import (
	"strings"
	"testing"
)

func TestResolveCited(t *testing.T) {
	cands := []*Candidate{
		{PageID: 1, Title: "Group (mathematics)", InScope: true, Verdict: "math", Rule: "strong"},
		{PageID: 2, Title: "Paris", Verdict: "not-math", Rule: "model"},
	}
	page := map[string]int64{"Group_(mathematics)": 1, "Group_theory_group": 1, "Paris": 2, "Earth": 3}
	rs := ResolveCited([]string{"Group (mathematics)", "Group theory group", "Paris", "Earth", "No such page"}, page, cands)
	got := make([]string, len(rs))
	for i, r := range rs {
		got[i] = r.Mention + r.Reason
	}
	want := []string{"wikipedia://en/1", "wikipedia://en/1", CitedOutOfScope, CitedNoSignal, CitedNoPage}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("outcomes %q, want %q", got, want)
	}
	if rs[2].Detail != "Paris, verdict not-math, rule model" {
		t.Errorf("detail %q", rs[2].Detail)
	}
	var b strings.Builder
	if err := CitedReport(&b, rs); err != nil {
		t.Fatal(err)
	}
	s := b.String()
	for _, w := range []string{"titles: 5\n", "mentions: 2\n", "no-page: 1\n", "out-of-scope: 1\n", "no-candidate: 1\n", "| Earth | 3 | no-candidate |  |\n", "| No such page |  | no-page |  |\n"} {
		if !strings.Contains(s, w) {
			t.Errorf("report has no %q:\n%s", w, s)
		}
	}
	if strings.Index(s, "no-candidate |") > strings.Index(s, "no-page |") {
		t.Errorf("rows are not sorted by reason:\n%s", s)
	}
}
