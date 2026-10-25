# 21. Open questions

This document lists the decisions that are not yet settled. Each entry gives the question, the options, and the current default (what the code does until the question is settled). It also gives **what would settle it**, which is usually a measurement. When a question is settled, its entry moves to the decision list in document 00, with a link to the PR and the measurement (16.7).

## 21.1 Content and model

**Q-01. Is `concept` separate from `definition` worth its cost?**
- Options: keep both kinds (04.2); or merge them, so that a concept is just its preferred definition plus equivalences.
- Default: keep both.
- Settles it: after M2, measure how many concepts have more than one definition node. Also measure how often curators make the split incorrectly (audit `S` findings on concept/definition misuse). If fewer than 10% of concepts have a second definition and the error rate is high, merge.

**Q-02. How should proof steps be represented?**
- Options: proofs as nodes with `uses` edges only (current); or a finer structure, with steps as sub-nodes, as in Lamport-style structured proofs or the Fermat run's DAG of intermediate statements.
- Default: coarse. An intermediate statement that is worth citing becomes its own `statement` node.
- Settles it: whether explain and the prove decomposition (12.4) produce many orphan intermediate statements (flag `orphan`). If they do, a lighter sub-node kind may be cheaper than full nodes.

**Q-03. How much of the OEIS and the LMFDB becomes nodes?**
- Options: every sequence and object as a node; only those cited by another node (current for the LMFDB); or all OEIS sequences but only cited LMFDB objects (current for the OEIS).
- Default: as stated in 06.7.
- Settles it: the repository size trigger (Q-13), and whether uncited object nodes ever receive edges.

**Q-04. Should math2 adopt a second informal format besides Markdown with LaTeX?**
- Options: stay with constrained Markdown (04.5); or allow Verso documents for nodes that are mostly Lean.
- Default: Markdown only.
- Settles it: the share of blueprint imports (09.4) that come from Verso, and whether the round trip loses information.

## 21.2 Formal

**Q-05. Isabelle, Rocq and Metamath bindings at E5?**
The AFP binding path exists (07.10). Rocq and HOL Light are crosswalk only. Metamath is a candidate for v2: its verifier is tiny and has independent implementations.
- Default: Lean and AFP only.
- Settles it: demand from projects, and whether the checker of a second system can go through the same replay discipline as the Lean checker (07.7).

**Q-06. What fraction of the build should go to replay?**
`lake check --paranoid` across several kernels is slow at FLT scale. 07.7 allows `external_replay` for proofs above a size threshold.
- Default: full replay for everything under 1M lines, and an external replay record for the rest.
- Settles it: measured replay time per release. If it fits the 24-hour build budget (02.4), the threshold is removed.

**Q-07. When should Math2 code move to Mathlib?**
- Options: as soon as it is stable; only on a Mathlib maintainer's request; or never move it, and let Mathlib cherry-pick.
- Default: a person offers it when its sanity lemmas and review are complete (16.5).
- Settles it: Mathlib's own policy on AI-assisted contributions, which is evolving, and the review-queue data from the Mathlib Initiative.

**Q-08. How should math2 handle Comparator's open issues?**
Comparator had known gaps in October 2026. 07.7 adds math2's own type-hash check as a second statement check.
- Default: both checks.
- Settles it: when Comparator ships in the Lean distribution with those issues closed. Then decide whether the type-hash check is still necessary. Measure whether it ever caught anything that Comparator missed.

## 21.3 Agents

**Q-09. Should math2 run its own fine-tuned prover models?**
- Options: hosted frontier models plus open prover models as they are released (current, 12.3); or fine-tune an open model on math2's run data (17.4).
- Default: no fine-tuning.
- Settles it: whether the quarterly run dataset shows a solve-rate gap on math2's own targets. It would have to be a gap that a fine-tune could close at a lower cost per solve than the ladder. A 2026 finding makes it likely that the default holds: general models with a minimal loop match bespoke provers (12).

**Q-10. Should agents be allowed to choose explore targets?**
12.5 says no: people define campaigns. Some open problems would plausibly benefit from wide automatic sweeps. For others, there are requests that they not be "harvested" (the 2026 debates around erdosproblems.com and the `openai/math` release).
- Default: no autonomous target selection.
- Settles it: this is a community decision, not a measurement. The proposal would need maintainer approval plus an RFC open for a month.

**Q-11. How should agent output be paced to match review capacity?**
Agents can produce much more than people can review. That is the Mathlib queue problem, and the lesson of the 719-manuscript release.
- Default: the orchestrator sets a weekly limit on the PR rate of each role. The limit is a multiple of the review throughput of the previous four weeks (11.5).
- Settles it: the multiple is tuned on the weekly report (12.9). The open question is whether review-queue age is a better control signal than throughput.

**Q-12. Should Managed Agents run the explain, curate and align roles?**
12.3 lists hosted agent infrastructure as an option for the long-running roles that need no Lean session.
- Default: self-hosted loop for every role.
- Settles it: a month of shadow runs that compare cost, latency and suite scores.

## 21.4 Storage and scale

**Q-13. When does the record plane move to its own repository?**
06.1 sets the trigger: more than 10 GB packed, or more than 5 million files, in `tamnd/math2`.
- Default: one repository until the trigger.
- Settles it: the trigger. The open part is whether to split by plane (`tamnd/math2-records`) or by MSC range. Split by plane, unless clone-time measurements say otherwise.

**Q-14. Should math2 serve vectors?**
14.4 forbids the return of raw embeddings. This protects record-class text and keeps the model swappable. Some research users want vectors for their own analysis.
- Default: no vectors.
- Settles it: a licence review of whether embeddings of `open` class text could be published, and whether anyone asks for them through a project.

## 21.5 Governance

**Q-15. Who are the maintainers, and how are they chosen?**
16.7 assumes a maintainer group with two-approval rules. It does not say how the group is formed after the founding team.
- Default: founding maintainers, then nominations by existing maintainers with a public comment period.
- Settles it: a governance document, before M3. The models to consider are Mathlib's maintainer process, the OEIS editors and the Stacks Project's single-editor model.

**Q-16. Should reviewers be paid?**
19 budgets reviewer stipends from M3. Payment for review could compromise independence. Or it could be the only way that research-level review happens at all.
- Default: stipends for undergraduate and graduate core review only. Research-level review is volunteer and credited.
- Settles it: measured re-review disagreement for paid against unpaid reviews (10.8).

**Q-17. Relationship with Palomar and erdosproblems.com.**
Both are community registries with their own norms. math2 imports from both and can export to Palomar (16.5).
- Default: import and link; never write to them except by people following their processes.
- Settles it: a conversation with their maintainers, before M2.
