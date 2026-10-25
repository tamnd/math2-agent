# 05. The knowledge graph

The graph is the box in the middle of the architecture diagram: Statements, Concepts, Dependencies. Every other box writes into it or reads from it. This document defines the edges of the graph and its alignment with the rest of the world's mathematical data. It also defines how the graph is stored and rebuilt, and the queries that it must answer quickly.

## 5.1 Where the graph lives

The graph has no separate database that holds the truth. **The truth is the node files in git** (document 04). `m2 build` derives the graph from them, together with three other derived inputs:

1. edges written by hand in node front matter (`uses`, `proofs`, `topics`, `same_as`, …);
2. edges extracted from the formal plane: the Lean dependency graph of every bound declaration, at the pinned Mathlib and math2 revisions (document 07.6);
3. edges from the record plane: citations between papers, and paper results aligned to nodes (document 09.6).

The derived graph goes to `build/graph/` as Parquet files (one for nodes, one for each edge family). It is then loaded into an embedded store for serving (5.7). It is never committed to `tamnd/math2`. It is published as a release artefact with a content hash (document 06). Anyone with the repository and the pinned toolchain can rebuild it byte for byte. Audit rule `R01` does exactly that in CI. For every merge it rebuilds a sample of shards, and for every release it rebuilds everything.

`tamnd/arxiv` made the same choice: commit what a person decided, and derive the rest. As a result, a reviewer reads a diff of decisions, not a diff of derived numbers.

## 5.2 Edge types

Edges are typed and directed, and their types come from a closed list. Each edge also carries its own evidence and provenance. "A implies B" is a mathematical claim, the same as a node. Its level can be anything from E1 to E5.

### Logical edges (they carry truth)

| Edge | From → to | Meaning | Can be E5 |
|---|---|---|---|
| `proves` | proof → statement | this argument establishes that statement | yes |
| `uses` | statement/proof/definition → any | the source depends on the target: a hypothesis, a lemma, a definition | yes (computed from Lean) |
| `implies` | statement → statement | A ⟹ B, proved by the proof node named in `via` | yes |
| `equivalent` | statement ↔ statement, definition ↔ definition | A ⟺ B, via a proof | yes |
| `refutes` | counterexample/statement → statement/conjecture | the source shows the target is false | yes |
| `resolves` | statement → conjecture/problem | the source settles the target, positively or negatively | yes |
| `generalizes` | statement → statement | the target is a special case of the source. Recorded with the specialising map | yes |
| `bounds` | statement → problem | the source gives a bound on the quantity the problem asks for (lower or upper, with the value) | yes |

### Conceptual edges (they carry meaning)

| Edge | From → to | Meaning |
|---|---|---|
| `defines` | definition → concept | this is one definition of that concept |
| `instance_of` | example/object → concept | the source is an instance of the concept |
| `about` | statement/conjecture/problem → concept | the result is about the concept, as one of its main subjects |
| `specializes` | concept → concept | every X is a Y (a ring is a semiring). These edges form a DAG, not a tree |
| `method` | proof → concept (in `methods`) | the proof uses this technique |
| `analogous_to` | any ↔ any | an analogy that mathematicians acknowledge, with a source. Soft, and never used for reasoning |
| `part_of` | any → topic | navigation only |

### Record edges (they carry provenance and identity)

| Edge | From → to | Meaning |
|---|---|---|
| `formalizes` | Lean declaration → node | the binding. Carries `match` and fidelity (document 07.4) |
| `source` | node → record | where this is stated or proved. Carries a locator |
| `aligned` | paper result → node | an extracted result (for example `ax://paper/2106.09685#AB12`) states this node. Carries a `match` grade |
| `same_as` | node → external id | the same thing in another system (5.3) |
| `close_to` | node → external id | a related but not identical entry in another system, with a note on how it differs |
| `cites` | record → record | the paper-to-paper citation graph, from zbMATH Open and arXiv |
| `resolved_by`, `successor` | retired node → node | lifecycle (document 04.3) |

There are 23 edge types. A new edge type is a schema change (document 04.9). The condition for a new type is: "some query in 5.6 needs it and cannot be written without it".

## 5.3 Alignment with the rest of the world

math2 does not try to replace nLab, Wikipedia, Wikidata, Mathlib, the Stacks Project, OEIS, LMFDB, zbMATH Open or MathOverflow. It aims to be the place that joins them together. Each of them does its own job very well, but none of them links reliably to the others. In its first year, math2 is most useful as a curated, versioned, machine-checkable **crosswalk** between them, with statuses attached.

`same_as` identifiers by external system:

| System | Identifier | Granularity | Notes |
|---|---|---|---|
| Wikidata | QID | concepts, named theorems, objects | the hub for every other crosswalk. The MaRDI knowledge graph is built on Wikibase and uses Wikidata-style ids |
| MaRDI portal | MaRDI QID | formulae, algorithms, software, research data | aligned through Wikidata where possible |
| Mathlib | fully qualified declaration name at a pinned revision | definitions, theorems | carried by `formalizes`, not `same_as` |
| Other formal libraries | Isabelle AFP entry + theorem, Rocq/MathComp qualified name, Metamath label, HOL Light name | definitions, theorems | `formalizes` with `system:` set accordingly |
| Stacks Project | 4-character tag | results, definitions | text under GFDL. We link and never copy (document 09.3) |
| Kerodon | 4-character tag | results, definitions | same as Stacks |
| nLab | page name + revision | concepts | |
| Wikipedia | page title + revision id | concepts, named theorems | revision-pinned, because titles move |
| ProofWiki | page title | statements, definitions, proofs | |
| OEIS | A-number | objects (sequences) | an OEIS sequence is an `object` node. Its formulae become `statement` nodes |
| LMFDB | the path of the object, for example `EllipticCurve/Q/11.a2` | objects | number fields, elliptic curves, modular forms, … The path has the collection and the label, because a label alone does not identify an object |
| zbMATH Open | document id; author id; MSC code | records | the main record plane source for "all mathematics" |
| MathSciNet | MR number | records | identifier only (no bulk licence) |
| arXiv | id + version, and per-paper result tag from `tamnd/arxiv` | records, results | `ax://paper/<id>#<tag>` |
| Erdős problems | problem number | problems/conjectures | |
| formal-conjectures (DeepMind) | file + declaration | conjectures with Lean statements | |
| MathOverflow / Math.SE | question id | sources for E1 claims | |
| Palomar | registry entry id | Lean-verified results | carries the entry's `formalization.yaml` disclosure and replay record (09.4) |
| TheoremSearch, TheoremGraph/LeanGraph, Matlas | native statement ids | extracted results | record plane `same_as`; their extractions are imported as candidates only (09.6) |
| Hexagon | `hexagon:` id + version | records | the 2026 repository for AI-era mathematics, with provenance labels |
| pi-Base, DaRT, FindStat, House of Graphs, GAP small groups, ATLAS | native ids | objects, properties, counterexamples | data sources with natural `instance_of` and `refutes` structure |

Alignment follows three rules.

1. **Alignment is a claim, and it has a grade.** A `same_as` that an agent writes starts as E1. A human confirmation raises it. An automatic match that cannot be ambiguous also raises it (an OEIS A-number cited verbatim in both places, an identical Lean declaration). The crosswalk publishes the grade of each edge, so consumers can choose their threshold.
2. **`same_as` is functional both ways within one system.** A node has at most one Wikidata QID, and a QID belongs to at most one live node (rule `G02`). When the granularity of a system does not match ours, we use `close_to` with a note. An example is one Wikipedia page that covers three theorems. Real systems disagree about granularity all the time. A forced bijection would make the crosswalk wrong in a way that nobody can see.
3. **Round-tripping is a test.** For every system with a public API, `m2 audit -plane align` fetches a sample of external records again. It checks that they still say what they said when the edge was made. If the external record is gone, or its revision moved and the content changed, that is a finding (rule `A03`). In this way, renames on Wikipedia, deprecations in Mathlib and merges on nLab appear in a report. They do not become silent link rot.

### Contributing the crosswalk back

Where the external system accepts them, alignments go back out to it. Wikidata gets `math2 tag` statements and `exact match` links. Mathlib gets a `math2` field in docstrings if the community wants one, and only as an opt-in suggestion (document 16.5). MaRDI gets the same through its Wikibase API. This is the cheapest way to make math2 useful to people who never visit it.

## 5.4 Granularity and deduplication

The hardest problem in a graph of all mathematics is not size. The hardest problem is this: the same theorem is stated a thousand times, in a thousand notations, at slightly different generality. These thousand statements have to collapse into one node or a small family of nodes. If they do not, the graph is only a pile of paper extracts.

math2 deduplicates in four passes. Each pass is cheaper and more certain than the one after it.

1. **Identifier collisions.** Two candidates cite the same external identifier, or bind to the same Lean declaration. These merge automatically.
2. **Formal equivalence.** Both candidates have formal statements, and a Lean proof shows that they are equivalent. The proof is an `Iff` between the two elaborated types. Usually `exact ⟨fun h => …, fun h => …⟩` from the formalize agent closes it, or `simp`/`grind` does. The candidates merge, and the equivalence becomes an E5 `equivalent` edge.
3. **Embedding neighbourhood plus judgement.** Statement embeddings (document 14.4) give the 20 nearest live nodes for each candidate. A judge model classifies each pair as one of `same`, `generalizes`, `specializes`, `related` or `different`. It has to justify the classification with the specialising map. A `same` from the judge is only a proposal. It becomes an edge only through pass 2 or a person.
4. **People.** Curators process the queue of ambiguous `same` proposals. The queue order is by how much of the graph each merge would connect (document 16.3).

Deduplication gives a canonical node plus `aligned` edges from every paper result that states it. This is also the only honest way to answer "where in the literature is this proved?". The answer is the set of aligned records in date order. That set gives a provenance chain for each theorem. Currently nobody maintains such a chain by hand.

### Families, not points

Some results have no canonical form, only a family: the dozen forms of the Hahn–Banach theorem, or the Cauchy–Schwarz inequality in each of its settings. math2 does not choose a winner. It makes one node for each form that people cite separately, `generalizes`/`specializes` edges between them, and a `concept`-like hub. Hubs are statements with `role: fact` and an empty formal binding. A hub is only a navigation point. The workspace shows it as "the Hahn–Banach family". A hub with fewer than three members is a finding (`G07`), because a family of two is only two nodes and an edge.

## 5.5 Dependency closure, with alternatives

"What does this theorem depend on?" has no single answer, because a statement can have several proofs (document 04.6). The closure is an AND–OR graph. A statement needs one of its proofs (OR), and a proof needs all of its `uses` (AND).

For every statement, the build computes:

- **min-depth closure**: the alternative with the smallest maximum depth down to axioms and definitions;
- **min-size closure**: the alternative with the fewest distinct nodes. This is an approximation, because the problem is NP-hard in general. The build uses a greedy algorithm with a recorded bound;
- **axiom profile**: which of the foundational nodes (choice, excluded middle, univalence where relevant, large cardinals, CFSG as a black box, the Riemann hypothesis as a hypothesis) occur in which alternative;
- **trust profile**: the weakest evidence level anywhere on the best alternative. Take a statement whose only proof is E5 but uses a lemma that is E3. It shows as "formal modulo 1 refereed lemma". This is the blueprint view, computed for the whole graph in one pass.

Conditional results are first-class. "Assuming GRH, …" is a statement whose `uses` includes the GRH conjecture node with `as: hypothesis`. When a conjecture is settled, the next build re-grades every conditional result that depends on it. When a conjecture is refuted, every such result gets a flag (`resolves` propagation, document 15.4).

## 5.6 The queries the graph must answer

The design of the graph starts from these queries. Each query has a latency target on the reference machine (document 02, axis 5). The benchmarks in `math2-agent/bench/graph/` measure it.

| # | Query | Example | Target p95 |
|---|---|---|---|
| Q1 | resolve an identifier | `m2://lean/Subgroup.card_subgroup_dvd_card`, `Q180967`, `A000045`, `0FD4` (Stacks) | 5 ms |
| Q2 | neighbourhood | everything within 2 hops of a node, typed | 20 ms |
| Q3 | dependency closure | best alternative down to axioms, with its trust profile | 100 ms at depth 30 |
| Q4 | reverse dependencies | what breaks if this lemma is false (all statements whose every proof uses it) | 200 ms |
| Q5 | prerequisite path | the shortest learning path from concept A to statement B, using only `defines` and `uses` edges into definitions | 100 ms |
| Q6 | status slice | all E1 statements in MSC 11 (number theory) with an E5 formal statement but no proof: the "formalised, open to prove" frontier | 500 ms |
| Q7 | open-problem frontier | open conjectures in a topic, with their best bounds and the dates those bounds moved | 200 ms |
| Q8 | method query | proofs using the method M about concept C | 200 ms |
| Q9 | provenance | every source that states this node, in date order, with the earliest | 50 ms |
| Q10 | change query | what changed in topic T between releases R1 and R2 | 1 s |
| Q11 | coverage | per MSC code: counts by kind and by evidence level, and Mathlib coverage | 1 s for the full table |
| Q12 | agent work queue | the next 100 tasks for the prove agent, ranked (document 11.4) | 500 ms |

If a query in this list cannot meet its target, that is a design bug in the storage layer. It is not a reason to drop the query.

## 5.7 Storage engine

The derived graph is small by graph-database standards, even at full scale. The estimate in document 18 is about 10 to 30 million nodes and about 300 million edges after the record plane is aligned. Most of the edges are `cites` and `aligned`. That is a workload for a single machine, and math2 treats it as one.

- **Build format:** Parquet, one file for each node kind and each edge family, sorted by tag. These are the release artefacts.
- **Serving:** an embedded store inside `m2 serve`:
  - **SQLite** for point lookups, FTS5 lexical search and the small fixed queries (Q1, Q2, Q9);
  - **DuckDB** over the Parquet for analytical queries (Q6, Q10, Q11);
  - an in-memory **CSR adjacency** built at startup for the closure and path queries (Q3, Q4, Q5). It uses about 2.5 GB for 300M edges at 8 bytes per edge;
  - a **vector index** for embeddings (document 14.4).
- **No external graph database.** math2 does not use Neo4j, a hosted Neptune or similar products. If the record plane grows by orders of magnitude, look at the project's own graph database research (`tamnd/graph-research`). At the sizes above, a server process that loads Parquet, plus SQLite and DuckDB files, is faster to operate, trivial to version and reproducible. Here those three properties are more important than distributed scale.

The query language for users and agents is not SQL or Cypher. It is a small typed API (document 11.2). Power users also get a read-only DuckDB SQL endpoint over the published Parquet schema. That schema is versioned together with the node schema.

## 5.8 Graph health

The graph has its own audit rules. Document 15 has the full list. These rules define a healthy graph:

- `G01` every `uses`, `proves`, `defines` and so on points to a live tag or to a retired tag that has a successor;
- `G02` `same_as` is functional both ways within each external system;
- `G03` `specializes` among concepts is acyclic. `uses` among statements is acyclic when proofs are taken into account. A cycle through two different proofs is allowed, because equivalences look like that;
- `G04` formal dependencies agree with hand-written `uses` (document 04.6);
- `G05` every live statement has at least one `source`, or at least one proof, or a formal binding. A statement with none of these is an assertion with no evidence at all, and it is not accepted;
- `G06` every conjecture has a `source` giving who posed it and where;
- `G07` hub families have at least three members;
- `G08` orphan rate per topic is below 2%;
- `G09` no concept has more than 12 definitions without a `concept`-level note explaining why.

Each rule writes into the committed report under `reports/graph/` (document 15). Thus a person can read the state of the graph in a diff.
