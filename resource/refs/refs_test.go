package refs

import (
	"context"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func TestBuild(t *testing.T) {
	in := &resource.Scope{Value: resource.In}
	rs := []resource.Resource{{ID: "8F2AAAAAA3", Scope: in}, {ID: "8F2BBBBBB6", Scope: in}, {ID: "8F2CCCCCC9", Scope: &resource.Scope{Value: resource.Out}}}
	weil := func(year string) *resource.Parsed {
		return &resource.Parsed{Authors: []string{"André Weil"}, Title: "Basic Number Theory", Year: year}
	}
	cs := []resource.Cite{
		{From: "8F2AAAAAA3/e1", N: 1, Parsed: weil("1967")},
		{From: "8F2BBBBBB6/e1", N: 4, Parsed: weil("1968")},
		{From: "8F2CCCCCC9/e1", N: 1, Parsed: weil("1967")},
		{From: "8F2AAAAAA3/e1", N: 2, Parsed: &resource.Parsed{Title: "Lone paper", Authors: []string{"B. Lee"}, Year: "2001", Venue: "Ann. Math.", IDs: resource.IDs{"doi": {"10.4007/annals.2001.1"}}}},
		{From: "8F2AAAAAA3/e1", N: 3, Parsed: &resource.Parsed{Title: "Notes", Authors: []string{"C. Kim"}, Year: "1999"}},
		{From: "8F2AAAAAA3/e1", N: 5, To: "8F2BBBBBB6", Parsed: weil("1967")},
	}
	looked := map[string]bool{}
	lookup := func(_ context.Context, key, v string) (bool, error) {
		looked[key+":"+v] = true
		return key == "doi", nil
	}
	recs, n, err := Build(context.Background(), Input{Resources: rs, Cites: cs, Confirmed: map[string]bool{"8F2AAAAAA3/e1#3": true}, Lookup: lookup})
	if err != nil {
		t.Fatal(err)
	}
	if n.Unresolved != 4 || n.Cited != 1 || n.ByID != 1 || n.Person != 1 || n.Lookups != 1 || len(recs) != 3 {
		t.Fatalf("counts %+v, records %+v", n, recs)
	}
	byPrefix := map[string]int{}
	for _, r := range recs {
		if !Confirmed(r.NativeID) || r.Source != Source {
			t.Errorf("record %+v", r)
		}
		p, _, _ := strings.Cut(r.NativeID, ":")
		byPrefix[p]++
		switch p {
		case "cited":
			if r.Title != "Basic Number Theory" || r.Date != "1967" || r.Kind != "monograph" || r.Creators[0].Name != "André Weil" {
				t.Errorf("cited record %+v", r)
			}
		case "id":
			if r.NativeID != "id:doi:10.4007/annals.2001.1" || r.Kind != "article" || r.IDs["doi"][0] != "10.4007/annals.2001.1" {
				t.Errorf("id record %+v", r)
			}
		case "person":
			if r.NativeID != "person:8F2AAAAAA3/e1#3" {
				t.Errorf("person record %+v", r)
			}
		}
	}
	if len(byPrefix) != 3 || !looked["doi:10.4007/annals.2001.1"] {
		t.Errorf("records %v, lookups %v", byPrefix, looked)
	}
	// One citing resource, or years more than one apart, confirm nothing.
	cs[1].Parsed = weil("1970")
	if _, n, _ := Build(context.Background(), Input{Resources: rs, Cites: cs}); n.Cited != 0 || n.Lookups != 0 {
		t.Errorf("counts %+v", n)
	}
	if Confirmed("x") || !Confirmed("id:doi:10.1/x") {
		t.Error("Confirmed")
	}
}
