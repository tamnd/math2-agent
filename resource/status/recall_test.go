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

func TestRecallsSkipLookups(t *testing.T) {
	var rs []resource.Resource
	for i, srcs := range [][]string{{"a", "zbmath_search"}, {"b", "s2"}, {"a", "b"}} {
		r := resource.Resource{ID: fmt.Sprintf("R%d", i), Kind: "monograph", Date: "1990", Scope: &resource.Scope{Value: resource.In}}
		for _, s := range srcs {
			r.Catalogue = append(r.Catalogue, resource.CatalogueRecord{Source: s, NativeID: r.ID})
		}
		rs = append(rs, r)
	}
	got := recalls(rs, &ingest.Catalogue{})
	if len(got) != 1 || strings.Join(got[0].Captures, ",") != "a,b" || got[0].Reason == "" {
		t.Fatalf("got %+v", got)
	}
}

func TestWithCitations(t *testing.T) {
	out := []Recall{
		{Kind: "book", Decade: "1970s", Resources: 100, Target: 0.97, Reason: "fewer than three captures hold works of the stratum"},
		{Kind: "book", Decade: "1980s", Resources: 100, Target: 0.97, Reason: "fewer than three captures hold works of the stratum"},
		{Kind: "article", Decade: "2020s", Resources: 100, Target: 0.99, Model: "pair openalex and zbmath", N: 1e6, High: 2e6},
	}
	rows := []RefRow{
		{Kind: "book", Decade: "1970s", Works: 50, Held: 20},
		{Kind: "book", Decade: "1980s", Works: 50, Held: 3},
		{Kind: "article", Decade: "2020s", Works: 50, Held: 20},
	}
	out = withCitations(out, rows)
	if r := out[0]; r.Model != "pair catalogue and citations" || r.N != round(census.Chapman(100, 50, 20)) || r.Reason != "" || !r.Below {
		t.Errorf("1970s = %+v", r)
	}
	if r := out[1]; r.Model != "" {
		t.Errorf("1980s with 3 held = %+v", r)
	}
	if r := out[2]; r.Model != "pair openalex and zbmath" {
		t.Errorf("a pair with a higher interval is replaced: %+v", r)
	}
}

func TestWithCitationsMerge(t *testing.T) {
	out := []Recall{
		{Kind: "book", Decade: "1900s", Resources: 40, Target: 0.9},
		{Kind: "book", Decade: "1910s", Resources: 60, Target: 0.97},
		{Kind: "book", Decade: "1920s", Resources: 150},
		{Kind: "book", Decade: "1930s", Resources: 500, Model: "pair a and b", N: 600, High: 700},
		{Kind: "course", Decade: "2010s", Resources: 10},
	}
	rows := []RefRow{
		{Kind: "book", Decade: "1900s", Works: 10, Held: 3},
		{Kind: "book", Decade: "1910s", Works: 10, Held: 3},
		{Kind: "book", Decade: "1920s", Works: 10, Held: 2},
	}
	got := withCitations(out, rows)
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if r := got[0]; r.Decade != "1900s to 1920s" || r.Resources != 250 || r.Target != 0.97 || r.N != round(census.Chapman(250, 30, 8)) {
		t.Errorf("merged = %+v", r)
	}
	if got[1].Decade != "1930s" || got[2].Kind != "course" || got[2].Model != "" {
		t.Errorf("rest = %+v", got[1:])
	}
}
