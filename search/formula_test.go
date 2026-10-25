package search

import (
	"slices"
	"testing"
)

func TestParseFormula(t *testing.T) {
	for src, want := range map[string]string{
		`a + b`:                               "+(V,V)",
		`x^2 + y^2 = z^2`:                     "=(+(^(V,2),^(V,2)),^(V,2))",
		`\sum_{d \mid n} \varphi(d) = n`:      `=(V,\sum(\mid(V,V),∅,apply(\varphi,V)))`,
		`\frac{a}{b} \cdot c`:                 "*(/(V,V),V)",
		`\left( a + b \right)^{n}`:            "^(()(+(V,V)),V)",
		`|G| = [G : H] \, |H|`:                "=(*([](:(V,V)),abs(V)),abs(V))",
		`\sqrt[3]{x}`:                         "root(V,3)",
		`\mathbb{R}^n`:                        `^(\mathbb{R},V)`,
		`\operatorname{rank} A + \dim \ker A`: `+(apply(\dim,apply(\ker,V)),apply(rank,V))`,
		`\lim_{x \to 0} \frac{\sin x}{x} = 1`: `=(1,\lim(\to(V,0),∅,/(apply(\sin,V),V)))`,
		`\{ x \in S : x > 0 \}`:               `set(:(\in(V,V),>(V,0)))`,
		`n! > 2^n`:                            ">(!(V),^(2,V))",
		`-x`:                                  "neg(V)",
		`a_{n+1} = 2 a_n`:                     "=(*(2,_(V,V)),_(V,+(1,V)))",
	} {
		tr := parseFormula(src)
		if tr == nil {
			t.Errorf("%s gives no tree", src)
			continue
		}
		tr.canonical()
		if got := tr.shape(); got != want {
			t.Errorf("%s gives %s, and it must give %s", src, got, want)
		}
	}
	for _, src := range []string{"", " ", `\,`} {
		if tr := parseFormula(src); tr != nil {
			t.Errorf("%q gives %s", src, tr.shape())
		}
	}
	// The parser accepts any input, also brackets that do not match.
	for _, src := range []string{")(", `\frac{`, `x^`, `\sum`, "]]a[[", `\sqrt[`, `\mathbb`, "|||", `\left(`} {
		parseFormula(src)
	}
}

func TestStructural(t *testing.T) {
	q, _ := indexFormula(`a^{p-1} \equiv 1 \pmod{p}`)
	same, _ := indexFormula(`x^{q-1} \equiv 1 \pmod{q}`)
	swapped, _ := indexFormula(`1 \pmod{q} \equiv x^{q-1}`)
	part, _ := indexFormula(`b^{m-1}`)
	other, _ := indexFormula(`\int_0^1 f(x) \, dx`)
	if s := structural(q, same); s != 1 {
		t.Errorf("a change of the names of the variables gives %v", s)
	}
	if s := structural(q, swapped); s != 1 {
		t.Errorf("a change of the sides of ≡ gives %v", s)
	}
	if s := structural(q, part); s <= 0 || s >= 1 {
		t.Errorf("a part of the formula gives %v", s)
	}
	if s := structural(q, other); s != 0 {
		t.Errorf("another formula gives %v", s)
	}
	// The variables of a subtree unify consistently: x + x is not y + z.
	xx, _ := indexFormula(`x + x = 0`)
	yz, _ := indexFormula(`y + z = 0`)
	if s := structural(xx, yz); s == 1 {
		t.Error("x + x = 0 and y + z = 0 match")
	}
}

func TestMathSpans(t *testing.T) {
	got := mathSpans(`Let $p$ be prime. Then $$a^p \equiv a$$ and \(x\) and \[y + 1\], but not \$5 or $ $.`)
	if want := []string{"p", `a^p \equiv a`, "x", "y + 1"}; !slices.Equal(got, want) {
		t.Errorf("mathSpans gives %q", got)
	}
}

func TestVector(t *testing.T) {
	v := quantize([]float32{0.6, -0.8, 0})
	back, err := readVector(v.bytes())
	if err != nil || dot(v, back) < 0.99 || dot(v, back) > 1.01 {
		t.Errorf("the vector after the bytes has the dot product %v, %v", dot(v, back), err)
	}
	if dot(v, quantize([]float32{1})) != 0 {
		t.Error("vectors of different lengths have a dot product")
	}
	if _, err := readVector([]byte{1}); err == nil {
		t.Error("readVector accepts one byte")
	}
	if z := quantize(make([]float32, 4)); dot(z, z) != 0 {
		t.Error("the zero vector has a length")
	}
}
