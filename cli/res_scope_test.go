package cli

import (
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/institutions"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

func TestCourseSignal(t *testing.T) {
	if courseSignal(nil, nil) != nil {
		t.Fatal("an empty register gives a signal")
	}
	ins, _ := institutions.Build(nil, nil, nil)
	rs := []resource.Resource{
		{ID: "C1", Kind: "course", IDs: resource.IDs{"course": {"042nb2s44:18.06"}}, Catalogue: []resource.CatalogueRecord{{Source: "ocw"}}},
		{ID: "C2", Kind: "course", IDs: resource.IDs{"course": {"042nb2s44:8.01"}}, Catalogue: []resource.CatalogueRecord{{Source: "ocw"}}},
	}
	fn := signals(nil, courseSignal(ins, rs))
	for _, c := range []struct {
		r    resource.Resource
		want bool
	}{
		{resource.Resource{Kind: "course", IDs: resource.IDs{"course": {"042nb2s44:18.06"}}, Catalogue: []resource.CatalogueRecord{{Source: "ocw"}}}, true},
		{resource.Resource{Kind: "course", IDs: resource.IDs{"course": {"042nb2s44:8.01"}}, Catalogue: []resource.CatalogueRecord{{Source: "ocw"}}}, false},
		{resource.Resource{Kind: "course", Catalogue: []resource.CatalogueRecord{{Source: "nptel"}}}, true},
		{resource.Resource{Kind: "course-material", Catalogue: []resource.CatalogueRecord{{Source: "nptel"}}}, false},
		{resource.Resource{Kind: "course", Catalogue: []resource.CatalogueRecord{{Source: "oyc"}}}, false},
		{resource.Resource{Kind: "course-material", Relations: []resource.Relation{{Kind: "part_of", To: "C1/e1"}}}, true},
		{resource.Resource{Kind: "course-material", Relations: []resource.Relation{{Kind: "part_of", To: "C2/e1"}}}, false},
	} {
		var rec resscope.Record
		fn(&c.r, &rec)
		if rec.MathUnit != c.want {
			t.Errorf("%s %v: MathUnit = %v, want %v", c.r.Kind, c.r.IDs, rec.MathUnit, c.want)
		}
	}
}

func TestOpenTextSignal(t *testing.T) {
	fn := openTextSignal(map[string]bool{"napkin": true})
	for _, c := range []struct {
		src  string
		want bool
	}{{"napkin", true}, {"springer-series", false}} {
		r := resource.Resource{Kind: "textbook", Catalogue: []resource.CatalogueRecord{{Source: c.src}}}
		var rec resscope.Record
		fn(&r, &rec)
		if rec.OpenText != c.want || resscope.S5(rec) != c.want {
			t.Errorf("%s: OpenText = %v, want %v", c.src, rec.OpenText, c.want)
		}
	}
}
