# math2 2027.04

These are the changes from 2027.03 to 2027.04. m2 release notes writes each section after the summary from the manifests, the levels and the node files of the two releases. Rule R03 checks that the counts table gives the cause of each change in a count of the manifest.

## Summary

This release settles a conjecture.

It has a | in it.

## Counts

| Count | Before | After | Change | Cause |
|---|---|---|---|---|
| by_evidence.E1 | 2 | 0 | -2 | 1 node moved to another level, 1 retired node |
| by_evidence.E5 | 0 | 1 | +1 | 1 node moved to this level |
| by_kind.concept | 1 | 2 | +1 |  |
| by_kind.example | 0 | 1 | +1 | 1 new node |
| by_kind.statement | 2 | 1 | -1 | 1 retired node |

## Topics

| Topic | Name | Before | After |
|---|---|---|---|
| TT0001 | Group theory | 2 | 3 |

## Evidence

| Node | Name | Before | After |
|---|---|---|---|
| SS0001 | Lagrange's theorem | E1 | E5 |

## Resolved

| Node | Name | Resolved by |
|---|---|---|
| CC0001 | A conjecture | SS0001 |

## Retired

| Node | Name | Reason | Successors |
|---|---|---|---|
| RR0001 | A copy of Lagrange's theorem | duplicate | SS0001 |

## Bindings

| Node | Old | New | Cause | Mathlib |
|---|---|---|---|---|
| SS0001 | `A.b` | `A.c` | rename | fedcba987654 |
