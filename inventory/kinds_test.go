package inventory

import "testing"

func TestNameKind(t *testing.T) {
	for name, want := range map[string]string{
		"Gershgorin circle theorem":              "statement",
		"knot group":                             "concept",
		"Second fundamental theorem of calculus": "statement",
		"Degree of a field extension":            Unknown,
		"perturbation theory":                    "topic",
		"Pollock's conjecture":                   "conjecture",
		"halting problem":                        "problem",
		"group":                                  Unknown, // one word
		"Erwin Schrödinger":                      Unknown,
		"Lie algebra":                            Unknown, // algebra heads names of two kinds
		"Frenet–Serret formulas (geometry)":      "statement",
	} {
		if got := NameKind(name); got != want {
			t.Errorf("NameKind(%q) = %q, want %q", name, got, want)
		}
	}
	for _, c := range []struct {
		names []string
		want  string
	}{
		{[]string{"CommRing"}, "concept"},
		{[]string{"Real.log"}, "concept"},
		{[]string{"intervalIntegral.integral_comp_mul_deriv"}, "statement"},
		{[]string{"CommRing", "add_comm"}, ""},
		{nil, ""},
	} {
		if got := mathlibKind(c.names); got != c.want {
			t.Errorf("mathlibKind(%v) = %q, want %q", c.names, got, c.want)
		}
	}
}
