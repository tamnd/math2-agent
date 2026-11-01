package zbsearch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

func doc(t *testing.T, s string) Doc {
	t.Helper()
	var d Doc
	if err := json.Unmarshal([]byte(s), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestBuild(t *testing.T) {
	in := &resource.Scope{Value: resource.In}
	lawler := []resource.Creator{{Name: "Gregory F. Lawler", Role: "author"}}
	rs := []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "monograph", Title: "Random Walk and the Heat Equation", Date: "2010", Creators: lawler, Scope: in, Expressions: []resource.Expression{{Key: "e1", IDs: resource.IDs{"doi": {"10.1090/stml/055"}, "isbn": {"9780821848296"}}}}},
		{ID: "8F2BBBBBB6", Kind: "edited-volume", Title: "Recent Progress in Algebra", Date: "1999", Scope: in, IDs: resource.IDs{"isbn": {"9780821809723"}}},
		{ID: "8F2CCCCCC9", Kind: "monograph", Title: "Wrong Year", Date: "1990", Creators: lawler, Scope: in, IDs: resource.IDs{"doi": {"10.1/c"}}},
		{ID: "8F2DDDDDD2", Kind: "monograph", Title: "Has All", Date: "1990", MSC: []resource.Code{{Code: "60-01"}}, Scope: in, Expressions: []resource.Expression{{Key: "e1", Language: "en"}}, IDs: resource.IDs{"doi": {"10.1/d"}}},
		{ID: "8F2EEEEEE5", Kind: "monograph", Title: "Out", Date: "1990", Scope: &resource.Scope{Value: resource.Out}, IDs: resource.IDs{"doi": {"10.1/e"}}},
		{ID: "8F2FFFFFF8", Kind: "monograph", Title: "No Id", Date: "1990", Scope: in},
		{ID: "8F2GGGGGG1", Kind: "monograph", Title: "No Codes", Date: "1990", Scope: in, IDs: resource.IDs{"doi": {"10.1/g"}}},
	}
	docs := map[string]string{
		"ti:random walk and the heat equation & py:2009-2011 & au:lawler": `{"id":1,"identifier":"1213.60001","title":{"title":"Random walk and the heat equation"},"year":"2010","language":{"languages":["English"]},"msc":[{"code":"60-01","scheme":"msc2020"},{"code":"60G50","scheme":"msc2020"},{"code":"60-01","scheme":"msc2020"}],"contributors":{"authors":[{"name":"Lawler, Gregory F."}]}}`,
		"ti:recent progress in algebra & py:1998-2000":                    `{"id":2,"identifier":"0920.00000","title":{"title":"Recent progress in algebra. Proceedings of the conference"},"year":"1999","language":{"languages":["English"]},"msc":[{"code":"00B20","scheme":"msc2020"}],"contributors":{"editors":[{"name":"Hahn, Sang Geun"}]}}`,
		"ti:wrong year & py:1989-1991 & au:lawler":                        `{"id":3,"identifier":"0000.00003","title":{"title":"Wrong year"},"year":"1993","language":{"languages":["English"]},"msc":[{"code":"60-01","scheme":"msc2020"}]}`,
		"ti:no codes & py:1989-1991":                                      `{"id":4,"identifier":"0000.00004","title":{"title":"No codes"},"year":"1990","language":{"languages":["Klingon"]},"msc":[{"code":"xyz","scheme":"msc2020"}]}`,
	}
	var asked []string
	search := func(_ context.Context, q string) ([]Doc, error) {
		asked = append(asked, q)
		if s, ok := docs[q]; ok {
			return []Doc{doc(t, s)}, nil
		}
		return nil, nil
	}
	recs, n, err := Build(context.Background(), rs, search)
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 5 || asked[3] != "ti:wrong year & py:1800-1988 & au:lawler" || n.Asked != 4 || n.Found != 2 || len(recs) != 2 {
		t.Fatalf("asked %q, counts %+v, records %+v", asked, n, recs)
	}
	a := recs[0]
	if a.NativeID != "1" || a.Kind != "monograph" || a.Language != "en" || strings.Join(a.MSC, " ") != "60-01 60G50" || a.IDs["doi"][0] != "10.1090/stml/055" || a.IDs["zbl"][0] != "1213.60001" || len(a.Creators) != 0 {
		t.Errorf("record %+v", a)
	}
	b := recs[1]
	if b.IDs["isbn"][0] != "9780821809723" || len(b.Creators) != 1 || b.Creators[0].Role != "editor" || b.Date != "1999" {
		t.Errorf("record %+v", b)
	}
}

func TestMatchesCreator(t *testing.T) {
	r := &resource.Resource{Title: "Basic Quadratic Forms", Date: "2008", Creators: []resource.Creator{{Name: "Larry J. Gerstein"}}}
	d := doc(t, `{"title":{"title":"Basic quadratic forms"},"year":"2008","contributors":{"authors":[{"name":"Gerstein, Larry J."}]}}`)
	if !matches(&d, r) {
		t.Error("the same work does not match")
	}
	d.Contributors.Authors[0].Name = "Lang, Serge"
	if matches(&d, r) {
		t.Error("another author matches")
	}
}

func TestMatchesAuthorReference(t *testing.T) {
	r := &resource.Resource{Title: "Geometrical Approaches to Differential Equations", Date: "1980", Creators: []resource.Creator{{Name: "Rodolfo Martini", Role: "editor"}}}
	d := doc(t, `{"title":{"title":"Geometrical approaches to differential equations"},"year":"1980","contributors":{"authors":[],"author_references":[{"name":"Martini, R."}],"editors":[]}}`)
	if !matches(&d, r) {
		t.Error("a name in the author references does not match")
	}
}

func TestMatchesYearAndTitle(t *testing.T) {
	r := &resource.Resource{Title: "Symmetry for Elliptic PDEs", Date: "2010", Creators: []resource.Creator{{Name: "Alberto Farina"}}}
	d := doc(t, `{"title":{"title":"Symmetry for elliptic PDEs. 30 years after a conjecture of De Giorgi"},"year":"2011","contributors":{"editors":[{"name":"Farina, Alberto"}]}}`)
	if !matches(&d, r) {
		t.Error("the title of a proceedings one year later does not match")
	}
	d.Year = "2012"
	if matches(&d, r) {
		t.Error("a document two years later matches")
	}
	d.Year = "2010"
	d.Title.Title = "Symmetry for elliptic PDEsX"
	if matches(&d, r) {
		t.Error("a title that is not at a word boundary matches")
	}
	if sameTitle("Groups", "Groups and rings") {
		t.Error("a title of one word starts another title")
	}
}

func TestNeedsHeld(t *testing.T) {
	r := &resource.Resource{Scope: &resource.Scope{Value: resource.In}, MSC: []resource.Code{{Code: "60-01"}}, Expressions: []resource.Expression{{Key: "e1", Language: "en"}}}
	if needs(r) {
		t.Error("a resource with codes and a language needs the search")
	}
	r.Catalogue = []resource.CatalogueRecord{{Source: Source, NativeID: "1"}}
	if !needs(r) {
		t.Error("a resource that a record of the last run is on does not need the search")
	}
}

func TestPlainTitle(t *testing.T) {
	r := &resource.Resource{Title: "Introduction to Some Methods of Algebraic 𝐾-Theory", Date: "1974-12-31", Creators: []resource.Creator{{Name: "Hyman Bass"}}}
	if q := query(r); !strings.Contains(q, "ti:introduction to some methods of algebraic k theory") {
		t.Errorf("query %q", q)
	}
	d := doc(t, `{"title":{"title":"Introduction to some methods of algebraic K-theory"},"year":"1974","contributors":{"authors":[{"name":"Bass, Hyman"}]}}`)
	if !matches(&d, r) {
		t.Error("a title with a math italic letter does not match")
	}
}

func TestSameName(t *testing.T) {
	for _, c := range [][2]string{{"van Neerven, Jan", "Jan van Neerven"}, {"Fernández López, Antonio", "Antonio Fernández López"}, {"De Boor, Carl", "Carl de Boor"}, {"Martinez-Maure, Yves", "Yves Martinez-Maure"}, {"Marsh, Duncan", "Marsh Duncan"}, {"Gerstein, Larry J.", "Larry J. Gerstein"}} {
		if !sameName(c[0], c[1]) {
			t.Errorf("%q and %q are not the same name", c[0], c[1])
		}
	}
	if sameName("Lang, Serge", "Larry J. Gerstein") || sameName("", "Larry J. Gerstein") {
		t.Error("another name is the same name")
	}
}

func TestTitleTeX(t *testing.T) {
	if !sameTitle(`Mixed twistor \(\mathcal D\)-modules`, "Mixed Twistor D-modules") {
		t.Error("a title with a command of TeX is not the same title")
	}
}

func TestBuildReprint(t *testing.T) {
	in := &resource.Scope{Value: resource.In}
	rs := []resource.Resource{{ID: "8F2HHHHHH4", Kind: "monograph", Title: "Rings, Modules, and Algebras in Stable Homotopy Theory", Date: "2007", Creators: []resource.Creator{{Name: "A. Elmendorf"}}, Scope: in, IDs: resource.IDs{"doi": {"10.1090/surv/047"}}}}
	docs := map[string]string{
		"ti:rings modules and algebras in stable homotopy theory & py:1800-2005 & au:elmendorf": `[{"id":5,"identifier":"0894.55001","title":{"title":"Rings, modules, and algebras in stable homotopy theory"},"year":"1997","msc":[{"code":"55-02","scheme":"msc2020"}],"contributors":{"authors":[{"name":"Elmendorf, A. D."}]}},{"id":6,"identifier":"0000.00006","title":{"title":"Rings, modules, and algebras in stable homotopy theory"},"year":"1995","msc":[{"code":"55P42","scheme":"msc2020"}],"contributors":{"authors":[{"name":"Elmendorf, A. D."}]}},{"id":7,"identifier":"0000.00007","title":{"title":"Rings, modules, and algebras in stable homotopy theory. A survey"},"year":"1999","msc":[{"code":"55-01","scheme":"msc2020"}],"contributors":{"authors":[{"name":"Elmendorf, A. D."}]}}]`,
	}
	search := func(_ context.Context, q string) ([]Doc, error) {
		s, ok := docs[q]
		if !ok {
			return nil, nil
		}
		var ds []Doc
		if err := json.Unmarshal([]byte(s), &ds); err != nil {
			t.Fatal(err)
		}
		return ds, nil
	}
	recs, n, err := Build(context.Background(), rs, search)
	if err != nil {
		t.Fatal(err)
	}
	if n.Found != 1 || len(recs) != 1 || recs[0].NativeID != "5" {
		t.Fatalf("counts %+v, records %+v", n, recs)
	}
}
