package wpcorpus

import (
	"bytes"
	"strings"
	"testing"
)

func goldCandidates() []*Candidate {
	var cs []*Candidate
	add := func(id int64, s Signals) {
		c := &Candidate{PageID: id, Title: "P" + string(rune('A'+id%26)), Signals: s, CatDepth: 3}
		Classify(c)
		cs = append(cs, c)
	}
	for i := int64(1); i <= 10; i++ {
		add(i, S1)
	}
	for i := int64(11); i <= 14; i++ {
		add(i, S4)
	}
	add(15, S4|S3) // no stratum: S4 with a project tag
	add(16, S6)    // one weak, S6 far, not in scope
	add(17, S5)    // one weak, in scope
	add(18, S6|S9) // no stratum: two weak
	return cs
}

func TestStratum(t *testing.T) {
	want := map[int64]string{1: StratumStrong, 11: StratumCited, 15: "", 16: StratumWeak, 17: StratumWeak, 18: ""}
	for _, c := range goldCandidates() {
		if w, ok := want[c.PageID]; ok && Stratum(c) != w {
			t.Errorf("page %d: stratum %q, want %q", c.PageID, Stratum(c), w)
		}
	}
}

func TestDrawGold(t *testing.T) {
	cs := goldCandidates()
	sizes := map[string]int{StratumStrong: 3, StratumCited: 10, StratumWeak: 10}
	a := DrawGold(cs, sizes, 7)
	b := DrawGold(cs, sizes, 7)
	if len(a) != 3+4+2 {
		t.Fatalf("%d rows, want 9", len(a))
	}
	n := map[string]int{}
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("two draws with one seed differ")
		}
		n[a[i].Stratum]++
		if a[i].Stratum == StratumStrong && a[i].Population != 10 {
			t.Errorf("population %d, want 10", a[i].Population)
		}
	}
	if n[StratumStrong] != 3 || n[StratumCited] != 4 || n[StratumWeak] != 2 {
		t.Errorf("strata %v", n)
	}
	var buf bytes.Buffer
	if err := WriteGold(&buf, a); err != nil {
		t.Fatal(err)
	}
	back, err := ReadGold(&buf)
	if err != nil || len(back) != len(a) || back[0] != a[0] {
		t.Errorf("round trip: %v %v", back, err)
	}
	bad := goldHeader + "\n1\tX\tstrong\tS1\tstrong\t10\tmaybe\t\t\n"
	if _, err := ReadGold(strings.NewReader(bad)); err == nil {
		t.Error("the mark maybe has no error")
	}
}

func TestMeasure(t *testing.T) {
	cs := goldCandidates()
	rows := []GoldRow{
		{PageID: 1, Stratum: StratumStrong, Signals: S1, Population: 10, Mark: VerdictMath},
		{PageID: 2, Stratum: StratumStrong, Signals: S1, Population: 10, Mark: VerdictNotMath},
		{PageID: 16, Title: "PQ", Stratum: StratumWeak, Signals: S6, Rule: "S6 far", Population: 2, Mark: VerdictMath},
		{PageID: 17, Stratum: StratumWeak, Signals: S5, Population: 2, Mark: VerdictMath},
		{PageID: 11, Stratum: StratumCited, Signals: S4, Population: 4},
	}
	m := Measure(rows, cs)
	// The weights are 10/2 for the strong rows and 2/2 for the weak rows.
	// True positives 5 + 1, false positives 5, false negatives 1.
	if m.Marked != 4 || m.Unmarked != 1 || m.TruePos != 2 || m.FalsePos != 1 || m.FalseNeg != 1 {
		t.Errorf("counts %+v", m)
	}
	if p := 6.0 / 11; m.Precision < p-1e-9 || m.Precision > p+1e-9 {
		t.Errorf("precision %f, want %f", m.Precision, p)
	}
	if r := 6.0 / 7; m.Recall < r-1e-9 || m.Recall > r+1e-9 {
		t.Errorf("recall %f, want %f", m.Recall, r)
	}
	var b bytes.Buffer
	if err := GoldReport(&b, m); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"4 marked rows and 1 rows", "| strong | 2 | 1 | 0.500 |", "| PQ | S6 | S6 far |"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("the report has no %q:\n%s", want, b.String())
		}
	}
}
