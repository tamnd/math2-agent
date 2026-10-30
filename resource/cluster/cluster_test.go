package cluster

import (
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func author(names ...string) []resource.Creator {
	var out []resource.Creator
	for _, n := range names {
		out = append(out, resource.Creator{Name: n, Role: "author"})
	}
	return out
}

func run(t *testing.T, in Input) *Result {
	t.Helper()
	if in.By == "" {
		in.By = "test"
	}
	if in.Day == "" {
		in.Day = "2026-10-30"
	}
	for i := range in.Records {
		if in.Records[i].Snapshot == "" {
			in.Records[i].Snapshot = "s1"
		}
		if in.Records[i].Level == "" {
			in.Records[i].Level = "expression"
		}
	}
	res, err := Run(in)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func live(res *Result) []resource.Resource {
	var out []resource.Resource
	for _, r := range res.Resources {
		if r.Live() {
			out = append(out, r)
		}
	}
	return out
}

func TestPreprintAndPaper(t *testing.T) {
	res := run(t, Input{Records: []Record{
		{Source: "arxiv", Snapshot: "s1", NativeID: "2301.00001", Level: "expression", Kind: "preprint", Title: "On the zeros of zeta", Creators: author("Ada Lovelace"), Date: "2023-01-02", Language: "en", MSC: []string{"11M06"}, IDs: resource.IDs{"arxiv": {"2301.00001"}}},
		{Source: "zbmath", Snapshot: "s1", NativeID: "7700001", Level: "expression", Kind: "article", Title: "On the zeros of zeta", Creators: author("Lovelace, Ada"), Date: "2024", Language: "en", MSC: []string{"11M26"}, IDs: resource.IDs{"zbl": {"1500.11001"}, "arxiv": {"2301.00001"}, "doi": {"10.1000/z"}}},
	}})
	rs := live(res)
	if len(rs) != 1 {
		t.Fatalf("got %d resources, want 1", len(rs))
	}
	r := rs[0]
	if r.Kind != "article" || r.Date != "2023-01-02" || len(r.Expressions) != 2 || len(r.Catalogue) != 2 {
		t.Fatalf("got kind %s, date %s, %d expressions, %d catalogue records", r.Kind, r.Date, len(r.Expressions), len(r.Catalogue))
	}
	if r.MSC[0].Code != "11M26" || r.MSC[0].From != "zbmath" {
		t.Errorf("got MSC %v, want the code of zbMATH", r.MSC)
	}
	if res.Joins[C1] != 1 {
		t.Errorf("got joins %v", res.Joins)
	}
	if ps := resource.Check(&r); len(ps) > 0 {
		t.Errorf("check: %v", ps)
	}
	again := run(t, Input{Records: []Record{
		{Source: "zbmath", Snapshot: "s1", NativeID: "7700001", Level: "expression", Kind: "article", Title: "On the zeros of zeta", Creators: author("Lovelace, Ada"), Date: "2024", Language: "en", IDs: resource.IDs{"arxiv": {"2301.00001"}}},
		{Source: "arxiv", Snapshot: "s1", NativeID: "2301.00001", Level: "expression", Kind: "preprint", Title: "On the zeros of zeta", Creators: author("Ada Lovelace"), Date: "2023-01-02", Language: "en", IDs: resource.IDs{"arxiv": {"2301.00001"}}},
	}})
	if live(again)[0].ID != r.ID {
		t.Errorf("the provisional id depends on the order of the records")
	}
}

func TestISBNNeedsTitle(t *testing.T) {
	isbn := resource.IDs{"isbn": {"9780000000002"}}
	res := run(t, Input{Records: []Record{
		{Source: "loc", NativeID: "1", Kind: "textbook", Title: "Linear Algebra Done Right", Creators: author("Axler, Sheldon"), Language: "en", IDs: isbn},
		{Source: "openlibrary", NativeID: "OL1M", Kind: "textbook", Title: "A Course in Topology", Creators: author("Smith, John"), Language: "en", IDs: isbn},
		{Source: "dnb", NativeID: "9", Kind: "textbook", Title: "Linear algebra done right", Creators: author("Sheldon Axler"), Language: "en", IDs: isbn},
	}})
	if n := len(live(res)); n != 2 {
		t.Fatalf("got %d resources, want 2", n)
	}
	if res.Joins[C3] != 1 {
		t.Errorf("got joins %v", res.Joins)
	}
}

func TestChapterGuard(t *testing.T) {
	doi := resource.IDs{"doi": {"10.1000/book"}}
	res := run(t, Input{Records: []Record{
		{Source: "crossref", NativeID: "a", Kind: "edited-volume", Title: "Topics in Analysis", IDs: doi},
		{Source: "zbmath", NativeID: "b", Kind: "chapter", Title: "A lemma on measures", Creators: author("Noether, Emmy"), IDs: doi},
	}})
	if n := len(live(res)); n != 2 {
		t.Fatalf("got %d resources, want 2", n)
	}
	if len(res.Pairs) != 1 || res.Pairs[0].Pass != C1 {
		t.Errorf("got pairs %v, want one refused join", res.Pairs)
	}
}

func TestEditions(t *testing.T) {
	res := run(t, Input{Records: []Record{
		{Source: "loc", NativeID: "1", Kind: "textbook", Title: "Real Analysis, 2nd edition", Creators: author("Royden, H. L."), Date: "1968", Language: "en", IDs: resource.IDs{"isbn": {"9780000000019"}}},
		{Source: "loc", NativeID: "2", Kind: "textbook", Title: "Real Analysis", Edition: "3rd ed.", Creators: author("Royden, H. L."), Date: "1988", Language: "en", IDs: resource.IDs{"isbn": {"9780000000026"}}},
		{Source: "bnf", NativeID: "3", Kind: "textbook", Title: "Real analysis", Edition: "3rd ed.", Creators: author("H. L. Royden"), Date: "1988", Language: "fr", IDs: resource.IDs{"isbn": {"9780000000033"}}},
	}})
	rs := live(res)
	if len(rs) != 1 {
		t.Fatalf("got %d resources, want 1", len(rs))
	}
	es := rs[0].Expressions
	if len(es) != 3 || es[0].Label != "2nd edition" || es[2].Kind != "translation" {
		t.Fatalf("got expressions %+v", es)
	}
	if len(es[0].Manifestations) != 1 || es[0].Manifestations[0].IDs["isbn"][0] != "9780000000019" {
		t.Errorf("got manifestations %+v", es[0].Manifestations)
	}
}

func TestStableAndDuplicate(t *testing.T) {
	old := func(id, native string) resource.Resource {
		return resource.Resource{ID: id, Kind: "article", Title: "Old title", Catalogue: []resource.CatalogueRecord{{Source: "zbmath", Snapshot: "s0", NativeID: native, Level: "expression"}}, History: []resource.Event{{Op: "add", At: "2026-01-01", By: "test"}}, Expressions: []resource.Expression{{Key: "e3", Kind: "version", Label: "published", Language: "en", IDs: resource.IDs{"doi": {"10.1000/q"}}}}}
	}
	res := run(t, Input{
		Records: []Record{
			{Source: "zbmath", Snapshot: "s1", NativeID: "1", Level: "expression", Kind: "article", Title: "Quadratic forms", Creators: author("Gauss, C. F."), Language: "en", IDs: resource.IDs{"doi": {"10.1000/q"}}},
			{Source: "zbmath", Snapshot: "s1", NativeID: "2", Level: "expression", Kind: "article", Title: "Quadratic forms", Creators: author("Gauss, C. F."), Language: "en", IDs: resource.IDs{"doi": {"10.1000/q"}}},
		},
		Existing: []resource.Resource{old("bbbbbbbb", "2"), old("aaaaaaaa", "1")},
		Order:    map[string]int{"bbbbbbbb": 1, "aaaaaaaa": 2},
	})
	var keep, dup *resource.Resource
	for i := range res.Resources {
		switch res.Resources[i].ID {
		case "bbbbbbbb":
			keep = &res.Resources[i]
		case "aaaaaaaa":
			dup = &res.Resources[i]
		}
	}
	if keep == nil || dup == nil || !keep.Live() || dup.Live() || dup.Scope.Target != "bbbbbbbb" {
		t.Fatalf("got %+v", res.Resources)
	}
	if keep.Title != "Quadratic forms" || keep.Expressions[0].Key != "e3" {
		t.Errorf("got title %q and key %s", keep.Title, keep.Expressions[0].Key)
	}
	if last := keep.History[len(keep.History)-1]; last.Op != "merge" {
		t.Errorf("got history %+v", keep.History)
	}
}

func TestMiddleBand(t *testing.T) {
	res := run(t, Input{Records: []Record{
		{Source: "openalex", NativeID: "W1", Kind: "article", Title: "Spectral gaps of random graphs", Creators: author("Erdos, Paul"), Date: "1990"},
		{Source: "crossref", NativeID: "c1", Kind: "article", Title: "Spectral gaps for random regular graphs", Creators: author("Paul Erdos"), Date: "1990"},
		{Source: "hal", NativeID: "h1", Kind: "article", Title: "Spectral gaps of random graphs", Creators: author("P. Erdos"), Date: "1991"},
	}})
	if n := len(live(res)); n != 2 {
		t.Fatalf("got %d resources, want 2", n)
	}
	if res.Joins[C4] != 1 || len(res.Pairs) == 0 || res.Pairs[0].Pass != C4 {
		t.Errorf("got joins %v and pairs %v", res.Joins, res.Pairs)
	}
}
