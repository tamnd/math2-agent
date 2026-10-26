# K15. Open questions

This document lists each question that the corpus spec leaves open. Each question has an id, the way to settle it, and the phase of K14 that it must be settled before. A question that blocks nothing has "no block".

## K15.1 How the list works

- **Owner.** The maintainers own the list. A question is closed by a commit that changes this document and the documents that the answer affects, with the source of the answer.
- **The safe course.** Until a question is closed, the corpus takes the safe course that the text gives: a source with a licence in doubt is `record`, a count in doubt is not quoted as a rule, and a check that is not calibrated is soft.
- **Asking a source.** Many questions are settled by asking the people who run a source. Each request is polite, short, and sent by a maintainer from the project, not by an agent. The answer is kept with the source document (K5.2).
- **Questions for a lawyer** are in K13.10. This list names them and does not repeat them.

## K15.2 Licences and terms of human sources

| Id | Question | How to settle | Before |
|---|---|---|---|
| OQ01 | Did the move of the Encyclopedia of Mathematics to EMS Press change the copyright of the Springer originals? What is its current size? | read the live site again; ask the EMS | phase 2 |
| OQ02 | Do the Stack Exchange Public Network Terms of 13 November 2025 add clauses on AI or on scraping? Does the public MCP server cover MathOverflow and Math.SE? | read the terms in a browser | phase 2 |
| OQ03 | Will the nLab steering committee give an explicit licence for new contributions? | ask the steering committee | no block: the nLab is `record` |
| OQ04 | Is the Manifold Atlas GFDL 1.3 or in the public domain? | ask the editors | no block: it is `gfdl` |
| OQ05 | Are the knowls of the LMFDB under the CC BY-SA licence of its database? Which bulk channel does the LMFDB want AI users to use? | ask the LMFDB | the first LMFDB import |
| OQ06 | Is the zbMATH knowledge graph CC BY 4.0 or CC BY-SA 4.0? What do the API terms say? | ask zbMATH, or open an issue on `zbMATHOpen/zbmath-open-kg` | phase 1 (until then, CC BY-SA 4.0) |
| OQ07 | What are the licences of the 14 AIM titles that nobody has read (Bruckner and Thomson, ORCCA, Manes, May and Bart, Marsden and Weinstein, Boyd and Vandenberghe, Treil, Fields, Bender and Williamson, Bard, Zimmermann, Crisman, Stein, Connell)? | read each at its source | before each book enters Table A |
| OQ08 | Do the books that the Open Textbook Library lists as CC BY or CC BY-SA (Zakon, Kuttler, Shimamoto, Petrunin, Callahan and others) have those licences at their source? Is Hitchman CC BY-SA or GFDL? | read each at its source | before each book enters Table A |
| OQ09 | Does the 3.0 US port of CC BY-SA allow adaptation under CC BY-SA 4.0, as §4(b) of the Unported licence does? | read the legal code of the port | the import of Hefferon |
| OQ10 | Which CC variant do the PreTeXt titles with a plain "Creative Commons" badge have? | read each at its source | before each book enters Table A |
| OQ11 | What licences do FindStat, House of Graphs, KnotInfo and the Knot Atlas give? Did the authors of KnotInfo consent to the GPL-3.0 CSV package? | ask each project | no block: they are `record` |
| OQ12 | What are the terms of HathiTrust, the Internet Archive, CORE, Unpaywall and EuDML, and the licence of the GDZ scans, the Project Euclid journals and the Semantic Scholar abstracts? | read each in a browser | before the first import from each |
| OQ13 | What is the licence of MathBases, and of the sources that the research could not reach (GroupNames)? | read again | no block |
| OQ14 | What share of LibreTexts pages has no declared licence? | a sample of pages, by the `license:` tag | no block: the corpus prefers the upstream source |

## K15.3 Formal libraries and datasets

| Id | Question | How to settle | Before |
|---|---|---|---|
| OQ15 | What are the licences of the 20 Formal Frontier repositories that nobody has read? | read each `LICENSE` | before each binding to them |
| OQ16 | Does the "not intended for ML" sentence of the ATLAS v2 preamble bind a user of its Apache-2.0 grant? | K13.10 | no block: ATLAS is `link` |
| OQ17 | Is Coquelicot LGPL-3.0 upstream, as its fork is? | read the Inria repository | no block: it is `crosswalk` |
| OQ18 | Is LeanDojo-v2 Apache-2.0 (the GitHub API) or MIT (its README)? | ask the authors | no block: it is `evaluate` |
| OQ19 | Do the terms of the model that generated a dataset (DeepSeek-V3, Qwen2.5-72B, Gemini, Kimina-72B) pass on to the dataset? | K13.10 | any use of such a dataset beyond evaluation |
| OQ20 | What is the size of Lean Pool, and what are the licences of the projects that it imports? | read its repository | before Lean Pool is bound |
| OQ21 | Does doc-gen4 publish JSON for each declaration that the binding can use? | read doc-gen4 | phase 0, item 2 |

## K15.4 Counts that do not agree

These counts change no rule. The corpus quotes the figure of the source and says that it is not settled, until a reading settles it.

| Id | Question | How to settle |
|---|---|---|
| OQ22 | Mizar: 1,493 articles (the version string) or 1,498 (spec 01)? | count the articles of the MML release |
| OQ23 | Tau Ceti: does "3 million lines" mean Tau Ceti or Mathlib? | count the lines at a pinned commit |
| OQ24 | Metamath: 51,207 statement numbers, "over 41,000" with mathboxes, and 74 or 75 of the 100 theorems. Which count does the corpus report? | define the count in the importer and record it |
| OQ25 | TheoremSearch: 1,239,720 rows in the card of the `-permissive` release against 2,677,439 in the viewer; 8.4M live against 9.2M in the paper | count the rows at a pinned revision |
| OQ26 | Mathlib `100.yaml` gives 78 theorems with declarations; Wiedijk gives 83 for Lean | compare the two lists item by item |
| OQ27 | 1000+: 228 entries on the site; spec 01 says 241 in Lean | parse the YAML files at a pinned commit |
| OQ28 | Are the ProofWiki Main Page counters (30,348 proofs, 36,102 definitions) live? | count from the daily dump |
| OQ29 | Lean Workbook: 57,231 and 82,893 rows in the paper, 25,214 in the default configuration | read each configuration |
| OQ30 | Is the move of the Wikipedia dumps from twice a month to once a month official? Were the free HTML dumps (last run 20 March 2025) stopped? | read the Wikimedia announcements |

## K15.5 The law

| Id | Question | Where |
|---|---|---|
| OQ31 | Is the notice of K13.2 enough? | K13.10 |
| OQ32 | Does the database right of the EU limit the import of catalogues beyond their terms? | K13.10, phase 1 |
| OQ33 | Do the terms of a provider reach users of math2? | K13.10 |
| OQ34 | The "availability to third parties" clauses of the Qwen 3.8 and Kimi K3 licences | K13.10 |
| OQ35 | The licence of DeepSeek-Prover-V2: the MIT file or the Model Agreement? | K13.10 |
| OQ36 | The copyright of model output in the United Kingdom and the EU, and the effect of the judgment in C-250/25 (expected 2027) and of the appeal in *GEMA v. OpenAI* | K13.10, phase 3 |
| OQ37 | What do the Commission's guidelines on Article 50 (reported for 20 July 2026) and the code of practice on transparency say? | read them; K13.5 |
| OQ38 | Is US public domain enough for a work that is not in the public domain in the EU? The corpus says no (K5.2.3). Should it accept a work that is in the public domain in all countries where math2 has users, if the EU rule is not met? | no block: the corpus takes the safe course |

## K15.6 Model generation and quality

These questions are settled by measurement in phase 2 (K14.4), not by reading.

| Id | Question | How to settle |
|---|---|---|
| OQ39 | What is the error rate of model-written definitions and statements in prose? No study has measured it | the sample audit of K9.7 in the pilot |
| OQ40 | How often do models reproduce mathematical text? The 27% figure is for encyclopedia prose in general, on models of 2024 | the copy check of K9.4 over the pilot, against the reference corpus |
| OQ41 | Which thresholds of the copy check give fewer than 1% false matches on text that people wrote? | the calibration of K9.4, before L05 blocks |
| OQ42 | How well do the current judges (Opus 5.5, Fable 5.1 and the models of other families) find wrong steps? The published numbers are for models of 2025 | a labelled set of the pilot area (K7.2) |
| OQ43 | May a model judge its own output, or must the judge be of another family? The studies disagree | the labelled set, with both arms |
| OQ44 | What is the first-pass acceptance rate, and so the real cost for each accepted node? | the run records of the pilot (K12.6) |
| OQ45 | How many hours does a person need for each node, by kind and writing level? | the review records of the pilot (K12.5) |
| OQ46 | Does any provider filter recitation, and at which threshold? Anthropic and OpenAI document none; Gemini has a RECITATION reason with no published threshold | read the documentation; no block, because the copy check of K9.4 does not rely on it |
| OQ47 | The figures in part D marked [snippet] or "derived" (the First Proof batch 2 tally, the later BrokenArXiv figures, Seed-Prover 1.5, the AlphaProof TPU-days) | read the primary PDFs before a spec section quotes them |

## K15.7 Representation and packaging

| Id | Question | How to settle | Before |
|---|---|---|---|
| OQ48 | When does MathML 4 reach Candidate Recommendation? | the W3C Math Working Group | no block: the corpus generates MathML |
| OQ49 | What are the licences of Verso Blueprint, LaTeXML, MMT and the SMGloM archives other than `smglom/mathsoft`? | read each `LICENSE` | before each tool is used in the packager |
| OQ50 | Which projects have moved to Verso Blueprint, and which keep leanblueprint? | read the catalogue again | the blueprint import |
| OQ51 | Is there a version of Common Cartridge after 1.3? Do Canvas, Moodle and Blackboard support QTI 3? | read the 1EdTech pages and the LMS documentation | the course output of K11 |
| OQ52 | Is the Learning Commons Knowledge Graph worth aligning to for school mathematics? | a sample alignment | no block |
| OQ53 | The citation of Leron (1983) and the date of Tao's post "There's more to mathematics than rigour and proofs" | read the primary sources | before a corpus document cites them |

## K15.8 Design questions of the corpus

These questions are about the corpus itself. The text gives a first answer, and the pilot tests it.

| Id | Question | First answer | How to settle |
|---|---|---|---|
| OQ54 | Are the sizes of K2.7 right, for example 300,000 to 1,000,000 named statements? | the estimates of K2.7 | capture and recapture (K4.7) after phase 1 |
| OQ55 | Is a recall of 90% for each MSC code at two digits (rule I06) the right bar? | 90% | the held-out checks of K4.7 in phase 1 and phase 2 |
| OQ56 | How many Mathlib declarations become nodes? | a rough guess of 40,000 to 80,000 (K6.4) | the outcomes of the pilot area, scaled by area |
| OQ57 | Is a gold set of 2,000 pairs for each layer enough to measure the clustering? | 2,000 (K4.3) | the intervals of the first measurement |
| OQ58 | Are the error thresholds of K9.7 right for each stratum? | the thresholds of K9.7 | the monthly review of the thresholds (K14.8) |
| OQ59 | How much review capacity can the project get, and from whom? | not known; it sets the speed of each phase (K12.5) | the pilot, and the curators who join it |
| OQ60 | Should ProofWiki text enter nodes as text, or only as a source for math2's own text? | as text, with attribution, by the importer of open text (K5.4.5) | the review of the pilot nodes that use it |
