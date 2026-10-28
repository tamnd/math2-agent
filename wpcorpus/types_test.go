package wpcorpus

import "testing"

func TestTypeOf(t *testing.T) {
	for _, tt := range []struct {
		title, short string
		cats         []string
		want         string
	}{
		{"Glossary of group theory", "", nil, TypeList},
		{"List of finite simple groups", "", nil, TypeList},
		{"Évariste Galois", "French mathematician (1811–1832)", nil, TypeBiography},
		{"Emmy Noether", "", []string{"1882 births", "German women mathematicians"}, TypeBiography},
		{"Noether's theorem", "Statement relating differentiable symmetries to conserved quantities", nil, TypeStatement},
		{"Lagrange's theorem (group theory)", "", nil, TypeStatement},
		{"Collatz conjecture", "Open problem in mathematics", []string{"Unsolved problems in number theory"}, TypeProblem},
		{"Basel problem", "", nil, TypeProblem},
		{"Bairstow's method", "", nil, TypeMethod},
		{"Euclidean algorithm", "Algorithm for computing greatest common divisors", nil, TypeMethod},
		{"Algebraic topology", "Branch of mathematics", nil, TypeField},
		{"Monster group", "In group theory, the largest sporadic simple group", nil, TypeObject},
		{"Pi", "Number, approximately 3.14; the mathematical constant", nil, TypeObject},
		{"Group (mathematics)", "Set with an operation that is associative", nil, TypeConcept},
	} {
		if got := TypeOf(tt.title, tt.short, tt.cats); got != tt.want {
			t.Errorf("TypeOf(%q) = %s, want %s", tt.title, got, tt.want)
		}
	}
}
