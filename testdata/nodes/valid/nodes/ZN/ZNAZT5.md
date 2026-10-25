---
tag: ZNAZT5
kind: computation
name: Goldbach's conjecture up to 10^4
topics: [3EYFQZ]
claim: Each even integer n with 4 <= n <= 10000 is the sum of two primes.
engine:
  image: m2-python@sha256:7777777777777777777777777777777777777777777777777777777777777777
  entry: goldbach.py
  inputs:
    limit: 10000
  seed: null
uses: [AG5NVJ]
---

## Description

The program checks each even number from 4 to 10,000 with a sieve and prints one pair of primes for each.
