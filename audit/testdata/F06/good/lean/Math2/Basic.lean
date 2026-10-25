import Mathlib

/-! The library. It never uses sorry. -/

namespace Math2

/-- A proof with no sorry and no admit. /- A nested comment with axiom. -/ -/
theorem two : 1 + 1 = 2 := rfl -- sorry is in a comment

def note : String := "sorry, admit, axiom and implemented_by"
def raw : String := r#"a "sorry" here"#
def quote : Char := '"'
def admitted' : Nat := 0
theorem Foo.sorry_free : True := trivial

end Math2
