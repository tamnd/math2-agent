package courses

import (
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/institutions"
)

func course(id, courseID, source string, offerings int) resource.Resource {
	r := resource.Resource{ID: id, Kind: "course", Catalogue: []resource.CatalogueRecord{{Source: source}}}
	if courseID != "" {
		r.IDs = resource.IDs{"course": {courseID}}
	}
	for range offerings {
		r.Expressions = append(r.Expressions, resource.Expression{})
	}
	return r
}

func TestCount(t *testing.T) {
	ins := []institutions.Institution{
		{Name: "MIT OpenCourseWare", Level: institutions.Platform, Tier: institutions.I1, Catalogue: &institutions.Catalogue{URL: "u", Format: "ocw"}},
		{ROR: "a", Level: institutions.University, Country: "US", Tier: institutions.I2},
		{ROR: "b", Level: institutions.University, Country: "US", Tier: institutions.I2},
		{ROR: "c", Level: institutions.University, Country: "US", Tier: institutions.I3},
		{ROR: "d", Level: institutions.University, Country: "US", Tier: institutions.I4},
		{ROR: "e", Level: institutions.University, Country: "FR", Tier: institutions.I4},
		{ROR: "f", Level: institutions.College, Country: "US", Tier: institutions.I4},
		{ROR: "g", Level: institutions.University, Country: "US", Tier: institutions.I4, Status: "withdrawn"},
	}
	rs := []resource.Resource{
		course("r1", "x:18.06", "ocw", 3),
		course("r2", "a:M1", "manual", 1),
		course("r3", "a:M2", "manual", 1),
		course("r4", "b:M1", "manual", 1),
		course("r5", "b:M2", "manual", 1),
		course("r6", "b:M3", "manual", 1),
		course("r7", "b:M4", "manual", 1),
		course("r8", "c:M1", "manual", 1),
		{ID: "r9", Kind: "book", Relations: []resource.Relation{{Kind: "based_on", To: "r2/e1"}, {Kind: "cites", To: "r10"}}},
		{ID: "r10", Kind: "book"},
	}
	n := Count(rs, ins)
	if n.I1Courses != 1 || n.I1Offerings != 3 || n.I2I3Courses != 7 || n.Named != 1 {
		t.Fatalf("Count = %+v", n)
	}
	// d and e use the US universities a and b with 2 and 4 courses, e by the fallback to each country; f has no group.
	if e := n.I4; e.Institutions != 3 || e.Estimated != 2 || e.Courses != 6 || e.Low != 5 || e.High != 7 {
		t.Fatalf("Estimate = %+v", e)
	}
}
