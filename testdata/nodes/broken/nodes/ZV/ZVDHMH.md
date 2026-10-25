---
tag: ZVDHMH
kind: statement
role: theorem
name: Mean value theorem
msc: [26A24]
topics: [CD2MSY]
uses: [HMXENA, HX0H8H]
proofs: [RDXM71]
formal:
  - system: lean4
    library: mathlib
    decl: exists_deriv_eq_slope
    match: exact
verified: true
---

## Statement

Let $f$ be continuous on $[a, b]$ and differentiable on $(a, b)$. Then $f'(c) = \frac{f(b) - f(a)}{b - a}$ for some $c \in (a, b)$.
