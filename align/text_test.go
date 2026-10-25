package align

import (
	"slices"
	"testing"
)

func TestKey(t *testing.T) {
	for _, tc := range []struct{ a, b string }{
		{"Lagrange's theorem", "Lagrange theorem"},
		{"Lagrange’s Theorem", "lagrange theorem"},
		{"Erdős–Szekeres theorem", "erdos szekeres theorem"},
		{"The theorem of Lagrange", "theorem lagrange"},
	} {
		if key(tc.a) != key(tc.b) {
			t.Errorf("the keys of %q and %q are %q and %q, want one key", tc.a, tc.b, key(tc.a), key(tc.b))
		}
	}
	for _, tc := range []struct{ a, b string }{
		{"A-group", "group"},
		{"Lagrange's theorem", "Lagrange's identity"},
	} {
		if key(tc.a) == key(tc.b) {
			t.Errorf("%q and %q have the key %q", tc.a, tc.b, key(tc.a))
		}
	}
}

func TestNames(t *testing.T) {
	got := names("Lagrange's theorem: the order of a subgroup", "lagrange theorem", "", "the")
	if want := []string{"lagrange theorem order subgroup", "lagrange theorem"}; !slices.Equal(got, want) {
		t.Errorf("names gives %q, want %q", got, want)
	}
}

func TestFormulas(t *testing.T) {
	got := formulas(`Let $G$ be a group. Then $|H| \mid |G|$, and $$ \left| H \right| \mid |G| $$ too, and $a^{2}+b^{2}$.`)
	if want := []string{`|H|\mid|G|`, `a^2+b^2`}; !slices.Equal(got, want) {
		t.Errorf("formulas gives %q, want %q", got, want)
	}
}

func TestProse(t *testing.T) {
	body := "## Statement\n\nEach [[subgroup|G5S1M9]] of a finite group.\n\n## Proof\n\nBy cosets.\n\n## Remarks\n\nA note.\n"
	if got, want := prose(body), "Each subgroup of a finite group."; got != want {
		t.Errorf("prose gives %q, want %q", got, want)
	}
}
