# K01, part B. Literature indexes, open textbooks and syllabi: sizes, licences, channels (research for math2)

Status: research note for spec 01 §1.7, spec 09 (all, especially the 9.3 licence gate) and spec 16 §16.6.
Read date: **every fact below was read on 2026-10-26** unless another date is given. Each fact has a URL next to it.
**UNVERIFIED** means I could not confirm the fact at a primary source. The reason is given each time.
Method: WebSearch, WebFetch and a few single `curl` requests with User-Agent `m2/research (+https://github.com/tamnd/math2-agent)`. There was no crawling and no bulk download. The largest samples were three days of arXiv OAI-PMH `set=math` (1,421 records), the Open Textbook Library mathematics JSON (22 pages), four Mathlib YAML files, two syllabus PDFs and one zbMATH API query.

This note does not repeat what the spec already says. The spec already has: arXiv about 15% open, `s3://arxiv` requester-pays and Kaggle CC0; zbMATH 168M triples; Stacks GFDL 1.2 with 21,446 tags; TheoremSearch 9.2M / 690k / 1.34M; TheoremGraph 11.7M / 18.3M / 48k; Matlas 8M / 435k / 1.9k; MSC2020 CC BY-NC-SA; the Stack Exchange no-LLM clause. Where my findings correct or refine those figures, I say so.

The access classes used in the "math2 can take" column are those of spec 9.3: `open`, `share-alike-nc`, `verbatim`, `gfdl`, `record`. Node prose is CC BY-SA 4.0, code is Apache-2.0 and metadata is CC0 (16.6).

---

## Part 1. Literature indexes and full text

### 1.1 arXiv

**Bulk and API terms**
- **S3 bulk:** the bucket `arxiv` (prefixes `pdf/`, `src/`) is requester-pays in us-east-1. Files come as tars of about 500 MB with manifests and are updated about monthly. The total is "about 9.2 TB" as of April 2025, growing by about 100 GB a month. Users "must link back to arXiv for downloads". (https://info.arxiv.org/help/bulk_data_s3.html, https://info.arxiv.org/help/bulk_data.html)
- **API Terms of Use** (https://info.arxiv.org/help/api/tou.html; the page carries no date):
  - Metadata is CC0.
  - "The redistribution of e-prints requires permission from the copyright holder." Storing and serving e-prints needs permission or a licence; personal and research use is allowed.
  - The rate limit is one request every 3 seconds on a single connection, counted across all of a user's machines.
  - Users may not claim arXiv endorsement.
- **AI and TDM terms:** none of the four arXiv help pages above says anything about AI, LLMs, TDM or training. So for arXiv the per-paper licence is the only AI-relevant term. (A missing clause is not the same as permission. See open doubt D3.)
- **Licence options for authors** (https://info.arxiv.org/help/license/index.html):

| Licence | 9.3 class |
|---|---|
| CC BY 4.0 | `open` |
| CC BY-SA 4.0 | `open` |
| CC0 | `open` |
| CC BY-NC-SA 4.0 | `share-alike-nc` |
| CC BY-NC-ND 4.0 | `verbatim` |
| arXiv non-exclusive distribution licence 1.0 (http://arxiv.org/licenses/nonexclusive-distrib/1.0/) | `record` |

**New measurement: the 2026 licence mix in math.** I harvested three single days from `oaipmh.arxiv.org/oai`, with `verb=ListRecords&metadataPrefix=arXiv&set=math`: from=until=2026-10-01, 2026-04-15 and 2026-07-08. That gave 1,421 records. The 1,026 records with 26xx identifiers (papers first submitted in 2026) break down as follows:

| Licence | Share |
|---|---|
| non-exclusive (record) | 55.3% |
| CC BY 4.0 | 34.0% |
| CC BY-NC-ND | 5.9% |
| CC BY-NC-SA | 1.9% |
| CC0 | 1.9% |
| CC BY-SA | 1.0% |

- **`open` share for node prose (BY + BY-SA + CC0): 36.9%.** By day it was 35.3%, 38.3% and 38.8%. Records with older identifiers that were re-versioned on those days (395 of them) were about 39.2% `open`.
- **Consequence:** the spec's "about 15% of arXiv is `open`" is the all-time stock. The 2026 flow in math looks more than twice as open. This is a small sample and needs confirmation (D2).
- **Caveats:**
  - The OAI datestamp also counts metadata updates, not just new submissions.
  - I did not establish whether `set=math` includes cross-lists.

### 1.2 zbMATH Open

- **OAI-PMH:** Identify at https://oai.zbmath.org/v1/?verb=Identify works. It reports earliestDatestamp 1755.
- **REST API** (https://api.zbmath.org/v1/document/_search):
  - It returns full review text (`editorial_contributions`) and a per-document `license` field. That field was empty in my sample.
  - The query `py:2025` returns `nr_total_results` 133,093. That is a reasonable yearly-volume figure for 2025.
- **API terms:** https://static.zbmath.org/legal/api-terms-and-conditions.html was blocked by a Cloudflare challenge, and so was https://zbmath.org/about/. **The API terms, and any AI clause in them, are UNVERIFIED.**
- **Knowledge graph dump** (Zenodo record 21497976, concept DOI 10.5281/zenodo.21497975; https://zenodo.org/records/21497976):
  - Published 2026-09-01 and modified 2026-10-24. File `zbmath-kg-ttl-2026.tar.gz`, 1.6 GB.
  - Built from the January 2026 API dump. It is updated about twice a year, not monthly as spec 9.2 says.
  - A temporary SPARQL endpoint is at http://212.227.170.235:8890/sparql.
  - Repository: https://github.com/zbMATHOpen/zbmath-open-kg. Paper: https://arxiv.org/abs/2609.00969.
  - Stats: 168M+ triples, 34M+ entities, 4M+ publications, 1M+ persons, 3M+ reviews.
  - Restricted abstracts appear as the literal placeholder "zbMATH Open Web Interface contents unavailable due to conflicting licenses".
- **The licence conflict:** the Zenodo metadata field says **CC BY 4.0**, but the record's description says **CC BY-SA 4.0** for the dataset, with the I4OC references CC0. The spec says "CC BY-SA 4.0". For math2 both are `open`. They do differ on whether derived exports must be BY-SA (D1).
- **What math2 can take:**
  - Records, MSC codes and links as metadata.
  - Review text is `open` under either reading. In practice it should be used as evidence and for locators, not pasted into prose. Reviews are third-party texts, and their per-item rights in the API `license` field are empty or unclear.

### 1.3 OpenAlex and Unpaywall

- **Snapshot** (https://help.openalex.org/download-all-data/openalex-snapshot, page updated 2026-09-24):
  - The bucket `s3://openalex` is free and anonymous (`--no-sign-request`). `LICENSE.txt` in the bucket is CC0 1.0, which I checked.
  - The September 2026 release is about 745 GB of compressed JSONL (works about 615 GB), or about 770 GB in Parquet.
  - The free snapshot is quarterly, on the second Wednesday of January, April, July and October. Daily snapshots are paid.
- **API key and pricing:** an API key has been required since 2026-02-13, with usage-based pricing. **Partly UNVERIFIED**: I found this only in secondary sources (https://casrai.org/news/openalex-api-keys-mandatory-usage-based-pricing-2026). https://help.openalex.org/api/ did not render the details for me.
- **Unpaywall:**
  - Unpaywall now runs inside OpenAlex (the "Walden" rewrite), and its data feed is a paid product.
  - The last free standalone Unpaywall snapshot I found is dated 2024-11-27, according to a SUB Göttingen note.
  - https://unpaywall.org/products/snapshot returned an empty page. **The current Unpaywall terms are UNVERIFIED.**
- **What math2 can take:** all of it, as CC0 metadata (works, authors, venues, concepts, OA locations). OpenAlex has no full text.

### 1.4 Crossref

- **REST API** (https://www.crossref.org/documentation/retrieve-metadata/rest-api/): no sign-up is needed. Crossref says "almost none of the metadata is subject to copyright", but "some abstracts ... may be subject to copyright".
- **2026 public data file** (https://www.crossref.org/blog/2026-public-data-file-now-available/, posted 2026-03-17): 180,034,490 records as of 2026-03-14, 208 GB of JSONL. It is distributed via Academic Torrents (doi 10.13003/nggf-vt1j) or AWS.
- **What math2 can take:** DOI metadata and reference lists as facts. Abstracts are `record` unless the item's own licence says otherwise.

### 1.5 DataCite

- **Public data file:** annual and CC0. The use policy asks for credit.
  - The 2025 file was generated on 2026-01-06 and announced on 2026-01-22. It has 108M findable DOIs and is at https://datafiles.datacite.org/.
  - The monthly file is for members only. The file is also on Dimensions' BigQuery.
  - Sources: https://support.datacite.org/docs/datacite-public-data-file, https://support.datacite.org/docs/datacite-data-file-use-policy, and the DataCite blog.
- **What math2 can take:** all of it, as metadata. Its use for math2 is datasets and software DOIs (Zenodo and others).

### 1.6 Semantic Scholar and S2ORC

- **API licence** (https://www.semanticscholar.org/product/api/license, "last updated" 2023-05-17):
  - The licence is non-transferable. You may not redistribute or sublicense the API.
  - S2 data is under its own licences, "such as CC BY-NC or ODC-BY". Attribution is required.
  - It has no AI-training clause.
- **S2ORC:**
  - https://github.com/allenai/s2orc says ODC-By 1.0 (earlier releases were NC). The Hugging Face card at https://huggingface.co/datasets/allenai/s2orc is tagged cc-by-2.0, which is an inconsistency.
  - Size: a 2026 arXiv paper reports the 2026-05-26 release as having 15,679,781 S2ORC full-text papers.
- **UNVERIFIED:**
  - The licence of the separate abstracts dataset (the datasets API docs returned HTTP 429).
  - The current total of papers. A third-party figure for 2025-09 says 200M+ papers and 100M+ abstracts.
- **What math2 can take:**
  - Paper ids as crosswalk targets.
  - S2ORC full text is ODC-By *as a database*, but each paper's text keeps the paper's own copyright. Treat it per paper, by its arXiv or publisher licence. It is never `open` just because it sits in S2ORC.

### 1.7 CORE

- **Terms** (https://core.ac.uk/terms; I read this via a search snippet because the page returned 403 to curl):
  - Dumps are ODC-By, covering CORE's own database rights only.
  - API access needs a licence; it is free for individuals and for public research.
  - A licensing-framework review proposes CC0/ODC-0 for metadata and "conditional access for lawful TDM" for full texts.
- **Size:** 291M metadata records and 32.8M full texts (February 2023, the CORE paper in *Scientific Data*). **The current size and the current dump terms are UNVERIFIED** (core.ac.uk/documentation returned 403).
- **What math2 can take:** records. Full texts follow each paper's licence, as for S2ORC.

### 1.8 HAL (France)

- **OAI-PMH** (https://api.archives-ouvertes.fr/oai/hal/?verb=Identify): "Metadata is licensed under CC0".
- **Math share, measured by API:** a query on `level0_domain_s:math` returned **170,406 records, 105,413 of them with a file**. (The query `domain_s:math*` returns 0.) The licence facet over the records with a file:

| Licence | Records | 9.3 class |
|---|---:|---|
| hal-authorisation-v1 (https://about.hal.science/hal-authorisation-v1/) | 84,016 | `record` |
| CC BY 4.0 | 9,239 | `open` |
| hal copyright (https://hal.science/licences/copyright/) | 4,272 | `record` |
| CC BY-NC-ND | 3,861 | `verbatim` |
| CC BY-NC | 1,796 | `share-alike-nc` |
| CC BY-NC-SA | 731 | `share-alike-nc` |
| CC0 | 492 | `open` |
| CC BY-SA | 449 | `open` |
| CC BY-ND | 254 | `verbatim` |
| Etalab 2.0 | 163 | `open` (attribution licence, compatible with CC BY) |
| PDM | 6 | `open` |
| none / empty | 113 + 21 | `record` |

- **`open` share: 10,349 / 105,413 = about 9.8%.**
- **Channels:** REST at api.archives-ouvertes.fr/search, SPARQL at https://data.hal.science/doc/sparql, and OAI-PMH.
- I found no AI clause.

### 1.9 Numdam (Mathdoc)

- **Size** (https://www.numdam.org/en/): 75,599 articles in 120 periodicals, 706 books in 8 collections, 416 theses, 1,428,508 pages.
- **Moving wall:** 2 to 10 years. Mersenne journals arrive after 1 year.
- **Conditions** (https://www.numdam.org/conditions/):
  - Metadata is CC0 and can be harvested by OAI-PMH.
  - Full texts are for individual download for research, study or training.
  - Downloading "un volume substantiel" without explicit permission is forbidden, and so is modifying the files.
  - Republishing is allowed only for public-domain items (described as "très rares sur Numdam") and for CC-licensed articles.
- **What math2 can take:** records (CC0). Text is `record` by default, or the per-article CC licence where one is given. Bulk full text is not allowed without permission.

### 1.10 EuDML

- **UNVERIFIED for current figures.** eudml.org, including its OAI endpoint, returned 403 or a Cloudflare challenge.
- What I could find:
  - Mathdoc's page (https://www.mathdoc.fr/en/realisations/eudml/, undated) says "over 270,000 documents".
  - A 2016 report says 257k items, 252,111 articles and 323 journals.
  - https://eudml.org/journals listed 339 journals in a search snippet.
  - EuDML's policy is to hold only content past its moving wall, with JATS-flavoured OAI. **Its metadata licence is UNVERIFIED.**
- **What math2 can take:** records via the partner sources (Numdam, GDZ, CEDRAM and others) rather than EuDML itself.

### 1.11 Project Euclid

- https://projecteuclid.org/about says "over 80% of the journal articles hosted ... are openly available". It is run by Duke University Press.
- "Openly available" means free to read, not openly licensed. **Per-journal licences are UNVERIFIED.** Default to `record`, and use the per-article licence where it is shown (for example, some IMS/Bernoulli journals).

### 1.12 GDZ (Göttinger Digitalisierungszentrum)

- **Size:** over 15M pages, with collections for Mathematica and FID Mathematik (https://gdz.sub.uni-goettingen.de/).
- **IIIF:** the manifest for *Mathematische Annalen* vol. 1 (1869) at https://manifests.sub.uni-goettingen.de/iiif/presentation/PPN235181684_0001/manifest has an attribution but **no `license` field**.
- **PDF terms:** a generated PDF (https://gdz.sub.uni-goettingen.de/download/pdf/PPN71557308X/LOG_0040.pdf) has a cover page saying reproductions may not be made for other repositories without written permission.
- **The licence of the scans is UNVERIFIED.** The underlying 19th-century texts are public domain.
- **What math2 can take:** records and locators. Statements from public-domain works, which math2 rewrites anyway. Not the scans.

### 1.13 The US public-domain line, HathiTrust, Internet Archive, Project Gutenberg

**The US public-domain line.** Works published in 1930 entered the US public domain on 2026-01-01 (https://web.law.duke.edu/cspd/publicdomainday/2026/). So in 2026, works published in the US before 1931 are public domain in the US. Public domain is `open` for prose: no attribution is needed, but math2 should give it anyway.

**HathiTrust**
- hathitrust.org (terms and acceptable-use pages) is behind Cloudflare. **The terms and any AI policy are UNVERIFIED.**
- https://github.com/hathitrust/datasets lists the public-domain text subsets `ht_text_pd`, `ht_text_pd_open_access`, `ht_text_pd_world` and `ht_text_pd_world_open_access`. The full `ht_text` is used only by HTRC for non-consumptive research.
- A related public-domain corpus: Harvard Institutional Books 1.0 has 983,004 public-domain volumes.

**Internet Archive**
- Counts from advancedsearch (https://archive.org/advancedsearch.php?q=subject%3Amathematics+AND+mediatype%3Atexts&rows=0&output=json):
  - `subject:mathematics AND mediatype:texts` gives 206,597 items.
  - Restricting to `year:[1500 TO 1930]` gives 8,076.
- https://archive.org/about/terms returned an empty page. **The IA terms are UNVERIFIED.** Uploads on IA include many in-copyright books. For example, Hatcher's *Algebraic Topology* is there, though its licence is noncommercial copying only (§2.4). IA hosting therefore says nothing about licence.

**Project Gutenberg**
- **Robot policy** (https://www.gutenberg.org/policy/robot_access.html): the site is "intended for human users only". Bulk users should use a mirror, or wget with `-w 2` through `/robot/harvest`. Catalogue data is public domain.
- **Licence** (https://www.gutenberg.org/policy/license.html): "Nearly all the individual works ... public domain in the United States". The trademark licence no longer applies once "all references to Project Gutenberg are removed".
- **Size:** about 78,000 items (a 2026-03-22 figure).
- **Mathematics bookshelf 102** (https://www.gutenberg.org/ebooks/bookshelf/102) includes:
  - Hardy, *A Course of Pure Mathematics* (#38769, with LaTeX source)
  - Dedekind (#21016)
  - Casey's Euclid (#21076)
  - Thompson, *Calculus Made Easy*
- **What math2 can take:** `open` (public domain in the US) with the Project Gutenberg header removed. The non-US public-domain status of each work is a separate check (D10).

### 1.14 Extraction projects (TheoremSearch, TheoremGraph and LeanGraph, Matlas, and new ones)

**TheoremSearch** (https://arxiv.org/abs/2602.05216, v2 dated 2026-03-07)
- **Size:** https://theoremsearch.com shows "8.4M" indexed. By source:

| Source | Statements |
|---|---:|
| arXiv | 8,104,539 |
| Lean | 285,213 |
| ProofWiki | 23,284 |
| Stacks | 10,645 |
| Lean Community | 1,888 |
| Open Logic | 588 |
| CRing | 498 |
| HoTT | 257 |
| Napkin | 228 |

  The spec's 9.2M figure is from the paper; the live site now shows 8.4M.
- **Channels:** a REST API at https://api.theoremsearch.com/search and MCP at https://api.theoremsearch.com/mcp. **No terms of use are published.**
- **Released data:**
  - https://huggingface.co/datasets/uw-math-ai/theorem-search-dataset is tagged `cc-by-sa-4.0`, with 1,341,083 theorems from 209,777 papers. That matches the spec's 1.34M. It mixes Stacks (GFDL), ProofWiki and other sources under one BY-SA tag, which is a licence inconsistency (D4). Its card says "~15% of arXiv papers use permissive licenses (CC BY, CC BY-SA, or CC0)".
  - https://huggingface.co/datasets/uw-math-ai/theorem-search-dataset-permissive is tagged `cc-by-4.0`. Its card says 1,239,720 theorems from 197,889 papers (arXiv 1,238,975, Open Logic 745), covering CC0, CC BY 4.0, CC BY 3.0 and PD. ShareAlike is excluded. The dataset viewer shows 2,677,439 rows, which does not match the card (D5). The "slogans" (natural-language summaries) were written by DeepSeek-V3.1.
- **What math2 can take:** the `-permissive` set as `open` candidate evidence (E1), keeping each row's per-paper licence. The BY-SA set should be used only after filtering out Stacks and any other GFDL or NC rows.

**TheoremGraph and LeanGraph** (https://arxiv.org/abs/2606.25363, 2026-06-24, cs.IR; the paper itself is under the arXiv non-exclusive licence)
- https://huggingface.co/datasets/uw-math-ai/math-graph:
  - The card says CC BY 4.0, with "per-record reuse governed by each statement's source license". It has 16,131,238 rows.
  - The graph structure is open. Statement text is filled in only for CC0/BY/BY-SA papers and is NULL otherwise. `paper_arxiv.csv` has a licence column.
- https://huggingface.co/datasets/uw-math-ai/theorem-matching has 101k rows, built from the Kaggle arXiv metadata.
- **What math2 can take:** the edges and ids (CC BY 4.0, so attribute them), and text only where it is non-NULL. This is the best-licensed of the three big extractions.

**Matlas** (https://arxiv.org/abs/2604.17484, 2026-04-19; the paper is CC BY 4.0)
- **Size:** 8.07M statements from 435K papers (1826–2025), 180 journals and 1.9K textbooks.
- **API:** https://matlas.ai/openapi.json describes `POST /api/search` (`num_results` 10–200). It returns the `statement` text with doi, title, authors, journal, year and type. There is also `/api/feedback`.
- No auth, no licence and no terms are published. **The licence of the corpus is UNVERIFIED and probably not open.** The statements are from journals and textbooks under copyright.
- **What math2 can take:** under spec 9.6, query-time matching only, storing only DOIs and ids (`record`).

**New 2026 projects not in spec 01**

| Project | Date | Licence | What it is |
|---|---|---|---|
| uw-math-ai/FormalArXiv (https://huggingface.co/datasets/uw-math-ai/FormalArXiv) | n/a | "other" | 89 Lean statements from August 2026 arXiv papers, via LemmaBench |
| Lean Pool (https://arxiv.org/abs/2609.25199, Ilin) | 2026-09-21 | paper CC BY 4.0 | an AI-maintained archive of formalised mathematics |
| AnnalsChallenge (https://arxiv.org/abs/2610.18301, Buzzard et al.) | 2026-10-13 | paper CC BY-SA 4.0 | 50 Lean statements of Annals results |
| Stacks↔Mathlib alignment (https://arxiv.org/abs/2610.31011; code https://github.com/AnkitSatpute/Formal-Informal-Mathematical-Alignment) | 2026-10-22 | paper CC BY 4.0 | added to MaRDI |
| KnowTeX (https://arxiv.org/abs/2601.15294) | n/a | n/a | n/a |
| uw-math-ai other artefacts (https://huggingface.co/uw-math-ai: MELD, Math2Vec, APRIL, MathLeap models; https://github.com/uw-math-ai/arXiTeX) | n/a | no licences on the org page | n/a |

---

## Part 2. Open textbooks

Licences were read at the book's own source (repository LICENSE, README or front matter) unless a row says "per OTL". In the Open Textbook Library (OTL), licences are sometimes stale; see §2.3.

### 2.1 Verified at source

| Book | Licence (exact) | Format | Repository / source URL | 9.3 class |
|---|---|---|---|---|
| Univalent Foundations, *Homotopy Type Theory* (HoTT book) | CC BY-SA 3.0 Unported | LaTeX | https://github.com/HoTT/book (README) | `open` |
| Open Logic Project | CC BY 4.0 | LaTeX | https://github.com/OpenLogicProject/OpenLogic | `open` |
| Hefferon, *Linear Algebra* | dual: GFDL **or** CC BY-SA 3.0 US, at the user's choice | LaTeX | https://hefferon.net/source.html, https://gitlab.com/jim.hefferon/linear-algebra | `open` (take it under BY-SA) |
| Lebl, *Basic Analysis* I/II | dual: CC BY-NC-SA 4.0 **and** CC BY-SA 4.0 | LaTeX | https://github.com/jirilebl/ra | `open` (take it under BY-SA) |
| Lebl, *Notes on Diffy Qs* | dual: CC BY-NC-SA 4.0 and CC BY-SA 4.0 | LaTeX | https://github.com/jirilebl/diffyqs | `open` (BY-SA) |
| Lebl, *Tasty Bits of Several Complex Variables* | dual: CC BY-NC-SA 4.0 and CC BY-SA 4.0 | LaTeX | https://github.com/jirilebl/scv | `open` (BY-SA) |
| Boelkins et al., *Active Calculus* (single variable, 2012–2025 edition) | CC BY-SA 4.0 | PreTeXt | https://github.com/active-calculus/active-calculus-single-mbx (`source/bibinfo.xml`) | `open` |
| Evan Chen, *An Infinitely Large Napkin* | text CC BY-SA 4.0; source code GPL v3 | LaTeX | https://github.com/vEnhance/napkin/ | `open` (text) |
| Levin, *Discrete Mathematics: An Open Introduction*, **3rd edition and earlier** | CC BY-SA 4.0 | PreTeXt | https://github.com/oscarlevin/discrete-book | `open` |
| Levin, same book, **4th edition** | CC BY-NC-SA 4.0 | PreTeXt | same | `share-alike-nc` |
| Keller and Trotter, *Applied Combinatorics* | CC BY-SA 4.0 | PreTeXt | https://github.com/mitchkeller/applied-combinatorics | `open` |
| Ernst, *An Introduction to Proof via Inquiry-Based Learning* | CC BY-SA 4.0 | LaTeX | https://github.com/dcernst/IBL-IntroToProof | `open` |
| Austin, *Understanding Linear Algebra* | CC BY 4.0 (licence link in README) | PreTeXt | https://github.com/davidaustinm/ula | `open` |
| Beck, Marchesi, Pixton, Sabalka, *A First Course in Complex Analysis* v1.6 | CC BY 4.0 | PDF (LaTeX not checked) | https://matthbeck.github.io/complex.html | `open` |
| Lehman, Leighton, Meyer, *Mathematics for Computer Science* (2018 rev.) | CC BY-SA 3.0 ("available under the terms of the Creative Commons Attribution-ShareAlike 3.0 license") | PDF | https://courses.csail.mit.edu/6.042/spring18/mcs.pdf | `open` |
| Judson, *Abstract Algebra: Theory and Applications* (AATA) | GFDL 1.3 or later | PreTeXt | https://github.com/twjudson/aata | `gfdl` |
| Beezer, *A First Course in Linear Algebra* (FCLA) | GFDL 1.2 or later | PreTeXt | https://github.com/rbeezer/fcla | `gfdl` |
| The Stacks Project | GFDL 1.2 | LaTeX | https://stacks.math.columbia.edu (COPYING) | `gfdl` |
| Hartman et al., *APEX Calculus* | CC BY-NC 4.0, © 2021 Hartman | PreTeXt | https://github.com/APEXCalculus/APEXCalculusPTX (`ptx/apex.ptx`) | `share-alike-nc` |
| Feldman, Rechnitzer, Yeager, *CLP Calculus* (UBC) | CC BY-NC-SA 4.0 ("It is licensed under a Creative Commons BY-NC-SA 4.0 International License") | LaTeX source published | https://personal.math.ubc.ca/~CLP/src/ | `share-alike-nc` |
| Axler, *Linear Algebra Done Right*, 4th edition | CC BY-NC 4.0 (Springer open access) | PDF; HTML via PreTeXt (David Farmer) | https://link.springer.com/book/10.1007/978-3-031-41026-0, https://library.oapen.org/handle/20.500.12657/85067, https://linear.axler.net/ (PDF dated 2026-08-16), https://ladr.pretext.plus/ | `share-alike-nc` |
| Hammack, *Book of Proof*, 3rd edition (v3.4, 2025-02-05) | CC BY-NC-ND 4.0 | PDF only | https://richardhammack.github.io/BookOfProof/ | `verbatim` |
| Hatcher, *Algebraic Topology* | not a CC licence: "Single paper or electronic copies for noncommercial use may be made freely without explicit permission from the author or publisher" (© 2002 CUP) | PDF | https://pi.math.cornell.edu/~hatcher/AT/ATpage.html | `record` |
| Lurie, *Kerodon* | **no licence statement found** anywhere on the site (about page: maintained by Jacob Lurie, built on Gerby). /statistics shows 8,189 tags | web (Gerby) | https://kerodon.net/ | `record` (all rights reserved by default). **This contradicts spec 9.3 and 16.6, which call it GFDL** (D6) |

### 2.2 OpenStax mathematics

Licences were read from each book's `collection.xml` `md:license` on GitHub (the openstax/osbooks-* repositories). openstax.org/subjects/math returned an empty page.

| Book (OpenStax repository) | Licence | 9.3 class |
|---|---|---|
| Calculus Vols 1–3, College Algebra 2e, Precalculus 2e, Algebra and Trigonometry 2e, Prealgebra 2e, Elementary Algebra 2e, Intermediate Algebra 2e, Introductory Statistics 2e, Introductory Business Statistics 2e, Contemporary Mathematics, Algebra 1, Principles of Data Science, *Cálculo* (Spanish) | CC BY-NC-SA 4.0 | `share-alike-nc` |
| osbooks-statistics (Statistics), *Precálculo* 2ed | CC BY 4.0 | `open` |

Earlier editions were CC BY: the OTL lists Prealgebra 2e (2015), Precalculus (2014) and Introductory Statistics 2e (2023) as BY. Earlier CC BY editions are still CC BY, and math2 could adapt those editions specifically (D7).

### 2.3 Aggregators and catalogues

**Open Textbook Library** (https://open.umn.edu/opentextbooks/subjects/mathematics.json)
- `total_count` is 214; 191 entries were retrieved. Licence counts:

| Licence | Books |
|---|---:|
| BY-NC-SA | 67 |
| BY | 51 |
| BY-SA | 38 |
| BY-NC | 17 |
| GFDL | 12 |
| BY-NC-ND | 3 |
| "No Rights Reserved" | 2 |
| BY-ND | 1 |

- So about 48% are `open` (BY, BY-SA, CC0) and 52% are not.
- The subject also contains some non-mathematics books (statistics, psychology).
- **OTL licences can be stale or simplified.** It lists *Active Calculus* 2.0 (2017) as BY-NC-SA, while the current edition is BY-SA. It lists *Book of Proof* as "Attribution-NoDerivs", while the author's page says BY-NC-ND. It lists Hefferon as BY-SA only, while the book is dual-licensed GFDL / BY-SA. **Verify at source before ingesting.**

**American Institute of Mathematics (AIM) Open Textbook Initiative, approved list** (https://textbooks.aimath.org/textbooks/approved-textbooks/)
- 68 unique titles grouped by course. **Licences are not shown on the list.** The verified licences for AIM titles are in §2.1, §2.5 and §2.6.
- AIM titles not otherwise covered in this note include Bruckner–Thomson, ORCCA, Manes, May–Bart, Marsden–Weinstein, Boyd–Vandenberghe, Treil, Fields, Bender–Williamson, Bard, Zimmermann, Crisman, Stein and Connell. Their licences were not checked (D11).

**PreTeXt catalogue** (https://pretextbook.org/catalog.html)
- 80 projects, 67 of them mathematics.
- **GFDL badges:** FCLA, Discover Linear Algebra, Fitch *Discrete Math*, Yoshiwara *Elementary Algebra*, LA Companion, LA Refresher, Yoshiwara *Modeling, Functions and Graphs*, Yoshiwara *Trigonometry*, AATA, AA Refresher, IBL Geometries, *Exploring Combinatorial Mathematics*, *First-Semester Abstract Algebra*, Hitchman *Geometry with an Introduction to Cosmic Topology*, *Models of Non-Euclidean Geometry*, *The ODE Project*, *Algebraic Graph Theory with Sage*, *Elementary Foundations*, *A Second Course in Linear Algebra*, *Abstract Algebra for Teachers*, Cryptology.
- **All rights reserved:** *Linear Algebra* (W214) and *Intro to Topology (IBL)*.
- The rest are badged only "Creative Commons", without saying which variant. Each needs a source check.

**LibreTexts** (https://math.libretexts.org/Bookshelves)
- The licence is set per page, through tags such as `license:ccbyncsa` and an auto-generated "Detailed Licensing" back-matter page. An example: https://math.libretexts.org/Bookshelves/Calculus/Calculus_(Guichard)/00%3A_Front_Matter/04%3A_Licensing.
- Many pages are "not declared". The licence mix inside a single book is common.
- The terms (https://libretexts.org/terms-conditions, effective 2020-10-15) allow copying of open-licensed content as its licence permits. They have no AI or scraping clause.
- There are no aggregate licence statistics, so the mix is **UNVERIFIED**.
- **For math2:** go to the upstream source of each LibreTexts book, not the LibreTexts copy. Use LibreTexts only page by page, by the page's `license:` tag. "Not declared" counts as `record`.

**MIT OpenCourseWare** (https://ocw.mit.edu/pages/privacy-and-terms-of-use/)
- Course materials are CC BY-NC-SA 4.0, so `share-alike-nc`.
- There is a **"Permitted Use of AI Training"** section: training is allowed with attribution, the models must be non-commercial, and derivative models must be released under BY-NC-SA.
- There is no bulk-download section.
- A note: MIT 6.042's *Mathematics for Computer Science* is distributed separately under CC BY-SA 3.0 (§2.1), so it is `open`, unlike the OCW course pages.

### 2.4 Books commonly assumed open that are not

| Book | Status |
|---|---|
| Hatcher, *Algebraic Topology* | noncommercial copying only; `record` |
| *Book of Proof* | ND, so `verbatim` |
| Axler LADR4 | NC |
| APEX | NC |
| CLP | NC-SA |
| OpenStax 2e math editions | NC-SA |
| Kerodon | no licence; `record` |

### 2.5 Table A: textbooks math2 may adapt into CC BY-SA 4.0 node prose (CC0, CC BY, CC BY-SA)

Attribution is required for all of them. CC BY-SA 3.0 (Unported or US) adaptations may be licensed under BY-SA 4.0: CC BY-SA 3.0 Unported §4(b) (https://creativecommons.org/licenses/by-sa/3.0/legalcode.en, read 2026-10-26) allows an adaptation to be distributed under "a later version of this License with the same License Elements". I did not re-read the 3.0 US port; I assume it has the same clause, which matters for Hefferon. Treat that as UNVERIFIED.

| Book | Licence math2 uses | Source format | Repository / URL | Notes |
|---|---|---|---|---|
| Open Logic Project | CC BY 4.0 | LaTeX | https://github.com/OpenLogicProject/OpenLogic | already in TheoremSearch (588 / 745 items) |
| HoTT book | CC BY-SA 3.0 → 4.0 | LaTeX | https://github.com/HoTT/book | |
| Hefferon, *Linear Algebra* | CC BY-SA 3.0 US option of the dual licence | LaTeX | https://gitlab.com/jim.hefferon/linear-algebra | record that math2 elected the BY-SA option |
| Lebl, *Basic Analysis* I/II | CC BY-SA 4.0 option of the dual licence | LaTeX | https://github.com/jirilebl/ra | elect BY-SA |
| Lebl, *Notes on Diffy Qs* | CC BY-SA 4.0 option | LaTeX | https://github.com/jirilebl/diffyqs | elect BY-SA |
| Lebl, *Tasty Bits of SCV* | CC BY-SA 4.0 option | LaTeX | https://github.com/jirilebl/scv | elect BY-SA |
| *Active Calculus* (single variable, 2012–2025) | CC BY-SA 4.0 | PreTeXt | https://github.com/active-calculus/active-calculus-single-mbx | the OTL's "BY-NC-SA" is for 2.0 (2017) and is stale |
| Napkin (text) | CC BY-SA 4.0 | LaTeX | https://github.com/vEnhance/napkin/ | do not take the GPL code |
| Levin, *Discrete Mathematics*, 3rd edition and earlier | CC BY-SA 4.0 | PreTeXt | https://github.com/oscarlevin/discrete-book | pin a pre-4th-edition commit or tag |
| Keller–Trotter, *Applied Combinatorics* | CC BY-SA 4.0 | PreTeXt | https://github.com/mitchkeller/applied-combinatorics | |
| Ernst, *Intro to Proof via IBL* | CC BY-SA 4.0 | LaTeX | https://github.com/dcernst/IBL-IntroToProof | |
| Austin, *Understanding Linear Algebra* | CC BY 4.0 | PreTeXt | https://github.com/davidaustinm/ula | |
| Beck et al., *First Course in Complex Analysis* | CC BY 4.0 | PDF | https://matthbeck.github.io/complex.html | |
| Lehman–Leighton–Meyer, *Mathematics for Computer Science* | CC BY-SA 3.0 → 4.0 | PDF | https://courses.csail.mit.edu/6.042/spring18/mcs.pdf | |
| OpenStax *Statistics*, *Precálculo* 2ed, and the earlier CC BY editions (Prealgebra 2015, Precalculus 2014) | CC BY 4.0 | CNXML | github.com/openstax (osbooks-statistics and others) | pin the CC BY edition |
| Per OTL, not verified at source: Zakon, *Mathematical Analysis I*; *Counting Rocks!*; *Calculus in Context*; Kuttler, *A First Course in Linear Algebra*; Shimamoto, *Multivariable Calculus*; Dumas–McCarthy; Wiggins, *ODE* | CC BY | various | https://open.umn.edu/opentextbooks/subjects/mathematics.json | verify at source (D8) |
| Per OTL, not verified at source: Kuttler, *Linear Algebra, Theory and Applications*; Hitchman (OTL says BY-SA, but the PreTeXt catalogue shows GFDL, D8); Petrunin, *Euclidean Plane and its Relatives*; Callahan, *Euclid's Elements Redux*; Davies, *A Cool Brisk Walk*; Brin, *Tea Time Numerical Analysis* and *Tea Time Linear Algebra*; Lippman, *Math in Society*; OpenIntro Statistics; *Introduction to Modern Statistics* 2e | CC BY-SA | various | same | verify at source |
| Public domain (US, pre-1931): e.g. Hardy, *A Course of Pure Mathematics* (PG #38769); Dedekind; Casey's Euclid; Thompson, *Calculus Made Easy* | public domain (US) | LaTeX / HTML | https://www.gutenberg.org/ebooks/bookshelf/102 | strip PG trademark text; check non-US status |

### 2.6 Table B: textbooks math2 may NOT adapt into node prose (NC, ND, GFDL, all rights reserved)

All of these can still give records, locators, tables of contents (9.7) and internal embeddings.

| Book | Licence | 9.3 class | URL |
|---|---|---|---|
| Judson, AATA | GFDL 1.3+ | `gfdl` | https://github.com/twjudson/aata |
| Beezer, FCLA | GFDL 1.2+ | `gfdl` | https://github.com/rbeezer/fcla |
| Stacks Project | GFDL 1.2 | `gfdl` | https://stacks.math.columbia.edu |
| The PreTeXt GFDL-badged titles (§2.3: Yoshiwara ×3, Fitch, ODE Project, etc.) | GFDL | `gfdl` | https://pretextbook.org/catalog.html |
| Per OTL: Grinstead–Snell, *Introduction to Probability*; Bogart, *Combinatorics Through Guided Discovery*; Corral, *Vector Calculus* and others; Collingwood et al., *Precalculus*; Yoshiwara, *Modeling* (12 GFDL in total) | GFDL | `gfdl` | https://open.umn.edu/opentextbooks/subjects/mathematics.json |
| Hartman et al., *APEX Calculus* | CC BY-NC 4.0 | `share-alike-nc` | https://github.com/APEXCalculus/APEXCalculusPTX |
| Axler, LADR 4e | CC BY-NC 4.0 | `share-alike-nc` | https://link.springer.com/book/10.1007/978-3-031-41026-0 |
| Per OTL: Axler, *Measure, Integration & Real Analysis*; Lafferriere et al. | CC BY-NC | `share-alike-nc` | OTL |
| *CLP Calculus* (UBC) | CC BY-NC-SA 4.0 | `share-alike-nc` | https://personal.math.ubc.ca/~CLP/src/ |
| Levin, *Discrete Mathematics*, 4th edition | CC BY-NC-SA 4.0 | `share-alike-nc` | https://github.com/oscarlevin/discrete-book |
| OpenStax Calculus 1–3, College Algebra 2e, Precalculus 2e, Algebra & Trig 2e, Prealgebra 2e, Elementary/Intermediate Algebra 2e, Intro Statistics 2e, Business Stats 2e, Contemporary Math, Algebra 1, Principles of Data Science, *Cálculo* | CC BY-NC-SA 4.0 | `share-alike-nc` | github.com/openstax |
| MIT OCW course materials | CC BY-NC-SA 4.0 (AI-training clause: non-commercial, BY-NC-SA models) | `share-alike-nc` | https://ocw.mit.edu/pages/privacy-and-terms-of-use/ |
| Per OTL: Guichard, *Calculus* (early transcendentals); Trench, *Introduction to Real Analysis* and *Elementary Differential Equations*; Sundstrom, *Mathematical Reasoning*; Doerr–Levasseur, *Applied Discrete Structures*; Rogers–Boman; Stitz–Zeager; Cherney et al., *Linear Algebra*; Nicholson, *Linear Algebra with Applications*; Morris, *Proofs and Concepts*; Sloughter; Hill–Thron; Strang, *Calculus* | CC BY-NC-SA | `share-alike-nc` | OTL |
| Hammack, *Book of Proof* 3e | CC BY-NC-ND 4.0 | `verbatim` | https://richardhammack.github.io/BookOfProof/ |
| Per OTL: Shoup, *A Computational Introduction to Number Theory and Algebra* | CC BY-NC-ND | `verbatim` | OTL |
| Hatcher, *Algebraic Topology* | © CUP 2002; noncommercial copies only | `record` | https://pi.math.cornell.edu/~hatcher/AT/ATpage.html |
| Lurie, *Kerodon* | no licence stated, so all rights reserved | `record` | https://kerodon.net/ |
| PreTeXt: *Linear Algebra* (W214), *Intro to Topology (IBL)* | all rights reserved | `record` | https://pretextbook.org/catalog.html |
| LibreTexts pages tagged "not declared" | none | `record` | https://math.libretexts.org |

---

## Part 3. Syllabi and theorem lists

### 3.1 MAA CUPM Curriculum Guide (2015)

- **Citation:** *2015 CUPM Curriculum Guide to Majors in the Mathematical Sciences*, Schumacher and Siegel (co-chairs), Zorn (editor). 116 pages. The PDF has CreationDate 2015-05-05.
- **Copyright:** "© 2015 The Mathematical Association of America (Incorporated)", ISBN 978-0-88385-917-9. I found no CC licence, so it is `record`.
- **URLs:**
  - Official page: https://maa.org/resource/cupm-guide/. It is JavaScript-rendered, so curl got no content.
  - Copy I read: https://www.amstat.org/docs/default-source/amstat-documents/cupmguide_print.pdf
  - The separate course-area reports (Abstract Algebra, Real Analysis, Topology, etc.) are linked from the MAA page. Their licences are not checked.
- **What math2 can take:** the structure (course areas, topic lists) as facts for `topics/curricula/`, and citations. Not the prose.
- Nothing newer than the 2015 guide was found (D12).

### 3.2 QAA Subject Benchmark Statement: Mathematics, Statistics and Operational Research

- **Current edition:** the **5th edition, published 2023-03-08** (the PDF's last page reads "Fifth Edition / Published - 8 March 2023"). It covers Master's level too.
- **Page:** https://www.qaa.ac.uk/the-quality-code/subject-benchmark-statements/subject-benchmark-statement-mathematics-statistics-and-operational-research
- **PDF:** https://www.qaa.ac.uk/docs/qaa/sbs/sbs-mathematics-statistics-and-operational-research-23.pdf, 2.1 MB, 35 pages.
- **Copyright:** "© The Quality Assurance Agency for Higher Education 2023". No CC or OGL licence is stated in the PDF, so it is `record`. (An older 2009 annex is in DERA under the Open Government Licence. That does not carry over to 2023.)
- **Upcoming:** the QAA site has a "Subject Benchmark Statement Reviews 2026-27" page, so a 6th edition may be in progress (D12).
- **What math2 can take:** its outcome categories, as facts.

### 3.3 Mathlib lists

Source: leanprover-community/mathlib4 `master` (repository HEAD 4c30f23495 at 2026-10-26T08:02Z). Licence Apache-2.0.

| File | Contents | Last change |
|---|---|---|
| `docs/undergrad.yaml` (https://raw.githubusercontent.com/leanprover-community/mathlib4/master/docs/undergrad.yaml) | 563 leaf items, 130 of them empty (not yet formalised). The topics were originally taken from the 2020 French *agrégation* programme | 58d1b21e88, 2026-10-07 |
| `docs/overview.yaml` | 511 leaves | n/a |
| `docs/1000.yaml` | 1,199 entries keyed by Wikidata id; 217 have `decl`(s), 30 have an external `url` | c6f03d20ca, 2026-10-17 |
| `docs/100.yaml` | 100 entries; 78 have `decl` | n/a |

**What math2 can take:** everything. These are Apache-2.0 code-zone data and facts, and the best machine-readable syllabus and theorem lists available. The Wikidata keys of `1000.yaml` are crosswalk ids.

### 3.4 Freek Wiedijk's 100 theorems

- **Page:** https://www.cs.ru.nl/~freek/100/, last modified 2026-09-17. All 100 theorems are formalised in at least one system.
- **Per system:**

| System | Theorems |
|---|---:|
| HOL Light | 95 |
| Isabelle | 95 |
| Lean | 83 |
| Rocq | 80 |
| Metamath | 74 |
| Mizar | 71 |
| ACL2 | 48 |
| ProofPower | 43 |
| PVS | 26 |
| Imandra | 20 |
| Megalodon | 12 |
| Naproche | 10 |
| NuPRL | 8 |

- **Licence:** no licence statement, so `record`. The list of theorem names is factual.
- **Discrepancy:** Mathlib's `100.yaml` has 78 entries with a `decl`, but Freek's page counts 83 for Lean. The difference is probably entries proved outside Mathlib, or stale yaml. Not resolved.

### 3.5 The 1000+ theorems project

- **Site:** https://1000-plus.github.io/. I counted 228 entries on the page.
- **Repository:** https://github.com/1000-plus-theorems/1000-plus, licence **Unlicense**, last pushed 2026-07-22. That makes it public-domain-equivalent, so `open`.
- **Relation to Mathlib:** Mathlib's `1000.yaml` is the Lean-side view of the same Wikidata-keyed list.

### 3.6 Department qualifying-exam lists

- **Example:** UC Berkeley Preliminary Examination (https://math.berkeley.edu/graduate/phd-program/preliminary-exam).
  - Two mornings of 3 hours each. Students attempt 5 of 8 questions per day (9 before Fall 2024).
  - Topics: real analysis and calculus, complex analysis, linear algebra, abstract algebra.
  - The syllabus PDF and past exams are linked at the bottom of the page.
- These are department web pages with no licence, so `record`. Topic lists are factual. Past exam problems are copyrighted text.
- *Berkeley Problems in Mathematics* (de Souza and Silva, Springer) collects 1,250 prelim problems from 1977 to 2003. It is all rights reserved.
- I did not survey other departments; other examples include UW (https://math.washington.edu/phd-preliminary-exams). **Breadth is UNVERIFIED** (D12).

---

## Summary table

| Item | Size (date) | Licence of text / data | Format | Bulk channel and terms | AI terms | What math2 can take (9.3) |
|---|---|---|---|---|---|---|
| arXiv | about 9.2 TB of files (April 2025), +100 GB/month | metadata CC0; text per paper. 2026 math sample is 36.9% `open` (BY/BY-SA/CC0) vs 15% all-time | TeX source, PDF | `s3://arxiv` requester-pays; API 1 req / 3 s; must link back; redistributing e-prints needs the holder's permission | none stated | records (CC0); text of `open` papers into prose; others `record` |
| zbMATH Open | 4M+ publications, 3M+ reviews, 168M+ triples (KG v1, 2026-09-01); 133,093 documents for 2025 | KG: Zenodo field CC BY 4.0 vs description CC BY-SA 4.0; I4OC references CC0 | RDF/TTL, JSON API, OAI-PMH | Zenodo dump (1.6 GB, twice a year); REST; OAI-PMH; API terms UNVERIFIED | UNVERIFIED | records, MSC, reviews as `open` evidence |
| OpenAlex | about 745 GB JSONL (September 2026) | CC0 | JSONL, Parquet | `s3://openalex` anonymous, quarterly; API key since 2026-02 (secondary source) | none | all metadata |
| Unpaywall | n/a | n/a | n/a | folded into OpenAlex; data feed paid; terms UNVERIFIED | n/a | OA locations via OpenAlex |
| Crossref | 180,034,490 records, 208 GB (2026-03-14) | metadata effectively free; some abstracts copyrighted | JSONL | public data file (torrent, AWS); open REST API | none | metadata, references; abstracts `record` |
| DataCite | 108M DOIs (2025 file, 2026-01-06) | CC0 | JSON | annual public file; monthly file members-only | none | all metadata |
| Semantic Scholar / S2ORC | S2ORC 15,679,781 full texts (2026-05-26) | API non-transferable; S2ORC ODC-By (database) plus per-paper copyright; abstracts UNVERIFIED | JSON | API key; Datasets API | none | ids; text only per paper licence |
| CORE | 291M metadata / 32.8M full texts (2023); current size UNVERIFIED | dumps ODC-By (database only) | JSON | API licence; dumps | proposed "lawful TDM" | records; text per paper |
| HAL (math) | 170,406 records, 105,413 with file (2026-10-26) | metadata CC0; files: 9.8% `open`, 80% hal-authorisation | PDF, TEI | OAI-PMH, REST, SPARQL | none found | records; the 10,349 `open` texts |
| Numdam | 75,599 articles, 706 books, 1.43M pages | metadata CC0; text: personal use, no bulk, no modification | PDF, OAI | OAI-PMH (metadata only) | none | records; text only where CC or PD |
| EuDML | over 270k documents (undated) | UNVERIFIED | n/a | blocked (403) | n/a | records via partner sources |
| Project Euclid | "over 80%" free to read | per journal, UNVERIFIED | PDF | none found | n/a | records; per-article licence |
| GDZ | over 15M pages | scans' licence UNVERIFIED; PDFs: no re-hosting without permission | IIIF, PDF | IIIF, OAI | n/a | records; public-domain statements rewritten |
| HathiTrust | n/a | public-domain subsets; terms UNVERIFIED (Cloudflare) | text | `ht_text_pd*` datasets | UNVERIFIED | US pre-1931 public-domain text |
| Internet Archive | 206,597 math texts; 8,076 dated 1500–1930 | mixed; hosting does not establish licence | PDF, OCR | advancedsearch; terms UNVERIFIED | UNVERIFIED | public-domain items after a per-item check |
| Project Gutenberg | about 78,000 items (2026-03) | public domain (US) | TXT, HTML, some LaTeX | mirrors or `/robot/harvest`, `-w 2` | none | `open` (strip PG trademark) |
| TheoremSearch | 8.4M indexed (live site); 1,341,083 BY-SA release; 1,239,720 permissive release | HF tags BY-SA / BY; BY-SA set includes GFDL Stacks | Parquet, REST, MCP | Hugging Face; API with no terms | none | `-permissive` set as E1 candidates |
| TheoremGraph / LeanGraph | 16,131,238 rows (math-graph) | CC BY 4.0 plus per-record source licence; text NULL unless open | CSV, Parquet | Hugging Face | none | edges, ids, open-only text |
| Matlas | 8.07M statements, 435K papers, 1.9K textbooks | not stated (UNVERIFIED; likely closed) | JSON API | `POST /api/search`, no terms | none | query-time ids only (`record`) |
| Open textbooks | 214 math books in OTL; 68 AIM-approved; 67 PreTeXt math | OTL: 48% `open`, 52% NC/ND/GFDL | LaTeX, PreTeXt, PDF | Git repositories | MIT OCW: NC models only | Table A only |
| MIT OCW | n/a | CC BY-NC-SA 4.0 | HTML, PDF | none | training allowed, NC models, BY-NC-SA | `share-alike-nc` |
| CUPM 2015 | 116 pages | © MAA | PDF | n/a | n/a | structure as facts |
| QAA SBS MSOR (5th edition, 2023-03-08) | 35 pages | © QAA 2023 | PDF | n/a | n/a | structure as facts |
| Mathlib yaml | undergrad 563 (130 empty); 1000.yaml 1,199; 100.yaml 100 | Apache-2.0 | YAML | Git | n/a | all |
| Freek's 100 | 100 (2026-09-17) | no licence | HTML | n/a | n/a | list as facts |
| 1000+ theorems | 228 entries on the site | Unlicense | Markdown, YAML | Git | n/a | all |

---

## Open doubts

1. **D1. zbMATH KG licence.** The Zenodo field says CC BY 4.0, but the record description says CC BY-SA 4.0. Ask zbMATH (or open a GitHub issue on zbMATHOpen/zbmath-open-kg) which one governs. Both are `open`, but BY-SA binds derived exports. The API terms are unread (Cloudflare).
2. **D2. arXiv `open` share.** The spec's "about 15%" is the all-time stock. My three-day 2026 math sample gives 36.9% (and about 39% for re-versioned older papers). Confirm this with a full 2026 `set=math` harvest from the `tamnd/arxiv` metadata plane. Separate first versions from updates, and decide whether cross-lists count. If it holds, spec 9.3 should state both figures.
3. **D3. Missing AI clauses.** arXiv, Crossref, OpenAlex, HAL and Gutenberg say nothing about AI. MIT OCW has an explicit NC-only AI clause. Stack Exchange forbids LLM use (already in the spec). The zbMATH, HathiTrust, IA and EuDML terms are unread. Silence in the terms is not permission: the per-item licence still governs.
4. **D4. TheoremSearch BY-SA set** puts Stacks (GFDL), ProofWiki and other sources under one `cc-by-sa-4.0` tag. math2 must filter by source and licence before treating any row as `open`.
5. **D5. TheoremSearch counts.** The card says 1,239,720 rows for the permissive set; the dataset viewer shows 2,677,439 rows. The BY-SA set has 1,341,083. The live site shows 8.4M against the paper's 9.2M. Not reconciled. The difference may be duplicated rows across splits, or slogans stored as separate rows.
6. **D6. Kerodon licence. Spec 9.3 and 16.6 list Kerodon as GFDL; I found no licence statement anywhere on kerodon.net.** Unless Lurie states a licence, Kerodon is all rights reserved, so `record`. The spec should change it to `record`, and §1.7's "[U]" should be resolved accordingly. The tag count on /statistics is 8,189; §1.7 says "2,200+ pages".
7. **D7. OpenStax edition drift.** The current 2e repositories are CC BY-NC-SA 4.0, but older editions were CC BY. Adapting an old CC BY edition is legal, but the edition and commit must be pinned.
8. **D8. OTL licences are stale or simplified** (*Active Calculus*, *Book of Proof*, Hefferon, Hitchman). Every Table A row marked "per OTL" must be verified at the source before ingestion.
9. **D9. Dual-licensed books** (Lebl ×3, Hefferon). math2 must record its election of the BY-SA option in the source entry.
10. **D10. Public domain is territorial.** The US pre-1931 rule does not make a work public domain elsewhere (life plus 70 years in the EU). Since math2 is published worldwide, decide whether US public domain is enough for `open`. For Hardy (died 1947) the text is public domain in the EU too; for some authors it is not.
11. **D11. AIM titles with unchecked licences** (Bruckner–Thomson, ORCCA, Manes, May–Bart, Marsden–Weinstein, Boyd–Vandenberghe, Treil, Fields, Bender–Williamson, Bard, Zimmermann et al., Crisman, Stein, Connell).
12. **D12. Syllabi.**
    - CUPM 2015 is the latest guide I found; I did not confirm whether a newer one exists.
    - The QAA 6th-edition review (2026–27) has not been checked.
    - Qualifying-exam lists were sampled at one department only.
    - The difference between Mathlib's `100.yaml` (78 with `decl`) and Freek's Lean count (83) is not resolved.
13. **D13. UNVERIFIED because of blocking or missing pages:**
    - zbMATH API terms
    - EuDML metadata licence and size
    - HathiTrust terms
    - Internet Archive terms
    - current CORE size and dump terms
    - Unpaywall terms
    - Semantic Scholar abstracts licence
    - Project Euclid per-journal licences
    - GDZ scan licence
    - OpenAlex API pricing (secondary source only)
14. **D14. Matlas** returns full statement text from copyrighted journals and textbooks through an open API with no terms. math2 should store only ids from it (spec 9.6). Its corpus licence is unknown.
15. **D15. LibreTexts** has no aggregate licence data. The per-page "not declared" share is unknown; treat it as `record` and prefer the upstream source.
