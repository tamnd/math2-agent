package wikipedia_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tamnd/math2-agent/importer/wikipedia"
	"github.com/tamnd/math2-agent/inventory"
)

func TestCandidatesAndReport(t *testing.T) {
	ms := []inventory.Mention{
		{ID: "wikipedia://en/100", Source: "wikipedia", Kind: "statement", Name: "Banach fixed-point theorem", MSC: []string{"47H10"}, ArticleType: "theorem"},
		{ID: "wikipedia://en/200#K3F9", Source: "wikipedia", Kind: "statement", Name: "Banach fixed point theorem", MSC: []string{"54E50"}, ArticleType: "concept"},
		{ID: "wikipedia://en/200#P1", Source: "wikipedia", Kind: "proof", AttachedTo: "wikipedia://en/200#K3F9", ArticleType: "concept"},
		{ID: "wikipedia://en/300", Source: "wikipedia", Kind: "definition", Name: "Metric space", MSC: []string{"54E35"}, ArticleType: "concept"},
		{ID: "m2://rec/wikidata/Q1#1", Source: "wikidata", Kind: "statement", Name: "Banach fixed-point theorem"},
	}
	rows := []inventory.Row{
		{ID: "R1", Mentions: []string{ms[0].ID, ms[4].ID}, Kind: "statement", MSC: []string{"47H10", "54E50"}, Outcome: &inventory.Outcome{Value: "aligned"}},
		{ID: "R2", Mentions: []string{ms[1].ID}, Kind: "statement", MSC: []string{"54E50"}},
	}
	cands := wikipedia.Candidates(rows, ms, "")
	if len(cands) != 1 || cands[0].Mention != ms[1].ID || cands[0].Row != "R2" || cands[0].Target != "R1" {
		t.Fatalf("the candidates are %+v", cands)
	}
	if got := wikipedia.Candidates(rows, ms, "47"); len(got) != 0 {
		t.Errorf("the candidates of the block 47 are %+v", got)
	}
	day := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	r := wikipedia.Report(rows, ms, cands, day)
	for _, want := range []string{"mentions: 3\nrows: 2\nanother_source: 1\nwaiting: 1\nname_candidates: 1\n", "3 Wikipedia mentions with no attachment to another item, in 2 rows. 1 row also holds a mention of another source. 1 mention waits for a row.", "| aligned | 1 |", "| open | 1 |", "| concept | 2 | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 1 | 0 | 1 | 1 |", "| theorem | 1 | 1 | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 1 | 0 | 0 |", "| 54 | 2 | 2 |"} {
		if !strings.Contains(r, want) {
			t.Errorf("the report has no %q:\n%s", want, r)
		}
	}
	names := string(wikipedia.FormatCandidates(cands, day))
	if !strings.HasPrefix(names, "---\ntitle: The name candidates of the Wikipedia mentions\ndate: 2026-11-02\ncandidates: 1\n---\n") || !strings.Contains(names, "| wikipedia://en/200#K3F9 | R2 | R1 | Banach fixed point theorem |") {
		t.Errorf("the candidates file is:\n%s", names)
	}
}
