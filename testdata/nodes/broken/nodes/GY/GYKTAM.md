---
tag: GYKTAM
kind: definition
name: Vector space (definition)
topics: &t [XM6NGM]
defines: 2GJPBC
formal:
  - system: lean4
    library: mathlib
    decl: Module
    match: variant
    adapter: Math2.Meta.Match.moduleField
---

## Definition

A *vector space* over a field $K$ is an abelian group $V$ with a map $K \times V \to V$ that satisfies the module axioms.
