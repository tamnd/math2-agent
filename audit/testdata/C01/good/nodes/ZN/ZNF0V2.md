---
tag: ZNF0V2
kind: computation
name: There are 14 groups of order 16
topics: [ZZZJE6]
claim: There are exactly 14 isomorphism classes of groups of order 16.
engine:
  image: m2-gap@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  entry: groups16.g
  inputs: {}
output:
  hash: blake3:3333333333333333333333333333333333333333333333333333333333333333
certificate:
  kind: group-id
  artefact: blake3:4444444444444444444444444444444444444444444444444444444444444444
  checker: m2-sage@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc
  checker_entry: check.sage
uses: [GG6BXV]
---

## Description

The program counts the groups of order 16 in the small groups library. A second system counts them again.
