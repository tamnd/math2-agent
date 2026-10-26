---
tag: K9KMX2
kind: statement
role: theorem
name: Intermediate value theorem
msc: [26A15]
topics: [CD2MSY]
uses: [hmxena]
proofs: [7XW8N7]
formal:
  - system: lean4
    library: mathlib
    decl: intermediate_value_Icc
    match: exact
---

## Statement

Let $f : [a, b] \to \R$ be [[continuous|W2DJ7A]] and let $y$ be between $f(a)$ and $f(b)$. Then $f(c) = y$ for some $c \in [a, b]$.
