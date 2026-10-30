package competitions

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/tamnd/math2-agent/resource"
)

const answer = `{"results":{"bindings":[
{"c":{"value":"http://www.wikidata.org/entity/Q1"},"cLabel":{"value":"Olympiad A"},"inception":{"value":"2019-01-01T00:00:00Z"},"site":{"value":"https://a.example"}},
{"c":{"value":"http://www.wikidata.org/entity/Q1"},"cLabel":{"value":"Olympiad A"},"site":{"value":"https://a2.example"}},
{"c":{"value":"http://www.wikidata.org/entity/Q2"},"cLabel":{"value":"Contest B"},"iso":{"value":"VN"},"inception":{"value":"1990-01-01T00:00:00Z"},"dissolved":{"value":"1995-06-01T00:00:00Z"}}]}}`

func wd(t *testing.T) []Competition {
	t.Helper()
	var asked string
	get := func(_ context.Context, u string) ([]byte, error) { asked = u; return []byte(answer), nil }
	cs, err := FromWikidata(context.Background(), get, Endpoint)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(asked, Endpoint+"?") || strings.Contains(asked, "mailto") {
		t.Errorf("the request was %s", asked)
	}
	return cs
}

func TestFromWikidata(t *testing.T) {
	cs := wd(t)
	if len(cs) != 2 {
		t.Fatalf("got %d competitions, want 2", len(cs))
	}
	if a := cs[0]; a.Wikidata != "Q1" || a.Since != 2019 || a.Site != "https://a.example" || a.Country != "" {
		t.Errorf("A is %+v", a)
	}
	if b := cs[1]; b.Country != "VN" || b.Since != 1990 || b.Until != 1995 {
		t.Errorf("B is %+v", b)
	}
}

func TestBuildAndGaps(t *testing.T) {
	rs := []resource.Resource{
		{ID: "8F2AAAAAA3", Kind: "competition", Title: "Olympiad A", IDs: resource.IDs{"wikidata": {"Q1"}}},
		{ID: "8F2BBBBBBT", Kind: "problem-set", Title: "A 2019", Date: "2019-07", Relations: []resource.Relation{{Kind: "part_of", To: "8F2AAAAAA3"}}},
		{ID: "8F2CCCCCCH", Kind: "problem-set", Title: "A 2021", Date: "2021", Relations: []resource.Relation{{Kind: "part_of", To: "8F2AAAAAA3"}}},
	}
	prev := []Competition{
		{Name: "Kvant problems", Since: 1970},
		{Wikidata: "Q9", Name: "Gone C"},
		{Wikidata: "Q1", Name: "A", Skipped: []int{2020}},
	}
	cs, c := Build(prev, wd(t), rs)
	if c.Wikidata != 2 || c.Added != 1 || c.Withdrawn != 1 || c.Linked != 1 {
		t.Errorf("the counts are %+v", c)
	}
	by := map[string]Competition{}
	for _, x := range cs {
		by[x.Key()] = x
	}
	if a := by["Q1"]; a.Resource != "8F2AAAAAA3" || a.Name != "Olympiad A" || !slices.Equal(a.Skipped, []int{2020}) {
		t.Errorf("A is %+v", a)
	}
	if by["Q9"].Status != "withdrawn" || by["name:Kvant problems"].Since != 1970 {
		t.Errorf("the register is %+v", cs)
	}
	gaps := Gaps(cs, rs, 2024)
	got := map[string]Gap{}
	for _, g := range gaps {
		got[g.Competition] = g
	}
	if g := got["Q1"]; !slices.Equal(g.Years, []int{2022, 2023}) {
		t.Errorf("the gap of A is %+v", g)
	}
	if g := got["Q2"]; g.Reason != "the competition has no resource" {
		t.Errorf("the gap of B is %+v", g)
	}
	if _, ok := got["Q9"]; ok {
		t.Error("a withdrawn competition has a gap")
	}
	if len(gaps) != 3 {
		t.Errorf("got %d gaps, want 3: %+v", len(gaps), gaps)
	}
}

func TestRegister(t *testing.T) {
	root := t.TempDir()
	if got, err := Read(root); err != nil || got != nil {
		t.Fatalf("an empty tree gives %v, %v", got, err)
	}
	if err := Write(root, wd(t)); err != nil {
		t.Fatal(err)
	}
	if got, err := Read(root); err != nil || len(got) != 2 {
		t.Errorf("read %d records: %v", len(got), err)
	}
}
