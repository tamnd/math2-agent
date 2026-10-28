package wpcorpus

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tamnd/wikipedia-cli/wiki/parsoid"
)

func extractPointers(t *testing.T, in *PointerFile) (*Result, *Manifests) {
	t.Helper()
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("testdata/pages/pointers.wiki")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ExtractPage(&Page{ID: 12345, Revision: 678, Title: "Gadget", Wikitext: string(src)}, m, "wpcorpus-test", in)
	if err != nil {
		t.Fatal(err)
	}
	return r, m
}

func objectOf(r *Result, ref string) *Object {
	for i := range r.Objects {
		if r.Objects[i].Ref == ref {
			return &r.Objects[i]
		}
	}
	return nil
}

func gadgetPointers() *PointerFile {
	return &PointerFile{Page: 12345, Title: "Gadget", Pointers: []Pointer{
		{Via: ViaRedirect, From: "Newton series", Anchor: "Newton's series"},
		{Via: ViaRedirect, From: "Newton sum", Anchor: "Newton's_series"},
		{Via: ViaRedirect, From: "Gadget rule", Anchor: "Rule"},
		{Via: ViaRedirect, From: "Gadget lemma", Anchor: "Lemma"},
		{Via: ViaRedirect, From: "Gadget fact", Anchor: "Facts"},
		{Via: ViaLink, From: "Calculus", Anchor: "Newton's series"},
		{Via: ViaLink, From: "Algebra", Anchor: "history"},
	}}
}

func TestExtractPointers(t *testing.T) {
	r, _ := extractPointers(t, gadgetPointers())
	c := &r.Census
	if len(c.Pointers) != 7 {
		t.Fatalf("pointers = %+v", c.Pointers)
	}
	at := func(i int) *Object { return objectOf(r, c.Pointers[i].Object) }

	// The redirects to a section with no item make one pointer-item, and
	// the link to the section matches it too.
	p := at(0)
	if p == nil || p.Kind != "pointer-item" || p.Name != "Newton series" || p.Section != "Newton's series" || !slices.Equal(p.Aliases, []string{"Newton sum"}) {
		t.Errorf("pointer-item = %+v", p)
	}
	if c.Pointers[1].Object != c.Pointers[0].Object || c.Pointers[5].Object != c.Pointers[0].Object {
		t.Errorf("the pointers to the section match %s, %s and %s", c.Pointers[0].Object, c.Pointers[1].Object, c.Pointers[5].Object)
	}
	cue := slices.IndexFunc(c.Cues, func(q Cue) bool { return q.Cue == "pointer" })
	if cue < 0 || c.Cues[cue].Object != c.Pointers[0].Object {
		t.Errorf("the census has no cue for the pointer-item: %+v", c.Cues)
	}

	// The second name of {{anchor}} matches the section that holds it.
	if o := at(2); o == nil || o.Kind != "section" || o.Name != "Notes" || !slices.Equal(o.Aliases, []string{"Gadget rule"}) {
		t.Errorf("the anchor Rule matches %+v", o)
	}
	// A broken anchor finds the item by the name of the redirect.
	if o := at(3); c.Pointers[3].Finding != FindingBrokenAnchor || o == nil || o.Name != "Gadget lemma" || o.Kind == "section" {
		t.Errorf("the broken anchor Lemma = %+v, object %+v", c.Pointers[3], o)
	}
	if q := c.Pointers[4]; q.Finding != FindingBrokenAnchor || q.Object != "" {
		t.Errorf("the broken anchor Facts = %+v", q)
	}
	// A link to a section with no item matches the section, also when the
	// case of the anchor differs.
	if o := at(6); o == nil || o.Kind != "section" || o.Name != "History" || len(o.Aliases) != 0 {
		t.Errorf("the link to history matches %+v", o)
	}
	if !slices.Equal(c.Witnesses, []string{WitnessText, WitnessIn}) || c.Strength != StrengthNormal {
		t.Errorf("witnesses = %q, strength = %s", c.Witnesses, c.Strength)
	}
	if g := c.PointerGaps(); len(g) != 0 {
		t.Errorf("gaps = %+v", g)
	}
}

func TestPointersStrength(t *testing.T) {
	rd := &parsoid.Reading{Revision: 678, Sections: []parsoid.Section{{ID: 0}, {ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}}, Anchors: []string{"Facts"}}

	// The anchor Facts is in the HTML, so it is not broken, but no object
	// matches it, so the witnesses do not agree.
	r, m := extractPointers(t, gadgetPointers())
	if err := AddWitness(r, m, rd); err != nil {
		t.Fatal(err)
	}
	c := &r.Census
	if q := c.Pointers[4]; q.Finding != FindingTemplateAnchor {
		t.Errorf("the anchor Facts = %+v", q)
	}
	if !slices.Equal(c.Witnesses, []string{WitnessText, WitnessIn, WitnessHTML}) || c.Strength != StrengthNormal {
		t.Errorf("witnesses = %q, strength = %s", c.Witnesses, c.Strength)
	}

	// With each pointer matched, the three witnesses agree.
	in := gadgetPointers()
	in.Pointers = in.Pointers[:4]
	r, m = extractPointers(t, in)
	if err := AddWitness(r, m, rd); err != nil {
		t.Fatal(err)
	}
	if r.Census.Strength != StrengthStrong {
		t.Errorf("strength = %s, differences = %q", r.Census.Strength, r.Census.HTMLDifferences())
	}

	// With no pointers file, the census has no witness W-in.
	r, m = extractPointers(t, nil)
	if err := AddWitness(r, m, rd); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Census.Witnesses, []string{WitnessText, WitnessHTML}) || r.Census.Strength != StrengthNormal || r.Census.Pointers != nil {
		t.Errorf("witnesses = %q, strength = %s", r.Census.Witnesses, r.Census.Strength)
	}
}

func TestLinkPointers(t *testing.T) {
	scope := map[string]string{"Gadget": "Gadget", "Widget": "Widget", "Finite_difference": "Finite_difference"}
	target := func(s string) string { return scope[normTitle(s)] }
	got := LinkPointers("Widget", "See [[Gadget#Newton's series|the series]], [[gadget#Newton's_series]], [[Widget#Self]], [[Gadget]], [[Other#X]] and [[Finite difference#Forward differences]].", target)
	want := map[string][]Pointer{
		"Gadget":            {{Via: ViaLink, From: "Widget", Anchor: "Newton's series"}},
		"Finite_difference": {{Via: ViaLink, From: "Widget", Anchor: "Forward differences"}},
	}
	if len(got) != len(want) {
		t.Fatalf("pointers = %+v", got)
	}
	for k, v := range want {
		if !slices.Equal(got[k], v) {
			t.Errorf("pointers of %s = %+v, want %+v", k, got[k], v)
		}
	}
}

func TestPointersFile(t *testing.T) {
	root := t.TempDir()
	in := gadgetPointers()
	SortPointers(in.Pointers)
	if in.Pointers[0].Via != ViaRedirect || in.Pointers[len(in.Pointers)-1].From != "Calculus" {
		t.Errorf("sorted = %+v", in.Pointers)
	}
	if err := WritePointers(root, in); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPointers(root, 12345)
	if err != nil || got == nil || !slices.Equal(got.Pointers, in.Pointers) || got.Title != "Gadget" {
		t.Fatalf("read = %+v, %v", got, err)
	}
	data, err := os.ReadFile(filepath.Join(root, PointersPath(12345)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("\n# Pointers to Gadget\n\n5 redirects and 2 links of other pages point to an anchor of the article.\n")) {
		t.Errorf("file = %s", data)
	}
	if f, err := ReadPointers(root, 7); f != nil || err != nil {
		t.Errorf("a missing file gives %+v, %v", f, err)
	}
}
