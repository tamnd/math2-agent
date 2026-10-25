---
tag: RQ9C3D
kind: computation
name: Fermat's little theorem for primes below 1000
topics: [3EYFQZ]
claim: For each prime p < 1000 and each a with 1 <= a < p, a^(p-1) mod p = 1.
engine:
  image: m2-python@sha256:7777777777777777777777777777777777777777777777777777777777777777
  entry: ../fermat.py
  inputs:
    limit: 1000
output:
  hash: blake3:5555555555555555555555555555555555555555555555555555555555555555
certificate:
  kind: witness
  artefact: blake3:6666666666666666666666666666666666666666666666666666666666666666
  checker: m2-python@sha256:7777777777777777777777777777777777777777777777777777777777777777
  checker_entry: check.py
uses: [X5X1EQ]
---

## Description

The program tests each pair p and a, and writes the pairs that it tested.
