---
tag: T9DAVN
kind: definition
name: Group (definition)
msc: [20-XX]
topics: [BB2PHH]
defines: DSG7KN
formal:
  - system: lean4
    library: mathlib
    decl: Group
    match: exact
sources:
  - book: isbn:9780471433347
    locator: Section 1.2
proved: true
---

## Definition

A *group* is a set $G$ with a map $G \times G \to G$, $(a, b) \mapsto ab$, such that:

1. $(ab)c = a(bc)$ for all $a, b, c \in G$;
2. there is an $e \in G$ with $ea = ae = a$ for all $a \in G$;
3. for each $a \in G$ there is a $b \in G$ with $ab = ba = e$.
