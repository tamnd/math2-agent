# K01, part A. Encyclopedic, reference and database sources of mathematics written by people

Research for the math2 spec. This file extends `spec/01-research-2026.md` §1.7, `spec/09-literature-ingestion.md` §9.2 and §9.3, and `spec/16-collaboration-and-governance.md` §16.6. It does not repeat what those sections already say, except where a figure has changed or a statement in the spec is wrong.

- **Read date.** Every fact below was read on **2026-10-26**, unless another date is given. Each fact has its URL.
- **Fetches.** These were polite, single-page fetches. Where a fetch tool was used, the User-Agent was `m2/research (+https://github.com/tamnd/math2-agent)`.
- **Own counts.** Some counts are mine, not the source's. They are marked **[measured]**. They come from one of these:
  - a Wikidata SPARQL query (WDQS, 2026-10-26 08:25 to 08:47 UTC);
  - a MediaWiki `siteinfo` API call;
  - a tree-only listing of a git repository (`git clone --filter=blob:none --no-checkout`, then `git ls-tree`; no file contents were downloaded).
- **No dumps.** No dump was downloaded. Dump sizes come from HEAD requests or directory listings.
- **UNVERIFIED** marks a fact that I could not confirm. The reason is given each time.
- **Licence gate.** "What math2 can take" uses the classes in spec 9.3: `open`, `share-alike-nc`, `verbatim`, `gfdl` and `record`. The three practical outcomes are:
  - **text with attribution**: `open`, so text can go into node prose in the BY-SA zone;
  - **link only**: `gfdl`, or no grant of rights; math2 stores the locator and writes its own statement;
  - **record only**: `record`; metadata, identifiers and facts, but no text.

  Facts, identifiers and numerical data are not copyrightable, so math2 can take them as CC0 metadata (16.6) from any source. The one exception is database rights in the EU sui generis sense. Rows below say so where a source asserts this.

---

## A.1 Wikipedia (mathematics)

**What it holds and its size**

English Wikipedia:
- The WikiProject Mathematics assessment table counts **33,957 pages** in the project. 33,913 are assessed and 44 unassessed.
  https://en.wikipedia.org/wiki/Wikipedia:WikiProject_Mathematics/Wikipedia_1.0/Assessment
  - The page gives no date for the counts. Its last edit was 28 Sep 2026 14:10 UTC.
- The table includes non-article pages: Category 2,302, Redirect 1,658, Template 1,012, Draft 172, and 503 others.
- The article classes are FA 32, FL 3, GA 231, B 1,446, C 4,459, Start 13,847, Stub 7,741 and List 437. They sum to **28,196 article pages** [measured from the table].
  - This replaces the "about 26k (estimate)" in spec 1.7.
- By importance: Top 208, High 955, Mid 4,363, Low 15,324.

Other languages. Each wiki counts in its own way, so the figures are not comparable:
- **German**: Portal:Mathematik says it is the central page for "20.732 mathematischen Artikeln". The figure is undated; the portal was last edited 7 May 2026. https://de.wikipedia.org/wiki/Portal:Mathematik
- **French**: Projet:Mathématiques has 12,192 project articles, of which 6,825 are evaluated (56%). It was "mis à jour 25 octobre 2026". https://fr.wikipedia.org/wiki/Projet:Mathématiques/Évaluation
- **Japanese**: Portal:数学 shows 8,865 articles. The page was last updated 1 Apr 2026, and the count is undated. https://ja.wikipedia.org/wiki/Portal:数学
- **Russian**: Проект:Математика lists 3,928 articles. This is the *project-tagged* set, not all mathematics articles. The figure is undated and "обновляется автоматически". https://ru.wikipedia.org/wiki/Проект:Математика
- **Chinese**: Wikipedia:数学专题 says "数学专题目前已有5,682篇条目". Its assessment tables sum to about 6,033. https://zh.wikipedia.org/wiki/Wikipedia:数学专题
  - The timestamp on the statistics equals the fetch time, so it is probably a live magic word, not a date of the count.

**Licence of the text**
- CC BY-SA 4.0. Text is also under the unversioned GFDL "unless otherwise noted". A page that includes text available *only* under CC BY-SA is not available under the GFDL. https://en.wikipedia.org/wiki/Wikipedia:Copyrights
- The dumps page says the same. It also says Wikidata structured data is CC0. https://dumps.wikimedia.org/legal.html

**Bulk channels and their terms**
- **XML dumps.**
  - In 2026 the `enwiki` dump directory has runs only on the 1st of each month: 20260401 to 20261001. https://dumps.wikimedia.org/enwiki/
    - Wikimedia used to run twice a month. Whether that changed formally is UNVERIFIED: I saw only the listing, not an announcement.
  - `enwiki-latest-pages-articles-multistream.xml.bz2` is 26,917,225,724 bytes, dated 08 Oct 2026. https://dumps.wikimedia.org/enwiki/latest/
- **HTML ("enterprise_html") dumps** on dumps.wikimedia.org. The last run listed is **20250320**. https://dumps.wikimedia.org/other/enterprise_html/runs/
  - The free public HTML snapshots look stopped, or moved to Wikimedia Enterprise. The reason is UNVERIFIED.
- **Wikimedia Enterprise.** https://enterprise.wikimedia.com/pricing/
  - Free account: Snapshot API with monthly updates, "up to 30/mo" requests and "up to 1,500/mo" snapshot chunks; On-demand API up to 50,000 requests a month.
  - Paid account: daily snapshots, the Realtime API, and bespoke pricing.
  - "Every response includes machine-readable license information."
- **API rate limits.** These are new in 2026 and "subject to experimentation and change". https://www.mediawiki.org/wiki/Wikimedia_APIs/Rate_limits (revision 8654799, edited 15 Oct 2026)
  - The limits are per minute, per user:

    | Client | Limit |
    |---|---|
    | unidentified (IP only) | **10** |
    | compliant User-Agent only | 200 |
    | authenticated, new user | 200 |
    | established editor | 2,000 |
    | bot flag, WMCS or known client | exempt |

  - Rollout: low limits from early March 2026, identified tiers from early April 2026. https://www.mail-archive.com/wikitech-l@lists.wikimedia.org/msg97202.html (2 Mar 2026; summary from search, not read in full)
- **Robot policy.** This rewrites the 2009 policy; the page was last edited 16 Mar 2026. https://wikitech.wikimedia.org/wiki/Robot_policy
  - Crawl only `/wiki/Article_name`, with fewer than 10 concurrent requests and fewer than 20 requests per second on average.
  - REST API: unauthenticated, concurrency 3 and fewer than 5 requests per second; authenticated, up to 10 per second.
  - Action API: unauthenticated, concurrency 1 and fewer than 5 requests per second.
  - "Consider dumps before making live requests."
  - There is no AI-specific clause.

**AI terms and 2024–2026 changes**
- **10 Nov 2025.** The Wikimedia Foundation asked AI developers to attribute the human contributors and to use Wikimedia Enterprise instead of scraping. It made no legal threat. https://wikimediafoundation.org/news/2025/11/10/in-the-ai-era-wikipedia-has-never-been-more-valuable/ (the post itself was not fetched; content from https://techcrunch.com/2025/11/10/wikipedia-urges-ai-companies-to-use-its-paid-api-and-stop-scraping/)
- **15 Jan 2026.** Enterprise announced Amazon, Meta, Microsoft, Mistral AI and Perplexity as partners, alongside Google and others. https://enterprise.wikimedia.com/blog/wikipedia-25-enterprise-partners/ (from a search summary)
- **No licence change.** The text stays CC BY-SA 4.0. What these deals buy is volume and an SLA, not rights.
- **Rogue OpenAI agents.** On 5 to 6 Oct 2026 the Foundation reported that rogue OpenAI agents made millions of API requests and that this may have contributed to the WDQS outage of 7 to 11 May 2026. https://www.theregister.com/ai-and-ml/2026/10/06/wikimedia-foundation-comes-forward-as-latest-openai-agent-assault-victim/5301400 ; https://thehackernews.com/2026/10/wikimedia-says-openai-agents-tried-to.html (from search summaries)
  - This is a likely driver of the stricter limits.
  - Wikipedia's own article on the Foundation states the dates in a confusing way.

**What math2 can take:** `open`.
- Short text with attribution can go into node prose, because CC BY-SA 4.0 equals the node prose zone. Each item needs the page and revision ID, and attribution through the history URL.
- Get articles from the monthly dump or the free Enterprise tier, never by crawling.
- An imported page may contain CC BY-SA-only text. That text cannot be used under the GFDL, but math2 does not need the GFDL.

---

## A.2 Wikidata

**What it holds** [measured, WDQS, 2026-10-26 ~08:25 UTC]
- **Defining formula (P2534): 113,118 items.** But **105,345 of these are instances of *prime number* (Q49008)**. Megaprimes, gigantic primes and Mersenne numbers add more.
  - Only **6,560** items with P2534 are *not* primes or prime-like. That is the real pool of mathematical formulas.
  - Only **5,492** P2534 items have an English Wikipedia article.
  - The top non-prime classes are formula 865, mathematical concept 458, theorem 436, function 95 and physical law 74.
  - **This changes the meaning of the "about 113k items with a defining formula" figure in spec 1.7 and 9.2.**
- **Theorem.** Instances of theorem (Q65943), counting subclasses: **3,208**. This matches spec 1.7.
- Crosswalk properties, as `P-id`: label, number of items:

  | Property | Label | Items |
  |---|---|---|
  | P2812 | MathWorld ID | 6,840 |
  | P4215 | nLab ID | 4,348 |
  | P8427 | KnotInfo ID | 2,978 |
  | P6781 | ProofWiki ID | 2,562 |
  | P8426 | Knot Atlas ID | 2,228 |
  | P7554 | Encyclopedia of Mathematics article ID | 1,224 |
  | P7726 | PlanetMath ID | 951 |
  | P12975 | Lexikon der Mathematik entry ID | 603 |
  | P829 | OEIS ID | 469 |
  | P12888 | Metamath statement ID | 438 |
  | P3285 | Mathematics Subject Classification ID | 408 |
  | P12934 | The House of Graphs ID | 213 |
  | P11497 | Digital Library of Mathematical Functions ID | 199 |
  | P14534 | Mathlib Declaration ID | 115 |
  | P12987 | LMFDB knowl ID | 109 |
  | P5610 | OpenMath ID | 54 |
  | P14106 | Erdős Problem number | 3 |

  Other relevant properties:

  | Property | Label | Items |
  |---|---|---|
  | P7263 | Prime Pages ID | 126,184 (the source of the prime items) |
  | P4252 | All-Russian Mathematical Portal ID | 5,090 |
  | P1482 | Stack Exchange tag | 7,215 (all sites) |
  | P894 | zbMATH Open document ID | 1,888 |
  | P889 | Mathematical Reviews ID | 141 |
  | P6830 | swMATH work ID | 125 |

  The query that found these matched property labels, so a property with an unusual label may be missing.
- **Gaps.** There is no Wikidata property for the Stacks Project, Kerodon, groupprops, Topospaces, π-Base, ISGCI, DaRT, FindStat, Fungrim or the Wolfram Functions Site. My label search found none.
  - P14106 (Erdős Problem number) is used on only **3** items, against 1,221 problems.

**Licence:** structured data CC0. Unstructured content, such as talk pages, is CC BY-SA 4.0. https://dumps.wikimedia.org/legal.html

**Bulk channels and their terms**
- **JSON dump.** `latest-all.json.bz2` is 103,488,697,311 bytes, dated 21 Oct 2026. There are also `.nt` and `.ttl` dumps. https://dumps.wikimedia.org/wikidatawiki/entities/
- **WDQS.** https://www.mediawiki.org/wiki/Wikidata_Query_Service/User_Manual
  - The hard timeout is 60 s.
  - One client, defined as User-Agent plus IP, gets 60 s of processing time per 60 s.
  - The limit is 5 parallel queries per IP and 30 error queries per minute. Clients over a limit get HTTP 429.
  - The manual says WDQS is "migrating to a new backend in 2026–2027" with new v2 endpoint URLs.
  - The details of the scholarly graph split were UNVERIFIED: the manual page does not describe them.
- The general API rate limits (A.1) apply.

**AI terms:** none specific; CC0. See A.1 for the May 2026 WDQS outage.

**What math2 can take:** `open`, CC0. All of it can go into the metadata zone.
- Use the weekly dump, not WDQS, for anything bulk.
- Treat P2534 as a source of formulas only after removing the prime-number items.

---

## A.3 nLab

**What it holds and its size**
- **20,778 pages** [measured]: the count of `content.md` files in `ncatlab/nlab-content` at the last commit, 2026-10-25T23:35:34Z ("Revision ID: 236426 … Page name: category theory").
  - The tree is not truncated: 73,134 entries in all. https://api.github.com/repos/ncatlab/nlab-content/git/trees/master?recursive=1
  - The repository has 208,017 commits. Each wiki edit is mirrored as a commit, in Markdown plus itex2MML. https://github.com/ncatlab/nlab-content
  - This replaces "about 19,193 entries (~2024)" in spec 1.7.

**Licence:** **none.**
- The HomePage says: "Using and distributing content obtained from the nLab is free and encouraged if you acknowledge the source" and "There is currently no consensus on a more formal license statement". It suggests checking the nLab homepages of individual contributors. https://ncatlab.org/nlab/show/HomePage
- The GitHub repository has no licence file (`license: null`). https://api.github.com/repos/ncatlab/nlab-content
- The history of the question is in a 2013 nForum thread, where the CC link was removed from the HomePage. https://nforum.ncatlab.org/discussion/3545/
  - In that thread Toby Bartels says a licence cannot be retroactive, and that copying nLab text into Wikipedia would be illegal even with a licence statement.
- There was no change in 2024 to 2026 that I could find.

**Bulk channel:** the git mirror, updated within minutes of each edit. No API terms are stated.

**AI terms:** none stated.

**What math2 can take:** **link only**, with records and identifiers.
- The "free … if you acknowledge the source" permission covers using and distributing the text. It grants no clear right to adapt and relicense under CC BY-SA, and it has not been agreed by the many authors. So the gate's `record`/link-only class still applies.
- Page names and the link graph are facts, so math2 can take them as CC0 metadata.
- To lift the restriction, math2 would need an explicit statement from the nLab steering committee. Even then, it could only cover authors who agree.

---

## A.4 ProofWiki

**What it holds and its size**
- Special:Statistics: **47,254 content pages**, 128,658 pages in all, 821,010 edits, 5,412 registered users, **12 active users** in the last 30 days. https://proofwiki.org/wiki/Special:Statistics
- The Main Page reports "30,348 Proofs" and "36,102 Definitions". https://proofwiki.org/wiki/Main_Page
  - These equal the figures in spec 1.7. Whether the Main Page counters are live is UNVERIFIED.

**Licence:** **CC BY-SA 3.0 and the GFDL** for all content except talk pages and user pages: "All content, except for that on Discusson pages and user home pages, is subject to the CC-by-SA 3.0 licence" and the GNU Free Documentation Licence. Talk pages are exempt from both. https://proofwiki.org/wiki/ProofWiki:Copyrights

**Bulk channel:** **a daily XML dump is live.**
- `https://proofwiki.org/xmldump/latest.xml.gz` returned HTTP 200, `content-length: 36599568` (about 36.6 MB gzipped), `last-modified: Mon, 26 Oct 2026 04:48:09 GMT`. This was a HEAD request only.
- The old URL `latest.xml` returns 301.
- Historical copies exist on the Internet Archive (WikiTeam 2017). https://archive.org/details/proofwikiorg_w-20170808-wikidump

**AI terms:** none stated. NaturalProofs (2021) and MathPile (2023) built training sets from the dump. https://arxiv.org/abs/2104.01112 ; https://arxiv.org/pdf/2312.17120

**What math2 can take:** `open`. Proofs and definitions can go into node prose with attribution.
- CC BY-SA 3.0 adaptations may be licensed under BY-SA 4.0, as spec 16.6 already says.
- Use the daily dump, not page crawls.
- Record the page title and revision ID for each item.

---

## A.5 PlanetMath

**What it holds and its size**
- **9,477 `.tex` entries** [measured]: tree-only clones of all 64 repositories in the `planetmath` GitHub organisation. Each repository is one top-level MSC class, plus `fem2016`. https://github.com/planetmath
  - For example: 03 Mathematical logic 906, 11 Number theory 1,071, 54 General topology 483.
- **The project is inactive.** The most recent push to any repository was 2024-02-19 (`16_Associative_rings_and_algebras`), and most last changed in March 2018.
  - The `planetmath.org` server's front page has `Last-Modified: Sat, 10 Mar 2018`.
  - Wikipedia's infobox says "Inactive". https://en.wikipedia.org/wiki/PlanetMath

**Licence:** **CC BY-SA 3.0 Unported**. The `LICENSE` file in each repository is the CC BY-SA 3.0 legal code. https://raw.githubusercontent.com/planetmath/11_Number_theory/master/LICENSE

**Bulk channel:** git. No API.

**AI terms:** none.

**What math2 can take:** `open`. Text can go in with attribution. Record the repository and the file path.

---

## A.6 Encyclopedia of Mathematics (encyclopediaofmath.org) and the Springer *Encyclopaedia of Mathematics*

**History**
- The original is the Soviet *Matematicheskaya entsiklopediya* (1977, ed. I. M. Vinogradov).
- The English edition, edited by Michiel Hazewinkel, came out from Kluwer in 10 volumes (1987–1994), with Supplements I to III (1997, 2000, 2002) and a CD-ROM (1998).
- Kluwer became part of Springer in 2003. "The 2002 version contains more than 8,000 entries".
- The free static online version redirected to the wiki from 29 Nov 2011. The wiki was "a collaboration between Springer and the European Mathematical Society". https://en.wikipedia.org/wiki/Encyclopedia_of_Mathematics
- An undated notice on the wiki's main page says the EoM "has moved from Springer Verlag to EMS Press". From search snippets, the snapshot is about mid-2020. https://encyclopediaofmath.org/wiki/Main_Page

**Licence: mixed, and mostly NOT open.**
- The main page says: "The original articles from the Encyclopaedia of Mathematics remain copyrighted to Springer but any new articles added and any changes made to existing articles within encyclopediaofmath.org will come under the Creative Commons Attribution Share-Alike License".
- The License Notice says new text is co-licensed under CC BY-SA 3.0 Unported and the GFDL. https://encyclopediaofmath.org/wiki/Encyclopedia_of_Mathematics:License_Notice
- Both statements were read through search-engine snippets. **The live site returned HTTP 502** on every attempt on 2026-10-26, between about 08:30 and 08:45 UTC, so the current wording is **UNVERIFIED**.
- Whether the move to EMS Press transferred or relicensed Springer's copyright is **UNVERIFIED**. I found no statement either way.
- A Creative Commons wiki case page from 2010 lists the Springer EoM as "copyright None" and "Open or Free Statement: no". https://wiki.creativecommons.org/wiki/Springer_-_Springer_Link_-_Encyclopaedia_of_Mathematics
- An EMS Magazine article (2025) by FIZ Karlsruhe staff says EoM runs the same wiki software as Wikipedia "albeit under a different license and editorial policies". zbMATH Open entity linking now links to EoM, nLab, MathWorld and Wikipedia. https://euromathsoc.org/magazine/articles/252 (from search summary)

**Size:** UNVERIFIED (the site was down). Wikidata has 1,224 items with an EoM ID (P7554) [measured].

**Bulk channel:** none known. There is a MediaWiki API (`api.php`); it was also 502 on the read date. No dump.

**AI terms:** none found.

**What math2 can take:** **record only, by default.**
- Most articles are derived from Springer-copyrighted originals. An edited article is a derivative of that text, so its BY-SA grant covers only the changes.
- Only an article that the page history shows to be wholly new on the wiki is `open`.
- Spec 1.7 and 9.2 list EoM as "CC BY-SA 3.0". **That is wrong for most of the text and should change.**

---

## A.7 Wolfram MathWorld

**Size:** **16,939 entries**, "Last Updated: Sun Oct 25 2026". https://mathworld.wolfram.com/ (Spec 1.7 says 16,849.)

**Licence and terms:** proprietary. https://www.wolfram.com/legal/terms/mathworld.html
- "these pages may not be copied, mirrored, redistributed, printed, or reproduced in bulk without permission."
- "Reproduction for commercial purposes is not permitted, nor is use of robots to create archival copies."
- Prefetching "may violate the above proscription against bulk downloading."
- "Limited use of material for academic research and teaching is permitted, providing conventional citation standards are followed" (quoted from the search snippet of the same page).
- The terms say nothing about AI.
- `robots.txt` says "Bulk downloading of these pages is not permitted for the purposes of making archival copies" and disallows only `/random.html`. It links a sitemap. https://mathworld.wolfram.com/robots.txt

**Bulk channel:** none. Wikidata P2812 holds 6,840 MathWorld IDs [measured], so the crosswalk does not need the site.

**What math2 can take:** **record only**: entry IDs from Wikidata, plus links. This confirms the spec.

---

## A.8 OEIS

**Size:** "Contains **400709** sequences". https://oeis.org/ (Spec 1.7 has 400,673 on 24 Oct.)

**Licence:** CC BY-SA 4.0. https://oeis.org/wiki/The_OEIS_End-User_License_Agreement (revision approved 24 Feb 2023)
- Attribution "should credit The Online Encyclopedia of Integer Sequences and provide a URL to the main page https://oeis.org/ or to a specific sequence".
- The EULA "may change … from time to time, without advance notice".

**Bulk channels**
- The download options are `stripped.gz`, `names.gz` and the git repository `oeis/oeisdata`, which has "incremental updates". https://oeis.org/wiki/Download
- `oeisdata` was last pushed 2026-10-26T07:03:53Z (commit "oeis data export"). GitHub reports its licence as "Other". https://api.github.com/repos/oeis/oeisdata
- `robots.txt` has `Crawl-Delay: 10`, disallows `/search`, `/edit` and `/history`, and fully blocks one named bot ("ShapBot"). https://oeis.org/robots.txt

**AI terms:**
- None in the EULA or the Download page.
- The Terms of Use text contains no clause on robots, automation, scraping, bulk use, machine learning or crawling. I checked by keyword search over https://oeis.org/OEISTermsOfUse.odt.txt.

**What math2 can take:** `open`, with text and attribution. This confirms the spec. Use `oeisdata` only.

---

## A.9 DLMF (NIST Digital Library of Mathematical Functions)

**Version:** 1.2.8, released 2026-09-15. https://dlmf.nist.gov/about/notices

**Licence:** **NIST-owned copyright; it is not public domain.**
- "All materials on this website are owned by NIST". NIST holds copyrights assigned by the DLMF authors.
- "Limited copying and internal distribution of the content of these pages is permitted for research and teaching."
- "Reproduction, copying, or distribution for any commercial purpose is strictly prohibited."
- "Bulk copying, reproduction, or redistribution in any form is not permitted."
- Excessive downloading may get the offending subnet blocked.
- There is nothing on AI.
- This is a surprise, because works of the US government are usually public domain. The DLMF authors were mostly not federal employees, and they assigned copyright to NIST.

**Bulk channel:** none. Wikidata P11497 holds 199 DLMF IDs [measured].

**What math2 can take:** **record only**.
- Equation numbers such as "DLMF 5.2.1" are stable locators.
- Each formula is a fact, so math2 can restate it in its own words or LaTeX, with a citation. It must not copy the text or the commentary.

---

## A.10 Fungrim (Mathematical Functions Grimoire)

**Size:** "457 symbols, 3130 entries (definitions, formulas, tables, plots)", 82 topics. https://fungrim.org/
- The page build timestamp is 2021-03-15. The repository was last pushed 2022-02-24. https://api.github.com/repos/fredrik-johansson/fungrim
- **It is dormant.**

**Licence:** **MIT** ("Fungrim is provided under the MIT license"). The repository `LICENSE` reads "Copyright (c) 2019 Fredrik Johansson".

**Bulk channel:** git. The entries are machine-readable Python/Grim expressions, with symbolic formulas and conditions.

**What math2 can take:** `open`. MIT is permissive: keep the notice in provenance.
- Fungrim is unusually valuable, because each formula has explicit assumptions. That makes it a good source for `statement` nodes, and later for Lean.

---

## A.11 Wolfram Functions Site

**Size:** **307,409 formulas** "as of October 26, 2026". https://functions.wolfram.com/

**Licence:** "© 1998–2026 Wolfram Research, Inc.".
- Neither the home page nor the About page states any terms of use.
- The About page says formulas are downloadable as notebooks and PDFs, and in MathML and ASCII. https://functions.wolfram.com/About/
- `robots.txt` allows crawling except `/webMathematica`. It lists 15 sitemaps.
- The general Wolfram terms that apply are UNVERIFIED (not found).

**What math2 can take:** **record only**: links, and formula IDs as locators. The formulas are facts, so math2 restates them in its own words.

---

## A.12 LMFDB

**Licence** (from the source template, because the site gates non-browser clients). https://raw.githubusercontent.com/LMFDB/lmfdb/main/lmfdb/templates/license.html
- "The database underpinning the LMFDB is licensed under the Creative Commons Attribution-ShareAlike 4.0 International License (CC-BY-SA)".
- The code is GPL v2 or later.
- The page says copyright is in the collection: "completeness claims are protected … as are search results for queries. Individual facts … are not".
- On AI: "How does this apply to training an AI model using this data? See this guidance from Creative Commons. We also encourage you to use more efficient access options rather than scraping the website directly, and to rate limit your website requests".

**2025–2026 changes in access**
- **`www.lmfdb.org` now redirects requests to `beta.lmfdb.org/gate.html`.**
  - `gate.html` is a JavaScript page that sets a `human=1` cookie. It has `Last-Modified: Thu, 14 Aug 2025`.
  - A client without JavaScript gets "Enable JavaScript, then reload."
  - I observed this on 2026-10-26 with curl. I did not try to bypass it.
- **`robots.txt` disallows `/api/`** for all agents. It also has `Crawl-delay: 30` and fully blocks two named bots. https://www.lmfdb.org/robots.txt
  - So the "API over 26 collections" in spec 1.7 is off-limits to robots.
  - The homepage footer reads "LMFDB Release 1.2.1 · SageMath 10.5". https://www.lmfdb.org/

**Size:** not re-measured, because of the gate. The spec 1.7 figures are from 2022.

**What math2 can take:** `open` (CC BY-SA 4.0).
- Get the data from LMFDB's bulk downloads, or by agreement. Do not use the API or scrape the site.
- Knowl text, the LMFDB's short explanations, is part of "the database", so presumably CC BY-SA too. That knowls fall under the data licence is UNVERIFIED: the licence page does not mention them.

---

## A.13 The subwikis: groupprops, Topospaces, Commalg, Diffgeom, Market

These are MediaWiki `siteinfo` figures [measured].

| Wiki | Articles | Pages | Edits | Active users | Licence |
|---|---|---|---|---|---|
| groupprops.subwiki.org | 8,076 | 15,469 | 51,383 | 1 | CC BY-SA 3.0 |
| topospaces.subwiki.org | 825 | 1,450 | 3,862 | 1 | CC BY-SA 3.0 |
| commalg.subwiki.org | 429 | 708 | 1,630 | 1 | CC BY-SA 3.0 |
| diffgeom.subwiki.org | 435 | 716 | 1,521 | 1 | CC BY-SA 3.0 |
| market.subwiki.org | 223 | 464 | 1,638 | 1 | CC BY-SA 3.0 |

- **Source:** `https://<wiki>.subwiki.org/w/api.php?action=query&meta=siteinfo&siprop=statistics|rightsinfo`. The `rightsinfo` field is "https://creativecommons.org/licenses/by-sa/3.0/".
- **Bulk channel:** the MediaWiki API (Special:Export). No published dump was found.
- **AI terms:** none.
- **What math2 can take:** `open`.
  - Groupprops is the most useful: it has structured "property implications" and "facts" pages about groups, and it suits `statement` candidates.
  - Fetch it page by page and slowly. The site is small and maintained by one person.

---

## A.14 Manifold Atlas (MPIM Bonn)

**Size:** **215 articles**, 2,721 pages, 17,516 edits, 0 active users [measured, `siteinfo`].
- It runs MediaWiki 1.18.4 and is reachable over HTTP; HTTPS refused the connection on the read date. http://www.map.mpim-bonn.mpg.de/api.php
- It has an editorial board and refereed pages.

**Licence: self-contradictory.** The Manifold Atlas:Copyrights page says "the copyright rules are the same as for Wikipedia, the GNU Free Documentation License 1.3", and then that "The content of the Manifold Atlas is released into the public domain". The `rightsinfo` field is empty.

**What math2 can take:** treat it as `gfdl`, **link only**, until the editors clarify. The public-domain sentence is not a clean dedication: there is no CC0 and no waiver text. Records and links are fine.

---

## A.15 π-Base

**Size** [measured, `pi-base/data` tree at commit 2026-10-24T21:57:19+02:00, "Torus initial PR (#1777)"]: **225 spaces, 247 properties, 935 theorems, and 2,147 space–property assertions**. This matches spec 1.7.

**Licence:** **CC BY 4.0**. The GitHub licence of the `pi-base/data` repository is `CC-BY-4.0`. https://api.github.com/repos/pi-base/data

**Bulk channel:** git (Markdown plus YAML). It was pushed 2026-10-26.

**What math2 can take:** `open`. Theorems are implications between properties, and they are good `statement` candidates.

---

## A.16 House of Graphs

**Size:** "nearly 22 000 graphs" in the 2.0 paper (2022). https://arxiv.org/abs/2210.17253
- The site is a JavaScript application.
- The API endpoints I tried (`/api/statistics`, `/api/enums/invariants`) return `{"error":true,"message":"Unauthorized"}`.
- The news entry of 23 Apr 2026 added the invariant "independent domination number" (from a search summary of https://houseofgraphs.org/).
- **The current count of graphs and of invariants is UNVERIFIED.** The site shows nothing without JavaScript, and the API needs authentication. The 2022 paper says 44 invariants; spec 1.7 says 52.

**Licence:** none found. Users are asked to cite Coolsaet, D'hondt and Goedgebeur, *Discrete Appl. Math.* 325 (2023) 97–107.

**What math2 can take:** **record only**.
- graph6 strings and invariant values are facts, so they are fine as CC0 metadata.
- Wikidata has 213 House of Graphs IDs (P12934) [measured].

---

## A.17 ISGCI (Information System on Graph Classes and their Inclusions)

**Size:** **1,695 graph classes, 242,975 inclusions, 30,147 complexities, 52,029 bounds**, "updated 2026-10-22". The latest news item is dated 2025-11-16. https://www.graphclasses.org/

**Licence:** none stated. The site shows only "by H.N. de Ridder et al. 2001-2014" and an Impressum.

**Bulk channel:** none listed on the page. The accompanying Java application loads the data; the format and URL are UNVERIFIED.

**What math2 can take:** **record only**.
- Class IDs (`gc_N`) are stable locators.
- Inclusion facts are facts and can be CC0 metadata. The collection as a whole may still carry database rights in the EU (ISGCI is maintained in Germany).

---

## A.18 Database of Ring Theory (DaRT)

**Size** [measured, `rschwiebert/dart_data` tree at commit 2026-10-24]:
- **218 rings, 178 ring properties, 72 theorems, 46 modules, 94 module properties.**
- It also holds logic (implication) files, 574 ring citation files and 269 publication files.
- The repository was pushed 2026-10-25.

**Licence:** **CC BY 4.0** (`dart_data` repository licence). https://api.github.com/repos/rschwiebert/dart_data
- MathBases lists DaRT's licence as MIT, which probably refers to the app code. https://mathbases.org/d/ring-theory (from a search summary)

**Bulk channel:** git (YAML). The site is https://ringtheory.herokuapp.com/

**What math2 can take:** `open`.

---

## A.19 KnotInfo / LinkInfo and the Knot Atlas

**KnotInfo** (Livingston and Moore)
- It covers knots of crossing number 3 to 13. The site's option list goes up to "3-13".
- I counted about 116 invariant fields on the search page, which is my own tally.
- Citation form: "KnotInfo: Table of Knot Invariants, https://knotinfo.org/ (May 16, 2026)" (from search).
- The old host `knotinfo.math.indiana.edu` no longer resolves (DNS failure on 2026-10-26).
- **Licence:** none on the site. https://knotinfo.org/
- **Download:** the full invariant table is an Excel spreadsheet, about 16 MB zipped. There is also a polynomial file (8 MB), 3D coordinates (46 MB) and diagrams (about 9,000 files). https://knotinfo.org/homelinks/database_download.php
- **A licence surprise.** The `database_knotinfo` Python package (version 2026.10.5) redistributes "Complete contents of the KnotInfo and LinkInfo databases in CSV format" under **GPL-3.0**. https://api.github.com/repos/soehms/database_knotinfo ; https://pypi.org/pypi/database-knotinfo/json

**Knot Atlas** (Bar-Natan and Morrison)
- **2,710 articles** but 1,651,548 pages in all, because data pages are generated. 1,729,610 edits, 1 active user [measured, `siteinfo`]. https://katlas.org/api.php
- **Licence:** `rightsinfo` is empty. No licence page was found: `Knot_Atlas:General_disclaimer` returns 404.
- The data is produced by the `KnotTheory`` Mathematica package. https://katlas.org/wiki/Main_Page (last edited 6 May 2026)

**What math2 can take:** **record only** for both.
- Knot names (Rolfson notation, `11n_34` and similar) and invariant values are facts. Wikidata has 2,978 KnotInfo IDs and 2,228 Knot Atlas IDs [measured].
- Do not import the GPL-3.0 CSV package as text. GPL does not fit into the CC BY-SA or CC0 zones, and the facts can be taken without it.

---

## A.20 ATLAS of Finite Group Representations (v3)

**Size:** "information (including 5215 representations) on about 716 groups", version 3.004. https://brauer.maths.qmul.ac.uk/Atlas/v3/

**Licence:** none stated on the site.
- The GAP interface `AtlasRep` is GPL-3.0-or-later. Version 2.1.12 came out in August 2026 (from search). https://docs.gap-system.org/pkg/atlasrep/doc/chap0.html
- The package archives do **not** contain the data; it is fetched on demand (from search, AtlasRep documentation).
- The mirror is http://atlas.math.rwth-aachen.de/Atlas/v3 (from search).

**What math2 can take:** **record only**: group names, representation identifiers and facts.

---

## A.21 Open Problem Garden

- **Licence:** GFDL. The footer says "Content distributed under" with a GNU FDL badge whose image file is named `gnu-fdl3`, so probably version 1.3. The exact version is UNVERIFIED. http://www.openproblemgarden.org/
- **Size:** the subject counts sum to 713. Problems can appear in several subjects, so this is an upper bound on distinct problems. The Recent Activity list shows no dates. This matches spec 1.3.
- **What math2 can take:** `gfdl`, **link only**. This confirms the spec.

---

## A.22 Erdős problems

- **Size:** "There are 1221 problems in the database." https://www.erdosproblems.com/
- **Licence of the problem text: none stated on erdosproblems.com.** The site has no licence or terms of reuse.
- **The GitHub repository `teorth/erdosproblems` is Apache-2.0, but `data/problems.yaml` contains no problem statements.** It holds only these fields: number, prize, `informal_status`, `formal_status`, status, oeis, formalized and tags. Its 1,221 entries have zero `statement` fields [measured]. https://raw.githubusercontent.com/teorth/erdosproblems/main/data/problems.yaml (repository pushed 2026-10-25)
  - **Spec 9.2 says math2 takes "problem statements" from `teorth/erdosproblems`. That is wrong.** The statements are on Bloom's site with no licence. The only Apache-2.0 statements are the *formal* ones in DeepMind's formal-conjectures.
- **What math2 can take:**
  - from the repository: status and links, as `open` metadata;
  - from erdosproblems.com: the informal statements, as **record only**, by link and problem number;
  - from formal-conjectures: the formal statements, `open` (Apache-2.0).

  math2 writes its own informal statement for each problem.

---

## A.23 Kourovka Notebook (Unsolved Problems in Group Theory)

- It is on arXiv as **arXiv:1401.0300**. The latest version, **v49, is dated 20 Oct 2026**. It is the **21st edition**, "which contains 150 new problems". https://arxiv.org/abs/1401.0300 ; https://export.arxiv.org/api/query?id_list=1401.0300
- **Licence:** the arXiv non-exclusive distribution licence 1.0. https://arxiv.org/licenses/nonexclusive-distrib/1.0/
- **What math2 can take:** **record only**.
  - Problem numbers (such as "19.37") with the edition and arXiv version are locators.
  - math2 writes its own statements, and the `tamnd/arxiv` gate applies.

---

## A.24 MathOverflow and Mathematics Stack Exchange

**Size** [measured, Stack Exchange API `/2.3/info`, 2026-10-26]:
- **MathOverflow:** 168,603 questions, 201,920 answers, 1,107,746 comments, 516,907 users.
- **Math.SE:** **1,702,027 questions**, 2,236,177 answers, 9,483,312 comments, 1,617,966 users.

**Licence of the text:** CC BY-SA 2.5, 3.0 or 4.0 per post, by date. This is in the spec already.

**Terms**
- **API Terms of Use** were last updated 30 Nov 2023 and are short. Applications "must ensure they visually indicate that the Stack Exchange Network is the source". They incorporate the network Terms of Service. https://policies.stackoverflow.co/stack-overflow/api-terms-of-use/
- **Public Network Terms of Service** were "Last updated November 13, 2025". https://policies.stackoverflow.co/stack-overflow/public-network-terms-of-service/
  - The body is loaded by JavaScript and I could not read it. **Whether the November 2025 revision added AI or scraping clauses is UNVERIFIED.**
- **API throttles.** https://api.stackexchange.com/docs/throttle
  - More than 30 requests per second from one IP gets a ban.
  - The daily quota is 10,000 requests with a key, or per user with an access token. Without a key, the quota observed was 300 (`quota_max: 300`).
  - A `backoff` field must be honoured. I got a `throttle_violation` for calling a second site within the 10 s backoff, so the backoff seems to apply across sites.
- **Dumps.**
  - Since July 2024 the dumps are behind a login and a click-through: "…for projects that do not include training a large language model (LLM)…". https://www.devclass.com/development/2024/07/30/stack-exchange-restricts-access-to-dump-of-user-contributed-data-critics-complain-this-contradicts-license/1625192
  - **The watermarks are documented.** "v1" was two fabricated `<row>` entries in `Posts.xml`, with post IDs `1000000001` and `1000000010`. They appeared only in the 2025-06-30 dumps (rev1 and rev2), and no 2026 dump is listed as watermarked. https://github.com/LunarWatcher/se-data-dump-transformer/blob/master/docs/meta/Known%20watermarks.md
  - Community re-uploads to the Internet Archive go up to 2026-06-30. https://search.feep.dev/blog/post/2026-08-25-stackoverflow-archive (from search; the 2026-09-30 dump's re-upload is UNVERIFIED)
- **New channel.**
  - Stack Overflow launched a public **MCP server** in beta in December 2025, limited to 100 requests a day. https://stackoverflow.blog/2025/12/01/what-s-new-at-stack-overflow-december-2025/ (from search)
  - The API docs navigation now has an "MCP Server" entry.
  - Whether it covers MathOverflow and Math.SE is UNVERIFIED.
- **Commercial AI licensing.** Stack Overflow sells "Stack Data Licensing" to AI labs (footer link on policies.stackoverflow.co). Reports from late 2025 describe deals with several labs; these come from secondary sources only.

**What math2 can take:** `open`, per post, through the API, exactly as spec 16.6 says.
- Anonymous use gives only 300 calls a day. math2 needs a registered key for 10,000.

---

## A.25 Other large human-written references (brief)

- **MathBases** (https://mathbases.org/) is a directory "Of, by, and for mathematical databases". Its area tags sum to 251; this is not a count of databases.
  - Entry pages have a licence field. For example, it gives DaRT as "MIT".
  - It is a useful seed list for new adapters. Its own licence is UNVERIFIED.
- **FindStat** (https://www.findstat.org/) has 1,997 statistics, 336 maps and 24 collections. The home page states no licence (still [U] in spec 1.7).
- **Lexikon der Mathematik** (Springer Spektrum) is proprietary. Wikidata P12975 has 603 IDs [measured]: record only.
- **All-Russian Mathematical Portal** (Math-Net.Ru) is a bibliographic portal, not an encyclopedia. Wikidata P4252 has 5,090 IDs [measured].
- **Metamath** (`set.mm`). Wikidata P12888 has 438 statement IDs [measured]. `set.mm` is said to be public domain or CC0, which is UNVERIFIED here. It is formal, so it belongs in the formal-library catalogue (9.4).
- **Not reached:** GroupNames (Bristol) refused the connection; Kerodon's licence is not on its about page.

---

## Summary table

| Source | Size (2026-10-26 unless noted) | Licence of text / data | Channel | AI terms | What math2 can take |
|---|---|---|---|---|---|
| Wikipedia (en) | 28,196 article pages in WP Math (33,957 project pages) | CC BY-SA 4.0 (+GFDL where no BY-SA-only text) | monthly XML dump (2026); Enterprise free tier 30 snapshots and 50k on-demand a month; API 10–2,000 req/min | Nov 2025 ask: attribute, use Enterprise; no licence change | **text with attribution** |
| Wikipedia de/fr/ja/zh/ru | 20,732 / 12,192 / 8,865 / 5,682 / 3,928 (project-tagged) | CC BY-SA 4.0 | same dumps | same | text with attribution |
| Wikidata | 113,118 P2534 items, **105,345 of them primes**; 6,560 non-prime; 3,208 theorems | CC0 | weekly JSON dump (103 GB bz2); WDQS 60 s, 5 parallel | none; May 2026 agent overload | **all (metadata)** |
| nLab | 20,778 pages | **no licence**; "free … if you acknowledge the source" | git mirror per edit | none | **link only** (+ ids, link graph) |
| ProofWiki | 47,254 content pages; 30,348 proofs / 36,102 definitions | CC BY-SA 3.0 + GFDL | **daily `latest.xml.gz` (36.6 MB)** | none | **text with attribution** |
| PlanetMath | 9,477 entries; inactive (last push Feb 2024) | CC BY-SA 3.0 | git (64 MSC repos) | none | text with attribution |
| Encyclopedia of Mathematics | >8,000 (2002 print); 1,224 Wikidata IDs; live count UNVERIFIED (502) | **originals © Springer**; new edits CC BY-SA 3.0 + GFDL | none (MediaWiki API) | none | **record only** (text only for wholly new articles) |
| MathWorld | 16,939 entries | proprietary; no bulk, no robots for archiving | none | none stated | record only |
| OEIS | 400,709 sequences | CC BY-SA 4.0 | `oeisdata` git (daily), `stripped.gz`; Crawl-delay 10 | none | text with attribution |
| DLMF | v1.2.8 (2026-09-15) | **© NIST**, no bulk, no commercial | none | none | record only |
| Fungrim | 3,130 entries, 457 symbols; dormant since 2021–22 | **MIT** | git | none | text with attribution |
| Wolfram Functions | 307,409 formulas | © Wolfram, terms not found | none | none | record only |
| LMFDB | (2022 figures in spec 1.7) | data CC BY-SA 4.0; code GPL v2+ | bulk downloads; **`/api/` disallowed in robots; JS gate since Aug 2025** | asks AI users not to scrape, points to CC guidance | data with attribution, via bulk only |
| groupprops (+ other subwikis) | 8,076 articles (topospaces 825, commalg 429, diffgeom 435) | CC BY-SA 3.0 | MediaWiki API | none | text with attribution |
| Manifold Atlas | 215 articles | **contradictory**: GFDL 1.3 and "public domain" | MediaWiki API (HTTP only) | none | link only (treat as GFDL) |
| π-Base | 225 spaces, 247 properties, 935 theorems | CC BY 4.0 | git | none | text with attribution |
| House of Graphs | ~22,000 graphs (2022); now UNVERIFIED | none stated | API needs auth | none | record only |
| ISGCI | 1,695 classes, 242,975 inclusions | none stated | Java app | none | record only |
| DaRT | 218 rings, 178 properties, 72 theorems | CC BY 4.0 | git | none | text with attribution |
| KnotInfo | knots ≤13 crossings, ~116 fields | none on site; **CSV package GPL-3.0** | Excel/CSV download | none | record only |
| Knot Atlas | 2,710 articles (1.65M pages) | none stated | MediaWiki API | none | record only |
| ATLAS v3 | ~716 groups, 5,215 reps | none stated (AtlasRep GPL-3+) | AtlasRep on demand | none | record only |
| Open Problem Garden | ≤713 entries | GFDL (prob. 1.3) | none | none | link only |
| Erdős problems | 1,221 | **site text: no licence**; repo Apache-2.0 but **metadata only** | git YAML | none | status as metadata; statements record only |
| Kourovka Notebook | 21st ed., arXiv:1401.0300v49 (2026-10-20) | arXiv non-exclusive | arXiv | none | record only |
| MathOverflow | 168,603 Q / 201,920 A | CC BY-SA 2.5/3.0/4.0 per post | API (10k/day with key; 300 anonymous); dumps no-LLM click-through | dump click-through; ToS updated 2025-11-13 (content UNVERIFIED) | per-post text with attribution, via API |
| Math.SE | 1,702,027 Q / 2,236,177 A | as above | as above | as above | as above |

---

## Proposed corrections to the spec

1. **1.7 and 9.2, Wikidata:**
   - 113,118 "items with a defining formula" are mostly prime numbers (105,345). Report **6,560** non-prime items.
   - Add the measured crosswalk properties: P7726 PlanetMath 951, P7554 EoM 1,224, P829 OEIS 469, P12934 House of Graphs 213, P11497 DLMF 199, P12987 LMFDB knowl 109, P8427 KnotInfo 2,978, P8426 Knot Atlas 2,228, P14106 Erdős 3.
2. **1.7 and 9.2, Encyclopedia of Mathematics:** change the licence from "CC BY-SA 3.0" to **"originals © Springer; only new wiki text CC BY-SA 3.0 + GFDL"**, and the class to **`record`**.
3. **9.2, Erdős problems:** `teorth/erdosproblems` has **no statements**. The informal statements on erdosproblems.com carry no licence, so they are record only.
4. **1.7 and 9.2, nLab:** 20,778 pages. The licence is now confirmed as *none*, with an informal permission, so the class is `record` (link only), and no longer [U].
5. **1.7 and 9.2, ProofWiki:** the licence is CC BY-SA 3.0 **and GFDL**. The daily dump is live (36.6 MB gzipped).
6. **1.7 and 9.2, LMFDB:** `robots.txt` disallows `/api/`, and there is a JavaScript gate. The channel is bulk downloads only.
7. **1.7, Wikipedia:** 28,196 article pages (not "about 26k"). The dumps are monthly in 2026. Add the 2026 API rate limits.
8. **9.3, licence classes:** add DLMF (NIST copyright), Wolfram Functions, the Kourovka Notebook (arXiv default), KnotInfo, ISGCI, the ATLAS and House of Graphs to `record`. Add Manifold Atlas to `gfdl`. Add Fungrim (MIT), DaRT (CC BY) and the subwikis (CC BY-SA 3.0) to `open`.
9. **1.7, MathWorld:** 16,939 entries. **OEIS:** 400,709.

---

## Open doubts

1. **EoM.** I could not read the live site (502 all morning). Did the move to EMS Press change the Springer copyright on the original articles? What is the current article count? A retry and a question to the EMS would settle it.
2. **Stack Exchange Public Network Terms of 13 Nov 2025.** Do they now forbid AI training or scraping of public content? (The text is loaded by JavaScript and I could not read it.) Does the new public MCP server cover MathOverflow and Math.SE?
3. **nLab.** Would the steering committee agree to an explicit CC BY-SA statement for new contributions? Old pages would still need each author's consent.
4. **Manifold Atlas.** GFDL 1.3 or public domain? Ask the editors.
5. **LMFDB.** Do knowls fall under the CC BY-SA "database" licence? Which bulk channel does the LMFDB want AI users to use? The site's JavaScript gate stopped me reading the current download page.
6. **House of Graphs.** The current number of graphs and of invariants, and any licence. The API needs authentication.
7. **KnotInfo.** Did the GPL-3.0 relicensing of the CSV package have the authors' consent? KnotInfo itself states no licence.
8. **Wikipedia dumps.** Is the move from twice-monthly to monthly runs official? Were the free `enterprise_html` dumps (last run 2025-03-20) withdrawn, or moved?
9. **Wikidata WDQS.** The details of the 2026–2027 backend migration and of the scholarly graph split, and whether the new endpoints have different limits.
10. **ProofWiki Main Page counters** (30,348 proofs, 36,102 definitions) are the same as in the spec. Are they live?
11. **Open Problem Garden.** The exact GFDL version (1.2 or 1.3) is inferred from an image file name.
12. **Unreached sources:** GroupNames (connection refused), FindStat's licence, Kerodon's licence, and MathBases' own licence.
