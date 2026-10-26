package inventory

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

// population gives the rows and the mentions of n items of the area 11, where each source captures each item with its probability.
func population(n int, p map[string]float64, seed uint64) ([]Row, []Mention) {
	r := rand.New(rand.NewPCG(seed, 1))
	var rows []Row
	var ms []Mention
	for i := range n {
		row := Row{ID: fmt.Sprintf("R%05d", i), MSC: []string{"11A41"}}
		for _, s := range []string{"wikidata", "zbmath", "mathlib", "mathlib_100"} {
			if r.Float64() < p[s] {
				m := Mention{ID: MentionID(s, row.ID, 1), Source: s, By: s + "/1"}
				ms = append(ms, m)
				row.Mentions = append(row.Mentions, m.ID)
			}
		}
		if len(row.Mentions) > 0 {
			rows = append(rows, row)
		}
	}
	return rows, ms
}

// A 95% interval holds the true total in about 95% of the samples. The test draws 200 fixed samples of a population with a known total, so it gives the same result on each run.
func TestEstimateKnownTotal(t *testing.T) {
	const n, draws = 1000, 200
	inTotal, inRecall := 0, 0
	for seed := range uint64(draws) {
		rows, ms := population(n, map[string]float64{"wikidata": 0.3, "zbmath": 0.4}, seed)
		areas := Estimate(rows, ms)
		if len(areas) != 1 || areas[0].Method != "lincoln-petersen" || len(areas[0].Captures) != 1 {
			t.Fatalf("got %+v", areas)
		}
		a, c := areas[0], areas[0].Captures[0]
		if c.Low <= n && n <= c.High {
			inTotal++
		}
		if want := float64(len(rows)) / n; *a.Low <= want && want <= *a.High {
			inRecall++
		}
	}
	if inTotal < draws*90/100 || inRecall < draws*90/100 {
		t.Errorf("the interval holds the total in %d of %d samples and the recall in %d, want at least 90%%", inTotal, draws, inRecall)
	}
	t.Logf("the interval holds the total in %d of %d samples", inTotal, draws)
}

func TestEstimateFamilies(t *testing.T) {
	// Mathlib and its list of 100 theorems are one family, so they give no estimate.
	rows, ms := population(200, map[string]float64{"mathlib": 0.5, "mathlib_100": 0.5}, 1)
	if a := Estimate(rows, ms); len(a) != 1 || a[0].Method != "none" || a[0].Recall != nil || a[0].Sources != 2 {
		t.Fatalf("got %+v", a)
	}
	// With a third source, the pair with the largest overlap is the pair of the area.
	rows, ms = population(200, map[string]float64{"mathlib": 0.5, "mathlib_100": 0.5, "wikidata": 0.9}, 1)
	a := Estimate(rows, ms)
	if len(a[0].Captures) != 2 {
		t.Fatalf("got the captures %+v", a[0].Captures)
	}
	for _, c := range a[0].Captures {
		if c.Default != (c.A == "mathlib" && c.B == "wikidata") {
			t.Errorf("the capture %+v", c)
		}
	}
}

func TestEstimateSkipsRows(t *testing.T) {
	ms := []Mention{{ID: "a", Source: "wikidata"}, {ID: "b", Source: "zbmath"}, {ID: "c", Source: "enumerate", By: "agent:enumerate"}}
	rows := []Row{
		{ID: "1", MSC: []string{"11A41", "11N05", "05C10"}, Mentions: []string{"a", "b"}},
		{ID: "2", MSC: []string{"11A41"}, Mentions: []string{"a"}, Outcome: &Outcome{Value: OutcomeOutOfScope}},
		{ID: "3", MSC: []string{"11A41"}, Mentions: []string{"c"}},
	}
	a := Estimate(rows, ms)
	if len(a) != 2 || a[0].MSC != "05" || a[1].MSC != "11" || a[1].Rows != 2 || a[1].Sources != 3 {
		t.Fatalf("got %+v", a)
	}
	// The overlap of wikidata and zbmath is 1 of 1, so the estimate is the one row, and the models give the second row.
	for _, c := range a[1].Captures {
		if c.A == "wikidata" && c.B == "zbmath" && (c.Total != 1 || c.Low != 1) {
			t.Errorf("the capture %+v", c)
		}
	}
}
