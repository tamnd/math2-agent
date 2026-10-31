package cli

import (
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/institutions"
	resscope "github.com/tamnd/math2-agent/resource/scope"
)

func TestCourseSignal(t *testing.T) {
	if courseSignal(nil) != nil {
		t.Fatal("an empty register gives a signal")
	}
	ins, _ := institutions.Build(nil, nil, nil)
	fn := signals(nil, courseSignal(ins))
	for _, c := range []struct {
		r    resource.Resource
		want bool
	}{
		{resource.Resource{Kind: "course", IDs: resource.IDs{"course": {"042nb2s44:18.06"}}, Catalogue: []resource.CatalogueRecord{{Source: "ocw"}}}, true},
		{resource.Resource{Kind: "course", IDs: resource.IDs{"course": {"042nb2s44:8.01"}}, Catalogue: []resource.CatalogueRecord{{Source: "ocw"}}}, false},
		{resource.Resource{Kind: "course", Catalogue: []resource.CatalogueRecord{{Source: "nptel"}}}, true},
		{resource.Resource{Kind: "course-material", Catalogue: []resource.CatalogueRecord{{Source: "nptel"}}}, false},
		{resource.Resource{Kind: "course", Catalogue: []resource.CatalogueRecord{{Source: "oyc"}}}, false},
	} {
		var rec resscope.Record
		fn(&c.r, &rec)
		if rec.MathUnit != c.want {
			t.Errorf("%s %v: MathUnit = %v, want %v", c.r.Kind, c.r.IDs, rec.MathUnit, c.want)
		}
	}
}
