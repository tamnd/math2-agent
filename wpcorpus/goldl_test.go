package wpcorpus

import "testing"

func TestDrawGoldL(t *testing.T) {
	var as []*CensusArticle
	for p := 1; p <= 300; p++ {
		as = append(as, &CensusArticle{Page: p, Title: "T", Type: Types[p%len(Types)], Census: &Census{Revision: 10}})
	}
	edges := func(page int) (*EdgeFile, error) {
		f := &EdgeFile{Page: page, Revision: 10, Edges: []Edge{
			{From: MentionID(page, ""), To: MentionID(1, ""), Method: "link-in-lead", Predicate: "uses"},
			{From: MentionID(page, "AB12"), Target: "Category:X", Method: "link-in-item", Predicate: "uses", Span: [2]int{1, 2}},
		}}
		switch {
		case page%10 == 0:
			f.Revision = 9 // a stale edge file
		case page%7 == 0:
			f.Edges = f.Edges[:1] // no link in an item
		}
		return f, nil
	}
	gs, err := DrawGoldL(as, edges, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(gs) != GoldLSize {
		t.Fatalf("%d articles, want %d", len(gs), GoldLSize)
	}
	for _, g := range gs {
		if g.Page%10 == 0 || g.Page%7 == 0 {
			t.Errorf("page %d has no current link in an item", g.Page)
		}
		if len(g.Links) != 1 || g.Links[0].To != "Category:X" || g.Links[0].Method != "link-in-item" {
			t.Errorf("page %d links %+v", g.Page, g.Links)
		}
	}
}
