# K05. Human sources

People have written down most of the mathematics that the corpus must hold: in reference works, wikis, textbooks, papers, problem lists, syllabi and exercise banks. This document lists those sources, says what the corpus takes from each one, by which channel, under which licence, and in which order. K06 does the same for the formal libraries.

Each fact about a source has its URL and its date of reading in K01, parts A and B. All counts were read on 2026-10-26, unless the text gives another date. A fact marked "not settled" is in the open questions of K15.

## K5.1 What a human source gives

A human source can give five things. Most sources give only some of them:

| What | Example | Licence that it needs |
|---|---|---|
| **mentions** | "the ProofWiki page *Lagrange's Theorem (Group Theory)*", "Theorem 3.14 of Rudin, 3rd edition" | none: a name and a locator are facts |
| **open text** | the statement and proof on that ProofWiki page | `open` class (spec 9.3) |
| **locators** | "Hefferon, *Linear Algebra*, Chapter Two, §III.1", "DLMF 5.2.1" | none |
| **a link structure** | the links between nLab pages, the cross-references of a textbook | none for the edges. The selection of a whole collection can have a database right (K13.4) |
| **curricula** | a table of contents, a syllabus, a list of theorems | none for the structure. The words of the source are its author's |

Every source gives mentions. Only sources of the `open` class give text. Mentions and locators are the larger part of the value: they fill the inventory (K04), and the inventory is the denominator of the goal. A node can state a theorem in math2's own words from a locator, so a source that gives no text still gives the item.

## K5.2 The source document

Each source has one entry in `manifests/sources.yaml`. A person approves each entry and each change to its licence fields (spec 9.2). The importer reads nothing from a source that has no approved entry.

```yaml
- id: lebl-ra
  title: Basic Analysis I and II
  authors: [Jiří Lebl]
  kind: textbook            # reference | wiki | textbook | paper-index | extraction | curriculum | exercises | database
  url: https://github.com/jirilebl/ra
  licence: CC-BY-SA-4.0 OR CC-BY-NC-SA-4.0   # SPDX expression, as the source states it
  election: CC-BY-SA-4.0    # for a dual licence: the option that math2 takes
  class: open               # spec 9.3
  licence_read:
    at: https://github.com/jirilebl/ra      # the source itself, never an aggregator
    on: 2026-10-26
    by: <person>
  per_item_licence: false   # true if items have their own licences (K13.4)
  public_domain: null       # or {us: true, eu: true, basis: "..."} (K5.2.3)
  authored: human           # human | ai | mixed (K6.5)
  edition: <tag or edition> # pinned edition, where editions differ in licence
  channel: git              # dump | git | api | bulk | page | catalogue
  snapshot: {commit: <commit>, read: <date>}
  terms:
    robots: <summary or n/a>
    rate: <limit that the importer keeps>
    ai: <clause on AI or machine learning, or "none stated">
  attribution: "Jiří Lebl, Basic Analysis I and II, CC BY-SA 4.0, https://github.com/jirilebl/ra"
  adapter: importer/latexthm
  approved_by: <maintainer>
```

### K5.2.1 Read the licence at the source

The licence of a book or a site is read at the source: the `LICENSE` file of its repository, its front matter, or the licence page of the site. A catalogue such as the Open Textbook Library or the PreTeXt catalogue is a list of candidates, not a licence. Research B found three stale or simplified licences in the Open Textbook Library in one afternoon (K5.4.2).

The licence is read again at each new snapshot. A change of licence between snapshots changes the class of the new snapshot only. The text that math2 took under the old licence keeps that licence, and the source document keeps both, with the snapshot of each.

### K5.2.2 Dual licences

When a source offers a choice of licences, math2 takes the option that the prose zone can hold, and the field `election` records the choice. Examples: Lebl's three books (CC BY-SA 4.0 or CC BY-NC-SA 4.0), Hefferon's *Linear Algebra* (GFDL or CC BY-SA 3.0 US), and ProofWiki (CC BY-SA 3.0 and the GFDL). The attribution names the elected licence.

### K5.2.3 Public domain

Public domain depends on the country. In 2026, a work published in the United States before 1931 is in the public domain there. In the EU, a work is in the public domain 70 years after the death of its author, so in 2026 the author must have died before 1956. math2 is published in all countries, so:

- A work is `open` as public domain only when it is in the public domain in the United States **and** in the EU. The field `public_domain` records both answers and the basis for each, for example the date of publication and the year of the death of the author.
- A work that is in the public domain in one of them only is `record`.
- A translation, a modern edition and a modern commentary have their own copyright. The importer takes only the text of the original, from an edition whose added text it does not use. Project Gutenberg texts lose the Project Gutenberg header and licence text, as its licence requires.

Hardy's *A Course of Pure Mathematics* (Project Gutenberg #38769) passes both tests: it was published before 1931, and Hardy died in 1947.

### K5.2.4 The `authored` field

The field `authored` of K6.5 applies to human sources too. A source that people wrote is `human`. A source with parts that a model wrote is `mixed`, and the source document says which parts. For example, the "slogans" of the TheoremSearch datasets were written by DeepSeek-V3.1, so they are `ai`, while the statements are from the papers. Text from an `ai` part counts as agent text for the rules of K07 and K09: it needs the same checks as text that a math2 role wrote.

## K5.3 Reference works and wikis

| Source | Size | Licence | Class | The corpus takes | Channel |
|---|---|---|---|---|---|
| Wikidata | 3,208 theorems; 113,118 items with a defining formula (P2534), but 105,345 of these are prime numbers, so 6,560 real formulas | CC0 | `open` | items, labels, crosswalk properties, formulas | weekly JSON dump (103 GB); WDQS only for checks |
| Wikipedia (en) | 28,196 mathematics articles (WikiProject Mathematics) | CC BY-SA 4.0 | `open` | mentions, text with attribution, the link graph | monthly XML dump |
| Wikipedia (de, fr, ja, zh, ru) | 20,732, 12,192, 8,865, 5,682, 3,928 (each wiki counts in its own way) | CC BY-SA 4.0 | `open` | mentions; text in a later phase | monthly dumps |
| ProofWiki | 47,254 content pages; 30,348 proofs and 36,102 definitions (Main Page) | CC BY-SA 3.0 and GFDL | `open`, elect CC BY-SA 3.0 | mentions, statements, proofs, definitions | daily dump `latest.xml.gz` (36.6 MB) |
| PlanetMath | 9,477 entries, inactive since 2018 | CC BY-SA 3.0 | `open` | mentions and text | git, 64 repositories by MSC class |
| nLab | 20,778 pages | none. The site allows use "if you acknowledge the source", but no right to adapt | `record` | page names, the link graph | git mirror `ncatlab/nlab-content` |
| Encyclopedia of Mathematics | live count not settled; 1,224 Wikidata ids | originals © Springer; only new wiki text is CC BY-SA 3.0 and GFDL | `record` | ids and links. Text only from an article that its history shows to be wholly new | none (the site gave HTTP 502 on the day of reading) |
| MathWorld | 16,939 entries | proprietary, no bulk copies | `record` | ids, from Wikidata (P2812, 6,840 items) | none |
| OEIS | 400,709 sequences | CC BY-SA 4.0 | `open` | names, terms, formulas, references | git `oeis/oeisdata` |
| DLMF | version 1.2.8 (15 September 2026) | © NIST, no bulk copies | `record` | equation numbers as locators | none |
| Fungrim | 3,130 entries, dormant since 2022 | MIT | `open` | formulas with their conditions | git |
| Wolfram Functions | 307,409 formulas | © Wolfram Research | `record` | formula ids as locators | none |
| LMFDB | figures of 2022 (not measured again) | data CC BY-SA 4.0; code GPL | `open` (data) | object records by label | bulk downloads only. Its `robots.txt` forbids `/api/` |
| groupprops, Topospaces, Commalg, Diffgeom | 8,076, 825, 429, 435 articles | CC BY-SA 3.0 | `open` | mentions and text, mostly facts about groups | MediaWiki API, slowly: one maintainer |
| Manifold Atlas | 215 articles | contradictory: GFDL 1.3 and "public domain" | `gfdl` until the editors answer | mentions and links | MediaWiki API |
| π-Base | 225 spaces, 247 properties, 935 theorems | CC BY 4.0 | `open` | theorems as statement candidates | git |
| DaRT | 218 rings, 178 properties, 72 theorems | CC BY 4.0 | `open` | the same | git |
| House of Graphs, ISGCI, KnotInfo, Knot Atlas, ATLAS v3 | about 22,000 graphs (2022); 1,695 graph classes; knots to 13 crossings; 2,710 articles; about 716 groups | no licence stated | `record` | ids and facts, as metadata | downloads where offered. Not the GPL-3.0 CSV package of KnotInfo |
| Stacks Project | about 21,000 tags | GFDL 1.2 | `gfdl` | tags, titles, the dependency graph | git |
| Kerodon | 8,189 tags | no licence stated | `record` | tags, titles, the dependency graph | none: page by page, slowly |
| Open Problem Garden | at most 713 problems | GFDL | `gfdl` | mentions and links | none |
| Erdős problems | 1,221 problems | the site states no licence; `teorth/erdosproblems` is Apache-2.0 but has no statements | `record` (statements), `open` (status) | status, links, the problem number | git `data/problems.yaml` |
| Kourovka Notebook | 21st edition, arXiv:1401.0300v49 (20 October 2026) | arXiv non-exclusive licence | `record` | problem numbers with the edition | arXiv |
| MathOverflow | 168,603 questions | CC BY-SA 2.5, 3.0 or 4.0, by the date of each post | `open`, for each post | mentions, references, text of single posts with attribution | the API, with a key |
| Math.StackExchange | 1,702,027 questions | the same | `open`, for each post | the same | the same |
| FindStat | 1,997 statistics | not settled | `record` until settled | ids | none |

Notes on the table:

- **Wikidata formulas.** The P2534 count is mostly primes. The importer drops the items that are instances of *prime number* (Q49008) and its kin before it treats P2534 as a source of formulas.
- **Stack Exchange dumps.** Since July 2024 the dumps are behind a click-through that excludes the training of large language models. math2 does not train a model on them, but the click-through is a contract, so the importer uses the API and not the dumps. The importer reads only posts that a node cites or that the enumerate role proposes as a source, not whole sites. Each post has its own licence, by date (rule L06).
- **Wiki text and the copy check.** Text from a wiki that enters a node keeps the licence of the wiki and the attribution of its history. The copy check (K9.4) sees it as a match with an open source. That match is allowed, and it adds the attribution.

## K5.4 Open textbooks

Open textbooks are the main source of text for the textbook layer (K2.7). They give statements, proofs, examples and exercises in the order that a course needs, and the theorem environments of their sources mark each item.

### K5.4.1 Table A: books that the prose zone can take

Each licence below was read at the source of the book. Each book needs attribution. A CC BY-SA 3.0 Unported text may be adapted under CC BY-SA 4.0 (its §4(b)). For the 3.0 US port of Hefferon, the same clause is assumed and not settled.

| Book | Licence that math2 uses | Source format | Notes |
|---|---|---|---|
| Open Logic Project | CC BY 4.0 | LaTeX | |
| *Homotopy Type Theory* (HoTT book) | CC BY-SA 3.0 | LaTeX | |
| Hefferon, *Linear Algebra* | CC BY-SA 3.0 US, elected from a dual licence | LaTeX | |
| Lebl, *Basic Analysis* I and II, *Notes on Diffy Qs*, *Tasty Bits of Several Complex Variables* | CC BY-SA 4.0, elected from a dual licence | LaTeX | |
| Boelkins et al., *Active Calculus* (single variable) | CC BY-SA 4.0 | PreTeXt | the Open Textbook Library entry is for the 2017 edition and is stale |
| Chen, *An Infinitely Large Napkin* | CC BY-SA 4.0 (text) | LaTeX | its code is GPL v3, and math2 does not take it |
| Levin, *Discrete Mathematics: An Open Introduction*, 3rd edition and earlier | CC BY-SA 4.0 | PreTeXt | pin a commit before the 4th edition, which is CC BY-NC-SA |
| Keller and Trotter, *Applied Combinatorics* | CC BY-SA 4.0 | PreTeXt | |
| Ernst, *An Introduction to Proof via Inquiry-Based Learning* | CC BY-SA 4.0 | LaTeX | |
| Austin, *Understanding Linear Algebra* | CC BY 4.0 | PreTeXt | |
| Beck, Marchesi, Pixton and Sabalka, *A First Course in Complex Analysis* | CC BY 4.0 | PDF | |
| Lehman, Leighton and Meyer, *Mathematics for Computer Science* (2018) | CC BY-SA 3.0 | PDF | the MIT OCW course pages are NC; this book is not |
| OpenStax *Statistics*, *Precálculo* 2nd edition, and the earlier CC BY editions of *Prealgebra* (2015) and *Precalculus* (2014) | CC BY 4.0 | CNXML | pin the CC BY edition |
| Hardy, *A Course of Pure Mathematics*, and other public-domain books that pass K5.2.3 | public domain | LaTeX, HTML | strip the Project Gutenberg text |

The Open Textbook Library lists more books under CC BY or CC BY-SA, for example Zakon, Kuttler, Shimamoto, Petrunin, Callahan and *OpenIntro Statistics*. They are candidates. Each one enters Table A only after a person reads its licence at the source.

### K5.4.2 Table B: books that the prose zone cannot take

These books give mentions, locators, tables of contents and internal embeddings, but no text in nodes.

| Book | Licence | Class |
|---|---|---|
| Judson, *Abstract Algebra: Theory and Applications* | GFDL 1.3 or later | `gfdl` |
| Beezer, *A First Course in Linear Algebra* | GFDL 1.2 or later | `gfdl` |
| the PreTeXt titles with a GFDL badge (21 titles, for example Yoshiwara's three books and *The ODE Project*), Grinstead and Snell, Bogart | GFDL | `gfdl` |
| Hartman et al., *APEX Calculus*; Axler, *Linear Algebra Done Right*, 4th edition | CC BY-NC 4.0 | `share-alike-nc` |
| *CLP Calculus* (UBC); Levin, 4th edition; the OpenStax 2nd editions of calculus and algebra; MIT OpenCourseWare | CC BY-NC-SA 4.0 | `share-alike-nc` |
| Hammack, *Book of Proof*, 3rd edition; Shoup | CC BY-NC-ND | `verbatim` |
| Hatcher, *Algebraic Topology* | © CUP 2002, noncommercial copies only | `record` |
| Lurie, Kerodon | no licence stated | `record` |
| the PreTeXt titles *Linear Algebra* (W214) and *Intro to Topology (IBL)* | all rights reserved | `record` |
| LibreTexts pages tagged "not declared" | none | `record` |

Three books on this list are often taken to be open: Hatcher, *Book of Proof* and Axler. They are not open for the prose zone.

### K5.4.3 Aggregators

- **Open Textbook Library.** 214 mathematics books. By its own licence data, about 48% are `open`. Its licences can be stale or simplified: it lists *Active Calculus* as CC BY-NC-SA (true for the 2017 edition only), *Book of Proof* as "Attribution-NoDerivs" (the author says CC BY-NC-ND), and Hefferon as CC BY-SA only (the book is dual-licensed). The corpus uses it as a list of candidates.
- **AIM Open Textbook Initiative.** 68 approved titles. The list shows no licences. Fourteen titles have licences that nobody has read yet (K15).
- **PreTeXt catalogue.** 67 mathematics projects. Many have only a badge that says "Creative Commons", with no variant. Each one needs a check at the source.
- **LibreTexts.** The licence is set page by page, and one book can mix licences. The importer goes to the upstream source of each book. Where it reads LibreTexts, it reads page by page, by the `license:` tag of each page. A page without a declared licence is `record`.

### K5.4.4 Editions

Some books changed licence between editions: Levin's 4th edition became CC BY-NC-SA, and the OpenStax 2nd editions are CC BY-NC-SA where the 1st editions were CC BY. An earlier edition keeps the licence under which it was published. So the source document of such a book pins the edition and the commit (field `edition`), and the importer reads nothing from a later commit until a person approves the licence of that commit.

### K5.4.5 What the importer takes from an open textbook

From the theorem environments of the source (`\begin{theorem}`, PreTeXt `<theorem>`, CNXML `<rule>`, and the like):

- the kind, the number and the title of each environment, as a mention with a locator;
- the statement, the proof, the examples and the exercises, as text of the record plane under the licence of the book;
- the cross-references between environments, as candidate `uses` edges;
- the table of contents and the index, as a curriculum (K5.8) and as index terms for K4.7.

The text enters a node only by the importer of `open` text (K14.4), with the attribution of the book, and the copy check marks it as a match with an open source. The write role does not paste it: a role that writes a node from scratch has no text of a restricted book in its context (K7.5), and it may have open text there, with its licence.

## K5.5 Closed textbooks

Most of the standard textbooks are not open. They give records and locators only:

- **The record of the book**: ISBN, edition, zbMATH and MR ids, from catalogues (K5.6).
- **Locators**: "Rudin, *Principles of Mathematical Analysis*, 3rd edition, Theorem 3.14". A locator is a fact.
- **The table of contents and the index**, where the publisher shows them in public, for example in a preview or on the site of the book. They go into the record plane as one locator for each item, not as a copy of the table (K13.4). An index term is a mention of a definition, and K4.7 uses the index terms of standard books to measure recall.
- **Not the text.** No text of a closed book enters the record plane, the context of a role, or a node. E2 needs two book locators that a person has checked (spec 9.7), because that check needs the book.

The list of closed textbooks in scope comes from the curricula of K5.8: each book that a syllabus names is a candidate.

## K5.6 Literature indexes and full text

The research literature is the record plane of spec 09. This section adds the facts of 2026 that change what the importers do.

| Source | Size | Licence | The corpus takes |
|---|---|---|---|
| arXiv (through `tamnd/arxiv`) | about 9.2 TB of files (April 2025) | metadata CC0; text by the licence of each paper | records; the text of `open` papers |
| zbMATH Open | 4M+ publications, 3M+ reviews; 133,093 documents for 2025 | knowledge graph: CC BY 4.0 or CC BY-SA 4.0 (the record says both) | records, MSC codes, reviews as evidence |
| OpenAlex | about 745 GB (September 2026) | CC0 | all metadata |
| Crossref | 180,034,490 records (March 2026) | metadata free; some abstracts have copyright | metadata and references; abstracts are `record` |
| DataCite | 108M DOIs | CC0 | metadata of datasets and software |
| Semantic Scholar, S2ORC | 15,679,781 full texts (May 2026) | ODC-By for the database, the copyright of each paper for its text | ids. Text only by the licence of each paper |
| CORE | 291M records (2023) | ODC-By for the database | records. Text by the licence of each paper |
| HAL (mathematics) | 170,406 records, 105,413 with a file | metadata CC0; 9.8% of the files are `open` | records; the 10,349 open texts |
| Numdam | 75,599 articles | metadata CC0; text for personal use, no bulk | records. Text only where an article has a CC licence or is in the public domain |
| EuDML, Project Euclid, GDZ | | not settled; "free to read" is not an open licence | records, through partner sources |
| HathiTrust, Internet Archive, Project Gutenberg | 206,597 mathematics texts on the Internet Archive | public domain for some items; a hosted copy says nothing about the licence | public-domain books that pass K5.2.3 |

Three facts change the plan of spec 09:

1. **More of arXiv is open than spec 9.3 says.** In a sample of three days of 2026, 36.9% of new mathematics papers were `open` (CC BY 34.0%, CC0 1.9%, CC BY-SA 1.0%). Spec 9.3 gives about 15%, which is the share of all papers since 1991. The sample is small, so phase 1 measures the share over all of 2026 from the metadata plane, and spec 9.3 then gives both figures.
2. **The zbMATH knowledge graph is updated about twice a year**, not each month. Its licence is CC BY 4.0 in one field of the Zenodo record and CC BY-SA 4.0 in its description. Both are `open`, but they differ for derived exports, so the importer treats it as CC BY-SA 4.0 until zbMATH answers (K15).
3. **Full-text aggregators do not change the licence of a paper.** S2ORC, CORE and the Internet Archive hold texts under the copyright of each paper. An importer takes a text from them only when the licence of that paper allows it.

## K5.7 Extraction projects

Spec 9.6 says that math2 aligns to the large extraction projects and does not compete with them. The data of 2026:

| Project | Size | Licence | The corpus takes |
|---|---|---|---|
| TheoremSearch | 8.4M statements in the live index | the CC BY-SA release mixes in GFDL text from the Stacks Project. The `-permissive` release (1,239,720 theorems) is CC BY 4.0 and excludes share-alike | the `-permissive` release as candidate mentions (E1), with the licence of each paper. Its slogans are `ai` (K5.2.4) |
| TheoremGraph (`math-graph`) | 16,131,238 rows | CC BY 4.0, with the licence of each statement. The text is empty unless the paper is open | the edges and the ids, with attribution. The text where it is present |
| TheoremGraph (`theorem-matching`) | 100,831 candidate pairs of Lean declarations and arXiv statements | CC BY-SA 4.0 (K01, part C) | candidate crosswalks (K6.7), with share-alike attribution |
| Matlas | 8.07M statements from 435K papers and 1.9K textbooks | no licence or terms stated | ids of matches at query time only (`record`) |
| the Stacks Project and Mathlib alignment (arXiv:2610.31011) | | paper CC BY 4.0 | candidate crosswalks, which the align role checks (K6.7) |

The BY-SA release of TheoremSearch is not taken as one block. The importer filters it by source and by the licence of each row, and drops the Stacks rows and every row whose licence is NC or not stated.

## K5.8 Curricula and theorem lists

Curricula say which items a course needs, and in which order. The corpus imports them as data (K10.3), never as text:

| Source | Size | Licence | The corpus takes |
|---|---|---|---|
| Mathlib `docs/undergrad.yaml` | 563 items, 130 of them not yet formalized | Apache-2.0 | all: items, structure, declarations |
| Mathlib `docs/overview.yaml` | 511 items | Apache-2.0 | all |
| Mathlib `docs/1000.yaml` | 1,199 entries keyed by Wikidata id; 217 with declarations | Apache-2.0 | all. The Wikidata keys are crosswalk ids |
| Mathlib `docs/100.yaml` | 100 entries; 78 with declarations | Apache-2.0 | all |
| 1000+ theorems project | 228 entries on the site | Unlicense | all |
| Wiedijk, *Formalizing 100 Theorems* | 100 theorems, with counts for 13 systems | no licence | the list of names and the counts, as facts |
| MAA CUPM Curriculum Guide (2015) | 116 pages | © MAA | course areas and topic lists, as facts |
| QAA Subject Benchmark Statement, Mathematics, Statistics and Operational Research, 5th edition (2023) | 35 pages | © QAA | outcome categories, as facts |
| qualifying exam syllabi (for example UC Berkeley) | | no licence | topic lists, as facts. Not the past exam problems |
| tables of contents of open and closed textbooks | | K5.4, K5.5 | the order of the items |

A curriculum is also a mention source: each item that it names is a mention, with the curriculum as its source. The `curriculum` term of the priority (K12.3) comes from these mentions. The difference between Mathlib's `100.yaml` (78 with declarations) and Wiedijk's count for Lean (83) is not settled.

## K5.9 Exercise banks

The large exercise banks have licences that the prose zone cannot take. They give links, and the exercise node (K8.7) is math2's own text:

| Bank | Size | Licence | The corpus takes |
|---|---|---|---|
| WeBWorK Open Problem Library | about 35,000 problems | CC BY-NC-SA 3.0 by default; an author can choose an OSI licence | the path of a problem as a link in `external`; its tags as metadata. No text |
| MyOpenMath and IMathAS | | question code GPL-2.0; the generated HTML CC BY 4.0 | links. The generated HTML of a question is `open`, but the importer takes it only after a person checks the licence of that question |
| STACK | | GPL | links |
| Numbas | | code Apache-2.0; public questions under Creative Commons, by question | links. Text only from a question whose licence is CC BY or CC BY-SA |
| Natural Number Game (NNG4), *Mathematics in Lean* | | Apache-2.0 | Lean exercises, into the code zone, with attribution |
| lean4game games | | the engine is GPL-3.0; each game has its own licence | links. A game with a permissive licence is like NNG4 |
| the exercises of Table A books | | the licence of the book | exercise text, with attribution |

The exercise role (K7.3) has no text of an NC bank in its context.

## K5.10 Channels and politeness

The importers keep each limit below, identify themselves with the User-Agent of the toolchain, `m2/<version> (+https://github.com/tamnd/math2-agent)`, with no personal address, and prefer a dump to live requests:

| Source | Channel | Limit that the importer keeps |
|---|---|---|
| Wikipedia | the monthly XML dump. The API only for single pages | API: a compliant User-Agent gets 200 requests a minute. The robot policy of 16 March 2026: dumps before live requests |
| Wikidata | the weekly JSON dump | WDQS: 60 s of query time a minute, 5 parallel queries, only for checks |
| ProofWiki | the daily dump | no crawling |
| nLab | the git mirror | no crawling |
| OEIS | git `oeisdata` | site: `Crawl-Delay: 10`, and `/search` is disallowed |
| LMFDB | bulk downloads | `/api/` is disallowed, and the site has a JavaScript gate. The importer does not try to pass it |
| MathOverflow, Math.SE | the API, with a key | 10,000 requests a day; keep the `backoff` field, which applies across sites |
| arXiv | `tamnd/arxiv` | one request each 3 seconds, for all machines together |
| Project Gutenberg | `/robot/harvest` or a mirror | `wget -w 2` |
| MathWorld, DLMF, Wolfram Functions | none | no requests. Ids come from Wikidata |
| Kerodon, groupprops, the Manifold Atlas | single pages | one request at a time, at most one each 10 seconds. These sites are small and have one maintainer or none |

A source that blocks automatic clients, or asks for a login or a click-through, is not read until a person decides how to proceed. The importer never passes a gate.

## K5.11 Adapters

Each source has one adapter, `importer/<source>`, in `tamnd/math2-agent` (K12.8). Spec 09.1 puts adapters in `internal/ingest/<source>`. The corpus uses `importer/<source>` instead, because the toolchain has no `internal/` packages. Most sources share a format, so most adapters are thin and use a format package:

| Format | Sources |
|---|---|
| MediaWiki XML dump | Wikipedia, ProofWiki |
| MediaWiki API | the subwikis, the Manifold Atlas |
| Markdown in git | nLab, π-Base |
| LaTeX theorem environments | PlanetMath, Open Logic, HoTT, Hefferon, Lebl, Napkin, Ernst |
| PreTeXt XML | *Active Calculus*, Levin, Keller and Trotter, Austin |
| CNXML | OpenStax |
| YAML in git | the Mathlib lists, 1000+, Erdős problems, DaRT |
| JSON dump | Wikidata |
| JSONL or Parquet | TheoremSearch, TheoremGraph, OpenAlex |
| PDF | Beck et al., *Mathematics for Computer Science*: locators and the table of contents only, until a source in LaTeX is found |

Each adapter has fixtures from a small sample of its source, and tests that use the fixtures and never the network (spec 19). An adapter writes mentions and nothing else (K4.2).

## K5.12 The order of import

The order follows K14. A source enters when the phase before it meets its exit:

1. **Phase 1, open data with no doubt about the licence.** Wikidata, the Mathlib lists, 1000+, OEIS names, π-Base, DaRT, Fungrim, and the records of OpenAlex, Crossref, DataCite, HAL and Numdam. Then the records and locators of the `record` and `gfdl` sources: the tags of the Stacks Project and Kerodon, the names of the nLab, the ids of MathWorld and the EoM from Wikidata, the tables of contents of closed textbooks where the publisher shows them.
2. **Phase 2, the pilot area (group theory).** Wikipedia, ProofWiki, PlanetMath and groupprops for the area, the Table A books that cover it (Judson is `gfdl`, so it gives locators only), the curricula of K5.8, and the Stack Exchange posts that the nodes cite.
3. **Phase 3 and later, the textbook layer.** The rest of Table A, the rest of the wikis, and TheoremSearch `-permissive` and TheoremGraph for the research layer. Each new source first goes through the held-out check of K4.7.

## K5.13 Corrections to the main spec

| Spec | Value now | Value on 2026-10-26 |
|---|---|---|
| 1.7, 9.2 | Wikidata: about 113k items with a defining formula | 113,118, but 105,345 are prime numbers. 6,560 are other formulas |
| 1.7, 9.2 | Wikipedia: about 26k mathematics articles; dumps twice a month | 28,196 articles; the dumps are monthly in 2026; new API rate limits |
| 1.7, 9.2 | nLab: about 19k pages, licence not known | 20,778 pages, no licence: `record` |
| 9.2 | ProofWiki: CC BY-SA 3.0, quarterly | CC BY-SA 3.0 and GFDL; a daily dump |
| 1.7, 9.2 | Encyclopedia of Mathematics: CC BY-SA 3.0 | originals © Springer: `record`, except wholly new articles |
| 9.2 | Erdős problems: statements from `teorth/erdosproblems`, Apache-2.0 | the repository has no statements. The statements on the site have no licence: `record` |
| 9.2 | LMFDB: API and bulk downloads | bulk downloads only: `robots.txt` forbids `/api/` |
| 9.2 | zbMATH Open: CC BY-SA 4.0, monthly | CC BY 4.0 or CC BY-SA 4.0 (not settled); twice a year |
| 9.3, 16.6 | Kerodon: GFDL | no licence stated: `record` |
| 9.3 | about 15% of arXiv is `open` | 15% of all papers; 36.9% of new mathematics papers in a 2026 sample |
| 9.3 | `gfdl` examples: Stacks, Kerodon, Open Problem Garden | Stacks, the Manifold Atlas, Open Problem Garden |
| 9.3 | `record` examples | add the DLMF, Wolfram Functions, the Kourovka Notebook, KnotInfo, ISGCI, the ATLAS and House of Graphs |
| 9.6 | TheoremSearch: 9.2M theorems; the 1.34M open release | 8.4M in the live index; take the `-permissive` release, because the BY-SA release mixes in GFDL text |
| 1.7 | MathWorld 16,849; OEIS 400,673 | 16,939; 400,709 |
| 9.1 | adapters in `internal/ingest/<source>` | `importer/<source>` |

## K5.14 Rules

| Rule | H/S | Checks |
|---|---|---|
| L08 | H | each source in `manifests/sources.yaml` has the fields `licence`, `class`, `licence_read`, `authored`, `channel`, `snapshot`, `attribution` and `approved_by`, and each mention names a source that has them (K5.2) |
| L09 | H | a source whose `licence` is a choice of licences has an `election`, and the elected licence gives the class (K5.2.2) |
| L10 | H | a source of the `open` class whose licence is public domain has `public_domain` with `us: true` and `eu: true` (K5.2.3) |
| L11 | H | a mention with text comes from a snapshot whose licence was read on or before the date of that snapshot (K5.2.1) |
| L12 | S | a source whose licence was last read more than twelve months ago |

With rules L01 to L07, these rules are the licence gate for human sources. The open questions of this document are in K15: the current licence of the EoM, the terms of Stack Exchange of 13 November 2025, the licences of the nLab and the Manifold Atlas, the knowls of the LMFDB, the zbMATH licence, the 14 AIM titles, the US port of CC BY-SA 3.0, and the sources that the research could not reach.
