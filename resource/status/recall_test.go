package status

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/census"
)

func TestRecalls(t *testing.T) {
	var rs []resource.Resource
	add := func(kind, date string, sources ...string) {
		r := resource.Resource{ID: fmt.Sprintf("R%d", len(rs)), Kind: kind, Date: date, Scope: &resource.Scope{Value: resource.In}}
		for _, s := range sources {
			r.Catalogue = append(r.Catalogue, resource.CatalogueRecord{Source: s, NativeID: r.ID})
		}
		rs = append(rs, r)
	}
	// Three captures for the articles of the 2000s: a, b, and c with its copy d.
	counts := map[string]int{"a": 30, "b": 20, "c": 15, "a,b": 12, "a,c": 9, "b,d": 8, "a,b,c": 6}
	for _, k := range slices.Sorted(maps.Keys(counts)) {
		for range counts[k] {
			add("article", "2004", strings.Split(k, ",")...)
		}
	}
	add("monograph", "1960", "a", "b")
	add("monograph", "", "a", "b", "c")
	rs = append(rs, resource.Resource{ID: "X", Kind: "article", Date: "2004", Scope: &resource.Scope{Value: resource.Out}, Catalogue: []resource.CatalogueRecord{{Source: "a"}}})
	cat := &ingest.Catalogue{Sources: []ingest.Source{{ID: "d", Lineage: []string{"c"}}}}
	got := recalls(rs, cat)
	if len(got) != 2 {
		t.Fatalf("got %d strata, want 2: %+v", len(got), got)
	}
	a := got[0]
	if a.Kind != "article" || a.Decade != "2000s" || a.Resources != 100 {
		t.Errorf("the first stratum is %+v", a)
	}
	if !slices.Equal(a.Captures, []string{"a", "b", "c+d"}) {
		t.Errorf("the captures are %v", a.Captures)
	}
	if a.Model == "" || a.N < 100 || a.Low > a.N || a.High < a.N || a.Recall > 1 || a.RecallLow > a.Recall {
		t.Errorf("the estimate is %+v", a)
	}
	b := got[1]
	if b.Kind != "book" || b.Model != "" || b.Reason == "" {
		t.Errorf("the second stratum is %+v", b)
	}
	if again := recalls(rs, cat); again[0].N != a.N || again[0].High != a.High {
		t.Errorf("two runs give %v and %v", a.N, again[0].N)
	}
	md := string((&Report{Recall: got}).Markdown())
	if !strings.Contains(md, "## 4. Recall by stratum") || !strings.Contains(md, "| article | 2000s | 100 | a, b, c+d | "+a.Model) {
		t.Errorf("the report has no row for the stratum:\n%s", md)
	}
}

func TestUnmet(t *testing.T) {
	rs := []Recall{
		{Kind: "article", Decade: "2000s", Model: "Mt", RecallLow: 0.95, Target: 0.99, Below: true},
		{Kind: "article", Decade: "2010s", Model: "Mt", RecallLow: 0.995, Target: 0.99},
		{Kind: "book", Decade: "1960s", Reason: "fewer than three captures", Target: 0.97},
		{Kind: "other", Decade: "2000s", Model: "M0", RecallLow: 0.5},
	}
	below, unmeasured := Unmet(rs)
	if len(below) != 1 || below[0].Decade != "2000s" || len(unmeasured) != 1 || unmeasured[0].Kind != "book" {
		t.Fatalf("below %+v, unmeasured %+v", below, unmeasured)
	}
	md := string((&Report{Recall: rs}).Markdown())
	for _, want := range []string{"| strata below the recall target | 1 | 0 |", "0.99, below", "| none |", "Below the target: article 2000s.", "With no estimate: book 1960s."} {
		if !strings.Contains(md, want) {
			t.Errorf("the report has no %q:\n%s", want, md)
		}
	}
}

func TestWithPairs(t *testing.T) {
	out := []Recall{
		{Kind: "article", Decade: "2000s", Resources: 900, Target: 0.99, Reason: "fewer than three captures hold works of the stratum"},
		{Kind: "book", Decade: "1990s", Resources: 10, Model: "m", N: 10, Low: 10, High: 10, Recall: 1, RecallLow: 1, Target: 0.97},
	}
	rows := []census.ReportRow{
		{A: "arxiv", B: "zbmath", Kind: "article", Decade: "2000s", N: 950, Low: 920, High: 1000},
		{A: "hal", B: "zbmath", Kind: "article", Decade: "2000s", N: 990, Low: 960, High: 1100},
		{A: "hal", B: "zbmath", Kind: "article", Decade: "2010s", N: 500, Low: 400, High: 600},
		{A: "hal", B: "zbmath", Kind: "book", Decade: "1990s", N: 99, Low: 90, High: 120},
		{A: "hal", B: "zbmath", Kind: "other", Decade: "2010s", N: 99, Low: 90, High: 120}, // no target
	}
	got := withPairs(out, rows)
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	a := got[0]
	if a.Decade != "2000s" || a.Model != "pair hal and zbmath" || a.High != 1100 || a.RecallLow != 0.8182 || !a.Below || a.Reason != "" {
		t.Errorf("article 2000s = %+v", a)
	}
	if b := got[1]; b.Decade != "2010s" || b.Resources != 0 || b.Recall != 0 || !b.Below {
		t.Errorf("article 2010s = %+v", b)
	}
	if c := got[2]; c.Model != "m" || c.Below {
		t.Errorf("book 1990s = %+v", c)
	}
}
