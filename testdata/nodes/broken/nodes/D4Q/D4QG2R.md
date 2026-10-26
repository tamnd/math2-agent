---
tag: D4QG2R
kind: statement
role: lemma
name: Euclid's lemma
msc: [20D6]
topics: [3EYFQZ]
uses: [FZ24AA, WM41VS]
proofs: [2R5WVM]
formal:
  - system: lean4
    library: mathlib
    decl: Nat.Prime.dvd_mul
    match: exact
---

## Statement

Let $p$ be prime. If $p \mid ab$, then $p \mid a$ or $p \mid b$.
