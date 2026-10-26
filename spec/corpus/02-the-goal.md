# K02. The goal of the corpus

This document says what the corpus programme must produce, how we know when it is complete, and what it must not do. Document K04 says how we count the items. The other documents say how we produce them.

## K2.1 The goal in one sentence

**Each definition, theorem, proof and example of mathematics that a person can find in a public source is in math2 as a node, or it has a recorded decision that says why it is not. Each node is written for people, it is checkable by a machine, and it is ready to go into a course, a book or an article.**

The sentence has five parts. Each part is a requirement with a measurement:

| Part of the sentence | Requirement | Measurement |
|---|---|---|
| "each ... that a person can find in a public source" | The universe is the union of the inventories of all the sources of K05 and K06, plus the items that models list (K07). | the inventory (K04): one row per distinct item |
| "is in math2 as a node, or it has a recorded decision" | No item is silently missing. Each inventory row has an outcome. | the **open count**: inventory rows with no outcome. The target is zero. |
| "written for people" | Each node has the sections of its kind card (K08), in the house style, with notation from the notation register (K10.4). | the writing level W0 to W3 (K2.4) |
| "checkable by a machine" | Each statement has a formal binding, or a recorded reason why it has none. Each proof has a formal proof, or it is an informal proof with `uses` edges that a machine can follow. | the E and F levels of spec 04.7, and the formal outcome of K2.5 |
| "ready to go into a course, a book or an article" | A selection of nodes compiles into a linear text with no manual edits (K11). | the pack check: `m2 pack -check` on the standard volumes (K11.6) |

## K2.2 Why this is a different goal from the main spec

The main specification (documents 00 to 21) builds the system: the object model, the graph, the formal engine, the agents, the audit and the workspace. It decided that node count is not an axis of progress (settled decision 23) and it measures coverage with C0 to C4 (18.1).

The corpus programme fills the system. It does not change any settled decision of the main spec. It adds three things that the main spec does not have:

1. **A denominator for "all".** C2 counts the items of a skeleton, and C3 and C4 count the items of a core. Both are curated lists of a few hundred to a few thousand items for each class. The corpus inventory (K04) is the full list: each distinct item that any source in scope holds. The skeletons and cores become views of the inventory.
2. **A production line.** The main spec has roles that improve one node at a time (12.1). The corpus programme runs these roles, and four new ones (K7.3), over millions of items. It needs queues, batch sizes, cost limits and a review budget (K12).
3. **A content standard.** The main spec says what a node file must contain to be valid (04.5). The corpus programme says what a node must contain to be **good**: the sections, the order, the style, the notation and the examples (K08).

Node count is still not an axis of progress. The axes of the corpus are the open count, the writing level, the trust levels and the pack check. A million nodes at W1 with no proofs is a bad result, and the reports show it as bad.

## K2.3 What "each definition, theorem, proof and example" means

The corpus holds the 11 kinds of spec 04.2. For the corpus, the important kinds and what counts as one item are:

| Kind | One item is | It is not |
|---|---|---|
| `concept` | one mathematical notion that sources name, for example "compact space" | one wording of the notion |
| `definition` | one definition of a concept that a source uses, for example "compact: each open cover has a finite subcover" | a restatement of the same definition in other words |
| `statement` | one proposition that a source states and calls true, at the generality that the source uses | each corollary that only renames variables |
| `proof` | one argument for one statement that differs from the others in what it uses or in its method | the same argument with other notation |
| `example`, `counterexample` | one specific instance that a source uses to illustrate or to refute | a family of instances (that is a statement) |
| `object` | one named object with an identity, for example a group in the ATLAS, an OEIS sequence or an LMFDB curve | each record of a database: K4.6 limits which objects become nodes |
| `conjecture`, `problem` | one open question that a source poses | each restatement of it |

The granularity rules of spec 05.4 apply. A family of near-identical forms becomes a hub and its members, not one node and not a thousand nodes.

Exercises are not a kind in node.v1. The corpus needs them for courses (K11). K8.9 proposes the kind `exercise` for node.v2 and gives the reason. Until node.v2, exercises are `example` nodes with the role `exercise` in `## Remarks`.

## K2.4 Writing levels

A writing level says how complete the human-readable part of a node is. The build computes it from the sections that are present and from their provenance, like the E and F levels. No person and no agent writes it.

| Level | Name | The node has |
|---|---|---|
| W0 | stub | a tag, a kind, a name, an inventory row and at least one `same_as` or source. It has no body. |
| W1 | stated | W0, plus the main section of its kind (`Statement`, `Definition`, `Proof` and so on) that passes the body rules B01 to B03, plus at least one source with a locator |
| W2 | usable | W1, plus the sections that the kind card marks as required for W2 (K08): for a statement, at least one proof node, one example or non-example, and an L2 explanation; for a definition, one example and one non-example |
| W3 | complete | W2, plus each section that the kind card lists, explanations at each level that applies, at least one exercise for a statement or definition in a curriculum, and a person confirmed each agent-written section (rule P04) |

W does not measure truth. A node at W3 can be at E1. A node at E5 can be at W0. The reports give both. A volume (K11) sets the levels that its nodes must reach.

## K2.5 Formal outcomes

The goal says "checkable by a machine". For each statement and definition the corpus records one formal outcome. The build computes it from the bindings and the registers:

| Outcome | Means |
|---|---|
| `bound` | at least one formal binding exists, at any F level |
| `proved` | the node is at E5 |
| `pending` | a formalize or prove job is in the queue or ran and failed with a reason |
| `blocked` | the formal statement needs a definition that no library has. The blocking definition is a node with outcome `pending` |
| `out_of_reach` | a person decided that formalization is not useful now. The decision has a reason, for example "needs the classification of finite simple groups". It is reviewed each year |

The target is that each node has an outcome, not that each node is `proved`. In 2026 the best systems formalized whole textbooks, but research-level statements still failed often (K1.4). An honest corpus says which statements are proved, which are bound and which are not.

## K2.6 What the corpus must not do

These rules come from the main spec and from the research of K01. They are not new trade-offs. They are the limits that the corpus works inside.

1. **It must not copy text that the licence gate refuses** (spec 9.3, 16.6). Most of the literature gives a record and a locator, not text. math2 writes its own statements.
2. **It must not take model output as truth.** A statement that a model wrote is E1 (spec 04.7). A proof that a model wrote is an informal proof at E1 until it is formal, certified or published. Agreement between models is evidence for triage. It never raises a level (settled decision 3).
3. **It must not publish text that a model reproduced from a source with a closed licence.** Each generated body goes through the copy check of K9.4 before it can merge.
4. **It must not invent sources.** A source is a record with a locator that the ingestion found or that a person entered. A model can propose a source. The proposal becomes a source only when the record exists in the record plane and a check or a person confirms the locator (K9.5).
5. **It must not flood the people.** Agent output is limited by the measured review capacity (settled decision 20). The corpus plan in K12 is set by that capacity, not by the speed of the models.
6. **It must not hide what it does not know.** Each report shows the open count, the items with the outcome `out_of_reach`, and the error rates from the sample audits (K9.7), including the bad ones.

## K2.7 The reference scale

The numbers in this table are estimates for planning. Spec 18.4 has the basis for the first rows. K01 has the basis for the others. When math2 has a measured number, the measured number replaces the estimate.

| Quantity | Estimate | Basis |
|---|---|---|
| distinct concepts | 100,000 to 300,000 | spec 18.4 |
| definitions (several for each concept) | 150,000 to 500,000 | about 1.5 definitions for each concept in textbooks and in Mathlib |
| named and textbook statements | 300,000 to 1,000,000 | Mathlib theorems that a person would cite, ProofWiki, the textbooks of K5.4 and the cores of spec 18.3 |
| statements in the research literature, after deduplication | 10 to 30 million | spec 18.4 |
| proofs | 1 to 1.5 for each statement in the textbook layer | ProofWiki and textbooks often give more than one proof of a named theorem |
| examples and counterexamples | 2 to 5 for each definition in the textbook layer | the kind card of K08 asks for at least one example and one non-example |
| exercises | 3 to 10 for each statement and definition in a curriculum | open textbooks (K5.4) |

The corpus programme has three layers. Each layer is a target with its own exit criterion in K14:

1. **The textbook layer**: everything that the undergraduate and graduate textbooks and the reference wikis hold. About 1 to 2 million nodes. This layer must reach an open count of zero, W2 for each node, and E2 or better for each statement.
2. **The formal layer**: everything that the formal libraries hold that a person would cite. About 300,000 to 600,000 nodes, most of them also in the textbook layer. This layer must reach an open count of zero and a binding for each node.
3. **The research layer**: the results of the research literature. 10 to 30 million items in the record plane. They become nodes only when the graph needs them (spec 9.8), or when a project or a campaign asks for an area (spec 18.5). This layer must reach an open count of zero in the record plane, and each record must have an outcome: a node, an `aligned` edge to a node, or "kept as a record".
