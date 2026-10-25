/-! The declarations that the replay tests check. -/

namespace Fixture

/-- A complete proof with no axioms. -/
theorem add_comm (a b : Nat) : a + b = b + a := Nat.add_comm a b

/-- A proof with a universe parameter. -/
theorem refl.{u} (α : Sort u) (a : α) : a = a := rfl

/-- A proof that uses the excluded middle, thus the three standard axioms. -/
theorem em (p : Prop) : p ∨ ¬p := Classical.em p

/-- A proof with sorry. -/
theorem incomplete (a b : Nat) : a * b = b * a := by sorry

end Fixture
