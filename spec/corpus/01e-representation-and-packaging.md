# K01, part E. Representation, pedagogy, exercises, packaging and notation

Research for the Math 2.0 spec (tamnd/math2). This file builds on spec 01 §1.7 (semantic formats), 01 §1.2 (blueprints), 04 (object model) and 13 (research workspace), and tries not to repeat them. Facts those documents already state are only referred to.

Rules for this file:
- I read every source on **2026-10-26**, unless the entry says otherwise.
- A fact I could not confirm is marked **UNVERIFIED**.
- "From search" means I saw the claim only in a search-result excerpt, not on the page itself.

---

## 0. What the spec already says (so it is not repeated)

**Spec 04:**
- **Node kinds:** concept, definition, statement, conjecture, problem, example, counterexample, object, proof, computation, topic.
- **Tags:** 6-character Crockford base32, permanent.
- **Body sections:** fixed per kind.
- **Explanations:** levels L1 to L4, each marked human-written or agent-written.
- **Math and links:** math is LaTeX limited to the macros in `macros/m2.sty` and `m2.katex.json`; links are `[[text|TAG]]`.
- **Status:** computed, never written by hand.

**Spec 13:**
- a fixed page model;
- prerequisite paths that start from the reader's background;
- a "Translate notation" feature;
- project blueprints generated from the graph.

**Spec 01 §1.7 already covers:**
- MathML 4 is still a Working Draft and has `intent`;
- MathML Core is a CR (June 2025);
- OpenMath 2.0 is inactive;
- sTeX4 = RusTeX + FLAMS + MathHub;
- arXiv HTML is made with LaTeXML;
- Wikidata math counts and P14534;
- MathGloss.

**Spec 01 §1.2 already covers:** leanblueprint, Verso Blueprint, LeanArchitect, Naproche and Megalodon.

What follows is new detail, plus a decision for each item.

---

## 1. Representation

### 1.1 OMDoc and MMT

**What it is**
- OMDoc is an XML format for mathematical documents. It has three levels:
  - object: formulas, in OpenMath or Content MathML;
  - statement: definitions, theorems, proofs, examples;
  - theory: theories, imports and views.
- The project started in 1998.
- KWARC says "the formal part of OMDoc has been refined into the MMT format", with the "MMT System as the reference implementation". KWARC is "extending MMT to cover the full flexiformal coverage of OMDoc".
- Source: https://kwarc.info/systems/omdoc/

**MMT in 2026**
- MMT is written in Scala and stores content in "archives".
- Repo: https://github.com/UniFormal/MMT. It has 7,458 commits and no releases listed on GitHub.
- The licence is in `COPYING.txt`. I did not read it, so the licence is **UNVERIFIED**.
- KWARC is now at FAU Erlangen-Nürnberg.

**Take for math2**
- The **theory/view** idea: content lives in theories, and a view maps one theory into another (§4.6, Realms).
- The **object/statement/theory** split matches math2's formula / node / topic split. math2 needs no XML for this.
- Do not adopt OMDoc as a storage format. Its tooling is research-grade.

### 1.2 sTeX and SMGloM

**sTeX 4**
- Version 4.1.0, dated 2026-06-24, under LPPL. Maintainers: M. Kohlhase and D. Müller. https://www.ctan.org/pkg/stex
- The toolchain (https://kwarc.info/systems/sTeX/):
  - RusTeX converts LaTeX to FTML (HTML with semantic annotations);
  - FLAMS, the "Flexiformal Annotation Management System", is GPL-3.0 (https://github.com/FlexiFormal/flams);
  - MathHub hosts the archives;
  - there is a VS Code IDE.
- The older LaTeXML plugin is deprecated.
- mathhub.info now shows a FLAMS-branded interface.

**Syntax**

From the manual (https://ctan.math.illinois.edu/macros/latex/contrib/stex/doc/stex-manual.pdf). I saw these in excerpts, not by reading the whole PDF.
- **Modules:** `\begin{smodule}{Name}` opens a module. A symbol must belong to a module.
- **Symbols and notations:**
  - `\symdecl{name}` declares a symbol, which is an identity with no notation. `\symdecl*` declares one without a semantic macro.
  - `\notation{name}[variant]{…}` attaches a notation to a symbol. A symbol can have several notations.
  - `\symdef` is `\symdecl` plus `\notation` in one command. Example:
    `\symdef{plus}[name=addition,args=2,type=\funspace{\Nat,\Nat}{\Nat}]{#1 \mathbin{\maincomp{+}} #2}`
  - `prec=` sets precedence for bracketing. `args=a` marks a flexary (associative) argument.
- **Variables:** `\vardef` declares a variable.
- **Imports:**
  - `\importmodule` is transitive. Example: `\importmodule[sTeX/MathBase/Functions]{mod?Function}`.
  - `\requiremodule` is not transitive.
  - `\usemodule` is for use outside a module.
- **Definitions:** `sdefinition` with `for=`, plus `\definiendum` / `\definame`. `\defnotation` marks where a notation is introduced.
- The syntax changed incompatibly on 2023-04-25. This shows the cost of tying content to a LaTeX package's macro API.

**SMGloM**
- The CICM 2022 sTeX3 paper (https://kwarc.info/people/dmueller/pubs/cicm22stexsd.pdf) reports:
  - at least 2,250 concepts;
  - verbalisations in English for 93% of them, German 71%, Chinese 11%.
- **Signature modules** (language-independent symbols, "modsig") are separate from **natural-language modules** ("mhmodnl"), one per language.

**SMGloM licences**
- The early SMGloM Public License (SPL 0.1) "protects symbols against non-conservative changes while allowing derived works". https://kwarc.info/people/mkohlhase/submit/cicm14-smglom-system.pdf
- The current `smglom/mathsoft` archive is CC BY-SA 4.0: https://gl.mathhub.info/smglom/mathsoft/-/blob/main/LICENSE
- I did not check the other SMGloM archives. That they share this licence is **UNVERIFIED**.

**ALeA (Adaptive Learning Assistant, KWARC)**
- ALeA is built on sTeX content. It was funded through the VoLL-KI project (BMBF, 2021 to 2025).
- It keeps a learner model with values M(C,D) in [0,1] for each concept C and each cognitive dimension D (Bloom-like).
- It has been used in 6 courses with more than 1,000 FAU students. The platform is at https://courses.voll-ki.fau.de/.
- **Guided tours:** the system lists all prerequisites of a target, removes the ones the learner already knows, and shows the rest as a sequence of definitions. It also generates flashcards and quizzes.
- Sources: https://sme.uni-bamberg.de/ai4ai/p5.pdf ; CICM 2023 paper, https://www.researchgate.net/publication/373539971

**Take for math2**
1. **Separate a symbol's identity from its notations.** math2 macros are currently only render macros. Each macro should declare the concept tag it denotes, its arity and its precedence. One concept may have several notations, and one of them is the default.
2. **Keep the language-independent part apart from the prose part.** Spec 13 §13.4 already keeps formal statements untranslated. The SMGloM split suggests also keeping a per-concept "verbalisation" record per language: the noun phrase, plural and gender.
3. **ALeA's guided tour** is close to the spec 13 prerequisite path. ALeA also tracks a cognitive dimension per concept, and math2 can do the same with the MATH-taxonomy groups (§2.2).
4. Do not depend on the sTeX macros themselves (LPPL, API churn). Export to sTeX/FTML only if someone asks for it.

### 1.3 OpenMath Content Dictionaries

**What it is**
- 218 Content Dictionaries (CDs) are listed at https://openmath.org/cdnames/. Each has a status: official, experimental, private or obsolete.
- `arith1` (version 3, 2004-03-30) defines abs, divide, gcd, lcm, minus, plus, power, product, root, sum, times and unary_minus. https://openmath.org/cd/arith1
- `transc1` follows Abramowitz and Stegun for branch cuts. https://openmath.org/cd/transc1
- The repo https://github.com/OpenMath/CDs was last updated 2022-06-28.
- Content MathML's `cd` / `cdgroup` mechanism refers to OpenMath CDs.

**Licence**

The CD licence is custom:
- verbatim redistribution is allowed;
- a derived work must say that it is derived and is not the original;
- a derived CD must use a different CDName, for example `private_mathN` instead of `mathN`.

There is also the OpenMath royalty-free IPR policy of 2013-12-01: https://openmath.org/documents/openmath-ipr/

**Take for math2**
- Use OpenMath symbol URIs (`cd/name`, for example `arith1/plus`) as **alignment targets** for concept nodes, next to Wikidata QIDs. They are stable and well defined, they include branch-cut conventions, and they are what Content MathML refers to.
- Do not copy CD text into nodes, because of the renaming requirement. Link only.

### 1.4 MathML 4 (presentation, content, intent)

**Status**
- The Editor's Draft is dated 24 October 2026: https://w3c.github.io/mathml/
- The latest published version is still a Working Draft (agrees with spec 01 §1.7). The w3.org/TR page returned HTTP 403 to my fetch, so I read the editor's draft and the charter instead.

**Charter**

The proposed 2026 charter is at https://w3c.github.io/mathml-docs/charter-2026.html.
- **Targets in the timeline:**
  - MathML 4 CR, May 2026;
  - a community intent repository, August 2026;
  - MathML Core Recommendation, December 2026;
  - MathML 4 Recommendation, March 2027.
- **Deliverables table:** Core Recommendation about Q4 2026; MathML 4 complete about Q1 2027.
- **The May 2026 CR target was missed.** As of 2026-10-26 the spec is still a WD.
- **Content MathML:** MathML 4 "largely excludes" new Content MathML work, which is deferred to MathML 5.
- **i18n review:** ran from 2026-03-18 to 2026-05-14. https://github.com/w3c/i18n-request/issues/299

**Intent**
- Syntax example: `intent="power($base,$exponent)"`, with `arg="base"` on the child elements.
- Intent concepts come from two lists in the w3c/mathml-docs repository:
  - a **Core** list: https://w3c.github.io/mathml-docs/intent-core-concepts/ (from search);
  - an **Open** list, to which contributions are invited and which "will never be complete" (Soiffer et al., https://arxiv.org/abs/2407.06720, from search).
- There is also a Core *property* list generated from `core-properties.yml`: https://w3c.github.io/mathml-docs/intent-core-properties/ (from search).
- Intent is meant for speech and assistive technology. It is not a semantic definition.

**LaTeX support**
- The LaTeX Project ships `latex-lab-mathintent` with `\MathMLintent` and `\MathMLarg` (v0.1c, 2026-01-16). https://ftp.icm.edu.pl/packages/tex/macros/latex-dev/required/latex-lab/latex-lab-mathintent.pdf

**Take for math2**
1. Each notation declaration (§1.2, §5) gets an optional `intent:` field holding a MathML intent concept name, from the Core list where one exists. The KaTeX build can then emit `intent` on the MathML it already produces (spec 13 §13.7). This costs very little and gives correct speech output for, say, $f^{-1}$ (inverse) versus $x^{-1}$ (power).
2. Do not build on Content MathML. W3C has deferred it.
3. Contribute missing concepts to the Open list upstream, instead of inventing private names.

### 1.5 LaTeXML

**What it is**
- Version 0.8.8.1 (Chocolatey package updated 2026-02-20: https://community.chocolatey.org/packages/latexml).
- Outputs: XML, HTML, MathML, ePub and JATS. https://github.com/brucemiller/LaTeXML
- The README licence badge reads as public domain / CC0. I did not read the LICENSE file, so the exact licence is **UNVERIFIED**.

**Rust port**
- arXiv:2605.16562 (Ginev, Caruso, Miller, Sank, Weiskoff; 15 May 2026) describes a Rust port that is in progress. It reports:
  - 75% of arXiv converts without errors, against a 90% target;
  - about half of roughly 6,000 user reports resolved;
  - initial work on MathML 4 intent.
- The paper uses the words "candidate recommendation draft" for MathML 4. That does not match the W3C status in §1.4 and looks like loose wording.

**Take for math2**
- LaTeXML is the converter for **ingesting** arXiv LaTeX (already the arXiv HTML path).
- math2 node math is restricted to KaTeX-renderable macros, so math2 does not need LaTeXML to render its own nodes. It needs LaTeXML only for the arXiv source side and possibly for the JATS export (§4.3).

### 1.6 PreTeXt

**What it is**
- An XML authoring vocabulary and conversion toolchain, under GPL (https://github.com/PreTeXtBook/pretext).
- Outputs (https://pretextbook.org/doc/guide/html/the-index.html):
  - HTML; PDF and print (via LaTeX or XSL-FO); EPUB and Kindle;
  - Runestone (interactive); Braille;
  - Reveal.js and Beamer slides; Jupyter notebooks; plain text and Markdown;
  - instructor versions; WeBWorK problem sets.

**Elements** (from the guide)
- **Theorem-like:** definition, theorem, corollary, claim, fact, algorithm.
- **Axiom-like:** assumption, axiom, conjecture, heuristic, hypothesis, principle.
- **Example-like:** example, problem, question. Each can have hint, answer and solution.
- **Remark-like** and **project-like** families also exist.
- **Exercises:**
  - `<exercise>` contains `<statement>`, `<hint>`, `<answer>` and `<solution>`;
  - `<exercisegroup>` groups exercises;
  - `<exercises>` is a whole division of exercises.
- **Proofs:** a proof can be detached from its theorem, and proofs can have `<case>`.
- **Interactive exercises:**
  - `<webwork>` takes a problem from the OPL or local PG source, reused by xml:id;
  - `<myopenmath problem="ID">`;
  - STACK;
  - Runestone types: Parsons, matching, fill-in, clickable.
- **Notation** (https://pretextbook.org/doc/guide/html/topic-notation.html):
  - a `<notation>` element contains `<usage>` (exactly one `<m>`) and `<description>`;
  - it is placed in a definition's metadata or in a paragraph's `<term>`, and it is **invisible where it is placed**;
  - `<notation-list/>` in an appendix generates a three-column table (usage, description, locator), in order of first appearance.
- I found no 2026 release notes.

**Take for math2**
1. **PreTeXt is the best single book target.** One PreTeXt file gives web, PDF, EPUB, braille and WeBWorK output, so math2 does not have to write five back ends. The `m2 book` compiler (§4.5) should emit PreTeXt XML first.
2. Most math2 node kinds map directly onto PreTeXt elements:
   - definition → `definition`; statement → `theorem` / `lemma` / `corollary`; conjecture → `conjecture`;
   - example / counterexample → `example`; proof → `proof`; exercise (new, §3.6) → `exercise`.
   - concept nodes and topic nodes become divisions and introductions.
3. Copy the **notation-list** model: a definition node declares the notation it introduces, and the book compiler builds the "List of notation" in reading order.

### 1.7 Lean Verso, Verso Blueprint, leanblueprint, doc-gen4

**Verso** (https://github.com/leanprover/verso; https://verso.lean-lang.org/)
- Licence: Apache-2.0. The vendored JavaScript is MIT.
- Genres: manual, blog/website, book, API docs, blueprint and paper.
- `xref.json` and `lake exe verso sync` give experimental cross-document references.
- Verso powers the Lean reference manual and lean-lang.org. L. de Moura moved his website to Verso on 2026-03-15: https://leodemoura.github.io/notes/2026-3-15-this-website-is-now-powered-by-verso/

**Mathlib docstrings in Verso** (all PRs by Vierkantor)
- mathlib4 #44635 (8 October 2026) enabled `doc.verso` in the first file.
- mathlib4 #45031 turned it on for 1,352 files, with no syntax change and identical doc-gen output.
  - It was merged on 20 October 2026, then reverted because of lean4#15817, a scoped-instance race.
  - The fix, lean4#15839, landed on 21 October.
  - Sources: https://github.com/leanprover-community/mathlib4/pull/45031 ; https://github.com/leanprover/lean4/issues/15817
- doc-gen4 does not render Verso docstrings yet, so they are converted back to Markdown.
- mathlib4 #44898 changed the Stacks/Kerodon cross-reference attribute so that it writes into the Verso docstring.
- **Consequence:** Mathlib docstring syntax is moving to Verso during 2026–27. Any math2 tool that scrapes Mathlib docstrings, for binding prose or for "Main definitions" sections, must handle both Markdown and Verso.

**Verso Blueprint** (https://github.com/leanprover/verso-blueprint; extends spec 01 §1.2)
- **Directives:** `:::definition "label" (lean := "Nat.add, Nat.succ")`, `:::theorem "label" (owner := …) (priority := …)`, `:::proof "label"`.
- **Dependencies:** written inline as `{uses "foo"}[]` or as `(uses := "foo, bar")`.
- **Lean side:** `@[blueprint "label"]` on Lean declarations. With `(autoDeps := true)`, edges are derived from Lean and recorded with origin "automatic".
- **Tooling:**
  - `--dump-manifest` writes a "semantic manifest";
  - `blueprint-html-cache.json` is a cache;
  - `lake exe vbp query status <label>` reports status and separates *known holes* from *unverified coverage*.
- **Catalogue:** the README lists `ejgallego/verso-flt` and `verso-sphere-packing` (Lean v4.35). Noperthedron and Carleson are excluded until they move from v4.34. v4.35 is the only maintained line.
- The FLT repository (https://github.com/ImperialCollegeLondon/FLT) contains **both** a `blueprint` folder and a `blueprint-verso` folder. So spec 01 §1.2's statement that "FLT, Carleson, Sphere Packing have moved to it" is too strong: FLT runs both, and Carleson is not yet in the catalogue (see Doubts).
- I could not fetch the licence (the raw LICENSE returned 404). **UNVERIFIED.**

**leanblueprint** (https://github.com/PatrickMassot/leanblueprint)
- A plasTeX plugin plus `plastexdepgraph`.
- Macros: `\lean`, `\leanok`, `\uses`, `\mathlibok`, `\notready`, `\proves`, `\discussion`.
- `checkdecls` checks that the Lean declaration names exist.
- Node colours: not_ready orange; can_prove blue; proved green; fully_proved dark green; mathlib dark green.
- **Pitfalls for importers:** a node that `\uses` itself causes a RecursionError; a remark inside `\uses` stops its dependants from becoming fully_proved.
- **LeanArchitect** (ITP 2026, LIPIcs 382, paper 25: https://drops.dagstuhl.de/storage/00lipics/lipics-vol382-itp2026/LIPIcs.ITP.2026.25/LIPIcs.ITP.2026.25.pdf) fills these macros automatically from Lean. There, `\leanok` means that `sorryAx` is absent from the declaration's closure.
- KnowTeX (arXiv:2601.15294) is a related tool. I did not read it in detail.

**doc-gen4** (https://github.com/leanprover/doc-gen4)
- Apache-2.0. Built with `lake build Lib:docs`. Environment variables include `DOCGEN_SRC` and `DISABLE_EQUATIONS`.
- I did not confirm whether it publishes machine-readable declaration data that math2 could consume. **UNVERIFIED.**

**Mathlib documentation style** (https://leanprover-community.github.io/contribute/doc.html)
- Math is written `$…$` or `$$…$$`, or `\begin{…}` environments without dollars, to match the MathJax configuration of the online docs.
- A module docstring has these sections, in order: **Main definitions, Main statements, Notation** (required if the file introduces notation), **Implementation notes, References, Tags**.
- References are keys into `docs/references.bib`, for example `[Boole1854]`.
- Named theorems are written in bold.
- The `docBlame` and `docBlameThm` linters enforce docstrings.

**Take for math2**
1. **Import:** `m2 ingest blueprint` (spec 09.4) should read the Verso Blueprint semantic manifest when one exists, and fall back to leanblueprint's plasTeX output. Keep the "automatic" versus "manual" edge origin as edge provenance. Map "known hole" and "unverified coverage" onto evidence levels without collapsing them.
2. **Export:** math2 topic nodes with Lean bindings can be emitted as a Verso Blueprint document (`:::definition "TAG" (lean := …)`). The math2 tag is the blueprint label, and `uses` comes from the graph. This gives a Lean project a blueprint generated by math2.
3. Model math2's per-file conventions on Mathlib's module-doc sections. A math2 topic node should have **Main definitions, Main statements, Notation, Conventions, References** sections. That matches what formal-library readers already expect.
4. Spec 21's open question on Verso documents: accept Verso only as an **export** format, not as a node source format. Node bodies stay Markdown + LaTeX (spec 04).

### 1.8 Wikidata formulas and Wikifunctions

**Wikidata properties**
- P2534 "defining formula".
- P7235 "in defining formula": one statement per symbol, with the qualifier P9758 "symbol represents". https://www.wikidata.org/wiki/Property:P7235
- P7973 "quantity symbol (LaTeX)".
- The `math` datatype stores LaTeX strings.
- Older usage counts: P2534 about 5,166 uses and P7235 about 703 (Scharpf et al. 2021, https://gipplab.uni-goettingen.de/wp-content/papercite-data/pdf/scharpf2021b.pdf). Current counts are not checked here; spec 01 §1.7 has the 2026 totals.
- A 2026 dataset of 2,405 formulae: arXiv:2608.02457 (title not recorded here).

**Wikifunctions**
- The Abstract Wikipedia beta started in the week before 2026-03-26: https://www.wikifunctions.org/wiki/Wikifunctions:Status_updates/2026-03-26. The exact date of 20 March is from search only.
- Wikifunctions passed 4,000 functions in mid-April 2026. The 4,000th was subtraction of complex numbers. https://www.wikifunctions.org/wiki/Special:MyLanguage/Wikifunctions:Status_updates/2026-04-16
- Its math functions are few and elementary, for example Z32261, congruence mod n.

**Take for math2**
1. The **P7235 + P9758 pattern** (one statement per symbol in a formula, naming the item the symbol stands for) is the same as a math2 notation declaration attached to a formula. Export math2 symbol declarations to it when aligning to Wikidata.
2. Wikifunctions is of low relevance for now. At most, a `computation` node could link to a Wikifunctions ZID as a further implementation.

### 1.9 Naproche and ForTheL

**What it is**
- Licence: GPL-3.0. https://github.com/naproche/naproche
- It uses the automated provers E, Vampire and SPASS.
- Input is controlled natural language inside `\begin{forthel}` environments in LaTeX.
- It was left out of Isabelle 2025 because of folder-naming errors (ITP 2025 perfectoid-rings paper, https://drops.dagstuhl.de/storage/00lipics/lipics-vol352-itp2025/LIPIcs.ITP.2025.6/LIPIcs.ITP.2025.6.pdf).
- Naproche-ZF is a grammar-based successor.
- Visored (arXiv:2606.17581) says ForTheL → Lean has been a stated direction since 2020 but is not in production.

**Take for math2**
- Controlled natural language is a useful **style target** for the Statement section: one claim per sentence, explicit quantifiers, defined terms only. A lint rule can check "every content word in the Statement is a defined term or a variable" without adopting ForTheL itself.
- Do not take a dependency on Naproche.

---

## 2. Levels of explanation and pedagogy

### 2.1 Stages of mathematical maturity

T. Tao describes three stages (https://terrytao.wordpress.com/career-advice/theres-more-to-mathematics-than-rigour-and-proofs/; the 2009 date is from search, **UNVERIFIED**):
- **pre-rigorous:** intuition, examples, computation;
- **rigorous:** formal definitions and proofs;
- **post-rigorous:** intuition again, now backed by rigour.

**Take:** spec 04's L1–L4 levels are defined by audience. The spec should also say what each level is *for*:
- L1 is pre-rigorous: picture, example, why anyone cares.
- L2 is rigorous: the definition unpacked, standard examples and non-examples.
- L3 and L4 are post-rigorous: the right way to think about it, how it generalises, where it breaks.

This gives the explanation agent and its reviewers a test that does not depend on audience labels alone.

### 2.2 Bloom and the MATH taxonomy

The MATH taxonomy is from Smith, Wood, Coupland, Stephenson, Crawford and Ball (1996), *IJMEST* 27(1):65–77. Sources: https://opus.lib.uts.edu.au/bitstream/10453/6622/1/2003001060.pdf ; https://bsrlm.org.uk/wp-content/uploads/2016/02/BSRLM-IP-33-1-02.pdf

It is a variant of Bloom made for mathematics. It has three groups:
- **Group A:** factual knowledge; comprehension; routine use of procedures.
- **Group B:** information transfer; application in new situations.
- **Group C:** justification and interpretation; implications, conjectures and comparisons; evaluation.

The groups describe "the nature of the activity … not the degree of difficulty". One study cited in the BSRLM paper found that Group A tasks carried 67 of the 75 marks on an A-level paper.

**Take:** each exercise node (§3.6) carries a `math_group: A|B|C` field, and optionally a subcategory. Coverage reports per topic can then show "no Group C exercises", which is a typical gap in generated banks.

### 2.3 Worked examples and their evidence

- **Meta-analysis:** Barbieri, Miller-Cotto, Clerjuste and Chawla (2023), *Educational Psychology Review* 35(1), article 11, doi:10.1007/s10648-023-09745-1 (https://eric.ed.gov/?id=EJ1364058). Findings:
  - worked examples in mathematics have an overall effect of g = 0.48;
  - correct examples alone did better than incorrect or mixed correct/incorrect examples;
  - self-explanation prompts moderated the effect *negatively*.
- **Expertise reversal** (Kalyuga et al. 2003): worked examples help novices but can hurt advanced learners.
- **Fading:** Renkl and Atkinson 2003, and Renkl et al. 2002. Worked steps are removed one at a time until the learner solves the whole problem.
- **Adaptive fading** beats fixed fading: Salden et al. 2010, https://link.springer.com/article/10.1007/s11251-009-9102-0
- **Self-explanation of proofs:** Hodds, Alcock and Inglis (2014), *JRME* 45(1):62–101 (https://eric.ed.gov/?id=EJ1028348). Self-explanation *training* for reading proofs gave d = 0.95, and a 15-minute intervention had a lasting effect.
  - This is different from the meta-analysis above. A trained reading strategy helps; prompts attached to worked examples did not.

**Take:**
1. An `example` node should be able to hold a **worked solution as a numbered list of steps**, so that a renderer can fade them: show steps 1 to k and ask for the rest.
2. Explanations at L1 and L2 should prefer **correct** worked examples. Incorrect "find the error" examples belong in exercises, marked as such.
3. Following expertise reversal, L3 and L4 explanations should *not* include full worked computations by default.
4. A short "how to read a proof" guide (after Hodds et al.) can be one topic-level explanation node, linked from proof pages. It should not be repeated on every proof.

### 2.4 Examples, non-examples and concept images

- **Concept image versus concept definition:** Tall and Vinner (1981), *ESM* 12:151–169.
- **Example spaces:** Watson and Mason (2002/2005). "Ideal non-examples are those that are very nearly examples." https://mrbartonmaths.com/resourcesnew/8.%20Research/Inquries/Extending%20Example%20Spaces%20-%20Watson%20and%20Mason.pdf
- **"Mis-in" and "mis-out" examples** (ESM 2023, https://link.springer.com/article/10.1007/s10649-023-10273-6). These are examples that learners wrongly include in a concept, or wrongly leave out of it.
- **Definition–theorem–proof format:** Weber (2004), *JMB* 23(2):115–133, studies teaching in this format (cited via https://sites.math.rutgers.edu/~jpmejia/files/Lew_JRME.pdf).

**Take:**
1. spec 04 has `counterexample` for statements. It also needs a **non-example** role for concepts and definitions: an object that fails exactly one clause of the definition. The edge should name the clause that fails. This is cheap to generate and to check, and it is what the research says helps most.
2. An example should declare which definition clauses it exercises. Then "is every clause covered by a near-miss non-example?" becomes a computable coverage check.

### 2.5 Structure of an entry and of a proof

- **nLab:** its page template has the sections **Idea, Definition, Properties, Examples, Related concepts, References**, plus redirects for synonyms. https://ncatlab.org/nlab/show/Template+page ; https://ncatlab.org/nlab/show/writing+in+the+nLab
- **Lamport:** "How to Write a 21st Century Proof", *J. Fixed Point Theory Appl.* 11:43–63 (2012) (https://lamport.azurewebsites.net/pubs/proof.pdf). Proofs are hierarchical numbered steps (⟨1⟩1, ⟨2⟩1 …), each with its own justification, readable at any depth.
- Leron, "Structuring Mathematical Proofs", *AMM* 90(3), 1983, describes levelled proofs. The citation details are **UNVERIFIED**.
- **Stacks Project rules:** each definition marks the terms it defines in `{\it …}`; references go in a `reference` environment. https://raw.githubusercontent.com/stacks/stacks-project/master/documentation/rules

**Take:**
1. spec 04's sections are close to nLab's. Add an **Idea** (motivation) section to concept nodes as the leading paragraph, kept separate from Explanations. nLab shows that a one-paragraph "Idea" is the most-read part of a page.
2. A proof body may be a **structured proof**: nested numbered steps in Lamport style, each step with its own justification links. The "Unfold this proof" feature (spec 13 §13.4) already assumes a step hierarchy for formal proofs, and this gives the same hierarchy to informal proofs. Use a Markdown ordered-list convention, not a new syntax.
3. Require the **definiendum** to be marked in definition bodies (Stacks `{\it}`, sTeX `\definiendum`). Then the linter can check that the defined term is the node's name or a declared synonym.

---

## 3. Exercises

### 3.1 WeBWorK Open Problem Library (OPL)

- **Licence:** CC BY-NC-SA 3.0 by default. Copyright stays with the authors, who may choose an OSI licence instead. https://github.com/openwebwork/webwork-open-problem-library/blob/main/OPL_LICENSE
- **Size:** about 35,000 problems. https://wiki.openwebwork.org/wiki/Open_Problem_Library
- **Tags on each problem:**
  - DBsubject / DBchapter / DBsection (taxonomy);
  - Date, Author, Institution;
  - MLT (more-like-this groups);
  - Level;
  - KEYWORDS;
  - textbook tags (title, edition, section, problem number).
- **Workflow:** problems go into `Contrib` first and are reviewed into the OPL.
- Problems are written in PG/PGML (Perl). Answers are checked by the PG answer evaluators.

### 3.2 MyOpenMath and IMathAS

- **IMathAS Community License:**
  - question *code* is GPL 2.0;
  - the generated HTML of a question must be CC BY 4.0.
  - https://www.imathas.com/communitylicense.html
- MyOpenMath has been run by a non-profit since 2018.

### 3.3 STACK and Numbas

- **STACK:**
  - GPL; uses the Maxima CAS; grades answers with "potential response trees";
  - v4.11.1 in January 2026;
  - the Moodle plugin page's latest release is dated 16 October 2026: https://marketplace.moodle.com/plugins/qtype_stack
- **Numbas:**
  - code Apache-2.0, © Newcastle University;
  - content stays with its authors, and public questions in the Numbas editor are under Creative Commons;
  - packages are exported as SCORM 2004;
  - expressions are written in its JME language.
  - https://docs.numbas.org.uk/en/latest/index.html

### 3.4 QTI 3.0

- The QTI v3.0.1 information model supports **MathML 3 only**; MathML 2 was removed.
- Portable Custom Interactions (PCI) are AMD modules; `customInteraction` is deprecated.
- The implementation guide §3.7.1 covers CAS-style custom response processing (Maxima-style).
- MathML support is required only for **Advanced** conformance.
- https://www.imsglobal.org/spec/qti/v3p0/impl ; https://www.imsglobal.org/spec/qti/v3p0/conf

### 3.5 Lean exercises

- **NNG4** (Natural Number Game) is Apache-2.0: https://github.com/leanprover-community/NNG4
- **lean4game**, the game engine, is GPL-3.0. Games are hosted at adam.math.hhu.de. They include:
  - Knights and Knaves, Linear Algebra, Logic, NNG, Robo;
  - Real Analysis (Kontorovich);
  - Reintroduction to Proofs (Riehl);
  - Set Theory (Velleman).
- **Mathematics in Lean (MIL):** Apache-2.0. Release v4.19.0, with a PDF dated 21 October 2026: https://leanprover-community.github.io/mathematics_in_lean/mathematics_in_lean.pdf. Exercises are Lean files in the `MIL` folder, with solutions kept separately.
- **Autograding:**
  - `robertylewis/lean4-autograder-main` uses the attributes `@[autogradedProof pts]` and `@[legalAxiom]`, with a Gradescope integration. https://github.com/robertylewis/lean4-autograder-main
  - `isun3071/lean-4-teaching` checks submissions with Comparator and supports tactic allowlists.
- **Verso game prototype:** https://github.com/pfaffelh/nng-verso

### 3.6 Open textbooks and their licences

| Book | Licence | Source |
|---|---|---|
| Judson, *Abstract Algebra: Theory and Applications* | GFDL 1.2 or later | http://abstract.ups.edu/ |
| Beezer, *A First Course in Linear Algebra* | GFDL | open.umn.edu (Open Textbook Library) |
| Hefferon, *Linear Algebra* | CC BY-SA | from search |
| OpenStax *Calculus* Vol. 1 | CC BY-NC-SA 4.0 | https://openstax.org/details/books/calculus-volume-1 |
| APEX Calculus | CC BY-NC 4.0 | from search |
| *Active Calculus* (single variable, 2018 and later) | CC BY-SA 4.0. Sources conflict; **UNVERIFIED** | from search |
| *Active Calculus* multivariable | CC BY-NC-SA | from search |

### 3.7 MyST / Jupyter Book 2

- The `exercise` and `solution` directives: a solution refers to its exercise by label. https://mystmd.org/guide/exercises
- `prf:` / `proof:kind` directives for theorem-like blocks (mystmd 1.11): https://mystmd.org/guide/proofs-and-theorems

### 3.8 Licence consequences for math2

Exercise banks are licensed differently from each other and from math2 prose. Spec 16 §16.6 says node prose is CC BY-SA 4.0 and Lean code is Apache-2.0. It already says GFDL sources are linked, never copied, and MSC text is never stored.
- OPL default: NC-SA. **Not compatible** with BY-SA.
- OpenStax: NC-SA. Not compatible.
- APEX: NC. Not compatible.
- IMathAS: code GPL-2.0, rendered output CC BY. Rendered text is fine; the code is GPL.
- GFDL books: not compatible with CC BY-SA in either direction, unless dual-licensed.
- Hefferon (BY-SA): compatible.
- NNG4 and MIL (Apache-2.0): fine for Lean code.

**Take:** math2 must **not copy** NC-licensed problems into nodes. It may **link** to an OPL path or a MyOpenMath ID as an external "interactive version". The exercise node itself is then math2's own text.

### 3.9 Take for math2: an `exercise` node kind

PreTeXt, OPL, MyST and MIL all treat an exercise as a first-class object with statement, hint, answer and solution. math2 should do the same.

**Body sections:** Statement, Hints (ordered), Answer, Solution (structured steps, §2.5).

**Front matter:**
- `exercises: [TAG…]`: the concepts or statements it practises. This edge gives schema.org `assesses` (§4.4).
- `math_group: A|B|C` (§2.2).
- `level: L1–L4`.
- `check:`, one of:
  - `lean`: a statement with `sorry`, graded by Comparator or the autograder, with a tactic allowlist;
  - `cas`: an answer expression plus an equivalence test, exportable to STACK, Numbas or PG;
  - `choice` / `numeric`: exportable to QTI 3;
  - `none`: a proof exercise checked by people.
- `external: [{system: webwork-opl, path: …}, {system: myopenmath, id: …}]`, as links only.

**Checking:** an exercise with `check: lean` gets evidence like a statement does. The reference solution must compile, and the `sorry` version must be the same type. This is the machine-checkable answer format that the task asks for. CAS answers are checked by running the CAS equivalence on the reference answer in CI.

---

## 4. Packaging

### 4.1 Book toolchains (status as of 2026-10-26)

| Tool | Status / version | Licence | Notes |
|---|---|---|---|
| PreTeXt | active; no 2026 release notes found | GPL | §1.6; the best multi-output target |
| Quarto | 1.8 (2025-10-13: https://quarto.org/docs/blog/posts/2025-10-13-1.8-release/); 1.9 adds Typst books ("orange-book"; blog 2026-03-31: https://quarto.org/docs/blog/posts/2026-03-31-typst-books-and-more/) | GPL (from Wikipedia) | theorems via `#thm-` cross-ref divs; amsthm in LaTeX output. A Rust "Quarto 2" for late 2026 is **UNVERIFIED** (search only) |
| Typst | 0.14 (2025-10-24, PDF/UA-1); 0.15.0 (2026-06-15, MathML export); 0.15.1 on 2026-07-17 per search (**UNVERIFIED**) | Apache-2.0 (https://typst.app/open-source/) | fast; its math syntax is not LaTeX, so a converter is needed |
| Pandoc | 3.11 (2026-08-29) | GPL (well known; not re-checked) | "mathml is now the default math-method" (https://github.com/jgm/pandoc/discussions/11835); `--math-method` |
| mdBook | 0.5.0 (2025-11-17); 0.5.4 (2026-07-06) | MPL-2.0 (not re-checked, **UNVERIFIED**) | `mathjax-support`; does not accept `$$` delimiters (https://github.com/rust-lang/mdbook/releases) |
| Jupyter Book 2 / MyST | mystmd 1.11 | MIT (not re-checked, **UNVERIFIED**) | `prf:` theorem directives and exercise/solution (§3.7) |
| LaTeX | TeX Live 2025 on arXiv | LPPL | the arXiv target (§4.3) |

**Take:**
- Emit **PreTeXt** for books.
- Emit **LaTeX** (amsthm + `m2.sty`) for arXiv and print.
- Emit **Typst** as a third option only after the macro → Typst mapping exists. That mapping can live in the notation register (§5), one Typst form per macro.
- Pandoc and Quarto are not needed as primary targets, since PreTeXt covers their outputs. Quarto is a fallback for notebook-style computation nodes.
- mdBook's lack of `$$` makes it a poor fit.

### 4.2 Course standards

- **Common Cartridge 1.3** (July 2013; https://www.imsglobal.org/cc/index.html):
  - carries web content, QTI assessments (six question types), LTI links and discussions;
  - "Thin CC" carries links only;
  - CC is "not designed as a replacement for SCORM".
  - I found nothing newer than 1.3. Whether a later version exists is **UNVERIFIED**.
- **LTI 1.3 Core:** dated 16 April 2019. Earlier LTI versions lost support on 30 June 2022.
- **xAPI:** IEEE 9274.1.1-2023 (published 2023-10-06; https://standards.ieee.org/ieee/9274.1.1/7321/), adopted as ISO/IEC/IEEE 39274-1-1:2025.
- **cmi5:** the xAPI profile for LMS launch, dated 2016-06-01.
- **SCORM 2004:** what Numbas exports (§3.3).

**Take:**
- A math2 "course" export is a **Common Cartridge 1.3** containing:
  - HTML pages generated per node;
  - QTI 3 items, converted to CC's QTI subset, from exercises with `check: choice|numeric`;
  - LTI links back to the live workspace for Lean and CAS exercises.
- **xAPI** statements are optional and live in the workspace (spec 13), never in `tamnd/math2`.
- Do **not** implement SCORM.

### 4.3 Article formats

- **JATS 1.4** = ANSI/NISO Z39.96-2024, approved 31 October 2024. It prefers MathML 3, allows `<tex-math notation="LaTeX" version="LaTeX2e">`, and allows both inside `<alternatives>`. https://jats.nlm.nih.gov/archiving/tag-library/1.4/element/tex-math.html
- **JATS4R** recommends math-mode LaTeX without `$` delimiters inside `<tex-math>`. https://jats4r.niso.org/math/
- **arXiv** (https://info.arxiv.org/help/00README.html):
  - TeX Live 2025 is the default; 2023 is the fallback.
  - Submission system 1.5 (April 2025).
  - `00README.json` keys: `process.compiler`, `sources`, `texlive_version`, `stamp`, `nohyperref`, `spec_version`.
  - Only packages in TeX Live are available, so `m2.sty` must be **bundled** in the submission.

**Take:**
- `m2 article` emits an arXiv-ready source tree:
  - `main.tex` with `m2.sty` bundled;
  - `00README.json` naming the compiler and the TeX Live year;
  - a `.bib` built from node Sources.
- It also emits JATS 1.4 with `<alternatives>` holding both `<tex-math>` (no delimiters) and MathML, the MathML coming from KaTeX at build time.
- Each theorem in the article carries its `m2://TAG@release` as a footnote or a link, so readers can reach the checked node.

### 4.4 Metadata: schema.org LearningResource and LRMI

**schema.org LearningResource** (https://schema.org/LearningResource)
- Properties include `teaches`, `assesses`, `competencyRequired`, `educationalAlignment`, `educationalLevel`, `educationalUse` and `learningResourceType`.
- Subtypes include Course, Quiz and Syllabus.
- The type is in schema.org's "new" area.

**LRMI** (https://www.dublincore.org/specifications/lrmi/)
- LRMI 1.1. DCMI has maintained it since 2014.
- It publishes LRMI Terms and SKOS concept schemes, for example for learning resource types.

**Learning Commons Knowledge Graph** (Chan Zuckerberg Initiative)
- Licence: code MIT, data CC BY 4.0. https://docs.learningcommons.org/knowledge-graph/resources/license
- It models learning components, prerequisite relations between standards, and the Eedi misconception graph.
- Its open platform launched on 2026-09-22. I read this date on the docs site; the details there are from my reading of 2026-10-26.

**Take:** the static site (spec 13) embeds JSON-LD on each page, mapped as follows.

| schema.org property | Source in math2 |
|---|---|
| `@type` | `LearningResource` for explanation and exercise views; `DefinedTerm` for concepts |
| `teaches` | the node's own concept tags |
| `competencyRequired` | the direct `uses` / prerequisite closure, depth 1 |
| `assesses` | the `exercises:` edge of exercise nodes |
| `educationalLevel` | the L1–L4 level, mapped to a published SKOS scheme rather than free text |
| `learningResourceType` | an LRMI term such as "worked example" or "exercise" |

**Learning Commons:** it is a possible alignment target for school-level (L1) concepts, and it supplies a misconception graph. That suggests a `misconception` role or edge for non-examples (§2.4).

### 4.5 Prerequisite graphs and datasets

- **Survey:** prerequisite relation learning, doi:10.1145/3733593 (ACM, 2025). I read the abstract only.
- **Datasets:**
  - RefD Math: 65 knowledge components, 658 pairs, 75 positive;
  - LectureBank;
  - MOOCCube / MOOCCubeX.
  - All are small or tied to particular courses. None is a usable source of math2 edges; they are useful only as evaluation sets.
- **Metacademy** (https://github.com/metacademy/metacademy-application/wiki/Alternative-learning-paths):
  - 487 machine-learning concepts and 1,208 prerequisite pairs;
  - each concept has **a single dependency set**, which the project itself names as a limitation, since there are alternative paths.
- **Knowledge space theory / ALEKS:**
  - the **outer fringe** of a learner's knowledge state is the set of items they are "ready to learn";
  - a learner succeeds on outer-fringe items with probability about 0.93;
  - https://www.sciencedirect.com/science/article/abs/pii/S0022249621000134 ; arXiv:1511.06757
- **PAIGOS** (Ullrich 2008, LNAI 5260, doi:10.1007/978-3-540-88215-2):
  - generates courses from OMDoc content by HTN planning over an ontology of instructional objects (definition, example, exercise, and so on);
  - outputs an IMS manifest;
  - the closest published predecessor of a "graph → course" compiler.
- **Recent graph-to-path work:**
  - AutoMathKG (arXiv:2505.13406);
  - GraphRAG learning paths (arXiv:2506.22303).
  - I found **no** published system that generates a full mathematics textbook from a knowledge graph.

**Take:**
1. **Alternative prerequisite sets.** A definition can be reached from different backgrounds. A concept can have several definitions (Realms, §4.6), each with its own `uses` set. So the prerequisite path in spec 13 should be computed over a *chosen definition per concept*, not over the union of all of them. This fixes Metacademy's limitation in a natural way.
2. **"Ready to learn".** Given a reader background B (spec 13 §13.5), the outer fringe is the set of nodes whose prerequisites are all in B. That is a one-line graph query, and it gives a principled "what next" list.
3. **Evaluation sets.** Use RefD Math / LectureBank style labelled pairs only to measure the edge-inference agent, never as content.

### 4.6 Turning a graph into a linear text

**Ordering**
- Choosing a linear order that minimises forward references is an instance of the minimum feedback arc set problem, which is NP-hard. arXiv:2304.15006 discusses this for documents.
- For a DAG, a topological order has zero forward references in proofs. The real choice is *among* topological orders, and that choice is about locality, keeping related nodes close.
- **Stacks rule:** "Proofs do not refer to results later in the stacks project." https://raw.githubusercontent.com/stacks/stacks-project/master/documentation/rules
- Each Stacks chapter opens with a **Conventions** section (for example tag 0439).
- Stacks tag 06G9 admits that notation conflicts between chapters. That is an example of local conventions being preferred over global consistency.

**Choosing among equivalent definitions**
- Realms (Carette, Farmer, Kohlhase, CICM 2014, LNCS 8543:252–266, arXiv:1405.5956) treat a concept with several equivalent axiomatisations as a **realm**: theories joined by views that prove them equivalent, plus a single external **interface**.
- Follow-up: "Lightweight Realms" (2025), https://link.springer.com/chapter/10.1007/978-3-032-07021-0_13
- Background: the "little theories" method (Farmer et al. 1992).
- **A concrete case:** Mathlib defines `IsCompact` with filters:
  `∀ ⦃f : Filter X⦄ [f.NeBot], f ≤ 𝓟 s → ∃ x ∈ s, ClusterPt x f`
  (https://leanprover-community.github.io/mathlib4_docs/Mathlib/Topology/Defs/Filter.html). Most textbooks use open covers. A book generated from math2 must choose the open-cover definition and present the filter one as a theorem, while the Lean binding goes to the filter one.

**Take: an `m2 book` compiler**

The input is a book manifest `books/<id>.yaml` in the content tree. It contains:
- `targets`: tags, or a topic;
- `background`: the reader background, i.e. the nodes the book assumes;
- `choose`: for each concept with more than one definition node, which one is primary in this book. The others become "equivalent characterisations" theorems, and the equivalence-proof nodes are pulled in;
- `notation`: per-book overrides of default notations (§5);
- `level`: which explanation level to inline;
- `format`: `pretext`, `latex`, `typst`, `jats` or `cc`.

The algorithm:
1. Compute the closure of `targets` minus `background`, using only the chosen definitions' `uses`.
2. Topologically sort it, breaking ties first by the order of the topic nodes' author-given outline, then by keeping a statement next to its proof and examples, then by MSC.
3. Reject the manifest if a proof would refer forward (the Stacks rule). Never silently reorder it.
4. Generate a **Conventions** section from the concept nodes' recorded conventions, and a **List of notation** (PreTeXt `notation-list`).
5. Write `book.lock` with the exact `TAG@release` of every included node, so that the book is reproducible and citable.

The manifest and the lock are the "published method" here. The decisions are recorded data, not hidden heuristics.

---

## 5. Notation

### 5.1 How large projects keep notation consistent

- **Mathlib:**
  - Notation is declared in Lean, often *scoped*. It is enabled with `open scoped X`, for example `open scoped PiNotation`.
  - `notation3` generates delaborators, so that the notation is also printed back: https://leanprover-community.github.io/mathlib4_docs/Mathlib/Util/Notation3.html
  - mathlib4 PR #45028 proposes renaming `notation3` to `notation +delab`. Its merge status is unclear.
  - Every file that introduces notation must have a "Notation" section in its module doc (§1.7).
- **Stacks Project:** a Conventions section per chapter, and tolerance of documented conflicts between chapters (tag 06G9).
- **sTeX:** the symbol is the identity, and notations are attached to it. Variant notations are selectable per document (§1.2).
- **PreTeXt:** `<notation>` with usage and description, attached where the notation is introduced, and collected into a notation list (§1.6).
- **Wikipedia MOS:MATH:** keep a list of "Mathematical conventions" (for example: does "compact" include Hausdorff? are rings unital?) and require "consistency within an article". https://en.wikipedia.org/wiki/Wikipedia:Manual_of_Style/Mathematics
  - arXiv:2412.20419 notes that consistency *across* articles is "too much to ask" on Wikipedia.
- **ISO 80000-2:2019:** ℕ = {0, 1, 2, …}, and ℕ* denotes the positive integers. https://cdn.standards.iteh.ai/samples/64973/c49664e1b9364d1d811246de9d1db4aa/ISO-80000-2-2019.pdf

### 5.2 Take for math2: a notation register

Replace the flat macro list (`macros/m2.sty` + `m2.katex.json`) with a **source register** `macros/notation.yaml` from which both files are generated. Each entry has these fields:

```yaml
- macro: \Nat              # the LaTeX command (the only thing a node body uses)
  denotes: 4M2XKQ          # concept tag (sTeX \symdecl identity)
  args: 0
  prec: null               # for operators, as sTeX prec=
  latex: \mathbb{N}        # expansion for m2.sty and KaTeX
  typst: NN                # for the Typst target (§4.1)
  intent: natural-numbers  # MathML intent concept, Core or Open list (§1.4); null if none
  openmath: setname1/N     # alignment (§1.3), optional
  convention: "includes 0 (ISO 80000-2)"  # one line, shown in the notation list
  variants:                # alternative notations for the same concept
    - {name: N-bold, latex: \mathbf{N}}
  introduced_by: 9QF3ZD    # definition node that introduces it (PreTeXt <notation> placement)
```

New lint rules:
1. Every macro used in a node body exists in the register. This extends B01.
2. A definition node that introduces a symbol has a register entry with `introduced_by` pointing to it.
3. One concept has exactly one default notation. Variants are allowed only through a book or page override (spec 13's "Translate notation" then becomes a lookup in this register).
4. Overloaded symbols, such as $\subset$ or $(a,b)$ for pair, interval and gcd, must use distinct macros: `\subset` versus `\subsetneq`, `\pair`, `\ointerval`, `\gcd`. Each macro has a single `denotes`. This is the sTeX principle, and it is what makes rendering by convention and intent emission possible.

**Conventions:**
- A concept node's front matter gets `conventions:`, a list of short statements such as "ring: unital, not necessarily commutative" or "ℕ: includes 0".
- The default follows ISO 80000-2 where ISO says anything.
- The book compiler (§4.6) collects these into the book's Conventions section, Stacks style.
- When a statement depends on a convention, for example on whether 0 ∈ ℕ, it must link the concept that carries the convention, so that a change of convention can find every statement it affects.

---

## 6. What the spec should adopt

Ordered by value for effort.

1. **Notation register** (§5.2): a symbol-identity + notation split (sTeX), with `denotes`, arity, precedence, Typst form, MathML `intent` and OpenMath alignment per macro. `m2.sty` and `m2.katex.json` are generated from it. Conventions are recorded on concept nodes, with ISO 80000-2 as the default.
2. **`exercise` node kind** (§3.9): Statement / Hints / Answer / Solution; an `exercises:` edge; `math_group` (MATH taxonomy A/B/C); and a `check:` of `lean` (with a `sorry` target, graded via Comparator or the autograder), `cas`, `choice`, `numeric` or `none`. NC-licensed banks (OPL, OpenStax, APEX) are linked only, never copied.
3. **Non-example role** (§2.4) that names the definition clause that fails. Examples declare the clauses they exercise, and clause coverage is a computed report.
4. **Pedagogy rules for explanations** (§2.1–2.3):
   - L1–L4 map onto Tao's pre-rigorous / rigorous / post-rigorous stages;
   - prefer correct worked examples at L1–L2, as numbered steps that a renderer can fade;
   - no full worked computations at L3–L4 (expertise reversal);
   - an Idea paragraph on concept nodes (nLab);
   - optional Lamport-style structured proofs.
5. **`m2 book` compiler with a manifest and a lock** (§4.6):
   - the closure of the targets minus the reader background;
   - one *chosen* definition per concept (Realms), with the others becoming equivalence theorems;
   - a topological order that keeps the author's outline and refuses forward references in proofs (Stacks rule);
   - a generated Conventions section and List of notation;
   - `book.lock` pinning `TAG@release`.
6. **Output targets, in this order:**
   1. PreTeXt XML (web, PDF, EPUB, braille, WeBWorK in one step);
   2. LaTeX + bundled `m2.sty` + `00README.json` for arXiv;
   3. JATS 1.4 with `<alternatives>` (tex-math without delimiters, plus MathML);
   4. Common Cartridge 1.3 with QTI items and LTI links for courses;
   5. Verso Blueprint for Lean projects;
   6. Typst later.
   - No SCORM. xAPI only in the workspace.
7. **JSON-LD on every static page** (§4.4): schema.org `LearningResource` / `DefinedTerm` with `teaches`, `competencyRequired` (from `uses`), `assesses` (from `exercises:`), and `educationalLevel` from a SKOS scheme for L1–L4.
8. **Prerequisite paths per chosen definition**, plus the KST "outer fringe" as the "ready to learn" query (§4.5).
9. **Blueprint import and export** (§1.7):
   - read Verso Blueprint's semantic manifest and keep the automatic/manual edge origin and the hole/unverified distinction;
   - read leanblueprint as a fallback, guarding against self-`\uses` and remarks in `\uses`;
   - accept Verso only as an export, not as a node source format (answers spec 21).
10. **Alignment targets:** add OpenMath CD symbols and MathML intent concept names next to Wikidata QIDs. Export symbol declarations in the Wikidata P7235 + P9758 pattern.
11. **Topic nodes follow Mathlib's module-doc sections** (Main definitions, Main statements, Notation, Conventions, References), and any scraper of Mathlib docstrings handles Verso docstrings as well as Markdown.
12. **Correct spec 01 §1.2.** Say that FLT keeps both a leanblueprint and a Verso blueprint, and that only verso-flt and verso-sphere-packing are in the Verso Blueprint catalogue (Carleson is pending a toolchain bump). "Moved to it" is too strong.

---

## 7. Open doubts

1. **MathML 4 status.** The charter's May 2026 CR target was missed, and the spec is still a WD with an editor's draft of 24 October 2026. The new CR date is unknown. w3.org/TR returned 403, so I could not read the TR page directly.
2. **Verso Blueprint licence:** UNVERIFIED (LICENSE fetch returned 404).
3. **Which projects have moved to Verso Blueprint.** FLT has both folders; Carleson and Noperthedron are excluded from the catalogue until they move to v4.35. Spec 01 §1.2's "FLT, Carleson, Sphere Packing have moved" needs rechecking.
4. **LaTeXML licence:** the badge suggests CC0 or public domain (NIST work). I did not read the LICENSE file.
5. **MMT licence** (COPYING.txt): not read.
6. **Licences of SMGloM archives other than `smglom/mathsoft`:** not checked. The old SPL 0.1 may still apply to some of them.
7. **Active Calculus licence:** sources conflict between CC BY-SA 4.0 and CC BY-NC-SA for the single-variable edition.
8. **Leron 1983** (*AMM* 90(3)): the citation is UNVERIFIED.
9. **Date of Tao's "more to mathematics than rigour" post** (2009?): UNVERIFIED.
10. **Quarto 2 (Rust) timing, Typst 0.15.1 date, mdBook and mystmd licences:** from search or memory only.
11. **Common Cartridge after 1.3:** I found no newer version, but did not prove that none exists. QTI 3 support in Canvas, Moodle and Blackboard as of 2026 was not checked.
12. **doc-gen4 machine-readable output:** whether doc-gen4 publishes per-declaration JSON that math2 can use for binding was not confirmed. doc-gen4 also does not yet render Verso docstrings.
13. **mathlib4 #45028** (`notation3` → `notation +delab`): merge status unknown.
14. **Learning Commons Knowledge Graph:** whether its K-12 math nodes are worth aligning to is unknown. Its scope is US standards.
15. **Graph-to-textbook generation:** I found no published system that generates a complete mathematics textbook from a knowledge graph. PAIGOS (2008) generates courses, not books. The `m2 book` design in §4.6 is therefore new and has no prior evaluation.
16. **Wikifunctions:** relevance to math2 is low today. Revisit if its math function catalogue grows.
17. **MathML intent Open list:** its exact published URL was not confirmed. The Core list URL and the concept-lists note (https://w3c.github.io/mathml-docs/concept-lists/) are from search results.
18. **Exercise answers and Lean code:** spec 16 §16.6 gives prose CC BY-SA 4.0 and `lean/` Apache-2.0. It does not say which zone a Lean exercise's statement and reference solution belong to. Presumably Apache-2.0, but the spec should say so.
