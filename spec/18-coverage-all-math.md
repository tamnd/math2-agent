# 18. Coverage: what "all mathematics" means

"All mathematics" is either a slogan or a measurable target. This document makes it a target. It says what counts as covered and how math2 measures coverage per area. It also gives the size of each layer and the order in which math2 fills it. Document 19 makes milestones from this order.

## 18.1 Two planes, five levels

Document 03.3 divides math2 into a **record plane** (what exists in the literature) and a **statement plane** (what math2 understands). math2 measures coverage on both planes, per MSC2020 code, at five levels:

| Level | Name | Means, for an MSC code | Plane |
|---|---|---|---|
| **C0** | recorded | every paper and book in the area has a record, with MSC codes, identifiers and references | record |
| **C1** | indexed | results are extracted from every licence-permitted full text, and every result has a locator and an embedding | record |
| **C2** | mapped | the area's concept skeleton exists as nodes: its main concepts and definitions, and the theorems that a survey or a graduate text would name, each with at least one source | statement |
| **C3** | standard | the area's core (18.3) is at E2 or better, with proofs or locators of proofs, and explanations at L1 and L2 | statement |
| **C4** | formal | the area's core is at E5 with F2 or better, and its conjectures have reviewed formal statements | statement |

An area is at level *k* when it meets the criterion of that level. Each level is a percentage, not a yes/no. math2 should be able to say a sentence such as "MSC 20 (group theory) is 100% C2, 71% C3 and 38% C4". It should also be able to defend that sentence line by line.

C0 for all of mathematics is possible within the first year, because zbMATH Open and arXiv already did that work (document 09). C4 for all of mathematics is not possible in any planning horizon. The purpose of the measurement is to know the gap, area by area, and where the gap is closing.

## 18.2 The coverage table

Q11 (05.6) computes, for every MSC2020 code at every level of the hierarchy:

- **record plane**: records; records with extracted results; results; results aligned to nodes;
- **statement plane**: nodes by kind; nodes by E-level and F-level; nodes with explanations at each level; executable examples (08.7);
- **formal**: bound nodes by match kind; Mathlib declarations in the area that are bound to a node, and the share that are not;
- **open problems**: conjectures and problems, with their formal-statement status;
- **movement**: the change in every count since the last release.

The table goes to `reports/coverage/<release>.parquet`, with a Markdown summary of all classes in `reports/coverage/<release>.md` and a summary per top-level code in `reports/coverage/<release>/` (20.1). The workspace shows it as a map: one cell per MSC code, coloured by level. Nodes get MSC codes as topic edges (05.2). The codes come from the codes of the sources and from a classifier. For C2 nodes, a curator confirms them.

MSC2020 has 63 top-level classes and about 6,000 codes in total. math2 computes coverage at all three levels. By default, it reports the top two levels.

The denominator of C0 is the number of documents of zbMATH Open in each class. `m2 coverage -count` asks the REST interface of zbMATH Open for it, one class at a time, and pins the count with its date in `manifests/coverage/expected.yaml`. The count is for the top-level classes only, thus C0 has a value only at the top level. A record or a node counts for each of its codes and for each code above them. A code that is not in MSC2020, such as an MSC2010 code, counts for the nearest code above it.

### Which areas count

Every MSC2020 class is in scope for C0 and C1. For the statement plane:

- **00, 01, 97** (general, history, education) are record-plane only. A history-of-mathematics source can be a node's source, but history is not a node.
- **Applied areas** (35, 49, 60 to 62, 65, 68, 70 to 94) are in scope for their mathematical results: the theorems, definitions and conjectures. A model of a physical, biological or economic system is not a node unless it comes with a mathematical statement. If a formal library exists for the area (Physlib for physics, CSLib for computer science), math2 aligns to it and leaves the modelling to it.
- **68 (computer science)** overlaps with the theory of computation. Its mathematical core (complexity classes, automata, type theory) is in scope. Software engineering is not.

## 18.3 Cores

A "core" is the set of nodes that an area must have before it can claim C3 or C4. Without cores, C3 would be a percentage of an unknown total. Each core is a curated list in `topics/cores/<name>.yaml`. It is built from standard syllabi and texts, and people who teach the area review it. Cores nest:

| Core | Basis | Size (nodes: definitions, statements, key examples) |
|---|---|---|
| **undergraduate** | about 40 standard courses, from first-year analysis and linear algebra through Galois theory, measure theory, topology, complex analysis, probability, number theory, combinatorics and logic; Mathlib's `undergrad.yaml` list and the "1000+ theorems" list as cross-checks | **about 25,000** (about 600 per course) |
| **graduate** | the standard first- and second-year graduate syllabi and qualifying-exam lists of several departments; one or two standard texts per subject | about 150,000 to 250,000 |
| **research, per area** | the concepts and named theorems that recent surveys in the area cite; the area's open problem lists | about 5,000 to 50,000 per top-level class, in total about 1M |

The undergraduate core is the first serious target, for three reasons. It is bounded. It is mostly in Mathlib already, so C4 is mostly binding and review, not new proofs. Most of the items on Mathlib's own `undergrad.yaml` list were formalized by 2025, and the list records the gaps. And in this core, explanations and learning paths (Q5) help the most people.

## 18.4 How big is mathematics?

These are estimates for planning. Each estimate has a basis. When math2 has the measured number, the measured number replaces the estimate.

| Quantity | Estimate | Basis (document 01) |
|---|---|---|
| papers and books | about 5M, growing by about 150k per year | zbMATH Open's coverage (over 4.9M documents) plus arXiv math, deduplicated |
| results stated in the literature | about 30 to 60M | about 13 to 19 results per paper that states results in theorem form (TheoremSearch: 9.2M from about 690k arXiv papers; Matlas: 8M from 435k journal papers); many older and applied papers state none |
| distinct results after deduplication | perhaps a third to a half of that | duplicated restatements, lemmas restated from earlier papers, and corollaries |
| concepts | about 100,000 to 300,000 | Wikidata items with a defining formula (about 113k), nLab (about 19k pages), Wikipedia mathematics (about 26k articles) and the Stacks Project (about 21k tags) overlap heavily, and each covers part of it |
| formal declarations | about 430k in Mathlib, and several million across all Lean libraries | Mathlib's own statistics; Tau Ceti is about 3M lines alone |
| open problems with a formal statement | about 1,000 open in formal-conjectures, plus FrontierMath Erdős and Annals-Challenge-style collections | those repositories |

Thus the statement plane at full graduate coverage, plus research cores, is about 1 to 2M nodes. If OEIS sequences and the most-cited objects are also nodes, it is 2 to 5M (06.7). The record plane is about 5M records and 30M results. Both fit the storage design without change.

## 18.5 The order of work

Breadth first on the record plane, depth first on the statement plane:

1. **C0 everywhere.** It is cheap. It makes everything else possible, because the source of every node needs a record to point at.
2. **C2 skeletons everywhere.** For each top-level MSC class: the 200 to 2,000 concepts and named theorems that define the area. These skeletons make the graph navigable across all of mathematics, even where the graph is shallow. This is curated work with much agent help. The proposals come from surveys, the tables of contents of textbooks, and Wikipedia's category structure.
3. **C3, then C4, for the undergraduate core.** Then the same for graduate cores, in order of Mathlib coverage, because a binding costs less than a proof.
4. **C1 everywhere,** as `tamnd/arxiv`'s content plane and the licence gate allow.
5. **Research cores on demand.** math2 builds the research core of an area when a project (13.6) or a campaign asks for it. The reason is that research-level curation needs the people who work in the area.

Two rules keep this honest:

- **The table is published every release, including the regressions.** Coverage can decrease. A Mathlib bump can break bindings, or a re-review can lower an F-level. The table shows that.
- **Counts are not the target.** People who teach the area review the composition of each core. Sometimes the node count of an area grows while its core coverage stays flat. That is a curation problem, and the coverage report is designed to show exactly that.
