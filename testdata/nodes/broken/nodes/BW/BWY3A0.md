---
tag: BWY3A0
kind: statement
role: theorem
name: Lagrange's theorem
msc: [20D60]
topics: [ZZZJE6]
uses: [N98ZA2, E18G47]
proofs: [ENS73X]
formal:
  - system: lean4
    library: mathlib
    decl: Subgroup.card_subgroup_dvd_card
    match: exact
sources:
  - book: isbn:9780471433347
    locator: Section 3.2, Theorem 8
same_as:
  wikidata: Q180967
  proofwiki: Lagrange's_Theorem_(Group_Theory)
history:
  first_stated: Lagrange 1771 (special case); Jordan 1861
---

## Remarks

The converse fails: $A_4$ has no subgroup of order 6 ([[counterexample|2J2QXT]]).
