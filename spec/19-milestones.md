# 19. Milestones

There are six milestones, M0 to M5. Each one ends in a release. Someone outside the project can check that release against written exit criteria. The `m2` command table (20.2) names the milestone in which each command arrives. A command that has not arrived exits non-zero and says which milestone delivers it.

The dates assume that work starts in November 2026 with a small core team. The team is two to three engineers, one Lean lead, and a curator group that grows from five to about thirty people. Each milestone has a short "out of scope" list. Thus work that slips is visible and does not move quietly to the next milestone.

## M0: the skeleton (Nov to Dec 2026)

**Deliverables**

- Both repositories, with the layout in document 20, the licence files, and contribution guides.
- The node schema v1 (04.5) as JSON Schema, with fixtures.
- Tag minting and the provisional-id flow (04.3).
- `m2 new`, `m2 fmt`, `m2 check`: schema, body grammar and link checks on a working tree, with no Lean.
- CI for both repositories. The `lean/` package pins a Lean stable release and a Mathlib tag (07.1), and builds an empty `Math2` with the module system.
- This specification, committed as `tamnd/math2-agent/spec/`.
- The public wiki at https://tamnd.github.io/math2. `m2 site build` makes it from `main`, and each push to `main` updates it. It has a page for each node and each spec document, and JSON, Markdown and `llms.txt` for agents. The plan moved this from M3 to M0, so that readers and agents can use the graph from the start.

**Exit criteria**

- `m2 check` runs on 100 hand-written fixture nodes that cover all 11 kinds. It catches every planted error in a fixture set of 200 broken nodes.
- A contributor who is new to the project uses only the README. With it, they can add a node with a provisional id, open a PR, and have the PR pass CI.
- The wiki shows the spec, the milestones and each node on `main`, with no manual step.

**Out of scope:** status computation, graph, any agent.

## M1: the seed and the trust core (Jan to Mar 2027)

**Deliverables**

- **The bootstrap seed**: about 2,000 curated nodes, with tags that begin with `0` (04.3). They cover the concept skeleton of first- and second-year undergraduate mathematics, with sources, edges and Mathlib bindings.
- `m2 build`: Lean build, extraction (07.4), status computation (04.7) and the derived graph.
- `m2 audit` with every T, S, B, G, F and V rule (15.3), and `m2 status`.
- The checker stack: `lake check --paranoid` and the comparator against pinned statements (07.7), with checker versions recorded.
- Fidelity reviews as data (10.6), signed by hand. There is no review tool yet.
- **Release 2027.03**: the first numbered release, with a DOI, archived to Software Heritage and Zenodo.

**Exit criteria**

- Every E5 node in the seed passes replay in at least two independent kernels.
- Every seed node is at E2 or better, and at least 30% are at E5 with F2.
- The release rebuilds bit-for-bit from its manifest on a second machine (rule `R01`).
- An external Lean user reviews 50 random F2 bindings and agrees with 49 or more.

**Out of scope:** ingestion beyond Mathlib, search, the workspace.

## M2: the graph meets the world (Apr to Jun 2027)

**Deliverables**

- `m2 graph`: queries Q3 to Q7 (05.6).
- `m2 ingest` for the first sources:
  - arXiv metadata through `tamnd/arxiv`;
  - zbMATH Open;
  - Mathlib (full extraction);
  - Wikidata;
  - OEIS;
  - formal-conjectures;
  - erdosproblems;
  - Palomar;
  - the Stacks Project (tags only);
  - LMFDB (labels).
- `m2 align`, with crosswalk edges and the curation queue (16.3).
- `m2 bump`: the monthly Mathlib bump, with its report (07.9).
- `m2 release`: monthly releases from this point. Each release pins a new Mathlib tag when the bump is green.

**Exit criteria**

- **C0 at 100%** for every MSC class: every zbMATH Open and arXiv math record is in the record plane.
- Three consecutive monthly bumps complete with no hand edits outside `Math2/` repairs.
- Alignment precision at least 0.98 on the held-out set (17.3.3).
- Q1 to Q7 meet their p95 targets at reference scale. Where the real graph is still smaller, the test uses synthetic nodes (02.4).

**Out of scope:** extraction of results from full text, public search.

## M3: open to readers (Jul to Sep 2027)

**Deliverables**

- `m2 search` with all four channels (14), and the benchmark in 14.7.
- `m2 serve`: the read API, the read-only MCP profile, and the workspace's node pages, search, explore views and graph (13).
- `m2 compute`: the computational engine with certificates (08). Executable examples have a "run this" button.
- `m2 prov`: show, credit and quarantine.
- Result extraction from licence-permitted arXiv full text (09.5), for C1.
- The offline archive of each release. The public site starts in M0. M3 adds the explore views, the graph view and the API to it.

**Exit criteria**

- The search benchmark beats the best external baseline on concept lookup and paper-to-node. The release notes report every metric on which it does not.
- The workspace meets the performance and accessibility targets of 13.7.
- Any MCP client, Claude Code included, can use the read-only profile against the public endpoint with no math2-specific code.
- C2 skeletons exist for 30 top-level MSC classes.

**Out of scope:** write access through the API, autoformalization.

## M4: fidelity at scale (Oct to Dec 2027)

**Deliverables**

- `m2 review`: the reviewer's tool and review packets (10.6).
- `m2 autoform`: the formalize pipeline with the full F1 battery (10.2 to 10.4).
- Fidelity evaluation: the battery suite with its confusion matrix (17.3.1), and the monthly re-review sample (10.8).
- Write access through the API, which opens PRs (11).
- Reviewer onboarding and the credit export (16.4).

**Exit criteria**

- **The undergraduate core (18.3) is at C3 100% and C4 50%.**
- Measured F2 disagreement is under 2%, from at least 500 re-reviews.
- The battery's recall is at least 0.9 on every misformalization class with 30 or more labelled items.
- At least 50 active reviewers, none of whom is a project employee.

**Out of scope:** unattended agent runs.

## M5: the research engine (Jan to Jun 2028)

**Deliverables**

- `m2 agent run` for every role in 12.1, and `m2 orchestrate` with budgets, the escalation ladder and the work queues (11.4, 11.5).
- Role versioning, shadow weeks and the weekly agent report (12.8, 12.9).
- Projects in the workspace, with project budgets for agent work (13.6).
- The novelty check (17.5) for every explore output.
- The quarterly public run dataset (17.4).

**Exit criteria**

- **Undergraduate core at C4 80%**, and every E5 node in it at F3. Most of the F3s come from cross-formalization.
- C2 skeletons for every top-level MSC class in scope (18.2).
- People outside the core team run at least three research projects. Each project produces work that they publish elsewhere.
- Per-role solve rates and costs are published for 12 consecutive weeks.
- No agent output raised a status level without a kernel check or a person (rule audit `X`, zero violations).

## After M5

The milestones stop at M5, because at M5 math2 is a complete system. After M5, the work is about depth: graduate cores, research cores on demand, C1 everywhere that licences allow, and more formal libraries bound. Releases continue monthly. Every release reports the axes in 02.3. The targets for 2029 will come from the measurements, and math2 does not set them now.

## Costs, roughly

These are planning numbers. math2 publishes them so that a funder can compare them with the weekly reports.

| Item | M0 to M2 | M3 to M5 |
|---|---|---|
| people | 3 to 4 FTE | 5 to 6 FTE plus paid reviewer stipends |
| model inference | under $5k per month, mainly alignment and extraction in batch | $20k to 60k per month, set by budget, not demand (11.5) |
| compute (CI, Lean runners, serving) | about $3k per month | about $10k per month |
| storage and archiving | small | under $1k per month |

The model line is the only line that can grow without limit. It is also the line with the clearest controls. Batch pricing, prompt caching and the escalation ladder (12.3) keep most calls on the cheapest model that passes the suite of the role. The weekly report shows what each dollar bought.
