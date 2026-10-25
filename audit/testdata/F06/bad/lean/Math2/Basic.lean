import Mathlib

namespace Math2

theorem one : 1 = 1 := by
  sorry

theorem two : 2 = 2 := by admit

axiom choice' : True

@[implemented_by Nat.add]
def plus (a b : Nat) : Nat := a + b

end Math2
