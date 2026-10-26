# K01, part C. Formal libraries and formal / informal-formal datasets

Research note for the Math 2.0 spec (tamnd/math2). This note extends spec 01 §1.2, spec 07 (bindings, M2Extract, §7.10 other proof assistants) and spec 09.4 (libraries enter as bindings, never as copies). It does not repeat those sections. Where a number has moved since the spec was written, the note says so.

Conventions:
- I read every source on **2026-10-26**, unless the entry says otherwise. "(read 2026-10-26)" is left out to keep the text short. Every fact has a URL.
- **UNVERIFIED** means I could not confirm the fact from a primary source. The reason follows the tag.
- File counts come from the GitHub git-trees API (`recursive=1`). The `truncated` flag was false in every case unless noted. They count files, not declarations.
- HF row counts come from `datasets-server.huggingface.co/size` or the dataset card.

---

## 1. Formal libraries

### 1.1 Lean 4

**Mathlib**
- Stats page https://leanprover-community.github.io/mathlib_stats.html: **139,616 definitions, 291,237 theorems, 772 contributors**. The page shows no date. Spec 01 had 291,178 / 139,569 on 2026-10-25, so the count grew in one day.
- Licence: Apache-2.0 (https://github.com/leanprover-community/mathlib4). The repo has 35,715 commits and about 4.4k stars.
- Tree API: **8,661 `.lean` files under `Mathlib/`**. The tree has 10,796 entries and was not truncated. Spec 01 says 9,437 files, which probably counts every file and not only `Mathlib/*.lean`. This is a definitional difference, not a conflict.
- Informal text: docstrings and module docs only. See the docstring coverage under 1.1.1.

**1.1.1 Official extraction route: Mathlib Initiative datasets (new; not in spec)**
- Tool: `lean_scout`, https://github.com/mathlib-initiative/lean_scout (Apache-2.0).
- Datasets on HF under `mathlib-initiative/`, all Apache-2.0. They were extracted at Mathlib commit `56a005855ea1…`, "chore: bump toolchain to v4.35.0 (#44806)", dated **2026-10-13**.
  - `mathlib-types`: **797,294 rows**. Fields are `name`, `module`, `type`, `docString` and `allowCompletion`.
    - `allowCompletion = True` for 483,112 rows.
    - `docString` is null for **681,418 rows (85.47%)**. So only about **115,876 constants (~14.5%) have a docstring**, with a mean length of about 130 characters.
  - `mathlib-const-dep`: 797,294 rows (dependency edges per constant).
  - `mathlib-tactics`: **1,999,349 rows** (tactic plus goal states).
- Relevance to M2Extract (spec 07): the schema `name / module / type / docString` already matches what a binding needs. The constant count of 797k covers auxiliary and internal constants. The `allowCompletion` filter, at about 483k, is a better match for "user-facing declarations". Neither number matches the 291k theorems plus 139k definitions on the stats page. Each counts a different thing, and M2Extract should record which one it uses.

**doc-gen4**
- https://github.com/leanprover/doc-gen4 (Apache-2.0).
- It runs as `lake build Lib:docs` from a nested docbuild project. The target library must compile first.
- Output is HTML plus a declaration-data JSON. That makes it heavier than lean_scout when only statements are needed.

**Reservoir (Lean package index)**
- https://reservoir.lean-lang.org/packages?sort=stars: **908 packages**. Reservoir shows no sizes or licences in the listing.
- Top packages by stars:

  | Package | Stars |
  |---|---|
  | mathlib | 4,352 |
  | openai/NavierStokesAndEuler | 2,110 |
  | teorth/Analysis | 1,971 |
  | ulfjack/ryu | 1,357 |
  | LeanCopilot | 1,337 |
  | formal_conjectures | 1,336 |
  | anthropics/flt_e2e | 1,292 |
  | ImperialCollegeLondon/FLT | 1,053 |
  | Physlib | 808 |
  | cslib | 777 |
  | mil | 623 |
  | equational_theories | 562 |
  | paperproof | 554 |
  | scilean | 539 |
  | glimpseOfLean | 426 |
  | batteries | 424 |
  | aesop | 410 |
  | verso | 398 |
  | Game | 387 |
  | PrimeNumberTheoremAnd | 351 |

- Licences of individual packages must be read per repo. I did not survey them all.

**Tau Ceti**
- https://github.com/TauCetiProject/TauCeti: Apache-2.0, 19,274 commits, 247 stars.
- **18,849 `.lean` files** (tree API), which is about twice Mathlib's file count.
- Written by AI and reviewed against human rubrics. It tracks Mathlib master with an hourly bump. It is incubated by the Lean FRO and the Mathlib Initiative.
- Docs are at taucetiproject.org/docs. The stats page returned 404.
- Xena blog, 2026-10-16 (https://xenaproject.wordpress.com/): "Tau Ceti is now slightly larger than Mathlib (3 million lines of code)".
  - **UNVERIFIED** whether "3 million lines" refers to Tau Ceti or to Mathlib. The sentence is ambiguous. Spec 01 reads it as Tau Ceti ≈ 3M. Spec 01 also gives Mathlib 2,481,682 lines, which fits "slightly larger" only if Tau Ceti is somewhat above 2.5M.
- Consequence for E-tiers: Tau Ceti is mostly AI-authored. Any provenance or trust flag in spec 07 should separate it from human-reviewed Mathlib.

**Formal Frontier**
- https://github.com/orgs/FormalFrontier/repositories: 50 repos. All 30 shown on the first page are **Apache-2.0**.
- Examples: projective-modules, algebraic-groups, group-theory, homological-algebra, complex-analysis, scheme-properties, sphere-topology, simplicial-objects, iwasawa-modules.
- Xena (2026-10-16) reports 41 released libraries, about 200k lines. Highlights include the hairy ball theorem, Kaplansky's theorem, Bass stable range, Tate periodicity and Stone duality.
- The page mathlib-initiative.org/formal-frontier names no licence. The licence of the remaining 20 repos is UNVERIFIED (not shown on the first page).

**ATLAS (Meta), licence warning (new)**
- Repo https://github.com/facebookresearch/atlas-lean; paper arXiv:2605.29955 (2026-05-28).
- Size: 26 textbooks, more than 45,000 declarations, about 500k lines.
- **v1 is under CC BY-NC 4.0 with a "no-training rider".** It prohibits commercial use and any use "to train, fine-tune, distill, evaluate, or otherwise develop ML models".
- **v2 (repo root) is labelled Apache 2.0, but the root LICENSE file opens with "Codebase is not intended for use to train, fine-tune, distill, evaluate, or otherwise develop ML models".** This is an Apache grant with a non-standard preamble, so its legal effect is unclear.
- AutoformBench (https://github.com/facebookresearch/autoformbench) is CC BY-NC 4.0.
- 36Kr (https://eu.36kr.com/en/p/3829852422940291) reports 183B tokens. This is a secondary source and UNVERIFIED in the paper.
- Recommendation: crosswalk only. Do not bind v1. Treat v2 as legally unclear until the preamble is resolved.

### 1.2 Isabelle

**AFP**
- https://www.isa-afp.org/statistics/: **1,042 entries, 614 authors, ~330,000 lemmas, ~5,518,000 lines**. Spec 01 had 1,038 / ~326,500 / ~5.44M. The page gives no per-year data.
- Licence (https://isa-afp.org/about/): "All entries … licensed under a BSD-style License or the GNU LGPL". Each entry page has a License field, so the licence must be recorded per entry.
- Access: https://isa-afp.org/release/afp-current.tar.gz (about 70 MB), or Mercurial at https://foss.heptapod.net/isa-afp.
- AI provenance: the entry Jacobian_Counterexample (2026-07-20, BSD) says "AI assistance was used for proof engineering". The AFP has no archive-wide AI policy that I found.
- Informal text: Isabelle theories carry `text \<open>…\<close>` blocks and each entry has an abstract. That is more prose than Mathlib docstrings, but I did not measure it.

**Isabelle2026**
- https://isabelle.in.tum.de/: "Now available: Isabelle2026 (October 2026)". This **resolves the [U] in spec 01**.
- Features include HOL-New_Algebra and cvc5 proof reconstruction.
- Licence: BSD-style for the core, mixed overall (bundled components).
- sketis.net shows RC4 dated 14-Oct-2026 and the final release planned for 24-Oct-2026.
- The size of the distribution library (HOL, HOL-Analysis and so on) is UNVERIFIED. I found no official count.

### 1.3 Rocq (formerly Coq)

| Library | URL | Licence (LICENSE file) | `.v` files |
|---|---|---|---|
| Rocq core | https://github.com/rocq-prover/rocq | LGPL-2.1 | – |
| Stdlib | https://github.com/rocq-prover/stdlib | LGPL-2.1 | 904 |
| MathComp | https://github.com/math-comp/math-comp | CeCILL-B | 147 |
| MathComp Analysis | https://github.com/math-comp/analysis | CeCILL-C | 131 |
| Odd Order | https://github.com/math-comp/odd-order | CeCILL-B | 34 |
| UniMath | https://github.com/UniMath/UniMath | MIT-style custom | 1,714 |
| Coquelicot | gitlab.inria.fr (blocked by Anubis) | **UNVERIFIED** | – |

- The coq-community org was renamed **rocq-community** and has 75 public repos. The math-comp org has 27 repos.
- The opam `released/packages` directory (rocq-prover/opam) has 603 entries.
- Coquelicot: the GitHub fork proux01/coquelicot is LGPL-3.0. The upstream licence is UNVERIFIED because the Inria GitLab did not load (an Anubis bot challenge).
- MathComp files are large, so file count understates size. I found no declaration count.
- CeCILL-B is BSD-like and CeCILL-C is LGPL-like. Both allow bindings.
- Informal text: comments only. MathComp has structured header comments.

### 1.4 HOL Light and HOL4

- **HOL Light**: https://github.com/jrh13/hol-light. BSD-style LICENSE, 664 `.ml` files. Blanqui's LPAR talk gives about 37,000 objects in 635 files. I have the talk only as a secondary reference, so that figure is UNVERIFIED in a primary listing.
- **Flyspeck**: MIT, 503 `.hl` files.
- **HOL4**: https://github.com/HOL-Theorem-Prover/HOL. BSD-style COPYRIGHT, 1,495 `*Script.sml` files.
- **hol2dk** (https://github.com/Deducteam/hol2dk, CeCILL 2.1): translates HOL Light to Dedukti, Lambdapi and Rocq.
  - `hol.ml` has 5,687 theorems.
  - Multivariate has 40,728 theorems. The dump is about 135 GB and Rocq checking takes about 21 h.
  - Output is in the opam packages `coq-hol-light` and `rocq-hollight-logic`.
  - **There is no Lean output.**
- arXiv:2609.24598 (Blanqui and Gontard, 2026-09-21, LPAR-26) maps HOL Light constants to idiomatic Rocq. This "alignment" step is what makes translations usable rather than literal.
- **HolLean** (https://github.com/hol-light-family/HolLean) ports the HOL Light *system* to Lean 4 as a meta-language. It does not translate the library.

### 1.5 Metamath

- **set.mm**: https://github.com/metamath/set.mm. **LICENSE = CC0 1.0** (read through the gh API). 17,928 commits; last push 2026-10-26.
- Sizes. The three pages count different things.
  - The theorem list https://us.metamath.org/mpeuni/mmtheorems.html has 513 pages, and its last statement number is **51,207**. That number includes axioms and definitions. The newest dated theorem is 27-Aug-2026.
  - https://us.metamath.org/mpeuni/mmset.html says "over 26,000 … main sections, over 41,000 counting mathboxes".
- **iset.mm** (https://us.metamath.org/ileuni/mmtheorems.html): about 17,395 statements (174 pages).
- **Metamath 100**: 75 as of 2026-04-28 (the mm_100 page). Wiedijk's list credits Metamath with 74. The gap is a sync lag.
- Tooling:
  - **mm0** (https://github.com/digama0/mm0, CC0) includes a set.mm to Lean 3 translation path.
  - digama0/mm-lean4 and zariuq/mm-lean4 are Metamath verifiers in Lean 4, not library translators.
- Informal text: every statement has a mandatory comment, usually a sentence or more. **set.mm is the only large library that is CC0 and has a prose comment on every statement.** This strengthens the "v2 candidate" status in spec 07 §7.10.

### 1.6 Mizar

- https://mizar.uwb.edu.pl/: **MML 5.94.1493 (2025-05-30)**, Mizar 8.1.15.
- **Possible conflict**: in MML naming, the last field of the version (1493) is normally the article count. Spec 01 gives 1,498 articles. UNVERIFIED which is right; the 73,460 theorems figure could not be rechecked.
- Licence: dual **GPLv3+ / CC BY-SA 3.0+** since MML 4.160.1126. Sources: https://mizar.uwb.edu.pl/forum/archive/1104/msg00000.html and arXiv:1107.3212.
- The system has been GPLv3 at https://github.com/MizarProject/system since about 2023.
- CC BY-SA is share-alike. Derived statements would inherit it, so it should be crosswalk-only unless the corpus accepts SA.

### 1.7 Agda

| Library | Licence | Files | Informal text |
|---|---|---|---|
| agda-stdlib (https://github.com/agda/agda-stdlib) | MIT-style | 1,183 `src/*.agda` | comments |
| agda-unimath (https://github.com/UniMath/agda-unimath) | MIT | 3,072 `.lagda.md` | **literate Markdown, prose throughout** |
| 1Lab (https://github.com/the1lab/1lab) | **AGPL-3.0** | 710 `.lagda.md` | **literate, prose throughout** |

- This fills the "Agda sizes [U]" gap in spec 01 with file counts. Declaration counts are still unknown.
- 1Lab is AGPL, so it should be crosswalk-only.
- agda-unimath is the cleanest literate library for informal and formal alignment: MIT, and prose sits next to each definition.

### 1.8 Naproche, Megalodon, PVS, ACL2

- **Naproche** (https://github.com/naproche/naproche): GPL-3.0, 32 `.ftl.tex` files. The text is controlled natural language, so the source is itself informal.
- **Megalodon**
  - ai4reason/Megalodon has an MIT-style licence (CIIRC/CTU).
  - The mgwiki/god1 and mgwiki/mgw_test repos have **no licence** (gh shows none).
  - arXiv:2601.03298 (Urban, 2026-01-06) reports about 160k lines and more than 1.5k lemmas. The work used ChatGPT 5.2 or Claude Sonnet 4.5 and cost about $100. Spec 01 had 130k lines for Munkres; the paper is newer.
- **NASALib** (https://github.com/nasa/pvslib)
  - The README says "63 top-level libraries, containing about 41K proven formulas". The file `nasalib.grandtotals` says 64 libraries.
  - Licence: the GitHub API shows none. The README says the NASA developments are "in the public domain"/"fundamental research that do not constitute software". Other contributions carry licences in each `top.pvs`, so licences are per sub-library.
  - The repo contains a `CLAUDE.md`, which suggests AI-assisted maintenance.
- **ACL2** (https://github.com/acl2/acl2): BSD-3 for the system. The `books/` directory has per-file licences. The tree has 40,929 entries.

---

## 2. Translation between systems

### 2.1 Infrastructure

| Tool | Status | Licence | Notes |
|---|---|---|---|
| Mathport (https://github.com/leanprover-community/mathport) | **archived**, last push 2024-11-21 | Apache-2.0 | Lean 3 to Lean 4 only; finished its job |
| lean2dk (Deducteam) | WIP, last push 2026-06 | Apache-2.0 | Lean 4 to Dedukti; spec 01 already says WIP |
| Logipedia | last push 2024-11 | – | All proofs in STTForall (arXiv:2305.00064). Dormant |
| OpenTheory | last milestone 2017 | – | Reads and writes HOL Light, HOL4 and ProofPower; reads Isabelle; no counts published |
| hol2dk | active (2026) | CeCILL 2.1 | HOL Light to Rocq, 40,728 theorems; no Lean target |
| Minilang (arXiv:2507.18885) | paper | – | Rule-based Isabelle Isar translation, about 340K proofs, about 85% success |

- No working, library-scale **X to Lean 4** translator exists as of today. The paths are hol2dk (to Rocq), mm0 (set.mm to Lean 3, legacy) and LLM-based porting.

### 2.2 LLM-based translation, with measured success

- **ITPEval** (arXiv:2607.19407, Wu, George and Anandkumar, 2026-07-07)
  - Covers 1,560 files and 6,848 theorems. It tests 12 directed pairs over Lean 4, Rocq, Isabelle and HOL Light, with 5 LLMs.
  - Statement translation peaks at **29.1% pass@1**; proof translation at **10.5%**.
  - Controlled files reach 29.7%, ecosystem files only 5.2%.
  - Multi-ITP context raises Lean success from 4.8% to 10.6%.
  - BEq confirms only **54.0%** of compiler-verified miniF2F-to-Lean statements.
  - Conclusion for the spec: compiling is not the same as being faithful.
  - The arXiv page gives no data URL, so dataset availability is UNVERIFIED.
- **miniF2F-rocq** (arXiv:2503.04763; https://github.com/LLM4Rocq/miniF2F-rocq, MIT): 478 of 488 problems translated, using GPT-4o mini, Claude 3.5 Sonnet, o1 mini and o1.
- **Megalodon Munkres** (arXiv:2601.03298): LLM-driven formalization of a full textbook for about $100 (see 1.8).
- **Lean Pool** (arXiv:2609.25199, 2026-09-21): an AI-maintained archive with a catalogue of imported projects. It gives no counts, so its size is UNVERIFIED.
- The Lean Zulip thread "Autotranslation of Logical Foundations from Rocq to Lean" (Jan 2026) is anecdotal evidence. I took no measured numbers from it.

### 2.3 Cross-system theorem lists

**Wiedijk 100** (https://www.cs.ru.nl/~freek/100/, last modified 2026-09-17):

| System | Count |
|---|---|
| HOL Light | 95 |
| Isabelle | 95 |
| Lean | 83 |
| Rocq | 80 |
| Metamath | 74 |
| Mizar | 71 |
| nqthm/ACL2 | 48 |
| ProofPower | 43 |
| PVS | 26 |
| Imandra | 20 |
| Megalodon | 12 |
| Naproche | 10 |
| NuPRL | 8 |

**1000+ theorems** (https://github.com/1000-plus-theorems/1000-plus)
- Licence: **Unlicense** (public domain). Last push 2026-07-22.
- **1,200 `_thm/*.md` YAML files keyed by Wikidata Q-id.**
  - Fields: `msc_classification`, `wikipedia_links`, and per-system blocks `isabelle`, `hol_light`, `rocq`, `lean`, `metamath`, `mizar`.
  - Entries come from Wikipedia's List of theorems.
- This is the best ready-made **crosswalk seed** for spec 07 §7.10: public domain, keyed by Wikidata, MSC-tagged and pointing into six systems.
- Per-system counts are UNVERIFIED. The web page showed about 228 formalized entries, which may be one page of a paginated list. Spec 01's "241 in Lean" could not be rechecked without parsing all 1,200 files.

---

## 3. Datasets

### 3.1 Benchmarks (formal statement plus informal statement)

**miniF2F**
- 488 problems: 244 valid, 244 test.
- Lean 4 ports:
  - yangky11/miniF2F-lean4 (MIT)
  - facebookresearch/miniF2F (MIT)
  - AI-MO/minif2f_test (Apache-2.0, 244 rows)
- Quality: "miniF2F-Lean Revisited" (arXiv:2511.03108, NeurIPS 2025) found discrepancies between formal and informal statements in **more than half** the problems.
  - Pipeline accuracy is 36–40% on the original and about 70% on the corrected v2.
  - The "over 300 problems corrected" figure is UNVERIFIED (search snippet only).

**ProofNet**
- MIT, 371 problems, originally Lean 3.
- ProofNet# (arXiv:2406.07222; HF PAug/ProofNetSharp, MIT) found **118 of 371 (31.8%)** wrong in the Lean 4 ports.
- ProofNet-Verified (HF marcusm117, Apache-2.0, NeurIPS 2026, targets Lean v4.34.0) found **205/367 (55.9%)** errors in ProofNet and **99/367 (27.0%)** in ProofNet#. A fixed benchmark still had errors in more than a quarter of its items.

**PutnamBench** (https://github.com/trishullab/PutnamBench)
- Lean 4 672, Isabelle 640, Coq 412, covering 1962–2025.
- Apache 2.0 for Lean and Isabelle, MIT for Coq.
- **The informal statements are used "with permission from the MAA"**. They are third-party text, so do not relicense them.
- The repo-level licence field is empty, so the licence is per subdirectory.

**FormalMATH**
- HF SphereLab/FormalMATH-All: MIT, 5,560 rows. GitHub Sphere-AI-Lab/FormalMATH-Bench: Apache-2.0 (EMNLP 2026 Findings). The two licences differ.
- About 72.09% of statements were kept before expert review. The best prover reached 16.46%.

**DeepSeek-ProverBench** (https://huggingface.co/datasets/deepseek-ai/DeepSeek-ProverBench)
- 325 problems: 15 from AIME 24/25 and 310 from textbooks.
- **No licence tag.**

**Defect audit across benchmarks: "Faults in Our Formal Benchmarking"** (arXiv:2606.29493, ICML 2026)
- 4,833 findings and **398 certified defects** in total.

  | Benchmark | Findings | Certified |
  |---|---|---|
  | FormalMath | 3,463 | 147 |
  | ProverBench | 370 | 208 |
  | ProofNet | 149 | 6 |
  | miniF2F (7 variants) | 773 | 36 |
  | CombiBench | 78 | 1 |

- ProverBench has **208 certified defects out of 325 problems**.

**Statement-faithfulness judges**
- EFG (arXiv:2506.10903) reports correctness of 76.64% for Isabelle miniF2F, 82.79% for Lean miniF2F and 64.84% for ProofNet Lean. These are UNVERIFIED: search snippet only, not confirmed on the abstract page.
- Beyond Compilation (arXiv:2606.31002): 227 statements. Its judge agrees with the human majority 91.5% of the time.

### 3.2 Large synthetic statement and proof sets (competition style)

| Dataset | Rows | Licence | Informal side | Notes |
|---|---|---|---|---|
| Lean Workbook (internlm, HF `internlm/Lean-Workbook`) | 57,231 + 82,893 (Plus) per paper/card; **default HF config 25,214 rows** | Apache-2.0 | Competition problems (AoPS) | Lean v4.8.0-rc1. A manual sample check found 93.5% accurate. 4,898 proved (pass@1024 is 8.6%). The row-count gap is UNVERIFIED |
| Goedel-LM/Lean-workbook-proofs | 29,750 | **no tag** | – | proofs for Lean Workbook |
| DeepSeek-Prover-V1 (HF) | 27,503 | "other" (deepseek-license; the card says commercial use is supported) | NL problem | The paper describes 8M synthetic statements; only 27.5k released |
| DeepSeek-Prover-V2 repo | – | NOASSERTION | – | cold-start data generated by DeepSeek-V3 |
| Goedel-Pset-v1 | 1,732,594 | **no tag** | NuminaMath problems | Two formalizers. Filtered by compile plus an FC test with Qwen2.5-72B-Instruct |
| Goedel-LM/SFT_dataset_v2 | 1,745,010 | Apache-2.0 | – | Goedel-Prover-V2 (arXiv:2508.03613) |
| Goedel-LM/RL_dataset_V2 | – | Apache-2.0 | – | 2026-03 |
| NuminaMath-LEAN (AI-MO) | 104,155 | Apache-2.0 | NuminaMath problem | Mathlib v4.15.0. `author` = human or autoformalizer. Proofs from Kimina-Prover-72B. **The card admits wrong reference answers and inaccurate formalizations** |
| Kimina-Prover-Promptset (AI-MO) | 24,418 | Apache-2.0 (stated in card text; no HF tag) | `natural_language` | Hard subset of NuminaMath-LEAN plus **Gemini-generated variants**, with duplication |
| NuminaMath-1.5 | 896,215 | Apache-2.0 | informal only | includes 268,819 from cn_k12 |
| AgenticCommons/formal-math-autoformalization | 3,623 | **CC0** | NL problem | Lean v4.30.0, compile and axiom gates |

- "Generated by model X" exposure:
  - DeepSeek-V3 for Prover-V2 data (DeepSeek licence).
  - Qwen2.5-72B for the Goedel filter.
  - Gemini for the Kimina variants.
  - Kimina-Prover-72B for the NuminaMath-LEAN proofs.
  - None of these cards adds a no-training clause. Whether the generating model's terms flow through to the data is a legal question I could not settle from the cards (UNVERIFIED).

### 3.3 Library-derived informal and formal pairs

| Dataset | Rows | Licence | Informal side | Measured quality |
|---|---|---|---|---|
| Herald (FrenzyMath; arXiv:2410.10878, ICLR 2025) | 579,883 statement pairs + 44,553 proofs (from about 110k Mathlib theorems) | Apache-2.0 | LLM informalization of Mathlib (Lean v4.11.0) | Translator gets 93.2% on miniF2F-test at pass@128. In a human check on ProofNet, 101/151 validated outputs were correct. **No score for the informalizations themselves.** The informalizing LLM is not named on the card |
| MMA (arXiv:2311.03755) | 244,238 AFP + 88,536 Mathlib4 (~332K) | **UNVERIFIED** (not on the arXiv page) | GPT-4 informalization | The paper calls them "noisy approximations"; no number |
| FrenzyMath/mathlib_informal_v4.19.0 | 218,071 | Apache-2.0 | `informal_name`, `informal_description`, plus `docstring` | none published (v4.16.0 also exists) |
| FrenzyMath/lsv2-mathlib-v4.28.0-rc1-jsonl | 310,579 | Apache-2.0 | LLM descriptions (LeanSearch v2, arXiv:2605.13137; https://github.com/frenzymath/LeanSearch-v2) | retrieval metrics only; no informalization accuracy |
| adeo1/mathlib_informal_v4.28.0 | 183,489 | MIT | LLM descriptions | none |
| phanerozoic/Lean4-Mathlib | 232,431 | Apache-2.0 | docstrings plus structure (imports, deps) | n/a; pinned to Mathlib commit `b9f1435…` |
| mathlib-initiative/mathlib-types | 797,294 | Apache-2.0 | human docstrings (~14.5% coverage) | human-written |
| TheoremGraph (arXiv:2606.25363; HF uw-math-ai/theorem-matching) | 100,831 candidate pairs (cosine ≥ 0.80); 388,105 Lean declarations from 25 projects vs 11.7M arXiv statements | **CC BY-SA 4.0** | real arXiv statements (not generated) | GPT-5.4 judge: 47,952 matches. DeepSeek-V4-Pro (lenient): 61,210. Acceptance about 48% overall, about 87% at similarity ≥ 0.9 |
| Stacks-to-Mathlib alignment (arXiv:2610.31011, CIKM '26; https://github.com/AnkitSatpute/Formal-Informal-Mathematical-Alignment) | – | MIT | Stacks Project tags | size UNVERIFIED |
| MathAtlas (arXiv:2605.14061; HF MathAtlas/MathAtlas) | 35,566 test + 698 hard (36,264), from 103 textbooks | **no tag** | informal only, with `mathlib_links` | best model 2.6% on the hard set |
| formalica/lean4oeis | – | CC BY-SA 4.0 | OEIS entries | Lean v4.34.0-rc1; size not recorded |

- TheoremGraph is the only large set that pairs Mathlib with **human-written** arXiv statements and not LLM paraphrases. Its share-alike licence must be propagated.

### 3.4 Proof-state and tracing datasets

- **LeanDojo** (https://github.com/lean-dojo/LeanDojo): MIT.
- **LeanDojo-v2**: licence inconsistent. The GitHub API says Apache-2.0 while the README says MIT. It traces repositories to JSONL and has a DynamicDatabase.
- The LeanAgent repo has **no licence**.
- HF tasksource/leandojo: cc-by-2.0, a re-upload.
- The size of LeanDojo Benchmark 4 is UNVERIFIED (not rechecked this session).
- l3lab/ntp-mathlib: 307,049 tactic examples (Lean v4.4.0).
- miniCTX and miniCTX-v2: Apache-2.0.
- MathlibPR: Apache-2.0, 15,895 PR snapshots.
- mathlib-initiative/mathlib-tactics: 1,999,349 rows, the newest (v4.35.0, 2026-10-13).

### 3.5 Pretraining corpora (informal math, some formal)

| Corpus | Size | Licence | Notes |
|---|---|---|---|
| FineMath (HuggingFaceTB) | 48,283,984 rows total. FineMath-3+: 34B tokens / 21.4M docs. 4+: 9.6B tokens | ODC-By + CommonCrawl ToU | classifier trained on Llama-3.1-70B annotations |
| OpenWebMath | 6.3M docs, 14.7B tokens | ODC-By 1.0 + CC ToU | |
| Proof-Pile-2 (EleutherAI) | 55B tokens: arXiv 29B, OWM 15B, AlgebraicStack 11B | "We do not alter the license of any of the underlying data" | AlgebraicStack formal part: Lean 285.6M, Isabelle 1,089.7M, Coq 281.9M, Agda 35.2M tokens |
| MathPile (GAIR) | 9.5B tokens (UNVERIFIED; the card is gated) | **CC BY-NC-SA 4.0**, gated | NC: exclude |
| AutoMathText | about 200 GB, 4,371,703 rows | CC BY-SA 4.0 | scored by Qwen-72B |

- Proof-Pile-2 has no licence of its own; every source keeps its upstream licence. Its AlgebraicStack Isabelle share (about 1.09B tokens) is larger than Lean, Coq and Agda combined.

---

## 4. Informalization tools and papers

| Tool / paper | What it does | Measured quality |
|---|---|---|
| doc-gen4 | HTML docs plus declaration JSON from compiled Lean | none needed (deterministic; human docstrings only) |
| lean_scout (Mathlib Initiative) | official extractor for types, docstrings, dependencies and tactics | deterministic |
| Herald pipeline (arXiv:2410.10878) | LLM informalization of Mathlib with dependency context | no direct informalization score; the downstream translator scores 93.2% on miniF2F |
| LeanSearch v2 (arXiv:2605.13137) | LLM descriptions of 310k Mathlib constants for retrieval | retrieval metrics only |
| MMA (arXiv:2311.03755) | GPT-4 informalization of AFP and Mathlib | "noisy"; no number |
| FormL4 (arXiv:2406.01940) | informalization judged by humans | Gemini-Pro-1.5 80% vs GPT-4 70% success |
| TheoremGraph (arXiv:2606.25363) | matches Lean declarations to real arXiv statements (not generated) | about 48% judge acceptance overall, about 87% at sim ≥ 0.9 |
| LeanAgent / LeanDojo-v2 | lifelong-learning prover and tracer | prover metrics only; not informalization |
| Beyond Compilation (arXiv:2606.31002) | faithfulness judge for formal statements | 91.5% agreement with human majority (n=227) |
| ITPEval BEq check (arXiv:2607.19407) | equivalence checking of translated statements | only 54.0% of compiled translations confirmed |

- **No published, measured error rate exists for LLM informalizations of Mathlib at library scale.** Herald, LeanSearch v2, the FrenzyMath datasets and adeo1 report downstream or retrieval metrics. FormL4 (80%) is the closest number, but it covers a different setting.
- If Math 2.0 generates informal text for bindings, it needs its own sampled human audit.

---

## 5. Summary tables

### 5.1 Libraries

| Library | Size (date) | Licence | Extraction route | Informal text |
|---|---|---|---|---|
| Mathlib | 291,237 thm / 139,616 def (stats page, read 2026-10-26); 797,294 constants at v4.35.0 (2026-10-13) | Apache-2.0 | lean_scout HF datasets; doc-gen4; LeanDojo | docstrings, ~14.5% of constants |
| Tau Ceti | 18,849 `.lean` files (2026-10-26); "3M lines" ambiguous | Apache-2.0 | same as Mathlib (Lean 4) | docs site; AI-written |
| Formal Frontier | 41 libraries, about 200k lines (Xena 2026-10-16) | Apache-2.0 (30/50 repos checked) | Lean 4 tools | per-repo |
| ATLAS | 26 books, more than 45k decls, about 500k lines (2026-05-28) | **v1 CC BY-NC + no-ML rider; v2 Apache with no-ML preamble** | Lean 4 | textbook-aligned |
| Reservoir | 908 packages (2026-10-26) | per package | Lake | per package |
| Isabelle AFP | 1,042 entries, ~330k lemmas, ~5.518M lines (2026-10-26) | BSD-style or LGPL, per entry | tarball / hg; Isabelle export | abstracts plus `text` blocks |
| Isabelle2026 | released Oct 2026; size UNVERIFIED | BSD-style core, mixed | Isabelle export | `text` blocks |
| Rocq stdlib | 904 `.v` files | LGPL-2.1 | coqdoc / SerAPI-style tools | comments |
| MathComp / Analysis / Odd Order | 147 / 131 / 34 `.v` files | CeCILL-B / CeCILL-C / CeCILL-B | opam | header comments |
| UniMath | 1,714 `.v` files | MIT-style | opam | comments |
| Coquelicot | – | UNVERIFIED (fork is LGPL-3.0) | opam | comments |
| HOL Light | 664 `.ml` files; about 37k objects (secondary) | BSD-style | hol2dk dump | comments |
| Flyspeck | 503 `.hl` files | MIT | hol2dk | comments |
| HOL4 | 1,495 `*Script.sml` files | BSD-style | HOL4 export | comments |
| set.mm | 51,207 statement numbers; over 41k theorems with mathboxes (2026-10-26) | **CC0 1.0** | plain-text `.mm` parse | **a comment on every statement** |
| iset.mm | about 17,395 statements | CC0 (same repo) | `.mm` parse | per statement |
| Mizar MML | 5.94.1493 (2025-05-30) | GPLv3+ / CC BY-SA 3.0+ | XML export | article prose limited |
| agda-stdlib | 1,183 files | MIT-style | Agda HTML/JSON | comments |
| agda-unimath | 3,072 `.lagda.md` | MIT | literate parse | **literate prose** |
| 1Lab | 710 `.lagda.md` | **AGPL-3.0** | literate parse | **literate prose** |
| Naproche | 32 `.ftl.tex` | GPL-3.0 | ForTheL | **the source is CNL** |
| Megalodon | about 160k lines, more than 1.5k lemmas (2026-01-06) | system MIT-style; mgwiki repos **no licence** | Megalodon | none |
| NASALib | 63 or 64 libraries, about 41K formulas | public domain (NASA) plus per-library | PVS | comments |
| ACL2 books | 40,929 tree entries | BSD-3 plus per-file | ACL2 | doc strings (XDOC) |

### 5.2 Datasets

| Dataset | Size | Licence | Informal side | Quality (measured) |
|---|---|---|---|---|
| miniF2F (Lean ports) | 488 (244 test) | MIT / Apache | competition NL | more than 50% discrepancies (arXiv:2511.03108); 36 certified defects (arXiv:2606.29493) |
| ProofNet | 371 | MIT | textbook NL | 55.9% erroneous (ProofNet-Verified) |
| ProofNet# | 371 | MIT | same | 27.0% still erroneous |
| ProofNet-Verified | 367 | Apache-2.0 | same | target Lean v4.34.0 |
| PutnamBench | 672 L / 640 I / 412 C | Apache (L, I), MIT (C); NL **by MAA permission** | Putnam NL | no audit number (not in the arXiv:2606.29493 table) |
| FormalMATH | 5,560 | MIT (HF) / Apache (GH) | Olympiad/college NL | 147 certified defects / 3,463 findings |
| DeepSeek-ProverBench | 325 | **none** | AIME/textbook NL | 208 certified defects |
| Lean Workbook (+Plus) | 57,231 + 82,893 (HF default 25,214) | Apache-2.0 | AoPS NL | 93.5% (sample) |
| Goedel-Pset-v1 | 1,732,594 | **none** | Numina NL | compile plus Qwen FC filter; no error rate |
| Goedel SFT v2 | 1,745,010 | Apache-2.0 | – | – |
| NuminaMath-LEAN | 104,155 | Apache-2.0 | Numina NL | card admits errors; no rate |
| Kimina-Prover-Promptset | 24,418 | Apache-2.0 (text only) | Numina NL plus Gemini variants | – |
| DeepSeek-Prover-V1 | 27,503 | DeepSeek "other" | NL | – |
| Herald | 579,883 + 44,553 proofs | Apache-2.0 | LLM informalization | no informalization score |
| MMA | ~332K | UNVERIFIED | GPT-4 informalization | "noisy" |
| FrenzyMath mathlib_informal v4.19.0 | 218,071 | Apache-2.0 | LLM | none |
| LeanSearch v2 JSONL | 310,579 | Apache-2.0 | LLM | retrieval only |
| mathlib-initiative/mathlib-types | 797,294 | Apache-2.0 | human docstrings | 14.5% coverage |
| TheoremGraph matching | 100,831 pairs (47,952 accepted) | **CC BY-SA 4.0** | real arXiv text | judge 48% / 87% at sim ≥ 0.9 |
| MathAtlas | 36,264 | **none** | textbook NL | best 2.6% hard |
| AgenticCommons autoformalization | 3,623 | CC0 | NL | compile and axiom gates |
| LeanDojo / LeanDojo-v2 | Benchmark 4 size UNVERIFIED | MIT / (Apache vs MIT conflict) | none | – |
| ntp-mathlib | 307,049 | – (not recorded) | none | – |
| FineMath | 48.3M rows; 34B tok (3+) | ODC-By + CC ToU | web math | classifier-filtered |
| OpenWebMath | 6.3M docs, 14.7B tok | ODC-By + CC ToU | web math | – |
| Proof-Pile-2 | 55B tok | upstream licences | arXiv/web/code | – |
| MathPile | 9.5B tok (UNVERIFIED) | **CC BY-NC-SA 4.0** | mixed | exclude |
| AutoMathText | 4,371,703 rows | CC BY-SA 4.0 | web/arXiv/code | Qwen-72B scored |

(The PutnamBench row: the defect study's table did not list PutnamBench, so I have no audit number for it.)

### 5.3 Changes against the spec

| Spec 01 value | Value now (read 2026-10-26) |
|---|---|
| Mathlib 291,178 thm / 139,569 def | 291,237 / 139,616 |
| Mathlib 9,437 files | 8,661 `.lean` under `Mathlib/` (a definitional difference) |
| AFP 1,038 / ~326,500 / ~5.44M | 1,042 / ~330,000 / ~5,518,000 |
| Isabelle2026 [U] | released (October 2026) |
| Agda sizes [U] | stdlib 1,183, agda-unimath 3,072, 1Lab 710 files |
| Megalodon Munkres 130k lines | about 160k lines, more than 1.5k lemmas (arXiv:2601.03298) |
| Mizar 1,498 articles | version string says 1493, a conflict |
| Metamath 100: 75 | 75 (mm_100); Wiedijk lists 74 |

### 5.4 Licence red flags for ingestion (spec 09.4)

- **No ML use or NC:** ATLAS v1 (CC BY-NC plus no-ML rider), ATLAS v2 preamble, AutoformBench (CC BY-NC), MathPile (CC BY-NC-SA).
- **Share-alike / copyleft:** Mizar (GPLv3 / CC BY-SA), 1Lab (AGPL-3.0), TheoremGraph (CC BY-SA 4.0), AutoMathText (CC BY-SA 4.0), lean4oeis (CC BY-SA 4.0), Naproche (GPL-3.0).
- **No licence at all:** Goedel-Pset-v1, Lean-workbook-proofs, DeepSeek-ProverBench, MathAtlas, LeanAgent, mgwiki Megalodon repos, NASALib (no SPDX; per-library).
- **Third-party text:** PutnamBench informal statements (MAA permission).
- **Clean and permissive:** Mathlib, the mathlib-initiative datasets, Tau Ceti, Formal Frontier, Herald, the FrenzyMath datasets, NuminaMath-LEAN, Goedel SFT v2.
- **Public domain:** set.mm and iset.mm (CC0), 1000+ list (Unlicense), AgenticCommons (CC0).

---

## 6. Open doubts

1. **Mizar article count.** The version string 5.94.1493 implies 1,493 articles; spec 01 says 1,498. The 73,460 theorems figure was not rechecked.
2. **Tau Ceti size.** It is unclear whether Xena's "3 million lines" refers to Tau Ceti or to Mathlib. The stats page returned 404. Only the file count (18,849) is confirmed.
3. **Formal Frontier licences.** Only 30 of 50 repos were checked, and all were Apache-2.0. The remaining 20 are unchecked.
4. **ATLAS v2 legal status.** The Apache-2.0 grant opens with a "not intended for … ML" sentence. Whether that is binding is a legal question.
5. **1000+ per-system counts.** The web page showed about 228 formalized entries, possibly one page of a paginated list. Spec 01's "241 in Lean" is unconfirmed; it needs a parse of the 1,200 YAML files (one tarball fetch).
6. **Metamath counts.** 51,207 statement numbers (including axioms and definitions) against "over 41,000 counting mathboxes" against Wiedijk's 74 / mm_100's 75. The pages count different things.
7. **Lean Workbook rows.** The paper and card give 57,231 + 82,893, but the default HF config has 25,214 rows. Other configs or splits may hold the rest.
8. **Coquelicot licence.** The Inria GitLab is blocked by Anubis. The fork is LGPL-3.0; upstream is unconfirmed.
9. **LeanDojo-v2 licence.** The GitHub API says Apache-2.0 and the README says MIT. The size of LeanDojo Benchmark 4 was not rechecked.
10. **MathPile token count.** 9.5B is from memory or secondary sources; the card is gated.
11. **EFG judge rates** (arXiv:2506.10903): search snippet only.
12. **miniF2F "over 300 corrected"**: search snippet only. The "more than half" figure is from the abstract.
13. **MMA licence**: not stated on the arXiv page.
14. **ITPEval data availability**: no data URL found.
15. **Model-terms pass-through.** Datasets generated with DeepSeek-V3, Qwen2.5-72B, Gemini or Kimina-72B carry permissive tags. Whether upstream model terms constrain the data is unresolved.
16. **Isabelle2026 distribution size and HOL-Analysis counts**: not found.
17. **HOL Light "~37,000 objects in 635 files"**: from Blanqui's talk, a secondary source. The repo shows 664 `.ml` files.
18. **Lean Pool** (arXiv:2609.25199): size and licence of imported projects are unknown.
19. **Mathlib declaration count definition.** 291k + 139k (stats page) against 483k `allowCompletion` against 797k constants (lean_scout). M2Extract must pick one definition and record it.
