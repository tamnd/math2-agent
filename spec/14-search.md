# 14. Search

Most people will use math2 through search first. Agents also use search to find premises, duplicates and prior work. A query from a mathematician can be a phrase ("finite subgroup of the multiplicative group of a field"), a formula (`\sum_{d \mid n} \varphi(d) = n`), a Lean type (`∀ n : ℕ, ∑ d ∈ n.divisors, φ d = n`), an identifier (`A000010`, `Q190026`, `0BQX`), or a mix of these. No single retrieval method can handle all of these queries. Thus math2 runs **four channels** and fuses their results.

The code is in `tamnd/math2-agent/internal/search`. Three things serve it: `m2 search`, the `search*` and `premises` methods of the API (11.2), and the workspace (13.2).

## 14.1 Channels

| Channel | Index | Good at | Bad at |
|---|---|---|---|
| **lexical** | SQLite FTS5 over names, aliases, statements, explanations; BM25 | names, rare words, exact phrases, identifiers | paraphrase, notation |
| **formula** | structural index over parsed LaTeX (14.2) | the same formula in different words or variable names | formulas that differ structurally but mean the same |
| **formal** | type-directed index over bound Lean declarations and Mathlib (14.3) | exact mathematical content, given a type pattern | natural-language queries |
| **embedding** | vector index over node text, formal statements and record plane results (14.4) | paraphrase, concepts, "the theorem that says…" | precision on near-duplicates, rare identifiers |

The **identifier resolver** runs before the four channels. The query can parse as a tag, alias, `m2://` URI, Lean name, QID, A-number, Stacks tag, arXiv id, DOI, MR or zbMATH id. In that case, it resolves directly (the `resolve` method of the API, 11.2), and search returns that node first.

## 14.2 Formula search

The parser changes LaTeX formulas into operator trees. It uses the same KaTeX-compatible macro set as the bodies (`macros/m2.sty`). Thus math2 interprets its own notation the same way in all places. The index has three parts:

- **Operator-tree paths.** The paths from leaf to root are tokenised. Variables are anonymised and constants are kept. Tangent-L and Approach0 use this method, and they are the strongest systems at ARQMath.
- **Symbol layout trees**, for formulas that have an ambiguous operator parse.
- **Canonical forms**, where a formula is in a class that can be normalised (polynomial identities, linear relations). SymPy computes a normal form in the compute image at build time.

Matching uses BM25 over the path tokens and then a structural re-score. The re-score is the size of the largest matching subtree with consistent variable unification. The index has only formulas from nodes and from record plane results. At full scale, this is approximately 40M formulas. The formula search of zbMATH Open (MathWebSearch) is a crosswalk target, and math2 does not replicate it.

## 14.3 Formal and type-directed search

For a Lean query, math2 does what Loogle does. It also does one thing that Loogle cannot do: it maps hits back to nodes.

- **Pattern search.** A query can be a pattern such as `Subgroup _ → Finite _ → IsCyclic _`, or a constant set such as `IsCyclic, Units, Field`. The channel matches it against the extracted statements (07.4) of each declaration in the pinned Mathlib and the bound libraries. It uses a discrimination-tree index over the elaborated types. It runs locally against the pinned versions of the release. It never runs against the public Loogle service, because that service has rate limits and follows a different Mathlib.
- **Goal search.** For a goal state, the channel finds declarations whose conclusions unify with it after instantiation (the `exact?`/`apply?` relation). It ranks them by the similarity of their hypotheses to the local context, as the embedding channel measures it.
- **Node mapping.** If a hit is bound to a node, the channel returns the node with its status. If a hit is not bound, the channel returns the declaration plus "no node yet". A reader can change this result into a curate request.

math2 uses the semantic Lean search tools of 2026 (LeanSearch v2, Lean Finder, LeanExplore; document 01) as **evaluation baselines**. Where their licences permit, it also uses them as sources of informalisation data to train the formal-statement embeddings of math2. math2 does not depend on these tools at runtime.

## 14.4 Embeddings

Each node gets several embedded texts. The index holds each text separately:

1. its name and statement section;
2. its Lean statement, with the Herald-style informalisation of it (a model's English rendering, generated at build time and cached by type hash);
3. its L2 explanation;
4. for record plane results, the unfolded statement (09.5).

Choices:

- **Model.** Each release pins one embedding model. math2 selects the model with its retrieval benchmark (14.7). The benchmarks of 2026 for mathematical retrieval (MIRB and its successors) put commercial general-purpose embedding models at the top. They also showed that generic rerankers often made results *worse*. Thus math2 benchmarks a reranker before it adopts it, and by default it has no reranker. If the benchmark says so, embeddings for formal statements may use a different model from the embeddings for prose.
- **Index.** A quantised HNSW index (int8, 1,024 dimensions) for nodes, and a separate index for the record plane. At 5M nodes, the node index is a few GB. At 30M results, the record index is approximately 60 GB. The release process builds both indexes, and the server holds them in memory (05.7).
- **No raw vectors in the API.** The API returns neighbours and scores. It never returns vectors. This protects record-class text (9.3) and lets math2 replace the model.
- **Re-embedding is a release event.** When the embedding model changes, a batch job embeds everything again. The release notes record the retrieval benchmark numbers before and after the change.

## 14.5 Premise selection

Premise selection is search for agents. For a Lean goal and its context, it ranks the lemmas that are most likely to help. It is the `premises` method (11.2), and it gives input to the prove loop (12.4). It has four parts:

1. **Formal search.** Declarations whose conclusions unify with the goal (14.3).
2. **Dependency priors.** Declarations that proofs of the graph neighbours of the target use, and declarations that Mathlib proofs of statements with similar types use. The formal dependency graph from extraction (07.4) gives this data. For a node with informal proofs, the declarations bound to the nodes that its proofs `use` get a strong prior.
3. **Embedding similarity** between the informalisation of the goal and the informalisations of declarations.
4. **Learned reranking**, trained from the run records of math2: the retrieved premises that were in the final proof (17.4). This is the only place where a learned reranker is on by default. It is trained on the data of math2, and math2 evaluates it on held-out runs before each deployment.

math2 also uses the built-in premise selection of Lean for `grind +suggestions` and the hammer. It runs inside the Lean session, and math2 merges its suggestions with its own.

## 14.6 Fusion and ranking

Reciprocal rank fusion combines the results from the channels. The weight of each channel is learned on the benchmark. Then two adjustments apply:

- **Kind and status priors.** For a natural-language query, statements and concepts rank above proofs, and E5/E3/E2 rank above E1. The facets of the workspace can override this. For example, an "open" filter inverts the status prior.
- **Duplicate folding.** If results are `same` or `same_as` the same node, they fold into one entry. Record plane results aligned to a node show under that node as "stated in N papers", not as N separate hits.

Each result reports the channels that found it and its rank in each channel (13.2). Thus a reader can see why the result is in the list.

## 14.7 Benchmarks

math2 measures search quality and does not only claim it. `bench/search/` holds the benchmarks below. Their results are in the notes of each release. If a headline metric gets worse by more than 2 points, the release is blocked.

| Benchmark | Queries | Metric | Source of labels |
|---|---|---|---|
| **concept lookup** | 2,000 natural-language descriptions of concepts and theorems, written by people, at undergraduate and graduate level | MRR, recall@10 | curated; each labelled with its target node |
| **formula** | ARQMath-style formula queries, remapped to math2 nodes | nDCG@10 | ARQMath judgements where licences permit, plus curated |
| **formal** | Lean types and goals with known Mathlib answers | recall@1, recall@10 | Mathlib proofs (the premise actually used) |
| **paper-to-node** | record plane results with known aligned nodes | recall@10 | confirmed alignments |
| **premise** | goals from held-out math2 and Mathlib proofs | recall@k of premises used | proofs |
| **cross-system** | queries that should resolve to an external system (nLab, OEIS, Stacks) | accuracy of "not held, see …" | crosswalk |
| **external baselines** | the same queries run against LeanSearch v2, Lean Finder, TheoremSearch, zbMATH formula search, where an API allows | the same metrics | none |

The external baseline row is important. On each type of query, the search of math2 should be better than the best single-purpose tool for that type. If it is not better, the release notes say so, and the workspace gives a link to the better tool for that query type.
