---
tag: GDVPGS
kind: statement
name: Extreme value theorem
msc: [26A15]
topics: [CD2MSY]
uses: [HMXENA]
proofs: [JCS7S2]
formal:
  - system: lean4
    library: mathlib
    decl: IsCompact.exists_isMaxOn
    match: stronger
---

## Statement

A continuous function $f : [a, b] \to \R$ has a maximum and a minimum on $[a, b]$.
