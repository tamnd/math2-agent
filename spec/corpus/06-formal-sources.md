# K06. Formal sources

The formal libraries are the second large source of the corpus, after the human sources of K05. Mathlib alone has more than 430,000 named theorems and definitions. The other proof assistants have about as many again. This document says what the corpus takes from each library, by which route, under which licence, and what it does not take. It also covers the datasets that pair formal and informal text, and the translators between systems.

Spec 07 says how a binding works, and spec 07.10 says which systems can give E5. This document does not repeat those sections. Each number comes from K01, part C, read on 2026-10-26. Where a number has changed since spec 01, K6.11 gives the new value.

## K6.1 What a library gives the corpus

A formal library gives four things. Each has its own rule:

1. **Mentions.** Each declaration is a mention in the inventory (K4.2). Most declarations get the outcome `formal_only` (K4.5), and only the ones that a person cites, or that the library marks as main results, become nodes.
2. **Bindings.** A Lean declaration can be the binding of a node, and so give E5 (spec 07.3). In v1, only Lean libraries give bindings.
3. **Crosswalks.** A declaration in another system is a `same_as` target of a node (spec 05.3). It helps the clustering (K4.3, pass 1), and it is evidence of an independent formalization towards F3 (spec 07.10).
4. **Informal text.** Docstrings, comments and literate prose. The corpus takes this text only when the licence of the library allows it in the prose zone (K13.1).

## K6.2 Intake classes

Each library has one intake class. The source document of the library in `manifests/sources.yaml` records it, with the licence and the reason:

| Class | The corpus takes | Example |
|---|---|---|
| `bind` | mentions, bindings, crosswalks and the informal text | Mathlib, Formal Frontier |
| `crosswalk` | mentions and crosswalks. Informal text only if the licence allows it | the AFP, MathComp, set.mm |
| `link` | crosswalks and links. No text, no statements | 1Lab, ATLAS |
| `evaluate` | nothing for the graph. The dataset tests the roles of K07 | miniF2F, ProofNet, PutnamBench |
| `exclude` | nothing | MathPile |

The class follows from three questions, in this order:

1. **Can we use it at all?** A non-commercial licence, a clause against machine learning, or no licence at all gives `link` for a library and `exclude` for a dataset. A dataset with no licence can become `evaluate` only when its authors confirm its terms in writing.
2. **Can its text go into the prose zone?** Apache-2.0, MIT, BSD, CC0 and the Unlicense allow it, with attribution. CC BY-SA allows it, with share-alike and attribution. GPL, AGPL, LGPL and CeCILL-C are licences for software, and math2 does not put their comments into prose. Their libraries give mentions and crosswalks only.
3. **Can it give bindings?** Only Lean 4 libraries, in v1 (spec 07.10).

## K6.3 The libraries

| Library | Size (date) | Licence | Class | Route | Informal text |
|---|---|---|---|---|---|
| Mathlib | 291,237 theorems and 139,616 definitions (stats page); 797,294 constants, 483,112 with `allowCompletion` (v4.35.0, 2026-10-13) | Apache-2.0 | `bind` | lean_scout datasets (K6.4) | docstrings on about 14.5% of constants |
| Tau Ceti | 18,849 `.lean` files | Apache-2.0 | `bind`, with the flag of K6.5 | as Mathlib | docs site; written by AI |
| Formal Frontier | 41 libraries, about 200k lines | Apache-2.0 (30 of 50 repositories checked) | `bind` | as Mathlib | per library |
| Other Reservoir packages | 908 packages | per package | per package | Lake | per package |
| ATLAS (Meta) | 26 textbooks, more than 45,000 declarations | v1: CC BY-NC 4.0 with a clause against ML. v2: Apache-2.0 with a preamble against ML | `link` | none | textbook-aligned |
| Isabelle AFP | 1,042 entries, about 330,000 lemmas | BSD-style or LGPL, per entry | `crosswalk` | the release tarball, Isabelle export | abstracts and `text` blocks; BSD entries only |
| Isabelle2026 library | released October 2026; size not known | BSD-style core, mixed | `crosswalk` | Isabelle export | `text` blocks of BSD parts |
| Rocq Stdlib | 904 `.v` files | LGPL-2.1 | `crosswalk` | opam | none |
| MathComp, Odd Order | 147 and 34 `.v` files | CeCILL-B | `crosswalk` | opam | header comments |
| MathComp Analysis | 131 `.v` files | CeCILL-C | `crosswalk` | opam | none |
| UniMath (Rocq) | 1,714 `.v` files | MIT-style | `crosswalk` | opam | comments |
| HOL Light, Flyspeck | 664 `.ml` and 503 `.hl` files | BSD-style, MIT | `crosswalk` | hol2dk dump | comments |
| HOL4 | 1,495 `Script.sml` files | BSD-style | `crosswalk` | HOL4 export | comments |
| Metamath set.mm, iset.mm | 51,207 and about 17,395 statement numbers | CC0 1.0 | `crosswalk`, with text (K6.6) | parse of the `.mm` file | a comment on each statement |
| Mizar MML | version 5.94.1493 | GPLv3+ or CC BY-SA 3.0+ | `crosswalk` | XML export | little |
| agda-stdlib | 1,183 files | MIT-style | `crosswalk` | Agda HTML | comments |
| agda-unimath | 3,072 `.lagda.md` files | MIT | `crosswalk`, with text (K6.6) | literate parse | prose on each page |
| 1Lab | 710 `.lagda.md` files | AGPL-3.0 | `link` | none | prose, not taken |
| Naproche | 32 `.ftl.tex` files | GPL-3.0 | `crosswalk` | ForTheL | not taken |
| Megalodon | about 160,000 lines | system MIT-style; the mgwiki repositories have no licence | `link` | none | none |
| NASALib (PVS) | 63 or 64 libraries, about 41,000 formulas | public domain for NASA work, per library for the rest | `crosswalk`, per library | PVS | comments |
| ACL2 books | 40,929 files and directories | BSD-3 and per file | `crosswalk`, per file | ACL2 | XDOC |

Two rules apply to each row:

- **The licence is recorded for each item where the library has a licence for each item.** The AFP has one for each entry, NASALib for each library, the ACL2 books for each file. The importer reads it, and rule F10 checks it.
- **The licence is read again at each pin.** A library that changes its licence changes its class from that pin on. Mentions from older pins keep the licence that they had.

## K6.4 Mathlib

**The route.** The Mathlib Initiative publishes the datasets of `lean_scout` (Apache-2.0) for each toolchain bump: `mathlib-types` (name, module, type, docstring, `allowCompletion`), `mathlib-const-dep` (the dependencies of each constant) and `mathlib-tactics` (goal states). The importer reads these datasets at the commit of the release pin. It does not run its own walk over the environment for mentions. `M2Extract` (spec 07.4) still runs in the build, for the bound declarations and their closure only, because the type hash and the axioms of a binding must come from the pinned build.

**What a mention is.** The three published counts measure different things: 430,853 named theorems and definitions on the stats page, 483,112 constants that the user can complete, and 797,294 constants in all. The corpus takes the second: a mention is a constant with `allowCompletion = true`. The other constants are internal or generated, and no person would cite them. The report gives all three numbers, with the definition that each one uses.

**Which mentions become nodes.** The rule of K4.5 applies. For Mathlib the corpus uses these signals, in order:

1. a human mention in the same row (pass 1 of the clustering uses the Wikidata and the docstring links);
2. the named-theorem lists: the "100 theorems" and "1000+ theorems" files, and the main results that module documentation names;
3. a declaration that a node already uses, or that a person asks for;
4. a definition that the statement of a selected theorem uses (`uses_constants_in_statement`).

A rough guess, not a measurement, is that 40,000 to 80,000 Mathlib declarations meet one of these. The first import measures the real number, and the report gives it.

**Informal text.** About 115,876 constants have a docstring, with a mean length of about 130 characters. A docstring is Apache-2.0 text that a person wrote, so it can be the start of a summary (K8.3), with attribution. The informalize role (K7.3) writes the statement of each selected declaration that has no human mention and no usable docstring.

**AI-written informal datasets are not text sources.** Herald, the FrenzyMath datasets, LeanSearch v2 and others hold informal text that a model wrote for Mathlib declarations. Their licences are permissive, but no study measures their error rate, and most of them do not name the model. Rule X08 needs each model that writes text for math2 to be in the model register. So these datasets are search aids and inputs to the clustering only, never text of a node.

## K6.5 Libraries that AI wrote

Tau Ceti is mostly written by AI, and reviewed against rubrics that people wrote. Megalodon's Munkres formalization was written with models. Some AFP entries say that AI helped with proof engineering, and NASALib has a `CLAUDE.md`. Other libraries will follow.

The kernel checks the proofs of each of these libraries, so the proofs are as good as any others. The risks are in the parts that the kernel does not check: whether a statement says what its name and docstring say, and whether its definitions are the standard ones.

So each library and each entry has a field `authored` in its source document, with the value `human`, `ai` or `mixed`, and the reason. The importer copies it to each mention. Then:

- **Docstrings of `ai` libraries are agent text.** They go through the copy check (K9.4), and they count as text that an agent wrote for rule P04. A person must confirm them before a node reaches W3.
- **Bindings to `ai` libraries need the F2 review of spec 07.3 like any other binding.** In addition, a binding to an `ai` declaration whose statement uses a definition of the same library needs the review to cover that definition too.
- **The report gives the share of bindings to `ai` libraries.**

The value `mixed` is for a library where both people and models write, with no record of which. It has the rules of `ai`.

## K6.6 Other systems

**No system translates a whole library into Lean 4 today.** hol2dk translates HOL Light to Rocq (40,728 theorems of Multivariate, a dump of about 135 GB), and not to Lean. Mathport is archived. lean2dk is still work in progress. In ITPEval, the best model translated 29.1% of statements and 10.5% of proofs between systems at pass@1, and an equivalence check confirmed only 54% of the translations that compiled. So the other systems enter the corpus as crosswalks, not as Lean. When a node needs a Lean binding of a result that only another system has, the formalize role writes it, with the source declaration in its context, and the binding goes through the F1 battery like any other.

**Metamath set.mm is the best informal and formal source after Mathlib.** It is CC0, and each statement has a comment that a person wrote, usually a sentence or more. The importer takes each comment as `open` text, with attribution (which CC0 does not need, but rule 2 of K13.8 does). A statement whose comment is a full sentence starts at W1, after the copy check and the body rules. The set.mm labels are `same_as` targets.

**agda-unimath** is MIT, and its pages are literate Markdown, with prose next to each definition. The importer takes the prose as `open` text, with the MIT notice in the attribution. It is the best source for definitions in homotopy type theory and univalent foundations.

**The AFP.** Each entry has an abstract, and the theories have `text` blocks. The importer takes the text of BSD entries as `open` text, and the text of LGPL entries not at all. The AFP is the largest source of crosswalks after Mathlib.

**Mizar.** The MML is dual-licensed, and one choice is CC BY-SA 3.0, which allows a later version (CC BY-SA 4.0) for adaptations. Its text is formal, and there is little prose. The corpus uses it for crosswalks only. The article count is not settled: the version string says 1,493 and spec 01 says 1,498 (K15).

**The others** (Rocq, HOL Light, HOL4, Naproche, NASALib, ACL2) give mentions and crosswalks. Their comments are not taken as text in v1.

## K6.7 Crosswalks

A crosswalk links a row to a declaration in a library. The corpus builds them in this order:

1. **From the libraries themselves.** Many declarations name the theorem that they state, in the docstring or in a list.
2. **The 1000+ theorems list.** It is in the public domain (the Unlicense), and it has 1,200 YAML files, one for each theorem, keyed by the Wikidata identifier, with MSC codes, links to Wikipedia and blocks for Isabelle, HOL Light, Rocq, Lean, Metamath and Mizar. It is the first crosswalk that the corpus imports. Wiedijk's 100 theorems list is the second. It has no licence, so the corpus takes only its facts: which theorem has a formalization in which system.
3. **Matching datasets.** TheoremGraph pairs 388,105 Lean declarations with real arXiv statements (not model text), with 47,952 to 61,210 pairs that a model judged to match. It is CC BY-SA 4.0. The Stacks-to-Mathlib alignment (MIT) pairs Stacks tags with Mathlib. The corpus uses these datasets as candidates. The align role judges each pair again, and an accepted pair becomes an edge of math2, with the dataset in its source. The corpus does not copy their text, and a pair that math2 did not judge is not an edge.
4. **The type hash and the clustering.** Two Lean declarations with the same type hash state the same thing (spec 07.4). The other systems need the align role.

## K6.8 Datasets

**Benchmarks are for evaluation only.** Their errors are measured, and they are many:

| Benchmark | Errors that a study found |
|---|---|
| miniF2F (Lean ports) | the formal and informal statements disagree in more than half of the problems |
| ProofNet | 55.9% of items have errors (ProofNet-Verified) |
| ProofNet# (the fixed version) | still 27.0% |
| DeepSeek-ProverBench | 208 certified defects in 325 problems |
| FormalMATH | 147 certified defects |

So a benchmark item never becomes a mention, a statement or an exercise of the corpus. The benchmarks test the roles of K07 (class `evaluate`). PutnamBench has one more rule: its informal statements are used by permission of the MAA, so math2 does not copy them anywhere, not even into a test set that it publishes.

**Synthetic statement sets** (Lean Workbook, Goedel-Pset, NuminaMath-LEAN and others) hold millions of formal statements of competition problems, made by models. Most of them have a measured fidelity of 60 to 90%, or no measure. Their informal side is often text from a third party, such as Art of Problem Solving. They are `evaluate` at most. An exercise of the corpus (K8.7) is not taken from them.

**Pretraining corpora** (OpenWebMath, FineMath, Proof-Pile-2, AutoMathText) are not sources. OpenWebMath is in the reference corpus of the copy check, for detection only (K9.4). MathPile is `exclude` (CC BY-NC-SA).

**Red flags.** The importer refuses a dataset in each of these groups until a person changes its class in the source document:

| Flag | Datasets |
|---|---|
| non-commercial, or a clause against ML | ATLAS v1 and v2, AutoformBench, MathPile |
| no licence | Goedel-Pset-v1, Goedel Lean-workbook-proofs, DeepSeek-ProverBench, MathAtlas, LeanAgent, the mgwiki Megalodon repositories |
| licence conflict | DeepSeek-Prover-V2 (MIT file against the Model Agreement), LeanDojo-v2 (Apache against MIT), FormalMATH (MIT on Hugging Face, Apache on GitHub) |
| third-party text | PutnamBench informal statements |
| made by a model under terms that may pass on | data from DeepSeek-V3, Qwen2.5, Gemini or Kimina models (K7.9) |

## K6.9 Informalization

The formal track is large and the human track is small for the same items: only 14.5% of Mathlib constants have a docstring. So the informalize role writes much of the text of the formal layer. K7.5 gives the job, and K9.7 gives the audit. Two rules from the evidence:

- **Compiling is not faithfulness.** A statement that compiles can still say something else. Each informalized statement has the F1 battery of spec 10 on the pair, and the sample audit checks fidelity as its own property.
- **There is no published error rate.** No study measures informalizations of Mathlib at scale. The closest number (FormL4) is 70 to 80% correct, in a different setting. The informalize role is its own stratum from its first merge, and the corpus publishes the first measured rate.

## K6.10 Pins and bumps

Each library is pinned in `manifests/sources.yaml`, with a commit and a toolchain (spec 07.1). The importer reads the library at its pin. When the pin moves:

- New declarations become new mentions.
- A removed or renamed declaration keeps its mention, with `retired` and its successor, if the library gives one (Mathlib's deprecation aliases do).
- A declaration whose type hash changed is a new mention. The bump role (spec 07.9) handles the bindings.
- The report gives the change in mentions, rows and nodes for each library, for each bump.

## K6.11 Corrections to spec 01

| Spec 01 value | Value on 2026-10-26 |
|---|---|
| Mathlib 291,178 theorems and 139,569 definitions | 291,237 and 139,616 |
| Mathlib 9,437 files | 8,661 `.lean` files under `Mathlib/`. Spec 01 probably counted all files |
| AFP 1,038 entries, about 326,500 lemmas, about 5.44M lines | 1,042, about 330,000, about 5.518M |
| Isabelle2026 not known | released in October 2026 |
| Agda sizes not known | agda-stdlib 1,183 files, agda-unimath 3,072, 1Lab 710 |
| Megalodon Munkres 130k lines | about 160k lines and more than 1,500 lemmas |
| Mizar 1,498 articles | the version string says 1,493 (not settled) |
| Metamath set.mm more than 40k theorems (2022) | 51,207 statement numbers, including axioms and definitions |
| Tau Ceti about 3M lines | 18,849 files. The source of "3M lines" may mean Mathlib (not settled) |

## K6.12 Rules

| Rule | H/S | Checks |
|---|---|---|
| F10 | H | each mention from a library records the library, the pin, the declaration name, the intake class and the licence of the item (K6.3) |
| F11 | H | no section of a node holds text from a library whose class is `link`, `evaluate` or `exclude`, or from an item whose licence is not open (K6.2) |
| F12 | H | each source document of a library has the field `authored`, and each mention copies it (K6.5) |
| F13 | S | a binding to an `ai` or `mixed` declaration whose F2 review does not cover the definitions of that library that its statement uses (K6.5) |

The open questions of this document are in K15: the Mizar article count, the size of Tau Ceti, the licences of the other 20 Formal Frontier repositories, the effect of the ATLAS v2 preamble, the Coquelicot licence, and whether the terms of a model pass on to the data that it made.
