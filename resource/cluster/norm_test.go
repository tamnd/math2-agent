package cluster

import "testing"

func TestEdition(t *testing.T) {
	cases := map[string]int{
		"2nd ed.":                                2,
		"Second edition":                         2,
		"Third revised edition":                  3,
		"2. Auflage":                             2,
		"Zweite Auflage":                         2,
		"3e édition":                             3,
		"Deuxième édition":                       2,
		"Издание второе":                         2,
		"2-е издание":                            2,
		"第二版":                                    2,
		"第3版":                                    3,
		"Edition 4":                              4,
		"Linear algebra done right, 3rd edition": 3,
		"Linear algebra done right":              0,
		"An introduction":                        0,
	}
	for in, want := range cases {
		if got := Edition(in); got != want {
			t.Errorf("Edition(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestWorkTitle(t *testing.T) {
	cases := map[string]string{
		"Linear Algebra Done Right, 3rd edition":     "linear algebra done right",
		"Linear algebra done right":                  "linear algebra done right",
		"Algebra: Chapter 0":                         "algebra",
		"Topology (Second Edition)":                  "topology",
		"Analysis I - A first course":                "analysis i",
		"Éléments de mathématique":                   "elements de mathematique",
		"Introduction to Topology. Pure and applied": "introduction to topology",
	}
	for in, want := range cases {
		if got := WorkTitle(in); got != want {
			t.Errorf("WorkTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFamily(t *testing.T) {
	for in, want := range map[string]string{"Erdős, Paul": "erdos", "Paul Erdős": "erdos", "J.-P. Serre": "serre", "": ""} {
		if got := Family(in); got != want {
			t.Errorf("Family(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	if s := Similarity("On the zeros of the zeta function", "On the zeros of the Zeta-function"); s < 0.9 {
		t.Errorf("similar titles = %v", s)
	}
	if s := Similarity("On the zeros of the zeta function", "Lectures on elliptic curves"); s > 0.3 {
		t.Errorf("different titles = %v", s)
	}
}
