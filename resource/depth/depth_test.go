package depth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/institutions"
)

func full() resource.Resource {
	return resource.Resource{
		ID: "8F2K1QW7ZJ", Kind: "monograph", Title: "A book", Date: "1976",
		Creators:  []resource.Creator{{Name: "Walter Rudin", Role: "author"}},
		MSC:       []resource.Code{{Code: "26-01", From: "zbmath"}},
		Catalogue: []resource.CatalogueRecord{{Source: "openlibrary", NativeID: "OL1W"}},
		Expressions: []resource.Expression{{Key: "e1", Kind: "edition", Language: "en", Structure: "8F2K1QW7ZJ/e1.json",
			Manifestations: []resource.Manifestation{{Key: "m1", Kind: "print", Access: &resource.Access{Class: "record"}}}}},
		Scope: &resource.Scope{Value: resource.In},
	}
}

func TestDepth(t *testing.T) {
	r := full()
	for _, c := range []struct {
		m    Mentions
		want int
	}{{Mentions{}, 2}, {Mentions{Total: 2, WithAt: 1}, 2}, {Mentions{Total: 2, WithAt: 2, Decided: 1}, 3}, {Mentions{Total: 2, WithAt: 2, Decided: 2}, 4}} {
		if got := Depth(&r, c.m); got != c.want {
			t.Errorf("depth %d with %+v, want %d", got, c.m, c.want)
		}
	}
	r.Expressions[0].Structure = ""
	if got := Depth(&r, Mentions{}); got != 1 {
		t.Errorf("depth %d with no structure", got)
	}
	r.Expressions[0].Manifestations[0].Access = nil
	if got := Depth(&r, Mentions{}); got != 0 {
		t.Errorf("depth %d with no access", got)
	}
	r.Catalogue = nil
	if got := Depth(&r, Mentions{}); got != -1 {
		t.Errorf("depth %d with no catalogue record", got)
	}
}

func TestTier(t *testing.T) {
	open := map[string]bool{"napkin": true}
	r := full()
	if got := Tier(&r, false, open); got != T4 {
		t.Errorf("tier %s of a monograph", got)
	}
	if got := Tier(&r, true, open); got != T1 {
		t.Errorf("tier %s of a named work", got)
	}
	r.Kind = "textbook"
	if got := Tier(&r, false, open); got != T2 {
		t.Errorf("tier %s of a textbook", got)
	}
	r.Catalogue = append(r.Catalogue, resource.CatalogueRecord{Source: "napkin"})
	if got := Tier(&r, false, open); got != T1 {
		t.Errorf("tier %s of an open textbook", got)
	}
	p := resource.Resource{Kind: "article", Expressions: []resource.Expression{{Manifestations: []resource.Manifestation{{Access: &resource.Access{Class: "open"}}}}}, Scope: &resource.Scope{Value: resource.In}}
	if got := Tier(&p, false, open); got != T3 {
		t.Errorf("tier %s of an open paper", got)
	}
	p.Scope.Value = resource.Border
	if got := Tier(&p, false, open); got != T6 {
		t.Errorf("tier %s of a border paper", got)
	}
	if got := Tier(&resource.Resource{Kind: "course", Scope: &resource.Scope{Value: resource.In}}, false, open); got != T5 {
		t.Errorf("tier %s of a course", got)
	}
}

func TestCompute(t *testing.T) {
	a := full()
	b := full()
	b.ID, b.Kind = "8F2AAAAAA3", "article"
	c := full()
	c.ID, c.Scope = "8F2BBBBBB6", &resource.Scope{Value: resource.Out}
	in := Input{
		Resources: []resource.Resource{b, a, c},
		Named:     map[string]bool{a.ID: true},
		Previous:  map[string]Row{a.ID: {ID: a.ID, Tier: T1, Below: "2026-01-01"}, b.ID: {ID: b.ID, Tier: T2}},
		Day:       "2026-10-31",
	}
	rows := Compute(in)
	if len(rows) != 2 || rows[0].ID != b.ID || rows[1].ID != a.ID {
		t.Fatalf("rows %+v", rows)
	}
	if r := rows[0]; r.Tier != T2 || r.Target != 3 || r.Depth != 2 || r.Below != "2026-10-31" || r.Late("2026-10-31") {
		t.Errorf("the tier moved down by itself, or the gap is wrong: %+v", r)
	}
	if r := rows[1]; r.Tier != T1 || r.Below != "2026-01-01" || !r.Late("2026-10-31") {
		t.Errorf("row %+v", r)
	}
	in.Decisions = map[string]string{b.ID: T4}
	if r := Compute(in)[0]; r.Tier != T4 || r.Below != "" {
		t.Errorf("the decision of a person does not win: %+v", r)
	}
}

func TestBuild(t *testing.T) {
	root := t.TempDir()
	a := full()
	rows, err := Build(root, "2026-10-31", []resource.Resource{a}, nil)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows %+v, %v", rows, err)
	}
	dir := filepath.Join(root, "records", "mentions", "x")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	line := `{"id":"m2://rec/x/a#1","source":"x","kind":"theorem","resource":"8F2K1QW7ZJ/e1","at":{"number":"1.1"},"class":"record","by":"t"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "a.jsonl"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root)
	if err != nil || got[a.ID].Depth != 2 || got[a.ID].Below != "" {
		t.Fatalf("read %+v, %v", got, err)
	}
	rows, err = Build(root, "2026-11-01", []resource.Resource{a}, nil)
	if err != nil || rows[0].Depth != 3 {
		t.Errorf("rows %+v, %v", rows, err)
	}
}

func TestComputeCourseTier(t *testing.T) {
	course := func(id, ror string) resource.Resource {
		return resource.Resource{ID: id, Kind: "course", Scope: &resource.Scope{Value: resource.In}, IDs: resource.IDs{"course": {ror + ":MATH101"}}}
	}
	in := Input{
		Resources:    []resource.Resource{course("8F2CCCCCC1", "01a"), course("8F2CCCCCC2", "03c"), course("8F2CCCCCC3", "04d"), course("8F2CCCCCC4", "09z")},
		Institutions: map[string]string{"01a": institutions.I1, "03c": institutions.I3, "04d": institutions.I4},
		Day:          "2026-10-31",
	}
	want := []string{T5, T4, T4, T5}
	for i, r := range Compute(in) {
		if r.Tier != want[i] || r.Target != Targets[want[i]] {
			t.Errorf("course %s has the tier %s and the target %d, want %s", r.ID, r.Tier, r.Target, want[i])
		}
	}
}
