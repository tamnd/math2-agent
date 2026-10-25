# 02. The goal

## 2.1 One sentence

**math2 is a versioned, executable, verifiable and navigable record of all of mathematics. Every statement in it carries the evidence for it, and agents and people extend it under rules that keep that evidence honest.**

Two repositories implement it. They split content from tools in the same way as `tamnd/arxiv` and its reader:

- **`tamnd/math2`** holds the content: nodes, edges, Lean code, computations, reviews, registers and releases. Prose is CC BY-SA, code is Apache-2.0 and metadata is CC0 (16.6).
- **`tamnd/math2-agent`** holds the tools: the `m2` binary in Go, with the build, audit, search, API, MCP server, workspace, orchestrator and agent roles. It also holds the evaluation suites.

## 2.2 Why now

By October 2026, three things had changed at the same time. Document 01 gives the sources.

1. **Proofs became cheap.** Agents saturated PutnamBench and IMO-level formal proof. They formalized Fermat's Last Theorem in about 13 million lines of Lean in eleven days. They produced research-level results that experts accepted, among them Erdős problems, OEIS conjectures and First Proof lemmas. They also produced hundreds of manuscripts in a single release.
2. **Checking did not become cheap in the same way.** The kernel checks proofs. Only people check that a formal statement says what was meant. Misformalization rates in public benchmarks were between a few percent and a third. The largest disputes of 2026 (Navier–Stokes among them) were about whether the formal statement was the correct one. They were not about whether the proof was valid. The Mathlib review queue continued to grow.
3. **The community set norms.** These include the Leiden Declaration, the AGMAI guidelines, journal disclosure rules and Palomar's registry. They also include the demand of erdosproblems.com for write-ups by people. All of them say the same thing: disclose how a result was produced, keep it independently checkable, and keep people responsible for it.

Terence Tao's "Math 2.0" lecture (October 2026) describes the result. Mathematics was built for a time when proofs were scarce. Now the bottlenecks are verification, exposition, attribution and understanding. math2 is designed for exactly those four:

| Bottleneck | What math2 does about it |
|---|---|
| **verification** | status is computed from evidence (04.7); formal proofs are replayed in independent kernels against pinned statements (07.7); fidelity is reviewed and measured (10) |
| **exposition** | every node is explained at levels, grounded in the graph, with unconfirmed agent prose marked (12.6, 13.4) |
| **attribution** | provenance on every field, to a person, source or agent run (06.5); credit statements (16.4); disclosure generated from data (16.5) |
| **understanding** | the graph: what depends on what, what is known, what is open, and how to get from here to there (05, 13.3) |

math2 does not try to be the fastest prover. Other groups build provers, and the provers become better every month. math2 is the place where their output becomes trustworthy, findable and connected to the rest of mathematics, or where it gets a mark that says what it is.

## 2.3 The axes of success

Each axis has a measure, a target, and a document that defines the measure. The targets apply at the end of the milestones in document 19. Release notes report progress on every axis.

| # | Axis | Measure | Target | Defined in |
|---|---|---|---|---|
| 1 | **coverage** | per-MSC coverage levels C0 to C4 | C0 100% everywhere; C2 skeleton for every top-level MSC class; the undergraduate core (about 25,000 nodes) at C3 100% and C4 ≥ 80% | 18 |
| 2 | **trust** | measured error rates of status levels | sampled E2 to E4 error under 1%; F2 re-review disagreement under 2%; F3 under 0.5%; zero E5 nodes that fail replay | 10.8, 15, 17.8 |
| 3 | **fidelity** | share of formal bindings at each F-level | every E5 node in the undergraduate core at F3 (most by cross-formalization); every conjecture with a formal statement at F2 or better | 10 |
| 4 | **executability** | share of nodes whose evidence reruns from the release alone | 100% of E4 to E5 evidence reruns from pinned images; ≥ 70% of `example` and `counterexample` nodes executable | 06.2, 08.7 |
| 5 | **performance** | latency on the reference machine (2.4) | every graph query Q1 to Q12 within its target (05.6); a node page under 1 s first paint (13.7); `m2 check` on a typical PR under 10 s | 05.6, 13.7 |
| 6 | **permanence** | tags and URIs that stop resolving | zero; every release reproducible and archived | 04.3, 06 |
| 7 | **research yield** | merged agent output per dollar, by role, with review cost counted | published every week; rising per release on the held-out suites | 12.9, 17 |
| 8 | **openness** | shares of content, runs and evaluations that are public | all content and merged runs public; every evaluation report public, including failures | 11.6, 17 |

Two measures are not on the list, and this is intentional. **Node count** is not an axis, because it is easy to inflate (18.5). **Problems solved** is also not an axis. Open problems get solved *in* math2, through projects that people run. math2 does not optimise for them. Tao's warning about "blind optimization of problem-solving" applies to infrastructure as much as to models.

## 2.4 Reference scale and reference machine

All performance targets are measured on a **reference machine**: 16 cores, 64 GB RAM, NVMe storage, no GPU. It serves the API and the workspace from the serving store of a release, at **reference scale**: 5M statement-plane nodes, 5M records, 30M results and 300M edges (05.7, 06.7, 18.4). A design that needs more than this to meet axis 5 is a design bug. The production deployment (03.6) uses a few of these machines for availability, not for capacity.

The build has a separate reference. A full release build includes Lean replay, extraction, graph, search and audit. It finishes within 24 hours on a 96-core build machine with the Mathlib cache. The incremental build for a PR finishes within 20 minutes.

## 2.5 Non-goals

- **Not a new proof assistant or a fork of Mathlib.** math2 uses Lean and Mathlib as pinned dependencies. Code that is worth upstreaming goes to Mathlib through Mathlib's own process (16.5).
- **Not a prover.** math2 orchestrates provers and models. It builds the verification, search, decomposition and bookkeeping around them (12).
- **Not a journal.** math2 does not referee. E3 records that a refereed publication exists (04.7). The reviews in math2 are fidelity reviews of formal statements. They are not judgements of significance.
- **Not a copy of the literature.** The record plane stores records, locators and derived data. The text stays where its licence lets it be (09.3).
- **Not a replacement for nLab, the Stacks Project, Wikipedia, the OEIS or the LMFDB.** math2 links to them and says which of its nodes they correspond to (05.3). Each of them does its own job better than math2 would.
- **Not autonomous.** Agents act for named people and projects, inside budgets, through PRs (12, 16). No agent action raises a status level without a kernel or a person.
