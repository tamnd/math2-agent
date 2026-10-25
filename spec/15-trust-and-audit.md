# 15. Trust and the audit

This document puts in one place what math2 trusts, what it checks, and the rules that do the checks. As in `tamnd/arxiv`, **the audit is the real contract**. A claim in the README, a status in the workspace or a number in a release is only as good as the rule that checks it. Each rule has a name and a state, and each rule writes into a committed report.

## 15.1 The trusted base

math2 states its trusted base explicitly. math2 believes that a statement at E5 is true **if and only if** all of these conditions are true:

1. **Lean's kernel is sound** for the type theory that it implements. We also require independent replay with `lake check --paranoid`. This replay checks again with nanoda, lean4lean and the verified con-leche and con-ron kernels (document 07.7). Thus the trusted base is "at least one of several independent kernel implementations is sound, and they agree", not "Lean's C++ kernel is sound". The kernel soundness bugs of 2026 (document 01) showed that this concern is not theoretical. They also showed that checkers which are out of date fail together. For this reason, the release records the version of each checker.
2. **The standard axioms are consistent**: `propext`, `Quot.sound` and `Classical.choice`, together with the type theory of Lean. This is equiconsistent with ZFC plus countably many inaccessible cardinals. That is a little more than ZFC, and much less than anything that mathematicians have doubts about.
3. **The formal statement means what the informal statement says.** This is the fidelity claim F2 or F3 (document 10). It is the weakest link. Each design decision about fidelity has the aim to make this link stronger.
4. **The binding is the right declaration**: the name, the Mathlib commit and the elaborated-type hash in the release manifest match what was built. The build checks this mechanically.

For E5, math2 trusts nothing else. Specifically, this includes:

- any model's output;
- the prover that found the proof;
- the implementation of any tactic, because its output is a term that the kernel checks;
- the `m2` toolchain, except for the code that extracts declarations and hashes. That code is small and reviewed, and it has its own tests in `math2-agent/trust/`.

For E4, the trusted base is the certificate checker for that certificate kind (document 08.3), plus the runtime of the checker. For E3 and E2, it is the literature and the curators who read it. For E1, it is nothing.

## 15.2 How the audit runs

```
m2 audit                     # all planes, all rules, on the working tree
m2 audit -plane nodes        # one plane
m2 audit -rule G04           # one rule
m2 audit -changed            # only shards touched by the current diff (what PR CI runs)
m2 audit -release 2026.11    # the full pre-release audit
```

Each rule has one of five states, and the report shows all five:

| State | Meaning |
|---|---|
| `pass` | checked something and found nothing |
| `fail` | a hard rule found something. Blocks merge or release |
| `found` | a soft rule found something. Does not block, and is in the report all the same |
| `n/a` | the rule has nothing it could look at in this scope. Never shown as `pass` |
| `skip` | the rule was not run (for example, a network rule offline), and the report says why |

math2 keeps `n/a` and `pass` separate for the same reason that `tamnd/arxiv` keeps them separate. If a rule passes because it had nothing to examine, the rule is not working. It should not look as if it works.

The report goes to `reports/audit/<scope>.txt` and `reports/audit/<scope>.json`. It is committed on `main` after each merge. The release report goes to `reports/releases/<release>.txt`.

```
$ m2 audit -plane formal -q
rule  state  checked   findings
F01   pass   41,203
F02   pass   41,203
F03   pass   41,203
F04   found  41,203    17
F05   pass   3,880
F06   n/a    0
...
41,203 bindings over 38,911 nodes, 17 soft findings, nothing failed
```

## 15.3 The rule catalogue

The letter tells the subject of the rule. Rules with the mark **H** are hard, and they block. Rules with the mark **S** are soft, and the audit reports them. The implementation of each rule is in `tamnd/math2-agent/audit/<id>.go`. With it is a fixture that triggers the rule and a fixture that does not.

### T: tags and identity

| Rule | H/S | Checks |
|---|---|---|
| T01 | H | no tag ever disappears from `registers/tags.reg` or from `nodes/` across git history |
| T02 | H | no two nodes share a tag; no minted tag collides with a reserved prefix |
| T03 | H | no provisional id (`~xxxxxxxx`) appears on `main`, and none is referenced outside its own PR |
| T04 | H | every stored reference is a tag, not an alias or slug |
| T05 | H | registers agree with node files (every live node registered, every retired node in `retired.reg`) |
| T06 | S | aliases are not ambiguous with tags, MSC codes, OEIS A-numbers or arXiv ids |

### S: schema

| Rule | H/S | Checks |
|---|---|---|
| S01 | H | every node file validates against its schema version |
| S02 | H | kind-specific required fields present (a `proof` has exactly one `proves`; a `computation` has an engine block) |
| S03 | H | no `status`, `verified`, `proved` or similarly named field anywhere in content (document 04.7) |
| S04 | H | node file size ≤ 64 KB |
| S09 | S | node uses a deprecated schema version during a migration window |

### B: body

| Rule | H/S | Checks |
|---|---|---|
| B01 | H | every body renders in KaTeX and MathJax headless with no error, using only declared macros |
| B02 | H | every `[[…|TAG]]` link targets a live or retired-with-successor tag; no URLs in statement sections |
| B03 | H | only the schema's sections for the kind, in order |
| B04 | S | statement section is under 2,000 characters (long statements are usually two statements) |
| B05 | S | explanations carry a level and a human/agent marker |

### G: graph

| Rule | H/S | Checks |
|---|---|---|
| G01 | H | every edge target resolves |
| G02 | H | `same_as` is functional both ways within each external system |
| G03 | H | `specializes` acyclic; `uses` acyclic modulo equivalence cycles through distinct proofs |
| G04 | S | formal dependencies of a bound proof appear in its `uses` |
| G05 | H | every live statement has a source, a proof or a formal binding |
| G06 | H | every conjecture and problem has a `source` naming who posed it |
| G07 | S | hub families have at least three members |
| G08 | S | orphan rate per topic < 2% |
| G09 | S | no concept has more than 12 definitions without a note |
| G10 | H | every `implies`/`equivalent`/`refutes` edge names its `via` proof or computation |

### F: formal

| Rule | H/S | Checks |
|---|---|---|
| F01 | H | every binding resolves to a declaration at the pinned commit |
| F02 | H | the elaborated-type hash recorded for reviews matches the built one, or the review is marked stale (never silently kept) |
| F03 | H | `#print axioms` for every E5 candidate is a subset of the standard three, or the node allows more and says so |
| F04 | S | `native_decide` auxiliary axioms (`*._native.native_decide.ax_N`) in closure, reported per node |
| F05 | H | external replay: every E5 declaration passes `lake check --paranoid` and the comparator against its pinned statement, with checker versions recorded (document 07.7) |
| F06 | H | no `sorry`, `admit`, `axiom` declarations or `@[implemented_by]` in the math2 library outside `Math2/Hypotheses/` |
| F07 | H | statement-only bindings (`match: partial`) are never shown as E5 |
| F08 | S | Mathlib deprecation warnings on bound declarations, with the replacement name |
| F09 | H | degenerate-statement checks pass for every F1+ binding: hypotheses satisfiable, conclusion not trivially true, no vacuous quantification (document 10.4) |

### V: review and fidelity

| Rule | H/S | Checks |
|---|---|---|
| V01 | H | a review is bound to both the informal statement hash and the elaborated-type hash |
| V02 | H | F3 by two reviews needs two reviewers with no shared affiliation listed in `reviewers.reg`, and neither is the binding's author |
| V03 | H | an agent cannot be a reviewer; an agent run started by a reviewer cannot produce the binding that the same reviewer reviews |
| V04 | S | review age > 24 months on a node whose neighbourhood changed (prompt for re-review, no downgrade) |

### C: computation

| Rule | H/S | Checks |
|---|---|---|
| C01 | H | every computation names an image by digest, an entry point and inputs |
| C02 | H | E4 candidates reproduce bit-identically on two runner architectures |
| C03 | S | a computation's image is older than the current release's; re-run scheduled |
| C04 | H | certificate checker is the fixed checker for that kind (document 08.8) |
| C05 | H | no network, no model calls in computation logs |

### A: alignment

| Rule | H/S | Checks |
|---|---|---|
| A01 | H | every `same_as` value is syntactically valid for its system (QID, A-number, Stacks tag…) |
| A02 | S | `same_as` written by an agent and not confirmed after 90 days |
| A03 | S | round-trip: the external record still exists and still matches at its pinned revision |
| A04 | S | candidate duplicate pairs (embedding plus judge `same`) unresolved after 30 days |

### L: licence

| Rule | H/S | Checks |
|---|---|---|
| L01 | H | every byte of imported text names a source and that source's licence (document 09.3) |
| L02 | H | no text from a source with an incompatible licence (GFDL-only, arXiv default licence, all-rights-reserved) appears in `nodes/` |
| L03 | H | share-alike sources carry attribution in the node's provenance and the release's attribution file |
| L04 | S | text similarity between a node body and its cited sources above threshold, for human review of copying |

### P: provenance

| Rule | H/S | Checks |
|---|---|---|
| P01 | H | every content change in a merge has a provenance line |
| P02 | H | every agent provenance line names a run id that exists in the artefact store |
| P03 | H | every agent run names the person or schedule it acted on behalf of |
| P04 | S | AI-produced text without human confirmation shown in the workspace without its marker |

### R: release and reproducibility

| Rule | H/S | Checks |
|---|---|---|
| R01 | H | derived graph rebuilds to the same hash (sample on merge, all on release) |
| R02 | H | `.olean` rebuild matches, or the difference is listed and explained |
| R03 | H | release notes account for every count change |
| R04 | H | last quarterly restore drill passed |
| R05 | H | the release pins exactly one Lean toolchain and one Mathlib commit, and the bump branch was green |

### X: agents

| Rule | H/S | Checks |
|---|---|---|
| X01 | H | an agent PR changes only the fields its role is allowed to change (document 12.1) |
| X02 | H | an agent PR stays within its run budget, and the run record shows the spend |
| X03 | S | agent acceptance rate per role and model below threshold over 7 days (flag for prompt or model review) |
| X04 | H | no agent wrote a status-like field or a review |

There are 65 rules. A new rule needs a fixture pair and a line in this table. Both must be in the same pull request.

## 15.4 Propagation when trust changes

Trust can change. The audit handles the events that change it:

- **A conjecture is refuted.** Some statements have a best alternative that uses the conjecture `as: hypothesis`. These statements do not change, because they are honest conditionals. Other statements have a proof that uses the conjecture *not* as a hypothesis. Each of these statements goes down to E1, gets a flag, and is in the list in the release notes. In that case, someone assumed an open conjecture and did not say so.
- **A formal binding loses its declaration** (a Mathlib rename without a deprecation alias). The binding is broken (`F01` fails on the bump branch). The bump agent (document 07.9) proposes the rename from the deprecation data of Mathlib, and a person merges it. The node keeps its tag and its history in `bindings.reg`.
- **A definition changes meaning underneath a review** (`F02`). The review becomes stale. For that binding, the node goes down to F0, and E5 is not available. This stays true until someone reviews again, or until a second formalisation is proved equivalent.
- **A kernel or checker bug is found.** Some E5 nodes had their replay on the affected checker version. Each of these nodes runs again on a fixed version, or on the other checker only. The release that contained them gets an erratum. At some time, this has occurred in every proof assistant. Thus math2 has a plan for it and does not only hope that it will not occur.
- **A model version is withdrawn** (document 06.5, the withdrawal test). A generated PR quarantines its unreviewed output. An E5 status reached through that output does not change, because the kernel and the fidelity reviews carried that status, not the model.

## 15.5 What the workspace says

The status that a reader sees is a short phrase that the system generates from the evidence. It is never a checkmark. For example:

- "Proved in Lean (Mathlib), statement reviewed by 2 people. Classical. Replayed by 2 kernels."
- "Proved in refereed literature (3 sources, earliest 1871). Formal statement exists, unreviewed. No formal proof."
- "Claimed (preprint, 2026). Not reviewed. 1 agent-produced informal proof."
- "Open. Posed by Erdős (1975). Best known bounds: … (2025, formal)."

Each phrase has a link to the evidence that it summarises. A reader who wants to know why we believe something can follow the link down to the kernel replay log, the reviews, or the page number in the 1871 source.
