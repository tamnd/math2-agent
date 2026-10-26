---
tag: SM49BG
kind: statement
role: lemma
name: Euclid's lemma
msc: [11A41]
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

Let $p$ be prime. If $p \mid ab$, then $p \mid a$ or $p \mid b$. See https://example.org/survey.
