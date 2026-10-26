---
tag: ZNAZT5
kind: computation
name: Goldbach's conjecture up to 10^4
topics: [3EYFQZ]
claim: Each even integer n with 4 <= n <= 10000 is the sum of two primes.
engine:
  image: m2-pari@sha256:9999999999999999999999999999999999999999999999999999999999999999
  entry: goldbach.py
  inputs:
    limit: 10000
  seed: null
output:
  hash: blake3:1111111111111111111111111111111111111111111111111111111111111111
certificate:
  kind: witness-table
  artefact: blake3:2222222222222222222222222222222222222222222222222222222222222222
  checker: m2-python@sha256:7777777777777777777777777777777777777777777777777777777777777777
  checker_entry: check.py
uses: [AG5NVJ]
---

## Description

The program writes, for each even n, two primes with the sum n. The checker reads the table and tests each row.
