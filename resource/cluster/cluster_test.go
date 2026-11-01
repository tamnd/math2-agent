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

func TestThesisLevel(t *testing.T) {
	nnt := resource.IDs{"nnt": {"2020UPASS008"}}
	res := run(t, Input{Records: []Record{
		{Source: "thesesfr", NativeID: "2020UPASS008", Kind: "thesis", Thesis: "doctoral", Title: "Limites d'ensembles quasiminimaux", Creators: author("Camille Labourie"), Date: "2020-01-06", IDs: nnt},
		{Source: "hal", NativeID: "tel-02500000", Kind: "thesis", Title: "Limites d'ensembles quasiminimaux", Creators: author("Camille Labourie"), Date: "2020-01-06", IDs: nnt},
	}})
	rs := live(res)
	if len(rs) != 1 || rs[0].Kind != "thesis" || rs[0].Level != "doctoral" {
		t.Fatalf("got %+v", rs)
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

func TestPersonMerge(t *testing.T) {
	res := run(t, Input{
		Records: []Record{
			{Source: "openalex", NativeID: "W1", Kind: "article", Title: "Spectral gaps of random graphs", Creators: author("Erdos, Paul"), Date: "1990"},
			{Source: "crossref", NativeID: "c1", Kind: "article", Title: "Spectral gaps for random regular graphs", Creators: author("Paul Erdos"), Date: "1990"},
			{Source: "hal", NativeID: "h1", Kind: "article", Title: "Spectral gaps of random graphs", Creators: author("P. Erdos"), Date: "1991"},
		},
		Decisions: []Decision{{Kind: "merge", A: "openalex:W1", B: "crossref:c1"}},
	})
	if n := len(live(res)); n != 1 {
		t.Fatalf("got %d resources, want 1", n)
	}
	if res.Joins[Person] != 1 || len(res.Pairs) != 0 {
		t.Errorf("got joins %v and pairs %v", res.Joins, res.Pairs)
	}
}

func TestPersonMergeWinsOverGuard(t *testing.T) {
	doi := resource.IDs{"doi": {"10.1000/book"}}
	res := run(t, Input{
		Records: []Record{
			{Source: "crossref", NativeID: "a", Kind: "edited-volume", Title: "Topics in Analysis", IDs: doi},
			{Source: "zbmath", NativeID: "b", Kind: "chapter", Title: "A lemma on measures", Creators: author("Noether, Emmy"), IDs: doi},
		},
		Decisions: []Decision{{Kind: "merge", A: "crossref:a", B: "zbmath:b"}},
	})
	if n := len(live(res)); n != 1 || len(res.Pairs) != 0 {
		t.Fatalf("got %d resources and pairs %v, want 1 and none", n, res.Pairs)
	}
}

func TestPersonSplit(t *testing.T) {
	old := resource.Resource{ID: "aaaaaaaa", Kind: "article", Title: "Quadratic forms", History: []resource.Event{{Op: "add", At: "2026-01-01", By: "test"}}, Catalogue: []resource.CatalogueRecord{
		{Source: "zbmath", Snapshot: "s0", NativeID: "1", Level: "expression"},
		{Source: "zbmath", Snapshot: "s0", NativeID: "2", Level: "expression"},
	}}
	recs := []Record{
		{Source: "zbmath", NativeID: "1", Kind: "article", Title: "Quadratic forms", Creators: author("Gauss, C. F."), Language: "en", IDs: resource.IDs{"doi": {"10.1000/q"}}},
		{Source: "zbmath", NativeID: "2", Kind: "article", Title: "Quadratic forms", Creators: author("Gauss, C. F."), Language: "en", IDs: resource.IDs{"doi": {"10.1000/q"}}},
	}
	res := run(t, Input{Records: recs, Existing: []resource.Resource{old}, Order: map[string]int{"aaaaaaaa": 1}, Decisions: []Decision{{Kind: "split", A: "zbmath:2", B: "zbmath:1"}}})
	rs := live(res)
	if len(rs) != 2 {
		t.Fatalf("got %d resources, want 2", len(rs))
	}
	var keep, split *resource.Resource
	for i := range rs {
		if rs[i].ID == "aaaaaaaa" {
			keep = &rs[i]
		} else {
			split = &rs[i]
		}
	}
	if keep == nil || split == nil || keep.Catalogue[0].NativeID != "2" || split.Catalogue[0].NativeID != "1" {
		t.Fatalf("got %+v", rs)
	}
	if last := split.History[len(split.History)-1]; last.Op != "split" || last.Note != "split from aaaaaaaa" {
		t.Errorf("got history %+v", split.History)
	}
	if len(res.Pairs) != 0 {
		t.Errorf("got pairs %v, want none", res.Pairs)
	}
}

func TestJoinedBy(t *testing.T) {
	res := run(t, Input{Records: []Record{
		{Source: "arxiv", NativeID: "2301.00001", Kind: "preprint", Title: "On the zeros of zeta", Creators: author("Ada Lovelace"), Date: "2023", IDs: resource.IDs{"arxiv": {"2301.00001"}}},
		{Source: "zbmath", NativeID: "7700001", Kind: "article", Title: "On the zeros of zeta", Creators: author("Lovelace, Ada"), Date: "2024", IDs: resource.IDs{"arxiv": {"2301.00001"}}},
		{Source: "crossref", NativeID: "c9", Kind: "article", Title: "Another paper on primes", Creators: author("Gauss, C. F."), Date: "1801"},
	}})
	got := res.JoinedBy([][2]string{{"arxiv:2301.00001", "zbmath:7700001"}, {"arxiv:2301.00001", "crossref:c9"}})
	if got[0] != C1 || got[1] != "" {
		t.Errorf("JoinedBy gave %q, want C1 and none", got)
	}
}

func TestCourseOfferings(t *testing.T) {
	instr := []resource.Creator{{Name: "Gilbert Strang", Role: "instructor"}}
	site := func(slug, term, label, year string) Record {
		return Record{Source: "ocw", NativeID: slug, Level: "manifestation", Kind: "course", Title: "Introduction to Linear Algebra", Creators: instr, Date: year, Language: "en", Edition: label, Publisher: "MIT OpenCourseWare",
			IDs: resource.IDs{"course": {"042nb2s44:18.06"}, "offering": {"042nb2s44:18.06:" + term}, "ocw": {slug}}}
	}
	res := run(t, Input{Records: []Record{
		site("18-06-linear-algebra-spring-2010", "2010-spring", "Spring 2010", "2010"),
		site("18-06-linear-algebra-spring-2005", "2005-spring", "Spring 2005", "2005"),
		{Source: "ocw", NativeID: "18-06-linear-algebra-spring-2010/resources/pset1", Level: "manifestation", Kind: "course-material", Title: "Problem set 1", Date: "2010", Language: "en",
			IDs: resource.IDs{"ocw": {"18-06-linear-algebra-spring-2010/resources/pset1"}}, Relations: []resource.Relation{{Kind: "part_of", To: "offering:042nb2s44:18.06:2010-spring", Source: "ocw"}}},
		{Source: "openlibrary", NativeID: "OL1W", Level: "work", Kind: "textbook", Title: "Introduction to Linear Algebra", Creators: author("Gilbert Strang"), Date: "2010", Language: "en"},
	}})
	rs := live(res)
	if len(rs) != 3 {
		t.Fatalf("%d resources, want the course, the material and the book apart: %+v", len(rs), rs)
	}
	var course, material *resource.Resource
	for i := range rs {
		switch rs[i].Kind {
		case "course":
			course = &rs[i]
		case "course-material":
			material = &rs[i]
		}
	}
	if course == nil || material == nil {
		t.Fatalf("the resources are %+v", rs)
	}
	if course.IDs.Get("course") != "042nb2s44:18.06" || len(course.Expressions) != 2 {
		t.Fatalf("the course is %+v", course)
	}
	spring := ""
	for _, e := range course.Expressions {
		if e.Kind != "offering" || len(e.Contributors) != 1 || len(e.Manifestations) != 1 || e.Manifestations[0].Kind != "page" {
			t.Errorf("the expression is %+v", e)
		}
		if e.IDs.Get("offering") == "042nb2s44:18.06:2010-spring" {
			spring = e.Key
			if e.Label != "Spring 2010" {
				t.Errorf("the label of the offering is %q", e.Label)
			}
		}
	}
	want := resource.Relation{Kind: "part_of", To: course.ID + "/" + spring, Source: Version}
	if spring == "" || len(material.Relations) != 1 || material.Relations[0] != want {
		t.Errorf("the relations of the material are %+v, want %+v", material.Relations, want)
	}
}

func TestResolveTargets(t *testing.T) {
	res := run(t, Input{Records: []Record{
		{Source: "imo", NativeID: "competition", Level: "work", Kind: "competition", Title: "International Mathematical Olympiad", IDs: resource.IDs{"wikidata": {"Q1"}}},
		{Source: "imo", NativeID: "2024", Level: "work", Kind: "problem-set", Title: "IMO 2024", Date: "2024", Relations: []resource.Relation{{Kind: "part_of", To: "wikidata:Q1", Source: "imo"}}},
		{Source: "imo", NativeID: "2023", Level: "work", Kind: "problem-set", Title: "IMO 2023", Date: "2023", Relations: []resource.Relation{{Kind: "part_of", To: "wikidata:Q9", Source: "imo"}}},
		{Source: "wd", NativeID: "Q1", Level: "work", Kind: "competition", Title: "IMO", IDs: resource.IDs{"wikidata": {"Q1"}}, Relations: []resource.Relation{{Kind: "part_of", To: "wikidata:Q1", Source: "wd"}}},
	}})
	by := map[string]resource.Resource{}
	for _, r := range live(res) {
		by[r.Title] = r
	}
	comp, ok := by["International Mathematical Olympiad"]
	if !ok || len(by) != 3 {
		t.Fatalf("the resources are %v", by)
	}
	if rels := by["IMO 2024"].Relations; len(rels) != 1 || rels[0].Kind != "part_of" || rels[0].To != comp.ID {
		t.Errorf("the relations of 2024 are %+v, want part_of %s", rels, comp.ID)
	}
	if rels := by["IMO 2023"].Relations; len(rels) != 0 {
		t.Errorf("the relations of 2023 are %+v, but no resource has Q9", rels)
	}
	if rels := comp.Relations; len(rels) != 0 {
		t.Errorf("the competition has the relations %+v to itself", rels)
	}
}

func TestExpressionGetsLanguage(t *testing.T) {
	old := resource.Resource{ID: "cccccccc", Kind: "textbook", Title: "A book", Catalogue: []resource.CatalogueRecord{{Source: "book", Snapshot: "s0", NativeID: "book", Level: "work"}}, History: []resource.Event{{Op: "add", At: "2026-01-01", By: "test"}},
		Expressions: []resource.Expression{{Key: "e1", Kind: "edition", Structure: "cccccccc/e1.json"}}}
	res := run(t, Input{
		Records:  []Record{{Source: "book", NativeID: "book", Level: "work", Kind: "textbook", Title: "A book", Creators: author("Chen, E."), Language: "en", Date: "2026-10-23"}},
		Existing: []resource.Resource{old},
	})
	rs := live(res)
	if len(rs) != 1 || len(rs[0].Expressions) != 1 {
		t.Fatalf("got %+v", rs)
	}
	if x := rs[0].Expressions[0]; x.Key != "e1" || x.Language != "en" || x.Structure != "cccccccc/e1.json" {
		t.Errorf("got expression %+v", x)
	}
}

func TestDefaultAccess(t *testing.T) {
	open := &resource.Access{Class: "open", Licence: "CC-BY-4.0"}
	old := resource.Resource{ID: "OLD", Kind: "monograph", Title: "Old", Scope: &resource.Scope{Value: resource.In}, Catalogue: []resource.CatalogueRecord{{Source: "x", NativeID: "o"}},
		Expressions: []resource.Expression{{Key: "e1", Kind: "edition", Manifestations: []resource.Manifestation{{Key: "m1", Kind: "print"}, {Key: "m2", Kind: "pdf", Access: open}}}}}
	res := run(t, Input{
		Records: []Record{
			{Source: "springer-series", NativeID: "b", Level: "expression", Kind: "monograph", Title: "Random Walk", Date: "2010", IDs: resource.IDs{"doi": {"10.1/b"}, "isbn": {"9780821848296"}}},
		},
		Existing: []resource.Resource{old},
	})
	for _, r := range live(res) {
		for _, e := range r.Expressions {
			for _, m := range e.Manifestations {
				want := "record"
				if m.Key == "m2" && r.ID == "OLD" {
					want = "open"
				}
				if m.Access == nil || m.Access.Class != want {
					t.Errorf("%s %s %s: access %+v", r.ID, e.Key, m.Key, m.Access)
				}
			}
		}
	}
	if old.Expressions[0].Manifestations[0].Access != nil {
		t.Errorf("the input changed")
	}
}

func TestFillerJoinsNothing(t *testing.T) {
	res := run(t, Input{
		Records: []Record{
			{Source: "springer-series", NativeID: "a", Level: "expression", Kind: "monograph", Title: "Convex Functions", Creators: author("Phelps, R."), Date: "1989-05-01", IDs: resource.IDs{"doi": {"10.1007/a"}}},
			{Source: "springer-series", NativeID: "b", Level: "expression", Kind: "monograph", Title: "Banach Spaces of Analytic Functions", Creators: author("Hoffman, K."), Date: "1993", IDs: resource.IDs{"doi": {"10.1007/b"}}},
			{Source: "zbmath_search", NativeID: "1", Level: "work", Kind: "monograph", Title: "Convex functions", Date: "1989", Language: "en", MSC: []string{"46-02"}, IDs: resource.IDs{"doi": {"10.1007/a"}, "zbl_id": {"1"}}},
			{Source: "zbmath_search", NativeID: "1b", Level: "work", Kind: "monograph", Title: "Convex functions", Language: "en", MSC: []string{"46-02"}, IDs: resource.IDs{"doi": {"10.1007/b"}, "zbl_id": {"1"}}},
			{Source: "zbmath_search", NativeID: "2", Level: "work", Kind: "monograph", Title: "Other", MSC: []string{"46-02"}, IDs: resource.IDs{"doi": {"10.1007/c"}, "zbl_id": {"2"}}},
		},
	})
	rs := live(res)
	if len(rs) != 2 {
		t.Fatalf("got %d resources: %+v", len(rs), rs)
	}
	for _, r := range rs {
		if len(r.MSC) != 1 || len(r.Expressions) != 1 || r.Expressions[0].Language != "en" {
			t.Errorf("got %+v", r)
		}
	}
	if rs[0].Date != "1989-05-01" && rs[1].Date != "1989-05-01" {
		t.Errorf("dates %q %q", rs[0].Date, rs[1].Date)
	}
}

func TestFillerFillsOnly(t *testing.T) {
	res := run(t, Input{
		Records: []Record{
			{Source: "springer-series", NativeID: "b", Level: "expression", Kind: "monograph", Title: "Random Walk and the Heat Equation", Creators: author("Lawler, G."), Date: "2010", IDs: resource.IDs{"doi": {"10.1090/stml/055"}}},
			{Source: "zbmath_search", NativeID: "1", Level: "work", Kind: "monograph", Title: "Random walk and the heat equation", Creators: author("Lawler, Gregory F."), Date: "2010", Language: "en", MSC: []string{"60-01"}, IDs: resource.IDs{"doi": {"10.1090/stml/055"}, "zbl_id": {"1"}}},
		},
	})
	rs := live(res)
	if len(rs) != 1 || len(rs[0].Expressions) != 1 {
		t.Fatalf("got %+v", rs)
	}
	r := rs[0]
	if r.Title != "Random Walk and the Heat Equation" || r.Creators[0].Name != "Lawler, G." || len(r.MSC) != 1 || r.MSC[0].From != "zbmath_search" || r.Expressions[0].Language != "en" {
		t.Errorf("got %+v", r)
	}
}

func TestScoreNumbers(t *testing.T) {
	ed := []resource.Creator{{Name: "Kvant editorial board", Role: "editor"}}
	a := Record{Kind: "problem-set", Title: "Kvant 1982, no. 1", Date: "1982", Creators: ed}
	b := Record{Kind: "problem-set", Title: "Kvant 1982, no. 2", Date: "1982", Creators: ed}
	if s := Score(&a, &b); s != 0 {
		t.Errorf("two issues: score %v", s)
	}
	k := []resource.Creator{{Name: "KöMaL editorial board", Role: "editor"}}
	e := Record{Kind: "problem-set", Title: "KöMaL Problems in Mathematics, September 2014", Date: "2014-09", Creators: k}
	f := Record{Kind: "problem-set", Title: "KöMaL Problems in Mathematics, December 2014", Date: "2014-12", Creators: k}
	if s := Score(&e, &f); s != 0 {
		t.Errorf("two months: score %v", s)
	}
	c := Record{Kind: "book", Title: "Linear algebra", Date: "1982", Creators: ed}
	d := Record{Kind: "book", Title: "Linear algebra, 2nd edition", Date: "1982", Creators: ed}
	if s := Score(&c, &d); s == 0 {
		t.Errorf("a title with no numbers: score %v", s)
	}
}
