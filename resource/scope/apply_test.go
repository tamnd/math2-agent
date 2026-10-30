package scope

import (
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

type fixed struct {
	v     string
	score float64
}

func (f fixed) Classify(*resource.Resource) (string, float64, string) { return f.v, f.score, "11" }
func (fixed) Version() string                                         { return "classify/1@test" }

func TestApply(t *testing.T) {
	rs := []resource.Resource{
		{ID: "A", Kind: "article", Title: "a", MSC: []resource.Code{{Code: "11M06", From: "zbmath"}}},
		{ID: "B", Kind: "article", Title: "b", Subjects: []resource.Subject{{System: "lcc", Code: "QH506"}}},
		{ID: "C", Kind: "article", Title: "c"},
		{ID: "D", Kind: "article", Title: "d", Scope: &resource.Scope{Value: resource.Duplicate, Target: "A", By: "x"}},
		{ID: "E", Kind: "article", Title: "e", MSC: []resource.Code{{Code: "11M06", From: "zbmath"}}},
		{ID: "F", Kind: "article", Title: "f", Scope: &resource.Scope{Value: resource.Deferred, Until: "2026-01-01", Reason: "no test decides the record, and the classifier has no model", By: "x"}},
	}
	in := Input{By: "m2 test", Day: "2026-10-30", Decisions: []PersonDecision{{ID: "E", Kind: "scope", Value: resource.Out, Reason: "a review of a film", By: "tamnd", At: "2026-10-01"}}}
	c, items, err := Apply(rs, in)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": resource.In, "B": resource.Out, "C": resource.Deferred, "D": resource.Duplicate, "E": resource.Out, "F": resource.Deferred}
	for _, r := range rs {
		if r.Scope.Value != want[r.ID] {
			t.Errorf("%s: got %s, want %s", r.ID, r.Scope.Value, want[r.ID])
		}
	}
	if rs[4].Scope.Rule != PersonRule || rs[2].Scope.Until != "2027-01-28" || rs[5].Scope.Until != "2026-01-01" {
		t.Errorf("got %+v %+v %+v", rs[4].Scope, rs[2].Scope, rs[5].Scope)
	}
	if c.Open != 1 || len(items) != 1 || items[0].ID != "F" || c.ByValue[resource.Duplicate] != 0 {
		t.Errorf("got counts %+v and items %+v", c, items)
	}
	if rs[0].Scope.Rule != "S1" || rs[0].Scope.By != "m2 test" || rs[0].History[0].Op != "scope" {
		t.Errorf("got %+v and %+v", rs[0].Scope, rs[0].History)
	}
	in.Classifier = fixed{"", 0.5}
	if _, items, _ := Apply(rs, in); len(items) != 2 || rs[2].Scope.Rule != ClassifierRule {
		t.Errorf("the middle band gave %+v and %+v", items, rs[2].Scope)
	}
	in.Classifier = fixed{resource.In, 0.99}
	if _, _, _ = Apply(rs, in); rs[2].Scope.Value != resource.In || len(rs[2].MSC) != 1 || rs[2].MSC[0].Code != "11-XX" {
		t.Errorf("the classifier gave %+v and %v", rs[2].Scope, rs[2].MSC)
	}
	if c, _, _ := Apply(rs, in); c.Changed != 0 {
		t.Errorf("a second run changed %d resources", c.Changed)
	}
}
