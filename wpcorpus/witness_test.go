package wpcorpus

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tamnd/wikipedia-cli/wiki/parsoid"
)

func readWitness(t *testing.T) *parsoid.Reading {
	t.Helper()
	f, err := os.Open("testdata/parsoid/witness.html")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rd, err := parsoid.Read(f)
	if err != nil {
		t.Fatal(err)
	}
	return rd
}

func TestAddWitness(t *testing.T) {
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	r := extractFile(t, "witness.wiki", "Widget")
	if err := AddWitness(r, m, readWitness(t)); err != nil {
		t.Fatal(err)
	}
	c := &r.Census
	want := Witness{Revision: 678, Math: 5, TeX: 3, ItemTemplates: map[string]int{"math theorem": 1}, Sections: 2}
	if w := c.HTML; w == nil || w.Revision != want.Revision || w.Math != want.Math || w.TeX != want.TeX || w.TeXMismatches != 0 || w.Sections != want.Sections || w.ItemTemplates["math theorem"] != 1 || len(w.ItemTemplates) != 1 {
		t.Fatalf("witness = %+v, want %+v", c.HTML, want)
	}
	if d := c.HTMLDifferences(); d != nil || c.Strength != StrengthNormal {
		t.Errorf("differences = %q, strength = %s", d, c.Strength)
	}

	// A reading that misses the sections and the theorem box differs.
	rd := readWitness(t)
	rd.Sections = rd.Sections[:1]
	rd.Transclusions = rd.Transclusions[:4]
	r = extractFile(t, "witness.wiki", "Widget")
	if err := AddWitness(r, m, rd); err != nil {
		t.Fatal(err)
	}
	want2 := []string{"the page has 2 sections in W-text and 0 in W-html"}
	if d := r.Census.HTMLDifferences(); !slices.Equal(d, want2) || r.Census.Strength != StrengthWeak {
		t.Errorf("differences = %q, strength = %s", d, r.Census.Strength)
	}

	rd.Revision = 679
	if err := AddWitness(r, m, rd); err == nil {
		t.Error("a reading of another revision gives no error")
	}
}

func TestJoinWitness(t *testing.T) {
	root := t.TempDir()
	m, err := Load(fixture)
	if err != nil {
		t.Fatal(err)
	}
	r := extractFile(t, "witness.wiki", "Widget")
	if err := JoinWitness(root, r, m); err != nil || r.Census.HTML != nil {
		t.Fatalf("no HTML: err = %v, witness = %+v", err, r.Census.HTML)
	}
	data, err := os.ReadFile("testdata/parsoid/witness.html")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(root, ParsoidPath(12345, 678)), data); err != nil {
		t.Fatal(err)
	}
	if err := JoinWitness(root, r, m); err != nil || r.Census.HTML == nil {
		t.Fatalf("HTML: err = %v, witness = %+v", err, r.Census.HTML)
	}
}

func TestNormTeX(t *testing.T) {
	for _, c := range [][2]string{
		{`x \le y`, `{\displaystyle x\leq y}`},
		{`2^p \equiv 1 \pmod{q}`, `{\displaystyle 2^{p}\equiv 1{\pmod {q}}}`},
		{`u^2_1`, `{\displaystyle u_{1}^{2}}`},
		{`x \ne 0`, `{\displaystyle x\neq 0}`},
	} {
		if a, b := NormTeX(c[0]), NormTeX(c[1]); a != b {
			t.Errorf("NormTeX(%q) = %q, NormTeX(%q) = %q", c[0], a, c[1], b)
		}
	}
	if NormTeX(`\lex`) == NormTeX(`\le x`) {
		t.Errorf("NormTeX joins \\le x and \\lex")
	}
}

func TestOver(t *testing.T) {
	for _, c := range []struct {
		a, b int
		want bool
	}{{0, 1, false}, {0, 2, true}, {100, 102, false}, {100, 103, true}, {200, 204, false}, {200, 205, true}} {
		if got := over(c.a, c.b); got != c.want {
			t.Errorf("over(%d, %d) = %v", c.a, c.b, got)
		}
	}
}
