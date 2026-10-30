package status

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/ingest"
	"github.com/tamnd/math2-agent/resource"
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
