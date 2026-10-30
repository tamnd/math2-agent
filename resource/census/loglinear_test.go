package census

import (
	"math"
	"testing"
)

// expected gives the expected counts of the histories of N works under the capture probabilities p, with each catalogue independent.
func expected(n float64, p []float64) Histories {
	h := NewHistories(len(p))
	for mask := range h {
		f := n
		for i, q := range p {
			if mask>>i&1 == 1 {
				f *= q
			} else {
				f *= 1 - q
			}
		}
		h[mask] = f
	}
	h[0] = 0
	return h
}

func TestLogLinearIndependent(t *testing.T) {
	h := expected(10000, []float64{0.5, 0.4, 0.3})
	e, ok := EstimateLogLinear(h, nil, 1)
	if !ok {
		t.Fatal("no estimate")
	}
	if math.Abs(e.N-10000) > 1 {
		t.Errorf("N = %.1f, want 10000", e.N)
	}
	if e.Model != "Mt" {
		t.Errorf("the model is %s, want Mt", e.Model)
	}
	if !(e.CI.Low < e.N && e.N < e.CI.High) {
		t.Errorf("the interval %+v does not hold %.1f", e.CI, e.N)
	}
	if math.Abs(e.Observed-10000*(1-0.5*0.6*0.7)) > 1e-6 {
		t.Errorf("observed = %v", e.Observed)
	}
}

func TestLogLinearDependent(t *testing.T) {
	// Catalogue 1 copies half of the works of catalogue 0 that it does not hold. Mt then gives too small an estimate, and the pair [0,1] corrects it.
	h := expected(10000, []float64{0.5, 0.4, 0.3})
	for mask := 1; mask < len(h); mask++ {
		if mask&1 == 1 && mask&2 == 0 {
			moved := h[mask] / 2
			h[mask] -= moved
			h[mask|2] += moved
		}
	}
	mt, ok := fit(h, Model{Name: "Mt", Main: true})
	if !ok {
		t.Fatal("Mt does not fit")
	}
	e, ok := EstimateLogLinear(h, [][2]int{{0, 1}}, 1)
	if !ok {
		t.Fatal("no estimate")
	}
	if e.Model == "Mt" || e.Model == "M0" {
		t.Errorf("the model is %s, and it must have the forced pair", e.Model)
	}
	if math.Abs(e.N-10000) > 1 {
		t.Errorf("N = %.1f with the pair, want 10000", e.N)
	}
	if mt.N >= e.N {
		t.Errorf("Mt gives %.1f, and it must be below %.1f", mt.N, e.N)
	}
}

func TestLogLinearTooFew(t *testing.T) {
	if _, ok := EstimateLogLinear(expected(100, []float64{0.5, 0.5}), nil, 1); ok {
		t.Error("two catalogues must give no log-linear estimate")
	}
	if _, ok := EstimateLogLinear(NewHistories(3), nil, 1); ok {
		t.Error("no works must give no estimate")
	}
}

func TestModels(t *testing.T) {
	got := map[string]bool{}
	for _, m := range Models(3, nil) {
		got[m.String()] = true
	}
	for _, want := range []string{"M0", "Mt", "Mh", "Mth", "Mt+[0,1]", "Mt+[0,2]", "Mt+[1,2]"} {
		if !got[want] {
			t.Errorf("Models(3) has no %s: %v", want, got)
		}
	}
	for _, m := range Models(3, [][2]int{{1, 2}}) {
		if m.Name == "M0" || m.Name == "Mh" {
			t.Errorf("a forced pair needs the main terms, and %s has none", m)
		}
		if len(m.columns(3, 0)) > 7 {
			t.Errorf("%s has more terms than histories", m)
		}
	}
	if n := len(Models(4, nil)); n != 4+6+1 {
		t.Errorf("Models(4) gives %d models, want 11", n)
	}
}

func TestCaptures(t *testing.T) {
	got := Captures([]string{"crossref", "openalex", "zbmath", "openlibrary", "wikidata"}, map[string][]string{
		"openalex":    {"crossref"},
		"openlibrary": {"loc"},
		"wikidata":    {"loc", "crossref"},
	})
	want := map[string]string{
		"crossref":    "crossref+openalex+openlibrary+wikidata",
		"openalex":    "crossref+openalex+openlibrary+wikidata",
		"openlibrary": "crossref+openalex+openlibrary+wikidata",
		"wikidata":    "crossref+openalex+openlibrary+wikidata",
		"zbmath":      "zbmath",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("Captures[%s] = %q, want %q", k, got[k], v)
		}
	}
}
