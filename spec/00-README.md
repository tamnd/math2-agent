# Math 2.0: specification

Math 2.0 is a versioned, executable, verifiable and navigable knowledge system for all of mathematics. It has four parts:

- a knowledge graph of concepts, statements, proofs, objects and problems;
- a formal engine and a computational engine, which make claims checkable;
- AI research agents, which extend and maintain the graph under human review;
- a workspace, where people search, explore, explain and collaborate.

Two repositories hold the work. This follows the `tamnd/arxiv` pattern:

| Repository | Holds | Licence |
|---|---|---|
| `tamnd/math2` | the content: node files, reviews, the `Math2` Lean package, computation specs, release manifests | prose CC BY-SA 4.0; code Apache-2.0; metadata CC0 (16.6) |
| `tamnd/math2-agent` | the toolchain: the Go binary `m2` (build, audit, ingest, align, serve, orchestrate, agents, site, TUI), Lean extraction, container images | Apache-2.0 |

The date of this specification is 2026-10-25. Document 01 is its evidence base. Document 01 gives the state of the field on that date.

## Documents

| # | Document | What it settles |
|---|---|---|
| 00 | README (this file) | index, reading order, the diagram map, settled decisions, conventions |
| 01 | [The state of the field, October 2026](01-research-2026.md) | the sourced evidence base: Lean and kernels, Mathlib, AI provers and results, misformalization, sources and licences, norms, benchmarks, funding |
| 02 | [The goal](02-the-goal.md) | the one-sentence goal, why now, the axes of progress, the reference scale, non-goals |
| 03 | [Architecture](03-architecture.md) | the eight boxes, the record and statement planes, the four flows, cross-cutting decisions, deployment |
| 04 | [The object model](04-object-model.md) | the 11 node kinds, tags and `m2://` URIs, the node file, proof nodes, computed status (E and F levels), provenance, prohibitions |
| 05 | [The knowledge graph](05-knowledge-graph.md) | the 23 edge types, alignment, granularity, AND–OR closure, queries Q1 to Q12, the storage engine, graph health |
| 06 | [Versioned storage](06-versioned-storage.md) | the three stores, calendar releases, content addressing, registers, provenance, mirrors, sizes |
| 07 | [The formal engine](07-formal-engine.md) | pinned Lean and Mathlib, the `Math2` package, bindings, extraction, the axiom policy, the checker stack, the monthly bump, other provers |
| 08 | [The computational engine](08-computational-engine.md) | CAS, SMT and numerical work as reproducible computations; certificates; the sandbox |
| 09 | [Literature ingestion](09-literature-ingestion.md) | sources, licence classes, extraction, alignment, library import |
| 10 | [Autoformalization and statement fidelity](10-autoformalization-and-fidelity.md) | the pipeline, the F1 battery, the M-* misformalization taxonomy, human fidelity review |
| 11 | [The Mathematical API and the agent orchestrator](11-api-and-orchestrator.md) | the read, check and propose surface over HTTP, gRPC and MCP; Lean sessions; jobs, priority, budgets, run records |
| 12 | [Research agents](12-research-agents.md) | the roles, the common loop, models, formalize and prove, explore, explain, untrusted text, the weekly report |
| 13 | [The research workspace](13-research-workspace.md) | node and topic pages, exploration, explanations at levels, projects, threads and claims |
| 14 | [Search](14-search.md) | the four channels, RRF fusion, formal search, premise selection, baselines |
| 15 | [Trust and the audit](15-trust-and-audit.md) | who may raise status, the 65 audit rules (families T S B G F V C A L P R X), incident response |
| 16 | [Collaboration and governance](16-collaboration-and-governance.md) | roles, review, projects, upstreaming, the licence zones, disclosure, funding |
| 17 | [Evaluation](17-evaluation.md) | metrics per box, agent evaluation, novelty checks, judges, external benchmarks |
| 18 | [Coverage](18-coverage-all-math.md) | what "all mathematics" means: C0 to C4 coverage levels by MSC area |
| 19 | [Milestones](19-milestones.md) | M0 (Nov 2026) to M5 (Jun 2028), with exit criteria and costs |
| 20 | [Repository layout](20-repository-layout.md) | the directory trees of both repositories and the `m2` command tree |
| 21 | [Open questions](21-open-questions.md) | Q-01 to Q-17, each with options, a lean and a deciding measurement |

## Reading order

- **Overview, in about 20 minutes:** 02, then 03, then 19.
- **Content contributors:** 04, then 05, 10 and 16.
- **Toolchain implementers:** 03, 04, 05, 06 and 07, then 11, 15 and 20. Then 08, 09, 12 and 14 as necessary.
- **Agent work:** 11, 12, 10 and 17.
- **Reviewers of the design:** 01 first, then 15 and 21. 01 is the evidence. 15 and 21 show where the design could be wrong.

## The diagram, box by box

The architecture diagram has eight boxes. This table gives the location and the specification of each box:

| Box | Lives in | Specified in |
|---|---|---|
| Research Agents | `math2-agent`: `agent/*` | 12 |
| Mathematical API and Agent Orchestrator | `math2-agent`: `api`, `mcp`, `orchestrator` | 11 |
| Formal Engine | `math2`: `lean/`; `math2-agent`: `lean`, `lean/M2Extract` | 07, 10 |
| Computational Engine | `math2`: `compute/`; `math2-agent`: `compute`, `images/` | 08 |
| Literature Ingestion | `math2-agent`: `ingest`, `extract`, `align` | 09 |
| Knowledge Graph | `math2`: `nodes/`, `reviews/`; `math2-agent`: `graph`, `store` | 04, 05 |
| Versioned Storage | git, plus the content-addressed artefact store | 06 |
| Research Workspace | `math2-agent`: `web/`, `m2 site`, `m2 tui` | 13, 14 |

Three supporting parts apply across all the boxes: search (14), autoformalization and fidelity (10), and the audit (15).

## Settled decisions

These decisions are settled. To change one, open a PR to this file. Link the PR to a measurement or an incident that shows the decision is wrong. Undecided items are in 21. When an open question in 21 is settled, move it here with its PR link.

### Architecture (03.5)

1. **Files in git are the truth.** Every database, index, embedding and site is derived from these files. Each of them can be rebuilt (05.1, 06.1).
2. **Status is computed, never asserted.** No one, person or agent, writes an evidence level. The build derives it from the evidence (04.7).
3. **Only the kernel and certificate checkers referee E4 and E5.** Models produce candidates, never verdicts. LLM judges triage. They never raise status (15.1).
4. **Fidelity is a separate axis.** Each F-level is bound to the type hash of the Lean statement. Only people raise an F-level to F2 or above (04.7, 10).
5. **Every byte has provenance.** Every agent run has a run record and a responsible person or schedule (06.5, 11.6).
6. **Tags are permanent.** A tag is never reused or deleted. Retirement keeps the tag and points to its successor (04.3).
7. **Every write is a pull request, and the audit is the gate.** The API has no other write path (11.1, 15).
8. **Each release pins everything:** the Lean toolchain, the Mathlib tag, container image digests, model ids and prompt versions (06.2).
9. **Link, do not copy.** math2 holds external records as locators and metadata only, unless the licence allows more (16.5, 16.6).
10. **There is one URI space, `m2://`,** and it is inside the `ant` URI space (04.4).

### Formal stack (07)

11. **Releases are calendar-versioned `YYYY.MM`.** A release is cut each month. Patch releases are `YYYY.MM.N`. Each release pins a Lean *stable* release and one compatible Mathlib revision. This revision is normally its `v4.X.Y` tag (06.2, 07.1).
12. **The `Math2` Lean package uses the module system only** (07.2).
13. **Proofs are checked by `lake check --paranoid`, plus comparator against the bound statement, plus math2's own type-hash check.** Each E5 result records the checker versions (07.7).
14. **Axioms are limited to `propext`, `Quot.sound` and `Classical.choice`.** A proof that uses `native_decide` or `implemented_by` is not E5. At best it is E4 under a named axiom class (07.5).
15. **Lean declaration names are time-scoped aliases, not identities** (05.3).
16. **Defer to Mathlib** and do not duplicate it. Send general-purpose material upstream. math2 holds the bindings and the material that is out of scope for Mathlib (07.3, 16.5).

### Content and licences (16.6)

17. **There are three licence zones:** prose CC BY-SA 4.0, code Apache-2.0, metadata CC0.
18. **Source rules:**
    - Link GFDL sources (the Stacks Project). Never copy them.
    - For arXiv papers under the default licence, store only metadata and locators.
    - Do not store MSC descriptions (CC BY-NC-SA).
    - Take Stack Exchange content per post through the API, with attribution.
19. **No raw embedding vectors are served** (14.4). The API returns neighbours and scores. It is still open whether embeddings of `open`-class text can ever be published. This is 21 Q-14.

### Agents and process

20. **Agent PR volume is capped by measured review throughput** (11.5).
21. **Novelty verdicts are never "new".** The best verdict for an explore claim is "no prior result found by the documented search" (17.5).
22. **No autonomous target selection for research.** People choose explore targets and campaigns. Agents rank within them (12.5). 21 Q-10 asks whether to relax this rule later. Until then, this rule applies.
23. **Node count is not an axis of progress.** Problems solved is also not an axis. The axes are in 02.3.
24. **Disclosure is generated, not written by hand.** `m2 prov disclose` produces AGMAI-style disclosure text and Palomar `formalization.yaml` from run records (16.5).

## Conventions

- **Section references** have the form `07.7` (document 07, section 7), `05.6 Q5` (a numbered query) or `21 Q-11` (an open question).
- **Tags** are 6 characters of Crockford base32 (`0-9 A-Z` without `I L O U`), for example `7K3QX2`. Tags that start with `0` belong to the M1 seed. A provisional id is `~` followed by 8 hex characters (e.g. `~3fa94c01`). Use a provisional id only before merge. The merge bot rewrites it to a tag (04.3).
- **URIs** have the form `m2://TAG`, `m2://TAG@2027.03` (at a release), `m2://TAG@<commit>` or `m2://TAG#formal` (a section). Aliases such as `m2://concept/compact-space` or `m2://lean/<decl>` resolve to tags. Never store an alias as a reference (04.4).
- **Evidence levels:**
  - E5 kernel-checked
  - E4 certified computation
  - E3 refereed
  - E2 standard
  - E1 claimed
  - E0 open
  - E⊥ refuted
- **Fidelity levels:** F0 bound but unchecked; F1 passes the machine battery; F2 one human review of this exact statement hash; F3 two independent reviews, or a second independent formalization proved equivalent (04.7, 10).
- **Coverage levels:** C0 to C4 (18).
- **Model ids:** `claude-haiku-5-5`, `claude-sonnet-5-5`, `claude-opus-5-5` and `claude-fable-5-1`, plus open-weight provers. Each release pins all of them (12.3).
- **Confidence tags** in 01: [P] primary, [S] secondary, [U] unverified.
- **Normative language:** "must" and "never" state requirements. "Should" states the default. Do something different only for a documented reason. Measurements tune the numbers marked as targets. These numbers are not promises.

## Note on inputs

The original request referred to two images. Only the second image arrived. It is the eight-box architecture diagram. This specification follows that diagram exactly (03.1). If the first image showed something that this specification does not cover, raise it as a new item in 21.
