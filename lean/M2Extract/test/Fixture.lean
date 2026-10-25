/-!
# Fixtures for the extraction tests

Each declaration here tests one property of the canonical form or of the axioms. The Go test in this package compares the records with `fixture.golden`.
-/

namespace Fixture

/-- The same statement as `add_comm_b`, with other binder names. The two type hashes must be equal. -/
theorem add_comm_a (m n : Nat) : m + n = n + m := Nat.add_comm m n

theorem add_comm_b (x y : Nat) : x + y = y + x := Nat.add_comm x y

/-- The same statement as `id_v`, with another universe name. The two type hashes must be equal. -/
def id_u.{u} {α : Sort u} (a : α) : α := a

def id_v.{v} {β : Sort v} (b : β) : β := b

/-- An instance argument. The type hash must differ from that of `inhabited_explicit`. -/
def inhabited_inst (α : Type) [Inhabited α] : α := default

def inhabited_explicit (α : Type) (_ : Inhabited α) : α := default

/-- A proof that uses `Classical.choice`. -/
theorem em (p : Prop) : p ∨ ¬p := Classical.em p

/-- A proof with `native_decide`. Lean adds an auxiliary axiom for it. -/
theorem native : 2 ^ 10 = 1024 := by native_decide

/-- A proof with `sorry`. -/
theorem incomplete : 1 = 2 := sorry

/-- A proof that uses `add_comm_a`. When `add_comm_a` is bound, it is in `uses_bound`. -/
theorem uses_a (m n : Nat) : (m + n) + 0 = n + m := by
  rw [Nat.add_zero]
  exact add_comm_a m n

end Fixture
