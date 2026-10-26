---
tag: MP5AHE
kind: statement
role: theorem
name: Cauchy's theorem for groups
msc: [20D60]
topics: [ZZZJE6]
uses: [E18G47, FZ24AA]
proofs: [VTPHZM]
formal:
  - system: lean4
    library: mathlib
    decl: exists_prime_orderOf_dvd_card
    match: exact
sources:
  - book: isbn:9780471433347
    locator: Section 3.2, Theorem 11
---

## Statement

Let $G$ be a finite group and $p$ a [[prime|NKCASP]] that divides $|G|$. Then $G$ has an element of order $p$.

The class $\htmlClass{a}{x}$ is set.
