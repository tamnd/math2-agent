/-!
# The statements of the fidelity suite

Each theorem is the formal statement of one item of `items.tsv`, with the same name. Some statements are faithful to the informal statement of their item, and the others have one fault of the taxonomy of spec 10.5. The proofs are `sorry`, because the battery reads the statement only, and a misformalized statement can be false.
-/

namespace Suite

-- Faithful statements.
theorem f01 (a b : Nat) : a + b = b + a := sorry
theorem f02 (a b : Nat) (h : a ≤ b) : ∃ c, a + c = b := sorry
theorem f03 (n : Nat) : n % 2 = 0 ∨ n % 2 = 1 := sorry
theorem f04 (n : Nat) : ∃ m, n < m := sorry
theorem f05 (a b : Nat) (h : b ≤ a) : a - b + b = a := sorry
theorem f06 (a b : Nat) (h : b ≠ 0) : a / b * b ≤ a := sorry
theorem f07 (α : Type) (l₁ l₂ : List α) : (l₁ ++ l₂).length = l₁.length + l₂.length := sorry
theorem f08 (α : Type) (l : List α) : l.reverse.reverse = l := sorry
theorem f09 (a b : Int) : (a * b).natAbs = a.natAbs * b.natAbs := sorry
theorem f10 (a : Int) : a - a = 0 := sorry
theorem f11 (a b c : Nat) : a * (b + c) = a * b + a * c := sorry
theorem f12 (n : Nat) (h : 1 ≤ n) : ∃ m, n = m + 1 := sorry
theorem f13 (a b : Int) (h : a ≤ b) : -b ≤ -a := sorry
theorem f14 (α β : Type) (f : α → β) (l : List α) : (l.map f).length = l.length := sorry

-- M-hyp-missing: a hypothesis of the informal statement is absent.
theorem h01 (a b : Nat) : a - b + b = a := sorry
theorem h02 (n : Nat) : n - 1 + 1 = n := sorry
theorem h03 (α : Type) (l : List α) : l.head? ≠ none := sorry
theorem h04 (a b : Nat) (h : a ≤ b) : a = b := sorry
theorem h05 (a b : Nat) : a % b < b := sorry
theorem h06 (a : Int) : a / a = 1 := sorry

-- M-hyp-extra: a hypothesis is added.
theorem e01 (a b : Nat) (h : 0 < a) : a + b = b + a := sorry
theorem e02 (α : Type) (l : List α) (h : l ≠ []) : l.reverse.reverse = l := sorry
theorem e03 (a : Int) (h : 0 ≤ a) : a - a = 0 := sorry
theorem e04 (α : Type) (l₁ l₂ : List α) (h : l₁.length ≤ l₂.length) : (l₁ ++ l₂).length = l₁.length + l₂.length := sorry
theorem e05 (a b c : Nat) (h : a ≠ 0) : a * (b + c) = a * b + a * c := sorry

-- M-quant: the order or the scope of the quantifiers is wrong.
theorem q01 : ∃ m : Nat, ∀ n : Nat, n < m := sorry
theorem q02 : ∃ c : Nat, ∀ a b : Nat, a ≤ b → a + c = b := sorry
theorem q03 : ∃ m : Nat, ∀ n : Nat, m = n + 1 := sorry
theorem q04 : ∀ n : Nat, ∃ m : Nat, m ≤ n := sorry

-- M-vacuous: the hypotheses cannot all be true.
theorem v01 (n : Nat) (h₁ : 0 < n) (h₂ : n = 0) : 1 ≤ n := sorry
theorem v02 (a b : Nat) (h₁ : a < b) (h₂ : b < a) : a + 1 ≤ b := sorry
theorem v03 (n : Nat) (h₁ : n % 2 = 0) (h₂ : n % 2 = 1) : (n + 1) % 2 = 1 := sorry
theorem v04 (α : Type) (l : List α) (h₁ : l ≠ []) (h₂ : l = []) : 0 < l.length := sorry
theorem v05 (a : Int) (h₁ : a < 0) (h₂ : 0 < a) : 0 < a * a := sorry

-- M-junk: a value outside the domain of the informal operation makes the statement true for the wrong reason.
theorem j01 (n : Nat) (h : 2 ≤ n) : 1 / n < 1 := sorry
theorem j02 (a b : Nat) (h : a < b) : a - b ≤ 0 := sorry
theorem j03 (n : Nat) (h : 1 ≤ n) : (n - 1) / n < 1 := sorry

-- M-def: a constant means something different from the informal notion.
theorem d01 (a b : Nat) : a ≤ min a b := sorry
theorem d02 : [1, 2, 3].head? = some 3 := sorry
theorem d03 : (-7 : Int) / 2 = -3 := sorry

-- M-type: the carrier is wrong.
theorem t01 (a b : Nat) : a - b + b = a := sorry
theorem t02 (a : Nat) : a + (0 - a) = 0 := sorry
theorem t03 (n : Nat) : (List.range n).sum * 2 = n * (n + 1) := sorry
theorem t04 (a : Nat) : 0 ≤ a * a := sorry

-- M-conv: a convention does not match.
theorem c01 (n : Nat) : 1 ≤ n := sorry
theorem c02 (a b : Nat) : a < a + b := sorry
theorem c03 (n : Nat) : (2 * n - 1) % 2 = 1 := sorry

-- M-weak: the formal statement is a special case.
theorem w01 (a : Nat) : a + 1 = 1 + a := sorry
theorem w02 (l : List Nat) : l.reverse.reverse = l := sorry
theorem w03 (a b : Nat) : a * (b + b) = a * b + a * b := sorry
theorem w04 (b : Nat) : ∃ c, 0 + c = b := sorry

-- M-strong: the formal statement is stronger than the informal statement, and it is false.
theorem s01 (n : Nat) : n ^ 2 < 2 ^ n := sorry
theorem s02 (n : Nat) : n < n * n := sorry
theorem s03 (n : Nat) : 5 < n := sorry
theorem s04 (a b : Nat) : a ^ 2 ≤ b ^ 2 := sorry
theorem s05 : 2 * 3 < 6 := sorry

end Suite
