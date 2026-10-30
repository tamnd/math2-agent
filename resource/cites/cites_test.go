package cites

import (
	"context"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/resolve"
)

var rudin = resource.Resource{
	ID: "8F2K1QW7ZJ", Kind: "textbook", Title: "Principles of Mathematical Analysis", Date: "1976",
	Creators:    []resource.Creator{{Name: "Walter Rudin", Role: "author"}},
	Expressions: []resource.Expression{{Key: "e3", Kind: "edition", Label: "3rd edition", IDs: resource.IDs{"isbn": {"9780070542358"}}}},
	Scope:       &resource.Scope{Value: resource.In},
}

var paper = resource.Resource{
	ID: "8F2AAAAAA3", Kind: "article", Title: "On primes",
	IDs: resource.IDs{"doi": {"10.1000/p"}},
	Expressions: []resource.Expression{
		{Key: "e1", Kind: "version", Manifestations: []resource.Manifestation{{Key: "m1", Kind: "pdf", Access: &resource.Access{Class: "open"}}}},
		{Key: "e2", Kind: "version", IDs: resource.IDs{"doi": {"10.1000/q"}}},
	},
}

func TestFromCrossref(t *testing.T) {
	c := FromCrossref(CrossrefRef{Unstructured: "W. Rudin, Principles of Mathematical Analysis, 3rd ed., McGraw-Hill, 1976.", ISBN: "0-07-054235-X"})
	if c.Year != 1976 || c.Edition != 3 || len(c.IDs) != 1 || c.IDs[0] != "isbn:9780070542358" {
		t.Errorf("citation %+v", c)
	}
	c = FromCrossref(CrossrefRef{Author: "Rudin", ArticleTitle: "Real and Complex Analysis", Year: "1987", JournalTitle: "J", Volume: "3", FirstPage: "11-20", DOI: "10.1000/ABC"})
	if c.Title != "Real and Complex Analysis" || c.Authors[0] != "Rudin" || c.Year != 1987 || c.Venue != "J" || c.Pages != "11-20" || c.IDs[0] != "doi:10.1000/abc" || c.Text != "Rudin, Real and Complex Analysis, J, 3, 1987" {
		t.Errorf("citation %+v", c)
	}
}

func TestCitingOpen(t *testing.T) {
	if f := Citing(&paper, "doi", "10.1000/q"); f != "8F2AAAAAA3/e2" {
		t.Errorf("citing %q", f)
	}
	if f := Citing(&paper, "doi", "10.1000/p"); f != "8F2AAAAAA3/e1" {
		t.Errorf("citing %q", f)
	}
	if f := Citing(&resource.Resource{ID: "X"}, "doi", "10.1000/p"); f != "" {
		t.Errorf("citing %q with no expression", f)
	}
	if !Open(&paper, "8F2AAAAAA3/e1") || Open(&paper, "8F2AAAAAA3/e2") {
		t.Error("the open class is wrong")
	}
}

func TestEdges(t *testing.T) {
	m := &Matcher{Resolver: &resolve.Resolver{Index: resolve.Build([]resource.Resource{rudin, paper})}, Decisions: map[string]string{Target("8F2AAAAAA3/e1", 3): "8F2K1QW7ZJ"}}
	cs := []resolve.Citation{
		FromCrossref(CrossrefRef{Unstructured: "W. Rudin, Principles of Mathematical Analysis, 3rd ed., McGraw-Hill, 1976."}),
		FromCrossref(CrossrefRef{Unstructured: "A. Nobody, A book that no one has, 1990."}),
		FromCrossref(CrossrefRef{Unstructured: "Some text"}),
	}
	es, weak, err := m.Edges(context.Background(), "8F2AAAAAA3/e1", cs, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 3 || len(weak) != 0 {
		t.Fatalf("edges %+v, weak %+v", es, weak)
	}
	if e := es[0]; e.N != 1 || e.To != "8F2K1QW7ZJ/e3" || e.Score == 0 || e.Matcher != resolve.Version || e.Text == "" || e.Parsed == nil || e.Parsed.Year != "1976" {
		t.Errorf("edge %+v", e)
	}
	if e := es[1]; e.To != "" || e.Matcher != resolve.Version {
		t.Errorf("edge %+v", e)
	}
	if e := es[2]; e.To != "8F2K1QW7ZJ" || e.Matcher != PersonMatcher {
		t.Errorf("edge %+v", e)
	}
	es, _, _ = m.Edges(context.Background(), "8F2AAAAAA3/e2", cs[:1], false)
	if es[0].Text != "" {
		t.Errorf("the edge of a closed expression keeps the text: %+v", es[0])
	}
	got := Merge([]resource.Cite{{From: "8F2AAAAAA3/e2", N: 1}, {From: "8F2K1QW7ZJ/e3", N: 1}}, es, map[string]bool{"8F2AAAAAA3/e2": true})
	if len(got) != 2 || got[0].From != "8F2K1QW7ZJ/e3" || got[1].To == "" {
		t.Errorf("merge %+v", got)
	}
}
