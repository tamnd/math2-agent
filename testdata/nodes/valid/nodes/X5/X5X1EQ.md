---
tag: X5X1EQ
kind: statement
role: theorem
name: Fermat's little theorem
msc: [11A07]
topics: [3EYFQZ]
uses: [FZ24AA, XE3D9F]
proofs: [FQAJ21, XCYCNZ]
formal:
  - system: lean4
    library: mathlib
    decl: ZMod.pow_card_sub_one_eq_one
    match: variant
    adapter: Math2.Meta.Match.zmodToInt
---

## Statement

Let $p$ be prime and $a \in \Z$ with $p \nmid a$. Then $a^{p-1} \equiv 1 \pmod{p}$.
