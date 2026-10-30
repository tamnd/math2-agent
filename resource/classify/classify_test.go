package classify

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/tamnd/math2-agent/resource"
	"github.com/tamnd/math2-agent/resource/scope"
)

func res(id, title string, msc string, subj ...string) resource.Resource {
	r := resource.Resource{ID: id, Kind: "article", Title: title}
	if msc != "" {
		r.MSC = []resource.Code{{Code: msc, From: "zbmath"}}
	}
	for _, s := range subj {
		r.Subjects = append(r.Subjects, resource.Subject{System: "lcc", Code: s})
	}
	return r
}

func corpus() []resource.Resource {
	var rs []resource.Resource
	math := []string{"Spectral theory of compact operators", "Zeros of the Riemann zeta function", "Homology of compact manifolds", "Spectral gaps of random graphs", "Zeta functions of graphs", "Compact operators on Banach spaces"}
	other := []string{"Protein folding in yeast cells", "Grain yields of wheat farms", "Cell membranes and yeast proteins", "Farm labour and wheat prices", "Yeast genetics of protein expression", "Wheat cell biology"}
	for i := 0; i < 60; i++ {
		msc := []string{"47B07", "11M06", "55N10"}[i%3]
		rs = append(rs, res(fmt.Sprintf("M%03d", i), math[i%len(math)], msc))
		rs = append(rs, res(fmt.Sprintf("O%03d", i), other[i%len(other)], "", "QH506"))
	}
	return rs
}

func TestFitAndDecide(t *testing.T) {
	labels, classes := Examples(corpus(), scope.Env{}, nil)
	if len(labels) != 120 || len(classes) != 60 {
		t.Fatalf("got %d and %d examples", len(labels), len(classes))
	}
	m := Fit(labels)
	m.Classes = Fit(classes)
	ev := m.Tune(labels)
	if !ev.Met || ev.Precision < TargetPrecision {
		t.Fatalf("got %+v", ev)
	}
	r := res("X", "Spectral theory of zeta functions", "")
	if v, s := m.Decide(Features(&r, nil)); v != resource.In || s < m.High {
		t.Errorf("got %s %v", v, s)
	}
	r = res("Y", "Protein folding in yeast cells and wheat", "")
	if v, _ := m.Decide(Features(&r, nil)); v != resource.Out {
		t.Errorf("got %s", v)
	}
	r = res("Z", "Zeta functions", "")
	if c, _ := m.Class(Features(&r, nil)); c != "11" {
		t.Errorf("got class %s", c)
	}
	p := filepath.Join(t.TempDir(), "scope.json")
	if err := m.Write(p); err != nil {
		t.Fatal(err)
	}
	again, err := Read(p)
	if err != nil || again.Version() != m.Version() || again.High != m.High {
		t.Fatalf("got %v, %v", again, err)
	}
	if missing, err := Read(filepath.Join(t.TempDir(), "none.json")); missing != nil || err != nil {
		t.Fatalf("a missing model gave %v, %v", missing, err)
	}
}

func TestWords(t *testing.T) {
	got := Words("Über die Nullstellen der Zetafunktion, a note")
	want := []string{"nullstellen", "zetafunktion", "note"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v", got)
	}
}
