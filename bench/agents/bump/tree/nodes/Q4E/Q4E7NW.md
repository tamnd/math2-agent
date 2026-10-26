---
tag: Q4E7NW
kind: statement
role: theorem
name: Infinitude of the primes
msc: [11A41]
uses: [P1RM3S]
formal:
  - system: lean4
    library: math2
    decl: Math2.NumberTheory.infinite_primes
    match: exact
---

## Statement

There are infinitely many [[prime numbers|P1RM3S]]. Thus for each natural number $N$ there is a prime $p$ with $p > N$.
