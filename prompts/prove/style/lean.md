# Lean conventions of math2

- Each file under lean/Math2/ is a module. It starts with the line `module`, then the imports. Use `public import` for a module whose declarations the statements of the file use, and `import` for a module that only the proofs use.
- After the imports, put a module docstring `/-! ... -/` that tells what the file has, then `@[expose] public section` and the namespace of the area, for example `namespace Math2.Algebra`.
- The names follow Mathlib: theorems and lemmas in snake_case, types and structures in UpperCamelCase, and the name of a theorem tells its conclusion, for example `card_subgroup_dvd_card`.
- Give each declaration a docstring `/-- ... -/` in plain words.
- Write `by` at the end of the line of the statement, and each tactic on its own line, with two spaces of indentation. Keep the lines at most 100 characters long.
- Prefer a short proof that uses Mathlib lemmas to a long proof that unfolds definitions. Do not use `set_option maxHeartbeats` to make a slow proof pass.
- Do not use `axiom`, `sorry` or `admit` in a proof, and use `native_decide` only when the first message of the run permits it. Do not redeclare, shadow or change a declaration that the statement uses.
- The statement is a definition `def S : Prop`. Most tactics, such as `intro`, `omega` and `simp`, see through it. Instance search does not unfold a definition, thus `decide` cannot find a `Decidable S` instance. Write `unfold S` or `show` with the proposition first.
