# 09. Literature ingestion: papers, books, existing libraries

The top-right box of the diagram has the label "Papers, Books, Existing Libraries". It is how math2 learns what mathematics already knows. Ingestion has three jobs:

1. **Fill the record plane** (03.3). It records each paper and book in the mathematical literature. Where possible, it also extracts and records each result that they state.
2. **Seed and source the statement plane.** The `sources:` of each node points at records with locators, and curation turns extracted results into nodes.
3. **Join the existing libraries**: Mathlib and other formal libraries, encyclopedias and databases. Crosswalk edges (05.3) connect them to nodes. math2 does not copy what they hold.

Ingestion code is in `tamnd/math2-agent/ingest/<source>`, with one adapter for each source. Each pull is pinned in `tamnd/math2/manifests/sources.yaml` and `manifests/ingest/`.

## 9.1 Principles

- **Dumps, not crawling.** Read each source through its published bulk channel or API, at the rate that it asks for. If a source has no bulk channel, link to it and do not ingest it.
- **Pinned snapshots.** A pull is a snapshot with a date, a URL, a hash and a licence. At build time, nothing in math2 depends on a live external service.
- **Licence decides what is taken** (9.3). Most of the literature can give a record and a locator, but not text. That is sufficient for the record plane, because a mathematical fact is not copyrightable and math2 writes its own statements.
- **Reuse other people's indexes.** Several 2026 projects have already extracted millions of statements from the literature (9.6). math2 aligns to them by id. It does not extract again what they hold, unless it needs something that they do not have.
- **Do not duplicate `tamnd/arxiv`.** arXiv papers come in through the two planes of `tamnd/arxiv`. math2 stores `ax://` references, not paper text.

## 9.2 The source catalogue

Sizes are from document 01, measured or reported in October 2026.

| Source | What math2 takes | Size | Channel | Licence of what is taken | Cadence |
|---|---|---|---|---|---|
| **arXiv** (via `tamnd/arxiv`) | records for every math paper; extracted results from the content plane | about 55k math primaries in 2026 so far, growing fast | `tamnd/arxiv` metadata and content planes | metadata CC0; text per paper licence (9.3) | daily |
| **zbMATH Open** | records for the published literature 1763–present, MSC codes, reviews' metadata, software (swMATH) | 4M+ publications, 168M+ triples in the KG | OAI-PMH, REST API, the KG dump | CC BY-SA 4.0 (publisher-restricted abstracts are not included) | monthly |
| **MathSciNet** | MR numbers only, via reference matching | 4M+ items | MR Lookup | identifiers only | on demand |
| **Crossref / DataCite** | DOIs, journal metadata, references where open | n/a | public APIs | CC0 metadata | monthly |
| **Mathlib** | declarations, types, docstrings, module structure, deprecations | about 290k theorems, 140k definitions | the pinned checkout plus `M2Extract` (07.4) | Apache-2.0 | every bump (07.9) |
| **other Lean libraries** | the same, for bound libraries: Tau Ceti, Formal Frontier textbooks, Physlib, CSLib, FLT (Imperial and Anthropic), PFR, Carleson, sphere packing | Tau Ceti about 3M lines | pinned checkouts | per repository (mostly Apache-2.0) | per release |
| **formal-conjectures** | formal statements of open problems, categories, solved markers | 4,652 statements on 25 October 2026 (1,602 open research problems, 2,816 solved, 234 textbook) | git | Apache-2.0 | weekly |
| **Palomar** | registered Lean-verified results, with `formalization.yaml` provenance | 760 results on 25 October 2026 (registry revision 517), growing since August 2026 | the listing API | CC0 metadata | daily |
| **Erdős problems** (`teorth/erdosproblems`) | problem statements, status, links to formal statements and OEIS | 1,221 problems | git (`data/problems.yaml`) | Apache-2.0 | weekly |
| **Isabelle AFP** | entries and theorem names, for bindings and crosswalk | 1,038 entries | the release tarball | per entry (BSD/LGPL) | per AFP release |
| **OEIS** | sequences: name, terms, formulas, references, conjectures in comments | about 400k sequences | `oeisdata` git export | CC BY-SA 4.0 | weekly |
| **LMFDB** | object records (curves, fields, forms, groups, L-functions) by label, for `object` nodes and crosswalk | about 3.8M curves over ℚ, 22M number fields, … | API and bulk downloads (the LMFDB asks AI users to prefer bulk) | CC BY-SA 4.0 | monthly |
| **GAP SmallGrp, House of Graphs, FindStat, π-Base, DaRT** | object identifiers and properties; π-Base implications as `statement` candidates | n/a | packages, downloads, git | varied; π-Base data CC BY | per release |
| **Wikidata** | QIDs, labels, existing crosswalk properties (nLab, MathWorld, ProofWiki, Mathlib) | about 113k items with a defining formula | weekly dump; SPARQL for checks | CC0 | weekly |
| **MaRDI** | crosswalk ids; model and algorithm records | 7M+ items | SPARQL | per dataset | monthly |
| **Wikipedia** | article ids for crosswalk; short quoted text where useful | about 26k mathematics articles | dumps | CC BY-SA 4.0 | monthly |
| **nLab** | page ids for crosswalk | about 19k pages | `nlab-content` git | link-only until the licence is confirmed | monthly |
| **ProofWiki** | page ids for crosswalk; proofs and definitions as candidate sources | 30k proofs, 36k definitions | XML dump | CC BY-SA 3.0 | quarterly |
| **Stacks Project, Kerodon** | tags, titles, dependency structure for `same_as` and `cites` | about 21k Stacks tags | git (`tags/tags`) | GFDL: **link only** | monthly |
| **MathOverflow, Math.SE** | question ids, titles, per-post records for E1 claims and references | 169k and 1.7M questions | the Stack Exchange API, per post | CC BY-SA, per-post version (16.6) | on demand |
| **Books** | records and locators (9.7) | n/a | catalogue metadata, open textbooks | per book | on demand |
| **MathWorld, Encyclopedia of Mathematics, PlanetMath** | ids for crosswalk | n/a | n/a | MathWorld proprietary: link only; others CC BY-SA | quarterly |

Each row is an adapter with its own fixtures. To add a source, write an adapter, add a manifest entry with a licence, and get the approval of a maintainer for the licence classification. That classification is the one decision that matters.

## 9.3 The licence gate

Each source gets an **access class**. Where licences are different within a source, each item gets one. The class decides what can go into which part of math2. math2 extends the four classes of `tamnd/arxiv` for its three licence zones (16.6):

| Class | Example licences | Record (metadata, locator) | Embeddings, internal | Text in the record plane | Text in node prose |
|---|---|---|---|---|---|
| `open` | CC0, CC BY, CC BY-SA, Apache-2.0 | yes | yes | yes, by reference to its home (e.g. `ax://`) | yes, with attribution (BY-SA zone) |
| `share-alike-nc` | CC BY-NC-SA, CC BY-NC | yes | yes | by reference only | **no**: NC is incompatible with CC BY-SA |
| `verbatim` | CC BY-NC-ND, CC BY-ND | yes | yes | by reference only | no |
| `gfdl` | GFDL (Stacks, Kerodon, Open Problem Garden) | yes | yes | by reference only | **no**: GFDL and CC BY-SA cannot mix |
| `record` | arXiv default licence, all-rights-reserved journals, MathWorld | yes | yes | no | no |

Three consequences:

- **math2 stores almost no third-party text.** Paper text stays where it is now. For arXiv, that is the content plane of `tamnd/arxiv`, which has its own licence gate. math2 holds references, locators, hashes and its own derived data: embeddings, extracted structure and alignments. Thus each corpus has one licence gate, not two.
- **About 15% of arXiv is `open` for node prose.** TheoremSearch measured the CC0, CC BY and CC BY-SA share. If you include the NC licences, the Creative Commons share is about 40%. `tamnd/arxiv` uses this figure for its content plane. For each other paper, math2 writes the statement itself and cites the locator. For a mathematical result, this is better in any case: we check the statement against the definitions and notation of the graph.
- **Embeddings are internal.** math2 embeds text from all classes, for alignment and search. It never publishes embeddings of `record` class text in a form that can reconstruct the text. The vector index of the serving store serves neighbours, not vectors (14.4).

Rules `L01` to `L04` (15.3) enforce the gate. `L02` fails the build if `nodes/` contains text from a class that must not go into node prose.

## 9.4 Existing formal libraries and blueprints

Formal libraries come into math2 as **bindings and alignments**, never as copies.

- **Mathlib** is extracted at each bump (07.4). Each declaration becomes a candidate for binding. The curate and align roles propose nodes for important declarations that do not have one, ranked by in-degree in the formal dependency graph. They also use type search and embeddings to propose `formal:` bindings for existing nodes.
- **Other Lean libraries** are extracted in the same way, at their pinned commits. We bind AI-written libraries (Tau Ceti, the textbook libraries of Formal Frontier, the Anthropic FLT repository) like all other libraries. The source entry records their provenance (AI-written, how reviewed), and each binding to them shows it. A binding to an AI-written library is not weaker for E5, because the kernel does not care who wrote the proof. But it is important for maintenance, and the bump report tracks it.
- **Palomar entries** come in with their `formalization.yaml`. That file records these facts: whether the formalization was human, AI-assisted or autonomous, which models were used, and how it was reviewed. The challenge statement of a Palomar entry becomes a binding candidate. Its replay record is accepted as `external_replay` (07.7) at its pinned commit. Palomar does not certify fidelity, so the binding starts at F0 or F1 like all other bindings.
- **formal-conjectures** statements become `partial` bindings for conjecture nodes. They are aligned by the source references of the repository: Erdős numbers, OEIS, Wikipedia, MathOverflow.
- **Blueprints.** `m2 ingest blueprint <repo>` reads the blueprint of a formalization project and imports it as nodes and edges. It reads a leanblueprint LaTeX source (`\label`, `\uses`, `\lean`, `\leanok`) or the `blueprint-manifest.json` of a Verso Blueprint. A Verso Blueprint derives status from the code, not from hand annotations. Each blueprint item becomes a provisional node with `uses` edges. If the item names a declaration, it also becomes a binding. The import is a PR. The repository of the project stays the authority for its blueprint. math2 imports again when the blueprint changes, and keeps its tags stable through the labels of the blueprint (`aliases:`).

## 9.5 Extracting results from papers

Extraction turns a paper into a list of **results**: the theorem-like environments, with their text, labels, internal references and locators. math2 does not implement extraction again. It uses the result tags of `tamnd/arxiv`, `2106.09685/AB12`, which are permanent per paper. It adds processing that is specific to math:

1. **Source first.** arXiv LaTeX source is the ground truth. We use HTML (LaTeXML) for structure where the conversion is clean, which is about three quarters of papers. PDF-only material, such as older journal papers in open repositories and scanned books, goes through a pinned OCR model. We select the OCR model from the current formula-recognition benchmarks. In 2026 the best open document-OCR models were above 97 on the formula metric of OmniDocBench. The outputs are marked `ocr` with the model version.
2. **Environments.** The extractor finds `theorem`, `lemma`, `proposition`, `corollary`, `definition`, `conjecture`, `example`, `remark` and their custom aliases. It resolves `\newtheorem` declarations. It also finds proofs and attaches them to their statements.
3. **Internal references.** `\ref`, `\cref` and "by Lemma 3.2" become dependency edges inside the paper. Citations become references between papers. They are resolved to records through the bibliography of the paper, and matched against zbMATH, Crossref and arXiv.
4. **Unfolding.** A result can say "under the hypotheses of Theorem 2.1". We rewrite it into a **self-contained statement**: we put in the hypotheses and the notation that the paper set up earlier. A model does this, and a second pass checks it against the original (Matlas uses the same step). We keep the original and the unfolded form. Alignment and search see the unfolded form. It is derived data, and it is under the licence class of the paper (9.3).
5. **Typing.** Each result gets a kind (04.2) and MSC codes, from the codes of the paper plus a classifier.
6. **Quality.** Each step has an evaluation set (17.3). Each month, people check a sample, and we publish the error rates for each step.

The output is one record plane entry for each result, under `ax://paper/<id>#<tag>` for arXiv. For other sources it is a math2 record id `m2://rec/<source>/<id>#<n>`.

## 9.6 Other people's indexes

In 2026 several projects extracted the literature at scale (document 01):

- TheoremSearch: 9.2M theorems from about 690k papers. It released the openly licensed 1.34M.
- TheoremGraph, with its formal half LeanGraph: 11.7M arXiv statements and 18.3M candidate dependency edges. It matches 48k statements to Lean declarations, and has an MCP interface.
- Matlas: 8M unfolded statements from 435k journal papers and 1.9k textbooks.

The position of math2 is to **align to these, not compete with them**:

- Their identifiers are crosswalk targets (`same_as` on record plane entries).
- If their data is openly licensed, we import their extractions and dependency edges as *candidate* evidence. Each item names the extractor that produced it. We treat it like the extraction of math2: E1, unconfirmed, subject to dedup.
- If their data is not openly licensed, math2 uses their public APIs at query time, in the novelty check (17.5) and in search. It stores only the ids of matches.

math2 adds what these indexes do not have: curated concept nodes, computed status, reviewed fidelity, and permanence.

## 9.7 Books

Books are the main source for E2 ("textbook-standard", 04.7). They are also the most difficult source to ingest, because most of them are not openly licensed. math2 handles them as follows:

- **Records and locators** for any book, from catalogue metadata (ISBN, zbMATH and MR records). A locator such as "§3.2, Thm 8" is a fact, and math2 can store it for any book.
- **Curricula.** We build reading lists in `topics/curricula/` from standard texts and their tables of contents. A table of contents is a factual structure that math2 can record. The full text stays with the book.
- **Open textbooks** (CC-licensed, or with explicit permission) can be extracted like papers. The 2026 automatic formalizations of whole textbooks (document 01) give these books formal counterparts, which align to the nodes of the books.
- **Two independent book locators** for E2 are checked by curators, not by agents, because the check requires a person to read the book. That is an intentional use of human time. Most nodes in the undergraduate core will be at E2 before they are formalised, so E2 must mean something.

## 9.8 Scale and cost

| Item | Size | Ingest cost |
|---|---|---|
| Records (zbMATH, arXiv, Crossref) | about 5M papers and books | negligible compute; storage about 20 GB Parquet |
| Results extracted (arXiv `open` and `share-alike` content, plus imported candidates) | 10–30M results | extraction through `tamnd/arxiv`; unfolding by a small model in batch, at a cost of roughly cents per paper |
| Embeddings of all results | 30M × 1,024 dims | a one-time batch job, then incremental; the index is about 60 GB quantised (14.4) |
| Alignment of new results to nodes | daily: about 1,000 papers → 10–20k results | judge calls only on candidate pairs above the similarity threshold, with Haiku-class pre-filtering and batch pricing (12.3) |

The monthly arXiv math volume grew by more than half between 2025 and 2026. Part of the growth is attributed to AI-assisted writing. Ingestion must scale with that volume, and it must not use curator time for it. For these reasons, extraction and alignment are agent work, and curators see queues ranked by importance (16.3). Also for these reasons, an extracted result stays E1, in the record plane, until something in the graph needs it.
