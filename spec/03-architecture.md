# 03. Architecture

This document is the map. It takes the eight boxes of the architecture diagram. For each box, it says what the box is, which repository holds it, what goes in and out of it, and which document specifies it. Then it follows four end-to-end flows through the boxes. It also gives the planes that divide the content, and it lists the cross-cutting decisions that all boxes share.

## 3.1 The diagram

```
                         ┌───────────────────────────────┐
                         │        Research Agents        │
                         │ Formalize, Prove, Explore,    │
                         │ Explain                       │
                         └──┬──────────────────┬─────────┘
                            │ ▲                │
                            ▼ │                ▼
┌──────────────────────┐  ┌─┴───────────────┐ ┌────────────────────┐ ┌────────────────────┐
│ Mathematical API and │  │  Formal Engine  │ │ Computational      │ │ Literature         │
│ Agent Orchestrator   │──┤  Lean 4,        │ │ Engine             │ │ Ingestion          │
│                      │  │  Mathlib,       │ │ CAS, SMT,          │ │ Papers, Books,     │
│                      │  │  Proof Checker  │ │ Numerical Exper.   │ │ Existing Libraries │
└──▲───────────────┬───┘  └──┬──────────┬───┘ └───┬──────────┬─────┘ └───┬─────────┬──────┘
   │               │         │          │         │          │           │         │
   │               ▼         ▼          │         ▼          │           ▼         │
   │          ┌─────────────────────────┴─────────────────┐  │                     │
   │          │              Knowledge Graph              │◄─┴─────────────────────┘ (via graph)
   │          │  Statements, Concepts, Dependencies       │
   │          └──────┬───────────────────────┬────────────┘
   │                 │                       │
   │                 ▼                       ▼
┌──┴─────────────────────────┐   ┌───────────────────────────────┐
│ Research Workspace         │   │ Versioned Storage             │◄── Formal, Computational,
│ Search, Explore, Explain,  │   │ Metadata, Artifacts,          │    Ingestion (artefacts)
│ Collaborate                │   │ Provenance                    │
└────────────────────────────┘   └───────────────────────────────┘
```

The arrows show the direction of data flow. Four of them carry most of the design:

1. **Research Agents → Formal Engine and → Computational Engine.** Agents do their work through calls to the engines. They never write directly to the graph or to storage.
2. **Formal Engine, Computational Engine, Literature Ingestion → Knowledge Graph.** Evidence enters the graph only from the three producers that can be checked: the kernel, certificates, and sources with locators.
3. **Everything → Versioned Storage.** Every producer writes artefacts and provenance. The graph writes its derived form.
4. **Workspace → API/Orchestrator → Agents.** People control agents through the same API that all other parts use.

## 3.2 The boxes

| Box | Repository · package | Specified in | In | Out |
|---|---|---|---|---|
| Research Agents | `math2-agent` · `agent/*` | 12 | jobs from the orchestrator; API reads; engine results | proposed diffs (nodes, bindings, proofs, alignments, explanations); run records |
| Mathematical API and Agent Orchestrator | `math2-agent` · `api`, `mcp`, `orchestrator` | 11 | reads from the graph store; requests from the workspace, external clients and schedules | query responses; jobs; PRs against `tamnd/math2` |
| Formal Engine | `math2` · `lean/`; `math2-agent` · `lean`, `lean/M2Extract` | 07, 10 | Lean code; bindings; Lean sessions from agents | kernel-checked declarations, dependency edges, type hashes, axiom profiles, replay results |
| Computational Engine | `math2` · `compute/`; `math2-agent` · `compute`, `images/` | 08 | computation records; ad hoc requests from agents | outputs, certificates, checker verdicts |
| Literature Ingestion | `math2-agent` · `ingest`, `extract`, `align` | 09 | pinned snapshots of external sources | record plane entries, extracted results, alignment proposals, imported nodes |
| Knowledge Graph | `math2` · `nodes/`, `reviews/`; `math2-agent` · `graph`, `store` | 04, 05 | node files; formal, compute and ingestion outputs | the derived graph (Parquet), computed status, the serving store |
| Versioned Storage | git (`tamnd/math2`) plus the artefact store | 06 | everything | releases, artefacts by hash, provenance |
| Research Workspace | `math2-agent` · `web/`, `m2 site`, `m2 tui` | 13, 14 | the API | pages, search, exploration, explanations, collaboration, work requests |

Three supporting parts are not boxes in the diagram, but they go through all of the boxes:

- **Search** (document 14) serves the workspace, the API and the retrieval step of the agents.
- **Autoformalization and fidelity** (document 10) is at the point where agents, the formal engine and reviews by people meet.
- **The audit** (document 15) is the gate in front of `main`.

## 3.3 Planes

The content is divided into planes. As in `tamnd/arxiv`, the planes exist because their parts differ in licence, scale and trust.

| Plane | What | Scale | Trust | Licence of our copy |
|---|---|---|---|---|
| **record** | bibliographic records of the mathematical literature, and the results extracted from licence-permitted full text, each with a paper-scoped id | ~5 M records, ~30 M results | records are facts; extracted results are E1 until aligned | CC0 metadata; extracted text only from open-licence papers, under their licence |
| **statement** | curated nodes: concepts, definitions, statements, proofs, conjectures, problems, examples, objects, computations, topics | 2 to 5 M nodes | E0 to E5, computed | CC BY-SA 4.0 prose, CC0 metadata |
| **formal** | Lean code in `Math2/`, plus bindings to Mathlib and other libraries | Mathlib's ~0.5 M declarations bound selectively; `Math2/` as small as possible | kernel | Apache-2.0 |
| **compute** | computation code, records and certificates | 10⁴ to 10⁶ records | E1/E4 | Apache-2.0 code; CC0 records |

The record plane covers **all** of mathematics at the level of "this paper exists and states these results". The statement plane covers all of it at the level of "this is a result, and this is why we believe it". With this split, "all mathematics" stays an engineering target and does not become a slogan. The record plane gives full coverage of what exists. The statement plane gives coverage of what is understood, and that coverage becomes deeper over time. Document 18 measures the distance between them per MSC code.

## 3.4 Four flows

### Flow A: a theorem in a paper becomes a formal node

1. **Ingestion** fetches a new arXiv batch and the matching zbMATH Open records. It uses the metadata plane of `tamnd/arxiv`, and its content plane where the licence permits. The extractor reads the theorem environments of each open-licence paper. It writes record plane results such as `ax://paper/2610.01234#7Q2M`, with their text, their labels and their internal references (document 09.5).
2. **Alignment** embeds each result and finds its neighbours among live nodes. Then it asks the judge about each pair. The result is new (no `same`), and a statement already in the graph cites it. So the curate agent proposes a new `statement` node with a `source:` that points at the paper result. After audit, a curator merges the PR, and the node is minted as E1, claimed.
3. The centrality and demand of the node put it on the **formalize queue**. The formalize agent drafts a Lean statement against Mathlib and runs the F1 battery (document 10.4). It opens a PR that adds the binding at F0. When the battery passes, the audit shows F1.
4. A **reviewer** signs a fidelity review in `m2 review`. The node is now F2.
5. The **prove agent** takes the job ("F2 statement with no formal proof"). It decomposes the proof into lemmas. Each lemma is a new provisional node with its own statement. The agent proves the lemmas in Lean sessions. Then it opens a PR with the proof nodes and the Lean code in `Math2/`.
6. **CI** builds, replays with the external checker and audits. The node is now E5/F2.
7. Weeks later, someone upstreams the Lean proof to Mathlib. The **bump agent** sees the new Mathlib declaration, because its type hash matches the type hash of the binding. It proposes to rebind the node to Mathlib and to deprecate the `Math2/` copy. The review stays valid, because the type hash did not change.

### Flow B: an experiment becomes a conjecture, and a conjecture is refuted

1. A person in a project asks the **explore agent** for patterns in a family of graphs. The agent writes computations and runs them in the **computational engine**. It finds a regularity up to n = 40.
2. The falsification pass of the agent extends the range to n = 60. It compares the sequence with OEIS and finds a match: the sequence is A-something, and OEIS already lists a conjectured formula. Where relevant, it also checks LMFDB-style databases. The agent files a `conjecture` node with `source:` set to the OEIS entry and the computation. It does not file it as a new discovery.
3. A second person asks for a counterexample search. An SMT encoding at n = 71 finds a model. Lean checks the witness of the computation. The counterexample node is E5. The conjecture gets a `refutes` edge, and it is now E⊥.
4. The release notes for that month list the refutation. A person reports the conjecture in the OEIS entry to the OEIS editors, through the normal OEIS process.

### Flow C: a Mathlib bump

1. Each week, the **bump agent** moves a branch to the newest Mathlib `master` that is compatible with the current Lean toolchain. Then it runs the build.
2. Bindings break where Mathlib changed names. The agent applies the renames from the Mathlib deprecation attributes. These changes are mechanical and are eligible for auto-merge. For each item without a deprecation alias, the agent proposes a rebinding. It finds the rebinding through a type-hash match against the new Mathlib.
3. For some bindings, the type hashes changed but the names did not, because a definition was refactored. The reviews of those bindings become stale. The nodes drop to F0 and go to the top of the fidelity queue.
4. The branch merges when it is green. The release at the end of the month pins it.

### Flow D: a reader asks a question

1. A student searches for "every finite subgroup of the multiplicative group of a field is cyclic". The embedding channel of the **search** engine finds the node. Its formula channel confirms it.
2. The page shows: proved in Lean, reviewed, classical; three proofs; prerequisite path from the student's declared background.
3. The student opens "Ask about this node" and asks why finiteness matters. The **explain agent** answers with $\mathbb{Q}^\times$ (infinite, not cyclic) as the example. It cites the counterexample node by tag.
4. No node for that counterexample exists. The answer marks the claim "supported by computation, not by a node". It also files a curate job. The next week, the node exists.

## 3.5 Cross-cutting decisions

These apply to every box. Document 00 lists them as settled.

- **Files in git are the truth. Everything else is derived and rebuildable** (05.1, 06.1).
- **Status is computed from evidence and never asserted** (04.7).
- **The kernel and certificates are the only referees for E4 and E5. Models produce candidates, never verdicts** (15.1).
- **Fidelity is a separate axis, bound to type hashes, and reviewed by people** (04.7, 10).
- **Every byte has provenance. Every agent run has a run record and a responsible person or schedule** (06.5, 11.6).
- **Tags are permanent. Deletion is retirement** (04.3).
- **Writes are pull requests. The audit is the gate** (11.1, 15).
- **Pin everything per release: Lean, Mathlib, images, models' recorded ids** (06.2).
- **Link, do not copy, wherever a licence or a community asks for it** (16.5, 16.6).
- **One URI space, `m2://`, inside `ant`'s.** The same tools dereference every math2 record and every other `tamnd` record (04.4).

## 3.6 Deployment

At reference scale (document 02), the deployment is small:

| Component | Runs on | Notes |
|---|---|---|
| `m2 serve` (API, MCP, workspace) | 2 VMs behind a CDN | stateless over a release's serving store; ~64 GB RAM each for the CSR and the vector index |
| static site | CDN only | per release |
| `m2 orchestrate` | 1 VM | job queue in SQLite or Postgres; restarts are idempotent |
| Lean session pool | autoscaled runner VMs, 32 to 64 GB each | images with Mathlib preloaded; each session sandboxed |
| compute runners | autoscaled, x86-64 and arm64 | the C02 two-architecture rule needs both |
| CI | GitHub Actions plus self-hosted large runners for full builds | a full Mathlib-dependent build of `Math2/` with the Mathlib cache takes minutes; a cold Mathlib build is avoided by the cache |
| model inference | Claude API (Anthropic); optional GPU nodes for open prover models | document 12.3 |
| artefact store | two S3-compatible providers | document 06.6 |

No component in this table needs to be custom-built. Only one part is expensive at scale: model inference. For this reason, the orchestrator has budgets (11.5) and the escalation ladder, and the agent report publishes costs every week.
