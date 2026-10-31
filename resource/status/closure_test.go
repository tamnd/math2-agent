package status

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
)

func TestRefClosure(t *testing.T) {
	in := &resource.Scope{Value: resource.In, By: "b"}
	rs := []resource.Resource{
		{ID: "A", Kind: "monograph", Scope: in},
		{ID: "B", Kind: "monograph", Scope: in},
		{ID: "C", Kind: "monograph", Scope: in}, // at D0: its references do not count
		{ID: "T", Kind: "article", Date: "1995", Scope: in},
	}
	gauss := &resource.Parsed{Title: "Disquisitiones arithmeticae", Authors: []string{"C. F. Gauss"}, Year: "1801"}
	cs := []resource.Cite{
		{From: "A/e1", N: 1, To: "T"},
		{From: "A/e1", N: 2, Parsed: gauss},
		{From: "B/e1", N: 1, Parsed: gauss},
		{From: "B/e1", N: 2, Parsed: &resource.Parsed{Title: "On a lemma", Authors: []string{"X. Y. Zed"}, Venue: "J. Math.", Year: "2004"}},
		{From: "B/e1", N: 3}, // no parsed fields and no target
		{From: "C/e1", N: 1, Parsed: gauss},
	}
	c := refClosure(rs, cs, map[string]int{"A": 1, "B": 2, "T": 1})
	if c.Citing != 2 {
		t.Errorf("citing = %d, want 2", c.Citing)
	}
	want := []RefRow{
		{Kind: "article", Decade: "1990s", Parsed: 1, Resolved: 1, Closure: 1, Target: 0.99},
		{Kind: "article", Decade: "2000s", Parsed: 1, Closure: 0, Target: 0.99, Below: true},
		{Kind: "book", Decade: "1800s", Parsed: 2, Closure: 0, Target: 0.95, Below: true},
	}
	if len(c.Rows) != len(want) {
		t.Fatalf("rows = %+v", c.Rows)
	}
	for i := range want {
		if c.Rows[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, c.Rows[i], want[i])
		}
	}
	if len(c.Unresolved) != 1 || c.Unresolved[0].Citing != 2 || c.Unresolved[0].Title != gauss.Title || len(c.Unresolved[0].Refs) != 2 {
		t.Errorf("unresolved = %+v", c.Unresolved)
	}
}

func TestStale(t *testing.T) {
	cat := &ingest.Catalogue{Sources: []ingest.Source{
		{ID: "old", Cadence: "monthly", Snapshots: []ingest.Snapshot{{Date: "2026-01-01"}}},
		{ID: "new", Cadence: "monthly", Snapshots: []ingest.Snapshot{{Date: "2026-10-20"}}},
		{ID: "none", Snapshots: []ingest.Snapshot{{Date: "2020-01-01"}}},
	}}
	got := stale(cat, "2026-10-31")
	if len(got) != 1 || got[0].Source != "old" || got[0].Days != 303 {
		t.Errorf("stale = %+v", got)
	}
}

func TestUnlisted(t *testing.T) {
	u := unlisted([]Recall{
		{Resources: 90, Model: "m", N: 100, Low: 95, High: 120},
		{Resources: 10, Model: "m", N: 8, Low: 8, High: 12},
		{Resources: 5},
	})
	if u != (Unlisted{Strata: 2, N: 10, Low: 5, High: 32}) {
		t.Errorf("unlisted = %+v", u)
	}
	var b bytes.Buffer
	(&Report{Unlisted: u}).staleMarkdown(&b)
	if !strings.Contains(b.String(), "the estimate of the works that no source lists is 10, with the interval 5 to 32") {
		t.Errorf("markdown = %s", b.String())
	}
}
