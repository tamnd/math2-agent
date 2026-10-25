package formalconjectures

import (
	"slices"
	"testing"
)

func TestScan(t *testing.T) {
	src := `/-! # Title

A module doc with a link, https://example.org/a.
-/

namespace A.B

/- A comment /- that nests -/ and has theorem fake : True := trivial in it. -/

-- theorem comment : True := trivial

/-- The first. -/
@[category research open, AMS 11]
theorem one (n : ℕ) (h : n = "a := b".length) :
    n = 6 := by
  sorry

def s : String := "theorem str : True := trivial"

section

/-- The second. -/
@[category textbook, foo [bar] "x]y"]
protected noncomputable lemma two : ∀ x : ℕ, x = x := fun _ => rfl

end

theorem no_attrs : True := trivial

@[category test]
theorem _root_.top : True := trivial

@[category research solved] theorem «odd name» : True := trivial

end A.B

/-- The third. -/
@[category research open]
theorem «three».four : 1 = 1 := rfl
`
	f := scan(src)
	if f.moduleDoc != "# Title\n\nA module doc with a link, https://example.org/a." {
		t.Errorf("the module doc is %q", f.moduleDoc)
	}
	want := []decl{
		{name: "A.B.one", line: 14, attrs: "category research open, AMS 11", doc: "The first.", statement: `(n : ℕ) (h : n = "a := b".length) : n = 6`},
		{name: "A.B.two", line: 24, attrs: `category textbook, foo [bar] "x]y"`, doc: "The second.", statement: ": ∀ x : ℕ, x = x"},
		{name: "top", line: 31, attrs: "category test", statement: ": True"},
		{name: "A.B.«odd name»", line: 33, attrs: "category research solved", statement: ": True"},
		{name: "«three».four", line: 39, attrs: "category research open", doc: "The third.", statement: ": 1 = 1"},
	}
	if !slices.Equal(f.decls, want) {
		t.Errorf("scan gives\n%+v\nwant\n%+v", f.decls, want)
	}
}

func TestLinks(t *testing.T) {
	got := links("See [a](https://a.org/x), https://b.org/y. And https://a.org/x again.")
	if want := []string{"https://a.org/x", "https://b.org/y"}; !slices.Equal(got, want) {
		t.Errorf("links gives %v, want %v", got, want)
	}
}
