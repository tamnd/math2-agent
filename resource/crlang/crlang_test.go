package crlang

import (
	"context"
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func TestBuild(t *testing.T) {
	works := map[string]*Work{
		"10.1090/a": {DOI: "10.1090/a", Language: "en"},
		"10.1007/b": {DOI: "10.1007/b"},
		"10.1007/c": {DOI: "10.1007/c", Language: "English"},
		"10.1007/e": {DOI: "10.1007/e", Language: "de"},
	}
	var asked []string
	lookup := func(_ context.Context, doi string) (*Work, error) {
		asked = append(asked, doi)
		return works[doi], nil
	}
	in := &resource.Scope{Value: resource.In}
	cat := []resource.CatalogueRecord{{Source: "ams-series", NativeID: "x"}}
	rs := []resource.Resource{
		{ID: "A", Kind: "monograph", Title: "A", Scope: in, Catalogue: cat, IDs: resource.IDs{"doi": {"10.1090/a"}}},
		{ID: "B", Kind: "monograph", Title: "B", Scope: in, Catalogue: cat, IDs: resource.IDs{"doi": {"10.1007/b"}}},
		{ID: "C", Kind: "monograph", Title: "C", Scope: in, Catalogue: cat, IDs: resource.IDs{"doi": {"10.1007/c"}}},
		{ID: "D", Kind: "monograph", Title: "D", Scope: in, Catalogue: cat, IDs: resource.IDs{"doi": {"10.1007/d"}}, Expressions: []resource.Expression{{Key: "e1", Language: "en"}}},
		{ID: "E", Kind: "monograph", Title: "E", Scope: in, Catalogue: cat, Expressions: []resource.Expression{{Key: "e1", IDs: resource.IDs{"doi": {"10.1007/e"}}}}},
		{ID: "F", Kind: "monograph", Title: "F", Scope: in, Catalogue: cat, IDs: resource.IDs{"doi": {"10.1007/f"}}},
		{ID: "G", Kind: "monograph", Title: "G", Scope: in, Catalogue: cat, IDs: resource.IDs{"isbn": {"9780000000002"}}},
	}
	recs, n, err := Build(context.Background(), rs, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if n.Asked != 5 || n.Found != 2 || len(recs) != 2 || len(asked) != 5 {
		t.Fatalf("counts %+v, asked %v, records %+v", n, asked, recs)
	}
	if r := recs[1]; r.Source != Source || r.NativeID != "10.1007/e" || r.Language != "de" || r.IDs["doi"][0] != "10.1007/e" {
		t.Errorf("record %+v", r)
	}
}

func TestURL(t *testing.T) {
	if got := URL(WorksURL, "10.1007/bfb0059573"); got != "https://api.crossref.org/works/10.1007%2Fbfb0059573" {
		t.Errorf("URL = %q", got)
	}
}

func TestNeedsHeld(t *testing.T) {
	r := &resource.Resource{Scope: &resource.Scope{Value: resource.In}, Expressions: []resource.Expression{{Key: "e1", Language: "en"}}}
	if needs(r) {
		t.Error("a resource with a language needs the lookup")
	}
	r.Catalogue = []resource.CatalogueRecord{{Source: Source, NativeID: "10.1/a"}}
	if !needs(r) {
		t.Error("a resource that a record of the last run is on does not need the lookup")
	}
}
