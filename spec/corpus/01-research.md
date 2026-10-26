# K01. Research

The corpus spec rests on five research notes, written on 2026-10-26. They are parts A to E of this document, each in its own file. This file says how the research was done, what each part covers, which findings change the plan, which corrections they make to the main spec, and what the research could not verify.

The main spec has its own research document (spec 01). The parts here extend it for the corpus. They do not repeat it, except where a figure has changed or a statement in spec 01 is wrong.

## K1.1 Method

- **One date.** Each fact was read on 2026-10-26, unless the part gives another date. Each fact has its URL next to it.
- **Polite reading.** The research made single requests, with the User-Agent `m2/research (+https://github.com/tamnd/math2-agent)`. It did not crawl and it downloaded no dump. Dump sizes come from HEAD requests and directory listings. The largest samples were three days of arXiv OAI-PMH for mathematics (1,421 records), the mathematics list of the Open Textbook Library, four Mathlib YAML files and some tree-only listings of git repositories.
- **Measured and reported.** A count that the research made itself, for example from a Wikidata query or a repository tree, is marked **[measured]**. Other counts are as the source reports them.
- **Primary and secondary.** Part D marks each fact **[P]** (paper, official page, licence text, court docket), **[S]** (a law-firm note, a tracker, news) or **[snippet]** (seen only in a search excerpt). A spec section may quote a [snippet] fact only after a person reads the primary source.
- **UNVERIFIED** marks a fact that the research could not confirm, with the reason. These facts are in K1.5 and in K15.
- **No model calls.** The research called no model API, and it used no account or key.

## K1.2 The parts

| Part | File | Scope | Used by |
|---|---|---|---|
| A | `01a-reference-works.md` | encyclopedias, wikis and databases that people wrote: Wikipedia, Wikidata, nLab, ProofWiki, PlanetMath, the EoM, MathWorld, OEIS, DLMF, LMFDB, the subwikis, π-Base, the graph and knot databases, problem lists, MathOverflow and Math.SE | K05, K13 |
| B | `01b-literature-and-books.md` | literature indexes and full text, extraction projects, open textbooks, syllabi and theorem lists | K05, K10, K13 |
| C | `01c-formal-libraries.md` | formal libraries, translation between systems, formal and informal datasets, informalization | K06 |
| D | `01d-model-generation.md` | the error rates of model-written mathematics, projects at scale, memorization and copy detection, the law on model output, provider terms, cost | K07, K09, K12, K13 |
| E | `01e-representation-and-packaging.md` | representation formats, levels of explanation, exercises, book and course formats, notation | K08, K10, K11 |

## K1.3 Findings that change the plan

**Sources (parts A and B).**

1. **Most of the Wikidata "formulas" are primes.** Of the 113,118 items with a defining formula, 105,345 are prime numbers. Only 6,560 are other formulas. Wikidata is still the best crosswalk: 6,840 MathWorld ids, 4,348 nLab ids, 2,562 ProofWiki ids, 1,224 EoM ids, 438 Metamath ids and 115 Mathlib ids.
2. **Some large sources give no text.** The nLab (20,778 pages) has no licence. The originals of the EoM are © Springer. The DLMF is © NIST and not in the public domain. Kerodon states no licence, so it is `record` and not GFDL as the main spec says. These sources give mentions, locators and links (K5.3).
3. **Some large sources give text with attribution.** Wikipedia (28,196 mathematics articles), ProofWiki (47,254 content pages, a daily dump), PlanetMath (9,477 entries), OEIS (400,709 sequences), groupprops, π-Base, DaRT and Fungrim are `open`.
4. **Access is closing.** Wikimedia set API rate limits in 2026 and rewrote its robot policy. The LMFDB has a JavaScript gate and forbids `/api/` to robots. Stack Exchange put its dumps behind a click-through that excludes the training of language models. The importers use dumps and bulk downloads, and keep each limit (K5.10).
5. **New arXiv mathematics is more open than the stock.** In a 2026 sample, 36.9% of new papers were `open`, against about 15% for all papers.
6. **Open textbooks are fewer than they look.** About 48% of the mathematics books of the Open Textbook Library are `open`, and its licence data is sometimes stale. Some well-known free books (Hatcher, *Book of Proof*, Axler) are not open for the prose zone. Table A of K5.4 lists the books that are.
7. **Extraction projects mix licences.** The CC BY-SA release of TheoremSearch includes GFDL text from the Stacks Project. Its `-permissive` release is clean. Matlas states no licence.

**Formal libraries (part C).**

8. **Mathlib has three different counts.** 430,853 theorems and definitions (the statistics page), 483,112 declarations that a user can complete, and 797,294 constants. The corpus counts the second (K6.4).
9. **Translation between systems is weak.** No tool translates the large libraries of other systems into Lean. Model translation succeeds in a minority of cases (K6.6).
10. **Benchmarks have errors.** Several formal benchmarks have measured error rates in their statements, so the corpus uses them to evaluate roles, not as sources (K6.8).

**Model generation (part D).**

11. **Checks by models alone are not enough.** Model judges accept many wrong steps, strong models "prove" false statements, and between 11.4% and 56.8% of generated citations do not exist (part D, §1.6). The checks of K09 follow from these numbers.
12. **Models reproduce text.** In ordinary use, encyclopedia-style output had more than 27% of its characters in verbatim matches of 50 characters or more with public web text. The human median is 0%. So the copy check compares with a reference corpus (K9.4).
13. **The law protects the human part.** In the United States, text that a model wrote with no human control has no copyright. The output cases are not decided. K13 gives the decisions that follow.
14. **The cost of model text is small next to review.** One million nodes cost about $4,000 to $400,000 in model calls, by configuration, before the acceptance rate. Review capacity is the limit (K12.5).

**Representation and packaging (part E).**

15. **MathML 4 is still a working draft.** Its charter target of a Candidate Recommendation in May 2026 was missed. QTI 3.0 supports MathML 3 only. So the corpus stores LaTeX with its notation register, and the packager generates MathML (K10.4, K11).
16. **No system makes a textbook from a knowledge graph.** The packaging of K11 is new and has no prior evaluation.
17. **Exercise banks are mostly non-commercial.** The WeBWorK OPL, OpenStax and APEX are NC. Exercises are math2's own text, with links to the banks (K8.7).

## K1.4 Corrections to the main spec

The research corrects the main spec in these places. The detail is in the section named:

| Main spec | Correction | Detail |
|---|---|---|
| 1.7, 9.2 | Wikidata formulas, Wikipedia, nLab, ProofWiki, EoM, Erdős problems, LMFDB, zbMATH, MathWorld and OEIS figures and licences | K5.13 |
| 9.3, 16.6 | Kerodon is `record`, not GFDL | K5.13 |
| 9.3 | the `open` share of arXiv: two figures, the stock and the 2026 flow | K5.6 |
| 9.6 | TheoremSearch: 8.4M live; take the `-permissive` release | K5.7 |
| 1.7, 9.4 | Mathlib, AFP, Agda, Megalodon, Mizar, Metamath and Tau Ceti figures | K6.11 |
| 1.2 | FLT keeps both a leanblueprint and a Verso blueprint. Only verso-flt and verso-sphere-packing are in the Verso Blueprint catalogue, and Carleson waits for a toolchain bump. "Moved to it" is too strong | part E, §1.7 |
| 16.6 | the zone of the Lean statement and the reference solution of an exercise is not stated. It is the code zone, Apache-2.0 | K13.1 |
| 9.1 | adapters are `importer/<source>`, not `internal/ingest/<source>` | K5.11 |

## K1.5 What the research could not verify

Each item below is in K15 with the question that settles it.

- **Sites that blocked or failed:** the Encyclopedia of Mathematics (HTTP 502), EuDML, the zbMATH API terms, the HathiTrust terms, CORE, the Unpaywall terms, the Internet Archive terms, GroupNames, and the current page of LMFDB downloads.
- **Terms that could not be read:** the Stack Exchange Public Network Terms of 13 November 2025 (loaded by JavaScript), and the Semantic Scholar licence of abstracts.
- **Licences that sources leave open:** the nLab, the Manifold Atlas, FindStat, House of Graphs, KnotInfo, the knowls of the LMFDB, the zbMATH knowledge graph (CC BY or CC BY-SA), the 14 AIM titles, the US port of CC BY-SA 3.0, Coquelicot, the remaining 20 Formal Frontier repositories, Verso Blueprint, LaTeXML and MMT.
- **Counts that do not agree:** the Mizar articles (1,493 or 1,498), the Tau Ceti size, the Metamath counts, the TheoremSearch rows, the ProofWiki Main Page counters, the Mathlib `100.yaml` and Wiedijk's Lean count, and the 1000+ counts.
- **Facts seen only in search excerpts:** marked [snippet] in part D and "from search" in parts A and B. Before a spec section quotes one of them as a rule, a person reads the primary source.
- **The law outside the United States** on the copyright of model output (part D, §3b).

## K1.6 Facts age

The research is a snapshot. Counts grow, licences change, and terms change without notice (the OEIS licence page says so). So:

- Each source document (K5.2) holds its own date of reading, and rule L12 reports a licence that is older than twelve months.
- Before each phase of K14, a person reads again the licences and terms of the sources that the phase imports, and updates the parts of this document that changed.
- A figure in a corpus document that comes from this research names the part and the date. A figure that a later reading changes is changed in the corpus document and in the part, in the same commit.
