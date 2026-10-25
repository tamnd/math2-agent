package search

import (
	"slices"
	"testing"
)

func TestTypeShape(t *testing.T) {
	// ∀ {α : Type u} [Group α] (s : Subgroup α), Nat.card s ∣ Nat.card α, in a short canonical form.
	canon := "s s(u0)\nc Group u0\nb 0\na 1 2\nc Subgroup u0\na 4 2\nc Dvd.dvd 0\nc Nat.card u0\na 6 7\np d 5 8\np c 3 9\np i 0 10\n"
	sh := typeShape(canon, []string{"Dvd.dvd", "Group"})
	if sh.concl != "Dvd.dvd" || !slices.Equal(sh.hyps, []string{"Group", "Subgroup"}) || len(sh.consts) != 2 {
		t.Errorf("typeShape gives %+v", sh)
	}
	for _, bad := range []string{"", "p d 9 9\n", "a x y\n", "x\n\n"} {
		typeShape(bad, nil)
	}
	if sh := typeShape("p d 0 0\n", nil); sh.concl != "→" {
		t.Errorf("a cycle gives %+v", sh)
	}
}

func TestParseLean(t *testing.T) {
	for q, want := range map[string]leanQuery{
		"Subgroup _ → Finite _ → IsCyclic _":        {concl: "IsCyclic", hyps: []string{"Finite", "Subgroup"}, consts: []string{"Finite", "IsCyclic", "Subgroup"}},
		"∀ n : ℕ, ∑ d ∈ n.divisors, φ d = n":        {concl: "Eq", hyps: []string{"Nat"}, consts: []string{"Finset.sum", "Membership.mem", "Nat", "Nat.totient", "divisors"}},
		"IsCyclic, Units, Field":                    {consts: []string{"IsCyclic", "Units", "Field"}},
		"h : p.Prime\n⊢ p ∣ a * b ↔ p ∣ a ∨ p ∣ b":  {concl: "Iff", hyps: []string{"Prime"}, consts: []string{"Dvd.dvd", "Iff", "Or", "Prime"}},
		"∀ (G : Type) [Group G] [Fintype G], 1 = 1": {concl: "Eq", hyps: []string{"Fintype", "Group"}, consts: []string{"Fintype", "Group"}},
	} {
		got := parseLean(q)
		if got.concl != want.concl || !slices.Equal(got.hyps, want.hyps) || !slices.Equal(got.consts, want.consts) {
			t.Errorf("parseLean(%q) gives %+v, and it must give %+v", q, got, want)
		}
	}
	for q, want := range map[string]bool{"IsCyclic, Units": true, "Nat.Prime": true, "∀ n, n = n": true, "a theorem, of groups": false, "group": false, `$\forall x$`: false} {
		if isLean(q) != want {
			t.Errorf("isLean(%q) = %v", q, !want)
		}
	}
}
