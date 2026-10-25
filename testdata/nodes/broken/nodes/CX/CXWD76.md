---
tag: CXWD76
kind: definition
name: Continuous function (definition)
topics: [CD2MSY]
defines: W2DJ7A
formal:
  - system: lean4
    library: mathlib
    decl: ContinuousAt
    match: same
---

## Definition

A function $f : \R \to \R$ is *continuous at* $a$ when for each $\varepsilon > 0$ there is a $\delta > 0$ such that $|x - a| < \delta$ gives $|f(x) - f(a)| < \varepsilon$.

## Remarks

The function is *continuous* when it is continuous at each point.
