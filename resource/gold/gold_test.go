package gold

import (
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/cluster"
)

func by(name string) []resource.Creator { return []resource.Creator{{Name: name, Role: "author"}} }

func TestValue(t *testing.T) {
	l := func(v, who string) Label { return Label{Value: v, By: who, At: "2026-10-31"} }
	cases := []struct {
		labels []Label
		want   string
	}{
		{nil, ""},
		{[]Label{l(Same, "a")}, ""},
		{[]Label{l(Same, "a"), l(Same, "judge")}, Same},
		{[]Label{l(Same, "a"), l(Different, "b")}, ""},
		{[]Label{l(Same, "a"), l(Different, "b"), l(Different, "c")}, Different},
	}
	for _, c := range cases {
		p := Pair{A: "x:1", B: "y:2", Labels: c.labels}
		if got := p.Value(); got != c.want {
			t.Errorf("Value of %v = %q, want %q", c.labels, got, c.want)
		}
	}
}

func TestReadWriteAndLabel(t *testing.T) {
	root := t.TempDir()
	if ps, err := Read(root, Work); err != nil || ps != nil {
		t.Fatalf("Read of an empty tree = %v, %v", ps, err)
	}
	ps := []Pair{{A: "zbmath:1", B: "crossref:2", Stratum: "article 1990s en title"}}
	if err := AddLabel(ps, "crossref:2", "zbmath:1", Label{Value: Same, By: "ann", At: "2026-10-31"}); err != nil {
		t.Fatal(err)
	}
	if err := AddLabel(ps, "zbmath:1", "crossref:2", Label{Value: Different, By: "ann", At: "2026-10-31"}); err == nil {
		t.Error("AddLabel took a second label of one person")
	}
	if err := AddLabel(ps, "zbmath:1", "crossref:9", Label{Value: Same, By: "bob"}); err == nil {
		t.Error("AddLabel took a pair that is not in the set")
	}
	if err := Write(root, Work, ps); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root, Work)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || len(got[0].Labels) != 1 || got[0].Labels[0].By != "ann" {
		t.Fatalf("Read gave %+v", got)
	}
	if done, open, disputed := Labelled(got); done != 0 || open != 1 || disputed != 0 {
		t.Errorf("Labelled = %d %d %d", done, open, disputed)
	}
	if _, err := Read(root, "edition"); err == nil {
		t.Error("Read took the set edition")
	}
}

func records() []cluster.Record {
	return []cluster.Record{
		{Source: "loc", NativeID: "1", Kind: "textbook", Title: "Lectures on Algebraic Topology", Creators: by("Dold, Albrecht"), Date: "1972", Language: "en"},
		{Source: "loc", NativeID: "2", Kind: "textbook", Title: "Lectures on Algebraic Topology", Creators: by("Massey, William"), Date: "1980", Language: "en"},
		{Source: "bnf", NativeID: "3", Kind: "textbook", Title: "Lecons de topologie algebrique", Creators: by("Dold, Albrecht"), Date: "1975", Language: "fr"},
		{Source: "dnb", NativeID: "4", Kind: "textbook", Title: "Analysis, Band 1", Creators: by("Forster, Otto"), Date: "1976", Language: "de"},
		{Source: "dnb", NativeID: "5", Kind: "textbook", Title: "Analysis, Band 2", Creators: by("Forster, Otto"), Date: "1977", Language: "de"},
		{Source: "arxiv", NativeID: "2301.00001", Kind: "preprint", Title: "On the zeros of zeta", Creators: by("Ada Lovelace"), Date: "2023", IDs: resource.IDs{"arxiv": {"2301.00001"}}},
		{Source: "zbmath", NativeID: "77", Kind: "article", Title: "On the zeros of zeta", Creators: by("Lovelace, Ada"), Date: "2024", IDs: resource.IDs{"arxiv": {"2301.00001"}}},
		{Source: "crossref", NativeID: "c1", Kind: "article", Title: "On the zeros of the zeta function", Creators: by("A. Lovelace"), Date: "2025"},
	}
}

func TestSampleWork(t *testing.T) {
	ps, err := Sample(records(), Work, 20, "s", nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{}
	for _, p := range ps {
		cases[p.Stratum[len(p.Stratum)-len(CaseTitle):]] = true
		switch p.Key() {
		case "loc:1 loc:2":
			if p.Stratum != "textbook 1970s en title" {
				t.Errorf("the stratum of %s is %q", p.Key(), p.Stratum)
			}
		case "dnb:4 dnb:5":
			if p.Stratum != "textbook 1970s de volume" && p.Stratum != "textbook 1970s de block" {
				t.Errorf("the stratum of %s is %q", p.Key(), p.Stratum)
			}
		}
	}
	keys := map[string]bool{}
	for _, p := range ps {
		keys[p.Key()] = true
	}
	for _, want := range []string{"loc:1 loc:2", "bnf:3 loc:1", "dnb:4 dnb:5"} {
		if !keys[want] {
			t.Errorf("the sample has no pair %s: %v", want, ps)
		}
	}
	again, _ := Sample(records(), Work, 20, "s", ps)
	for _, p := range again {
		if keys[p.Key()] {
			t.Errorf("the second sample took %s again", p.Key())
		}
	}
}

func TestSamplePreprint(t *testing.T) {
	ps, err := Sample(records(), Preprint, 10, "s", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, p := range ps {
		got[p.Key()] = p.Stratum
	}
	if got["arxiv:2301.00001 zbmath:77"] != "2020s id" || got["arxiv:2301.00001 crossref:c1"] != "2020s title" || len(got) != 2 {
		t.Errorf("Sample gave %v", got)
	}
}

func TestEval(t *testing.T) {
	recs := records()
	for i := range recs {
		recs[i].Snapshot, recs[i].Level = "s1", "expression"
	}
	res, err := cluster.Run(cluster.Input{Records: recs, By: "test", Day: "2026-10-31"})
	if err != nil {
		t.Fatal(err)
	}
	l := func(v string) []Label { return []Label{{Value: v, By: "ann"}, {Value: v, By: "judge"}} }
	ps := []Pair{
		{A: "arxiv:2301.00001", B: "zbmath:77", Labels: l(Same)},
		{A: "loc:1", B: "loc:2", Labels: l(Different)},
		{A: "dnb:4", B: "dnb:5", Labels: l(Different)},
		{A: "arxiv:2301.00001", B: "crossref:c1", Labels: l(Same)},
		{A: "loc:1", B: "bnf:3"},
	}
	scores, met, n := Eval(res, ps)
	if n != 4 {
		t.Fatalf("Eval counted %d pairs, want 4", n)
	}
	all := scores[len(scores)-1]
	if all.Pass != "all" || all.Wrong != 0 || all.Precision != 1 {
		t.Errorf("the score of all passes is %+v", all)
	}
	if scores[0].Pass != cluster.C1 || scores[0].Merged != 1 {
		t.Errorf("the score of C1 is %+v", scores[0])
	}
	if met != (all.Recall >= TargetRecall) {
		t.Errorf("met is %v for %+v", met, all)
	}
}
