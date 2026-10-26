# K01, part D. LLM generation at scale: quality, prior projects, memorization, law, terms and cost

Research for the math2 spec (tamnd/math2, tamnd/math2-agent). Written 2026-10-26.

This document extends spec 01 (§1.4 to 1.6, §1.12), spec 10 (autoformalization and fidelity) and spec 12 (research agents). It does not repeat them. The items below that spec 01 already covers are cited there and only extended here:
- PutnamBench costs;
- the FLT project;
- Automatic Textbook Formalization;
- Lean Pool;
- Tau Ceti;
- the Aletheia 6.5% figure;
- the DeepSeekMath-V2 verifier.

**Conventions**

- Every fact has the URL where it was found. All pages were read on **2026-10-26** unless another date is given.
- **[P]** means a primary source: the paper, the official page, the licence text or the court docket.
- **[S]** means a secondary source: a law-firm note, a tracker or news.
- **[snippet]** means the fact was seen only in a search-result excerpt, not on the page itself.
- **UNVERIFIED** marks a fact that could not be confirmed. The reason is given next to it.
- "Derived" marks a figure we computed from a source's numbers. The source does not state it.
- Most numbers were taken from a fetch tool's extraction of the page, not from the raw PDF. Re-check any number against the PDF before the spec quotes it as normative.
- Model names are written as each source wrote them.
- No model API was called for this research.

**Contents**

1. Quality: measured error rates of LLM-written mathematics
2. Scale projects: throughput, cost, fidelity
3. Memorization and copyright
   - 3a. Reproduction and its detection
   - 3b. The legal position
4. Provider terms and open-weight licences
5. Cost of one million nodes
6. Community norms on LLM text (short)
7. What the spec should require because of this
8. Open doubts

## 1. Quality: measured error rates of LLM-written mathematics, 2024 to 2026

All sources were read on 2026-10-26. The numbers come from the fetch tool's extraction of each page. Anything that will be quoted in the spec should be checked against the PDF first. Model names are as the papers give them. "Derived" means our own calculation.

### 1.1 Informal proofs graded by humans

| Study | Setting | Key measured result | Source |
|---|---|---|---|
| Proof or Bluff (Petrov et al., arXiv:2503.21934, v5 4 Sep 2025) | USAMO 2025, expert graders | Best model Gemini-2.5-Pro 10.1/42 (about 25%). All others under 5%: R1 2.0, o3-mini 0.9, o1-pro 1.2 | https://arxiv.org/html/2503.21934v5 [P] |
| MathArena "Not Even Bronze" (17 Jul 2025) | IMO 2025, 2 judges per response, best-of-32 | Best model Gemini 2.5 Pro 31% (13/42), below bronze (19). Gemini "cite[s] non-existent theorems when it fails to find a valid proof" | https://matharena.ai/imo/ [P] |
| Open Proof Corpus (Dekoninck et al., arXiv:2506.21621, v2 14 Jan 2026) | over 5,000 human-graded proofs | **43% of proofs correct overall.** Only 59.5% of o3's correct final answers carried a correct proof. Models admitted failure in 114 of more than 1,700 wrong solutions | https://arxiv.org/html/2506.21621v2 [P] |
| IMO-Bench (Luong et al., arXiv:2511.01846, EMNLP 2025) | IMO-ProofBench, advanced set | Gemini Deep Think 65.7%; next best is 42.4 points lower | https://arxiv.org/abs/2511.01846 [P] |
| ProofBench / ProofGrader (Ma et al., arXiv:2510.13888, ICLR 2026) | 145 problems, 435 solutions, 0-7 expert scale | Best-of-16 picked by ProofGrader 4.14/7; human oracle 4.62; binary evaluator 2.48 | https://arxiv.org/abs/2510.13888 [P] |
| Hard2Verify (Pandit et al., arXiv:2510.13744) | 1,860 PhD-annotated steps (GPT-5, Gemini 2.5 Pro, Claude Sonnet 4) | 58% of steps correct. Claude Sonnet 4 had 4 of 72 solutions fully correct | https://arxiv.org/html/2510.13744v1 [P] |
| IneqMath (Lu et al., arXiv:2506.07927, NeurIPS 2025) | 29 LLMs, step-wise grading | o1 under 10% overall; step-wise grading lowers accuracy by up to 65.5 points from the final-answer score | https://arxiv.org/abs/2506.07927 [P] |
| QEDBench (Gonzalez et al., arXiv:2602.20629, ICML 2026) | university level, human experts | Gemini 3.0 Pro 0.91. GPT-5 Pro and Claude Sonnet 4.5: 0.72 and 0.63 in discrete math, 0.74 and 0.50 in graph theory | https://arxiv.org/abs/2602.20629 [P] |
| LemmaBench (Peyronnet et al., arXiv:2602.24173) | lemmas from recent arXiv papers | SOTA "around 10-15% accuracy" pass@1 | https://arxiv.org/abs/2602.24173 [P] |
| MathArena platform (arXiv:2605.00674, v2 15 May 2026) | USAMO 2026; research-level questions | GPT-5.5 "reaches 98% on the 2026 USA Math Olympiad and 74% on research-level questions" | https://arxiv.org/abs/2605.00674 [P] |
| First Proof, batch 1 (arXiv:2602.05192) | 10 unpublished research questions | No score is given; best answers were essentially correct on 2 of 10. Cases of "citing lemmas that did not exist" | https://arxiv.org/html/2602.05192v2 [P] |
| First Proof, batch 2 (arXiv:2606.18119, 16 Jun 2026) | referee verdicts on 4 systems | Derived and to be rechecked: about 6 of 9, 5 of 10, 5 of 10 and 1 of 10 rated flawless or minor-revision. One system "hallucinates a theorem from the literature which exactly matches the theorem it was asked to prove". Citation problems on 4 to 8 of 10 problems for most systems | https://arxiv.org/html/2606.18119v1 [P] |

**Proving false statements** is the measured proxy for the "misstated theorem" risk:
- **BrokenMath** (arXiv:2510.04721) [P]. https://arxiv.org/html/2510.04721v1
  - Sycophancy rates: GPT-5 29.0%, Gemini-2.5-Pro 37.5%, DeepSeek-V3.1 70.2%.
  - Rates are higher on proof problems, by up to 22.5 points. They are higher again when the false statement replaces the model's own theorem, by up to 15.6 points.
  - Self-verification lowers sycophancy by 7.6 to 12.5 points but does not remove it.
- **BrokenArXiv** (MathArena, 13 Mar 2026; 31 perturbed arXiv statements) [P]. https://matharena.ai/brokenarxiv/
  - Share of false statements rejected: GPT-5.4 "just under 40%"; Gemini-3.1-Pro 18.5%, rising to 71% with a "Prove or disprove" prompt; Claude-Opus-4.6 3.2%.
  - Later figures (GPT-5.5 72%) are [snippet].

### 1.2 Reliability of LLM judges

- **Proof or Bluff, Table 2** [P]. LLM judges inflated scores "by a factor of up to 20".
  - Examples: R1 scored 2.0 by humans and 19.3 by an o3-mini judge; QwQ 1.2 and 23.8.
  - Derived means: humans about 2.6/42, the o3-mini judge about 18.3, the Claude 3.7 judge about 16.6.
- **OPC judge benchmark** (293 human-labelled proofs) [P].
  - Human inter-annotator agreement: 90.4%.
  - Judge accuracy, pass@1 / maj@5:

    | Judge | pass@1 | maj@5 |
    |---|---|---|
    | GPT-5 | 89.3 | 90.8 |
    | Grok-4 | 88.3 | 89.8 |
    | Gemini-2.5-Pro | 85.4 | 88.1 |
    | o3 | 83.1 | 84.3 |
    | Claude-4-Sonnet | 70.6 | 75.0 |
    | GPT-4.1 | 61.4 | 60.8 |

  - All but one model judge their own proofs more harshly than other models' proofs.
- **Hard2Verify, step-level** [P].
  - Balanced accuracy: GPT-5 86.5, Gemini 2.5 Pro 83.4, Claude Sonnet 4 70.6, Qwen2.5-Math-PRM-72B 55.8.
  - True-negative rate on incorrect steps: GPT-5 78.7, Gemini 78.6, Claude Sonnet 4 43.7, Llama-3.3-70B 10.1.
  - Derived: **the best judges accept about 21% of wrong steps; weak judges accept 56 to 90%.** The paper concludes: "weaker verifiers cannot catch errors."
- **IMO-Bench** [P]. https://arxiv.org/html/2511.01846v1
  - ProofAutoGrader against humans: Pearson r of 0.96 (basic set) and 0.93 (advanced set).
  - 4-class IMO-GradingBench accuracy is only 44 to 54% for every model tested.
- **ProofGrader** [P]: mean absolute error of 0.926 on the 0-7 scale.
- **QEDBench** [P]: judges show positive bias, from +0.18 for Claude Opus 4.5 up to +0.36 for Llama 4 Maverick on a 0-1 scale.
- **Grayzel, "Cost-Effective Automated Judging..."** (arXiv:2608.00004, v2 5 Oct 2026) [P]. https://arxiv.org/abs/2608.00004
  - GPT-OSS-120B, DeepSeek-V4-Flash and their consensus are "statistically no worse than the frontier (Claude Opus 4.7, Gemini 3.1 Pro)" at "4-100x lower cost".
  - On the 1,000 IMO-GradingBench items, a unanimous vote has the highest precision (0.855) and a majority vote the highest recall (0.912).
- **"One Token to Fool LLM-as-a-Judge"** (arXiv:2507.08794) [P]: reference-based reward models "can consistently elicit false positive rewards without any substantive reasoning". No rate is given.

### 1.3 Wrong definitions, missing hypotheses, misnamed theorems

- **No study measures the rate at which LLMs write a wrong definition, drop a hypothesis or misname a standard theorem in informal text.** This is a gap. math2 will have to measure it on its own audit sample.
- The nearest measured evidence:
  - **Re²Math** (Lyu et al., arXiv:2605.09012, 9 May 2026) [P]. https://arxiv.org/html/2605.09012
    - Task: cite a source theorem that exists and also justifies the step. The best success rate is 7.0% (Claude Opus 4.5).
    - Breakdown for n=200 (no grounded support / grounded but insufficient / success):
      - GPT-5.2: 75.0 / 22.0 / 3.0%
      - Gemini 3.1 Pro: 88.0 / 10.0 / 2.0%
      - Claude Opus 4.5: 76.0 / 17.0 / 7.0%
      - Grok 4: 58.5 / 38.0 / 3.5%
    - "Models often retrieve or restate true mathematics without making it usable."
  - **Banerjee and Bhattacharjee** (arXiv:2606.24902) [P]. Every one of 8 Gemini 2.5 Flash research proofs contained an unsupported load-bearing "fundamental result" ("premise smuggling"). None had a confirmed fabricated citation. The sample is tiny.

### 1.4 Hallucinated citations

| Study | Measured | Source |
|---|---|---|
| Naser, arXiv:2603.03299 | 10 commercial LLMs, 69,557 citations: **11.4% to 56.8% hallucinated** | https://arxiv.org/abs/2603.03299 [P] |
| Rao et al., arXiv:2604.03173 | 3 to 13% of citation URLs hallucinated, 5 to 18% do not resolve | https://arxiv.org/abs/2604.03173 [P] |
| Linardon et al., JMIR Mental Health, 12 Nov 2025 | GPT-4o: 19.9% fabricated, 45.4% of the real ones have errors; 6% on a familiar topic against 28 to 29% on unfamiliar ones | https://pmc.ncbi.nlm.nih.gov/articles/PMC12658395/ [P] |
| Ansari, arXiv:2602.05930 | 100 hallucinated citations in 53 accepted NeurIPS 2025 papers (about 1%) | https://arxiv.org/abs/2602.05930 [P] |
| Russinovich et al., arXiv:2607.00738 | reference-level rate usually below 1%; about 1 in 20 NeurIPS and USENIX Security 2025 papers has 2 or more likely hallucinated references | https://arxiv.org/abs/2607.00738 [P] |
| Zhao et al. (with Ginsparg), arXiv:2605.07723 | 111M references: "conservative estimate of 146,932 hallucinated citations in 2025 alone"; no figure for mathematics alone | https://arxiv.org/abs/2605.07723 [P] |
| QED (An et al., arXiv:2604.24021) | in research-level math proofs, "citation hallucination was not a rare edge case"; no number given | https://arxiv.org/html/2604.24021 [P] |

The rate of unfamiliar-topic fabrication matters for math2. Niche research mathematics is the "unfamiliar topic" case.

### 1.5 Which checks catch the errors (measured)

| Check | Measured effect | Source |
|---|---|---|
| Strong judge plus majority vote | maj@5 adds 1.5 to 4.3 points over pass@1 (OPC). Unanimous vote gives precision 0.855; majority vote gives recall 0.912 | 2506.21621, 2608.00004 [P] |
| Pseudo-formalization (structured semi-formal rewriting) | FirstProof errors: recall 0.66 at precision 0.77, against 0.54 at 0.76 for an LLM judge (GPT-5.5). On arXiv errors, both "leave roughly 40% of annotated ground-truth errors uncaught" | https://arxiv.org/html/2605.20531 [P] |
| Lean formalization of an informal proof | LeanTutor: only 13.0 to 30.1% of 146 wrong student proofs were faithfully autoformalized so that Lean flagged the error. "Don't Trust: Verify": more than 12% gain on GSM8K (answers, not proofs) | https://arxiv.org/html/2506.08321v2, https://arxiv.org/abs/2403.18120 [P] |
| Verifier-gated agent | QED: 0 false positives among 17 accepted out of 214; false negatives not measured | 2604.24021 [P] |
| Cross-model agreement on citations | a work cited by more than 3 LLMs is real 95.6% of the time; within-prompt repetition 88.9%; classifier AUC 0.876 | 2603.03299 [P] |
| Retrieval or liveness check on citations | cuts non-resolving URLs "by 6-79x to under 1%" | 2604.03173 [P] |
| "Prove or disprove" prompting | Gemini-3.1-Pro rejects false statements 71% of the time, against 18.5% | matharena.ai/brokenarxiv [P] |
| Self-verification | the evidence is mixed. Hard2Verify finds models more lenient on their own work; OPC finds them harsher | 2510.13744, 2506.21621 [P] |

### 1.6 What section 1 says, in eight numbers

1. Unchecked LLM judges inflate proof scores by up to 20x (Proof or Bluff).
2. The best step judges accept about 21% of wrong steps; weak judges accept 56 to 90% (Hard2Verify).
3. Strong models "prove" false competition statements 29% to 70% of the time (BrokenMath). At research level they rejected under 40% of false statements (BrokenArXiv, Feb 2026).
4. A correct final answer is not a correct proof: 59.5% for o3 (OPC).
5. The ceiling for single-judge agreement with humans is about 90%, which is also the human inter-annotator rate (OPC).
6. Theorem references in research proofs are usable at most 7% of the time (Re²Math).
7. Even structured verification misses about 34 to 40% of known research-proof errors (Pseudo-Formalization).
8. 11.4 to 56.8% of generated citations are hallucinated. Agreement among more than 3 models gives 95.6% accuracy.

### 1.7 Could not verify (quality)

- Per-model proof-correctness rates in OPC and Hard2Verify; they appear only in figures.
- ProofBench OpenReview numbers [snippet].
- BrokenArXiv later figures [snippet].
- Phantom References per-venue shares [snippet].
- ProofGrid self-grading figures [snippet].
- IMProofBench per-model scores: v2 has none in the abstract, and v1 is [snippet].
- The First Proof batch 2 tally is derived and needs checking against its Tables 1 to 5.
- The Linardon abstract and results disagree on one subgroup.
- The FormalAlign arXiv id from search pointed to an unrelated paper, so it is excluded.

## 2. Scale projects: throughput, cost, fidelity

All sources were read on 2026-10-26. A figure marked "derived" is our own division, not the source's. **Main finding:**
- Measured fidelity numbers exist only for the autoformalized-statement datasets: DeepSeek-Prover V1, Goedel, AlphaProof, Lean Workbook, Herald and CriticLean.
- The large agent runs (Gauss, Textbook Formalization, Lean Pool, Tau Ceti) report size, and sometimes cost.
- **None of the agent runs measures a misformalization rate.**

### 2.1 Agent runs on research mathematics

- **Math Inc Gauss, Strong PNT.** https://github.com/math-inc/strongpnt, https://www.math.inc/gauss [P]
  - Output: about 25k lines of Lean and about 1.1k theorems and definitions, done in three weeks.
  - Compute: "thousands of concurrent agents", "multiple terabytes of cluster RAM".
  - Human input: "Humans provided the high-level blueprint, reviewed critical lemmas, and adapted prior work where needed."
  - Cost: not published.
  - Fidelity: no rate is published; humans reviewed the key lemmas.
  - Announcement date 11 Sep 2025 [snippet].
- **Math Inc sphere packing.** https://www.math.inc/sphere-packing [P]; the page is undated, and its latest milestone is February 2026.
  - Size: about 200k lines, after a peak of 500k before refactoring.
  - Dimension 8: "In just 5 days, Gauss automatically proved all remaining results".
  - Dimension 24: two weeks, "using only the original paper as input".
  - Cost and compute: not stated.
- **FormalQualBench.** https://www.math.inc/formalqualbench [P]
  - Setup: 23 graduate-level theorems, 4-hour timeout.
  - Results:

    | System | Solved (of 23) | Mean cost per solve |
    |---|---|---|
    | OpenGauss | 8 | $24.93 |
    | Claude Code | 4 | $23.16 |
    | Claude Code (Skills) | 5 | $13.47 |
    | Codex | 5 | not given |
    | opencode | 5 | $22.22 |
    | Aristotle | 6 | not given |

  - Aristotle ran with no timeout and was "not Comparator-validated", so it "may contain false positives".
  - The statements were "verified by a Lean expert".
  - Codex and opencode tried elaborator-level workarounds. **This is direct evidence that an agent will game a checker if it can.**
- **Automatic Textbook Formalization** (arXiv:2604.03071) [P]. The spec 01 entry is extended here with cost.
  - Cost: "around $100K, or around $200 per page and $300 per target theorem or definition". Without caching it would have been $430K.
  - Tokens: 83B input and 561M output. Output tokens cost $14K of the total.
  - Agents: about 3,300 of 30K succeeded.
  - Derived: about $17 per Lean declaration ($100K / 5,900).
  - Fidelity: there is no rate. Key statements were "manually spot-checked". The authors say it "remains a possibility that individual theorems and definitions have not been translated semantically faithfully".
- **Lean Pool** (arXiv:2609.25199, 21 Sep 2026) [P]
  - Size: 211 projects, 3,228,485 lines, 193,862 declarations.
  - Provenance: 70 human, 102 AI, 39 mixed.
  - Review cost estimates: $308.52 over 285 reports, median $0.20. These are "not cash payments", and generation cost was not logged.
  - Review agreement: repeated reviews of the same PR agreed on 37 of 69 pairs (about 54%).
- **Tau Ceti.** https://github.com/TauCetiProject/TauCeti [P]
  - Status: 19,275 commits.
  - Not published: line counts, cost and rejection rates.
  - Launch around July 2026 [snippet].
- **Erdős problems.** Tao's wiki "AI contributions to Erdős problems", frozen as of 30 Jun 2026. https://github.com/teorth/erdosproblems/wiki/AI-contributions-to-Erd%C5%91s-problems [P]
  - Tally (approximate, from a summary of the tables rather than stated totals): about 90 full-solution rows and about 64 marked "(Lean)".
  - Problems found: some AI entries are marked incorrect, and some problems were misstated at the source.

### 2.2 Competition provers

- **Harmonic Aristotle** (arXiv:2510.01346) [P]
  - IMO 2025: verified solutions to 5 of 6 problems.
  - Human input: the statements were "translated … by hand", which the paper calls "the only human formalization step".
  - Model: the search model has more than 200B parameters.
  - Not reported: compute, wall time and cost. API price and throughput were not found (UNVERIFIED).
- **Seed-Prover 1.0** (arXiv:2507.23726) [P]
  - IMO 2025: 5 of 6 problems.
  - Time per problem, from the README: P3 took 3 days (a 2,000-line proof), P4 3 days (4,000 lines), P5 1 day.
- **Seed-Prover 1.5** (arXiv:2512.17260) [P]
  - Results: 88% on PutnamBench (580/660 [snippet]) and Putnam 2025 11/12 within 9 hours.
  - Compute: "10 H20 days / problem", against 18 H20-days for 1.0. The text elsewhere says 20 for 1.0, which is inconsistent. The Putnam cap was 40 H20-days per problem.
- **AlphaProof** (Nature, 12 Nov 2025). https://www.nature.com/articles/s41586-025-09833-y [P]
  - Dataset: about 1M natural-language problems became about 80M formal problems.
  - Compute:
    - Autoformalization: about 100,000 TPU-days, derived about 1.25 TPU-days per 1,000 formal problems.
    - Reinforcement learning: about 80,000 TPU-days.
  - Fidelity, judged by a panel of about three Lean experts, pass@1: **60% on 50 IMO problems** (combinatorics 33.3%) and **64% on 50 Putnam problems**.
  - The paper holds that an unfaithful statement still "provides a valid formal problem". That is fine for training, but not for a library.
  - IMO 2024: the statements were formalized by experts. Each solved problem needed 2 to 3 days of test-time RL.
  - The "500 TPU-days per problem" figure appears only in Seed-Prover 1.5's citation.

### 2.3 Autoformalized statement datasets (the only measured fidelity)

| Dataset | Size | Measured fidelity | Source |
|---|---|---|---|
| DeepSeek-Prover V1 | 712,073 statements from 869,659 problems; about 8M with proofs | "at least 20% … even after quality filtering, were incorrect" | https://arxiv.org/html/2405.14333 [P] |
| DeepSeek-Prover-V2 | corpus size not reported | 2 PutnamBench and 2 CombiBench statements found misformulated; no audit | https://arxiv.org/html/2504.21801 [P] |
| Goedel-Pset-v1 | 1.64M from 928K problems | compiles 88.5%; LLM faithfulness pass@1 80.4%. Goedel-V2: "over 80% of unsolved problems are incorrectly formalized" (sample size not given) | https://arxiv.org/html/2502.07640, https://arxiv.org/html/2508.03613 [P] |
| Goedel-Formalizer-V2 vs Kimina | 300 Omni-MATH problems | 228/300 against 161/300 (judging method unconfirmed) | 2508.03613 [P] |
| Kimina-Autoformalizer | about 100k (+10k human) | 90% compile, **66% accurate** on a human-curated set of about 1,000 | https://arxiv.org/html/2504.11354 [P] |
| NuminaMath-LEAN | 104,155 | admits "wrongly formalized" items; no rate | https://huggingface.co/datasets/AI-MO/NuminaMath-LEAN [P] |
| Lean Workbook | 57,231 + 82,893 | authors 93.5% (5 to 10 samples per tag); CriticLean re-audit **84%** (n=50) | arXiv:2406.03847, 2507.06181 [P] |
| Herald | 580k | 101/151 correct on ProofNet-valid (derived 67%); 24 minor and 26 major errors | arXiv:2410.10878 [P] |
| FineLeanCorpus (CriticLean) | 285,957 | human accuracy 69% to 100% by source; best critic (Gemini 2.5 Pro) 89.2% on the 500-item CriticLeanBench | arXiv:2507.06181 [P] |
| FormalMATH | 5,560 | 72.09% survive LLM semantic checks before expert review | arXiv:2505.02735 [P] |
| miniF2F revisited | 488 | discrepancies in "more than half of the problems" | arXiv:2511.03108 [P] |
| "Faults in Our Formal Benchmarking" | five benchmarks | 4,833 findings, 398 "mechanically certified issues" | arXiv:2606.29493 [P] |

Compute for Goedel V1:
- Pass@16 over 1.78M statements took about 6 hours on 64 H100s. Verification took about 10 hours on 8,000 CPUs.
- Derived: about 0.23 H100-hours per 1,000 statements per pass@16 sweep.

Compute for Kimina: the Lean server verifies up to 100 proofs per second on 64 cores.

**Reading.** Without a human in the loop, autoformalized statements are faithful about 60 to 90% of the time, depending on source difficulty. Competition-style problems sit at the upper end; graduate and research material is lower (Herald's textbook set: 22.5% proved). Compiling is not evidence of fidelity: Kimina compiles 90% of statements but only 66% are accurate.

### 2.4 LLM-written encyclopedias

- **Grokipedia**, from Triedman and Mantzarlis (arXiv:2511.09685, 12 Nov 2025) [P]:
  - 883,858 articles were scraped.
  - **56% carry the Wikipedia CC footer.** Similarity to Wikipedia is 0.90 for those and 0.77 for the rest.
  - 5.5% cite a blacklisted source, about 9x the rate on Wikipedia.
  - No factual-error rate was measured. Grokipedia's self-reported "92.5% factually correct" is UNVERIFIED (no evaluator is named).
- **AI-written Wikipedia articles.** Brooks et al. (arXiv:2410.08044) [P]
  - "Over 5%" of new English articles were flagged, at a 1% false-positive rate.
  - On Wikipedia's side, speedy-deletion criterion G15 for LLM-generated pages was adopted in August 2025 [S].
- **STORM** (arXiv:2402.14207) and **Co-STORM** (arXiv:2408.15232) [P]
  - Gains: +25% absolute on organization and +10% on coverage.
  - Failure modes: "source bias transfer" and "over-association of unrelated facts".
- No 2025 to 2026 LLM-written mathematics encyclopedia or textbook with a measured error rate was found.

### 2.5 Could not verify (scale)

- Not published:
  - Aristotle API price, throughput and training-set size;
  - Gauss agent counts, compute and dollar cost;
  - the DeepSeek-Prover-V2 synthetic data size;
  - the sample sizes behind Goedel-V2's ">80%" and CriticLean's per-source rates.
- [snippet] only:
  - the dates of the Gauss announcement and the Tau Ceti launch;
  - Seed-Prover 1.5's 580/660;
  - AlphaProof's "500 TPU-days per problem", which is only in Seed-Prover 1.5's citation.
- The Erdős wiki tallies are approximate.

## 3. Memorization and copyright

Part 3a covers what models reproduce and how to detect it. Part 3b covers what the law says about the output.

## 3a. Memorization and copy detection

All sources read 2026-10-26, mostly arXiv abstract or HTML pages. Numbers are as extracted from the page. "[snippet]" means the item was seen only in a search result.

### 3a.1 Ordinary, non-adversarial use

- **Aerni, Rando, Debenedetti, Carlini, Ippolito, Tramèr, "Measuring Non-Adversarial Reproduction of Training Data in LLMs"**, arXiv:2411.10242 (Nov 2024). https://arxiv.org/abs/2411.10242 [P]
  - **Definition.** A generated substring counts as reproduced if it is at least **50 characters** long and appears verbatim in AuxDataset, about 10 TB of public web text up to March 2022. The authors call the result a lower bound.
  - **Setup.** 3,696 prompts over 15 tasks, at temperature 0.7. Temperature had a negligible effect.
  - **Models.** GPT-4o, GPT-4o-mini, GPT-4 Turbo, Claude 3 Opus, Claude 3.5 Sonnet, Claude 3 Haiku, Llama 3.1 (8B, 70B, 405B), Gemini 1.5 Flash and Gemini 1.5 Pro. o1 is in an appendix.
  - **Overlap rate**, meaning the share of output characters inside such substrings:
    - It ranges from about 7% (Gemini) to over 15% (Claude 3 Opus).
    - Expository writing overlaps 3x to 10x more than creative writing.
    - **Encyclopedia-style prompts average over 27%.**
  - **Human baseline.** The human median is 0%. LLM medians go up to 7.5%.
  - **Tail.** Nearly all generations contain a 30-character match. About 2.5% contain a 100-character match, and about 0.01% a 1,000-character match.
  - **Example.** Claude 3 Opus reproduced 1,170 characters of the Wikipedia article on black holes.
  - **Mitigation.** An anti-reproduction system prompt cut Llama's mean overlap from about 16% to about 6%. GPT and Claude dropped by 3 points or more. Matches of 600 to 700 characters still occurred.
  - **Real chat logs.** WildChat and LMSYS-Chat-1M show similar rates.
- **Lu et al., "AI as Humanity's Salieri" (Creativity Index)**, arXiv:2410.04265. https://arxiv.org/abs/2410.04265 [P]
  - On average, professional human authors score 66.2% higher than LLMs.
  - The method, DJ Search, combines verbatim matches with near-verbatim matches under a Word Mover's Distance threshold δ. The value of δ was not seen.
- **Lee et al., "Deduplicating Training Data Makes LMs Better"**, arXiv:2107.06499. https://arxiv.org/abs/2107.06499 [P]
  - Over 1% of unprompted output is copied from training data.
  - Models trained on deduplicated data emit memorized text 10x less often.
- **Lee et al., "Do Language Models Plagiarize?"** (WWW'23, arXiv:2203.07618) [P]
  - Rates for GPT-2 outputs: 0.47 to 0.70% verbatim, 0.16 to 0.32% paraphrase, 0.05 to 0.38% idea plagiarism.
  - The detector favours precision, so these rates are lower bounds.

### 3a.2 Books and extraction

- **Cooper et al., arXiv:2505.12546** (v6 dated 20 Jul 2026; COLM 2026). https://arxiv.org/abs/2505.12546 [P]
  - Study: 200 books and 14 open-weight models.
  - Finding: most models do not memorize most books, but Llama 3.1 70B memorizes *Harry Potter 1* nearly entirely.
- **Ahmed, Cooper, Koyejo, Liang, "Extracting books from production language models"**, arXiv:2601.02671 (6 Jan 2026). https://arxiv.org/abs/2601.02671 [P]
  - **Metric (nv-recall).** Ordered difflib verbatim blocks, merged across small gaps. Blocks of 20 words or more are kept, then 100 words or more after a second merge. The rules are deliberately conservative.
  - **Harry Potter 1 results:**

    | Model | nv-recall | Jailbreak |
    |---|---|---|
    | Claude 3.7 Sonnet | 95.8% | Best-of-N jailbreak, N=258 |
    | Gemini 2.5 Pro | 76.8% | None |
    | Grok 3 | 70.3% | None |
    | GPT-4.1 | 4.0% | Refused after chapter 1 |

  - Most other runs scored 10% or less. The tests ran from August to September 2025.
- **Liu, Mireshghallah, Ginsburg, Chakrabarty, "Alignment Whack-a-Mole"**, arXiv:2603.20957 (v5 Sep 2026). https://arxiv.org/abs/2603.20957 [P]
  - Method: GPT-4o, Gemini 2.5 Pro and DeepSeek-V3.1 were fine-tuned to expand summaries.
  - Result: they reproduced up to 85 to 90% of held-out books, with single spans of more than 460 words.
- **Nasr et al.**, arXiv:2311.17035. https://arxiv.org/abs/2311.17035 [P]
  - Memorized is defined as a verbatim span of 50 tokens or more in AuxDataset.
  - About $200 of queries extracted more than 10,000 unique examples from gpt-3.5-turbo. The divergence attack raises the emission rate 150x.
- **Ippolito et al.**, arXiv:2210.17546. https://arxiv.org/abs/2210.17546 [P]
  - A 10-gram Bloom filter blocks all verbatim memorization.
  - Style-transfer prompts still leak near-verbatim text.
  - **So an exact-match filter alone gives false safety.**
- **Shilov, Meeus, de Montjoye, "The Mosaic Memory of LLMs"**, arXiv:2405.15523. https://arxiv.org/abs/2405.15523 [P]
  - A fuzzy duplicate contributes "as much as 0.8 of an exact duplicate" to memorization.
- **Benchmarks:**
  - **CopyBench** (arXiv:2407.07087), literal copying, defined as ROUGE-L > 0.8 on a 50-word continuation:

    | Model | Literal copying | Other |
    |---|---|---|
    | Llama3-8B to 70B | 0.2% to 10.5% | |
    | GPT-4-Turbo | 0.4% | 3.4% plot events |

  - **CoTaEval** (arXiv:2406.18664). [P]
    - It uses eight metrics: LCS, ROUGE, word ACS, Levenshtein, MinHash Jaccard over 3-grams, and MiniLM cosine.
    - The MemFree decoding filter (n=6) cost MMLU 53.9 to 47.3, and misspellings bypass it.
    - No mitigation won on all metrics.

### 3a.3 Mathematics specifically

- **Llemma** (arXiv:2310.10631). https://arxiv.org/html/2310.10631 [P]
  - 30-gram hits in OpenWebMath and AlgebraicStack cover about 7% of MATH test statements and 0.6% of solutions.
- **Wu et al.** (arXiv:2507.10532). https://arxiv.org/html/2507.10532v3 [P]
  - Prompted with 60% of a MATH-500 problem, Qwen2.5-Math-7B completes the rest exactly 54.6% of the time. Llama3.1-8B does so 3.8% of the time.
  - On newer sets (AIME2025, LiveMathBench) exact completion is about 0 to 5%.
- **GSM1k** (arXiv:2405.00332). https://arxiv.org/abs/2405.00332 [P]
  - Accuracy drops by up to 8% on fresh problems of the same kind. Frontier models show minimal overfitting.
- **Infini-gram mini** (arXiv:2506.12229). https://arxiv.org/abs/2506.12229 [P]
  - It indexes 83 TB, and the index is about 44% of the corpus size.
  - GSM8K contamination reaches up to 74.2%.
- **MathPile** (arXiv:2312.17120) reports near-duplicates within ProofWiki. [snippet; not read from the primary source]
- **No study was found that measures how much LLM-written mathematical prose copies Wikipedia, ProofWiki or textbook phrasing.**
  - The closest proxy is Aerni et al.'s encyclopedia category: more than 27% overlap at the 50-character threshold.
  - **Inference, not a measurement:** math prose plausibly sits at the high end, because definitions and statements are stereotyped and expository.

### 3a.4 Detection methods and parameters used in the literature

| Method | Parameters | Source |
|---|---|---|
| Exact substring (suffix array) | 50 BPE tokens (Lee ExactSubstr, Nasr); 50 characters (Aerni) | 2107.06499, 2311.17035, 2411.10242 [P] |
| Infini-gram | suffix-array n-gram counts over 5T tokens, millisecond queries | https://arxiv.org/abs/2401.17377 [P] |
| OLMoTrace | maximal verbatim spans in 4.6T tokens, keeping the K=ceil(0.05 L) lowest-unigram-probability spans (median 10 tokens); a tracing tool with no accuracy figure | https://arxiv.org/html/2504.07096 [P] |
| MinHash/LSH | Lee NearDup: 5-grams, 9,000 hashes in 450x20 bands, Jaccard 0.8, then edit similarity > 0.8. FineWeb: 5-grams, 112 hashes in 14x8 bands, about 0.75 target (match probability 77% at 0.75, 92% at 0.8) | 2107.06499, https://arxiv.org/abs/2406.17557 [P] |
| ROUGE-L / BLEU | ROUGE-L > 0.8 (CopyBench); BLEU > 0.75 (Ippolito) | as above [P] |
| Embeddings | MiniLM cosine with no threshold (CoTaEval); WMD below δ (DJ Search); no reported accuracy | as above [P] |
| Paraphrase plagiarism | BM25 top-10, then PAN-2014 alignment, then a RoBERTa classifier. Precision 0.96 to 1.0 on verbatim copies, recall 0.72 to 0.79 on paraphrases and 0.3 to 0.41 on ideas | 2203.07618 [P] |
| Human detection | People spot GPT-3 paraphrases with 53% accuracy; the best detector reaches 66% F1 | https://arxiv.org/abs/2210.03568 [P] |
| GitHub Copilot filter | blocks matches of 65 lexemes or more (about 150 characters) against public code | https://github.com/features/copilot [P] |
| Gemini API | `finishReason: RECITATION`; `citationMetadata.citationSources[]` with start and end index, uri and license; no published threshold | https://ai.google.dev/api/generate-content [P] |

No Anthropic or OpenAI documentation of an output recitation filter was found (UNVERIFIED, absence of evidence).

### 3a.5 Recommended detection recipe (proposal, not validated on math text)

1. **Reference corpus.** Use a reference corpus, not only the cited sources:
   - Wikipedia math pages, ProofWiki, nLab, PlanetMath;
   - Math.StackExchange and MathOverflow dumps;
   - Stacks and Kerodon (GFDL);
   - OpenWebMath or arXiv;
   - licensed textbooks where indexing is permitted;
   - optionally, the infini-gram or infini-gram mini web-scale API.
2. **Normalize.** Lowercase, collapse whitespace and canonicalize LaTeX. Mask display and inline formulas as a placeholder, so that standard formulas do not trigger matches.
3. **Exact check (suffix array).**
   - Flag any normalized prose span of **50 characters or more**.
   - Block for rewrite at **100 characters or more**, or when the 50-character overlap rate exceeds about 10% of the node.
   - Use an allowlist of stock phrases ("let G be a group"), calibrated on human-written math.
4. **Near-duplicate check.** Run 5-gram MinHash per paragraph, with FineWeb-style 112 hashes in 14x8 bands, taking candidates at Jaccard 0.5 or more. Confirm with either of these:
   - ROUGE-L of 0.8 or more;
   - difflib blocks of 20 words or more (nv-recall-style rules).
5. **Paraphrase check.** Retrieve the top-k candidates by embedding or BM25. Send pairs above a high cosine threshold, initially 0.9 and uncalibrated, to human review. Expect low recall.
6. **Calibrate** every threshold by running the same pipeline over human-written Wikipedia and ProofWiki pages checked against each other.
7. **Record match spans** (source, uri, licence, offsets), as Gemini's citation metadata does. When a match is to a CC BY-SA source, attribute it and keep the share-alike terms. Never keep a GFDL match.

### 3a.6 Could not verify (memorization)

- How ParaPO's regurgitation metric is defined (arXiv:2504.14452 reports 17.3 to 12.9 for Llama3.1-8B creative writing).
- The value of δ in DJ Search.
- Cooper et al.'s coverage figure of 90.89% [snippet, older version].
- The GPT-4 50-character and Llama 3 8-gram contamination rules [secondary snippet only].
- MathPile's ProofWiki duplication [snippet].
- BloomScrub numbers (arXiv:2504.16046) [snippet].
- Gemini's recitation threshold.
- The Nature Communications version of Mosaic Memory, which needs a login.

## 3b. The legal position of LLM output

### United States

- **Copyright Office, Part 2: Copyrightability (29 Jan 2025)** [P]. https://www.copyright.gov/ai/Copyright-and-Artificial-Intelligence-Part-2-Copyrightability-Report.pdf (read 2026-10-26). Its summarised conclusions:
  - "Copyright does not extend to purely AI-generated material, or material where there is insufficient human control over the expressive elements."
  - "Based on the functioning of current generally available technology, prompts do not alone provide sufficient control."
  - "Human authors are entitled to copyright in their works of authorship that are perceptible in AI-generated outputs, as well as the creative selection, coordination, or arrangement of material in the outputs, or creative modifications of the outputs."
  - "The case has not been made for additional copyright or sui generis protection for AI-generated content."
  - Whether a human contribution is enough is decided "on a case-by-case basis".
- **Copyright Office, Part 3: Generative AI Training** [P]. The pre-publication version came out on 9 May 2025. On 2026-10-26, https://www.copyright.gov/ai/ still lists it as pre-publication: "A final version of Part 3 will be published in the future, without any substantive changes expected." Two of its points matter here (PDF read 2026-10-26):
  - When a model "can generate verbatim or substantially similar copies of a training example, without that expression being provided externally in the form of a prompt or other input, it must exist in some form in the model's weights". There is then "a strong argument that copying the model's weights implicates the right of reproduction for the memorized examples."
  - "copying the resulting weights will only infringe where there is substantial similarity."
- No Part 4 and no 2026 report is listed on https://www.copyright.gov/ai/ (read 2026-10-26).
- **Thaler v. Perlmutter.** The D.C. Circuit held on 18 Mar 2025 (No. 23-5233) that the Copyright Act requires a human author. The Supreme Court denied certiorari on **2 Mar 2026** (No. 25-449; docket entry "Petition DENIED") [P]. https://www.supremecourt.gov/docket/docketfiles/html/public/25-449.html (read 2026-10-26). A denial is not a ruling on the merits. The human-authorship rule of the D.C. Circuit stands.
- **Training cases.** All of these concern inputs, not the copyrightability of outputs.
  - *Bartz v. Anthropic* (N.D. Cal.): Judge Alsup's order of 23 Jun 2025 held that training on lawfully acquired books was fair use ("exceedingly transformative"). Building a library of pirated copies was not fair use. The case settled for $1.5B. Final approval came on **20 Jul 2026** (3:24-cv-05417-AMO; about 482,460 works) [S: Authors Alliance https://www.authorsalliance.org/2026/07/21/bartz-v-anthropic-settlement-receives-final-approval/ ; vLex https://case-law.vlex.com/vid/bartz-v-anthropic-pbc-1129578441 ; read 2026-10-26].
  - *Kadrey v. Meta* (N.D. Cal., 25 Jun 2025, Chhabria J.): fair use on that record, because the plaintiffs showed no market dilution. The judge warned that "in many circumstances it will be illegal to copy copyright-protected works to train generative AI models without permission" [S: https://chatgptiseatingtheworld.com/2025/06/25/meta-prevails-on-fair-use-in-ai-training-in-kadrey-v-meta-but-judge-chhabria-cautions-a-better-record-of-dilution-or-market-harm-could-prevail-in-other-cases/ ; read 2026-10-26].
  - *Thomson Reuters v. Ross* (3d Cir., No. 25-2153, decided 29 Sep 2026; the opinion was first issued under seal): this is the first appellate ruling on fair use in AI training. Copying Westlaw headnotes to train a non-generative legal search tool was **not** fair use, and the headnotes were held original. The court said that generative AI is a different case [S: https://www.ballardspahr.com/insights/alerts-and-articles/2026/10/third-circuit-addresses-fair-use-in-ai-training-but-leaves-generative-ai-questions-unresolved ; https://patentlyo.com/patent/2026/10/ease-is-not-necessity-third-circuit-affirms-no-fair-use-in-thomson-reuters-v-ross.html ; read 2026-10-26].
  - *NYT v. OpenAI* (S.D.N.Y.): this is the main US case about regurgitated **outputs**. It had no decision on the merits by late October 2026 [S: https://www.nortonrosefulbright.com/en/knowledge/publications/ce8eaa5f/ai-in-litigation-series-an-update-on-ai-copyright-cases-in-2026 ; read 2026-10-26]. Search summaries said that the DOJ filed in support of OpenAI on 28 Sep 2026. UNVERIFIED: the filing was not seen.

### European Union, United Kingdom, Germany

- **GEMA v. OpenAI**, LG München I, 42 O 14139/24, 11 Nov 2025 [S]. The court held that memorisation of song lyrics in the model is a reproduction, and that so is the output of the lyrics in response to prompts. It held that the TDM exception covers making the training corpus but not "permanent embodiment of works in trained models". OpenAI was held liable for the outputs. OpenAI appealed, and no appellate decision was found [S: https://www.williamfry.com/knowledge/gema-v-openai-europes-direction-on-ai-infringement/ ; https://ailawsuittracker.com/cases/gema-v-openai/ ; read 2026-10-26]. The primary judgment text was not read (merlin.obs.coe.int and cms.law returned 403).
- **CJEU C-250/25 *Like Company v. Google*** is the first reference to the CJEU on generative AI. It asks whether chatbot output that partly matches press articles is a reproduction or a communication to the public. The Grand Chamber heard it on 10 Mar 2026. No Advocate General's opinion had been found by late September 2026, and a judgment is expected in 2027 [S: https://www.create.ac.uk/blog/2026/03/03/copyright-and-ai-case-c-250-25-like-company-before-the-grand-chamber-of-the-court-of-justice-of-the-european-union-opinion-by-the-european-copyright-society/ ; https://www.stibbe.com/publications-and-insights/ai-copyright-claims-in-europe-key-considerations ; read 2026-10-26]. UNVERIFIED: whether the opinion came out in October 2026. The CURIA page was not checked.
- **Getty Images v. Stability AI**, [2025] EWHC 2863 (Ch), 4 Nov 2025: the model weights "do not store an infringing copy". Permission to appeal was granted on 16 Dec 2025. No Court of Appeal judgment was found [S: https://www.taylorwessing.com/en/insights-and-events/insights/2026/01/next-steps-for-getty-v-stability-why-has-permission-to-appeal-been-granted ; read 2026-10-26].
- **EU AI Act, duties for GPAI providers (Art. 53 to 55).** They apply from **2 Aug 2025**. The Commission's powers to enforce and fine apply from **2 Aug 2026**. Models placed on the market before 2 Aug 2025 have until 2 Aug 2027 [P: https://digital-strategy.ec.europa.eu/en/policies/guidelines-gpai-providers ; read 2026-10-26]. The duties fall on **providers** of models: technical documentation, a copyright policy and a public summary of the training content. A user of a model's outputs has none of these duties. A "downstream modifier" becomes a provider only if its modification uses more than one third of the original model's training compute (Commission guidelines, July 2025, para 63) [S: https://digital-strategy.ec.europa.eu/en/faqs/guidelines-obligations-general-purpose-ai-providers via search summary; read 2026-10-26]. A math2 fine-tune of an open prover would be far below that threshold.
- **EU AI Act, Art. 50, transparency.** It applies from **2 Aug 2026** [S: https://artificialintelligenceact.eu/article/50/ ; read 2026-10-26].
  - Art. 50(2): providers of systems that generate text must mark outputs "in a machine-readable format ... detectable as artificially generated". This duty falls on the model provider.
  - Art. 50(4): **deployers** that publish AI-generated text "with the purpose of informing the public on matters of public interest" must disclose that the text is artificially generated. This duty does not apply "where the AI-generated content has undergone a process of human review or editorial control and where a natural or legal person holds editorial responsibility for the publication".
  - The Digital Omnibus on AI (Regulation (EU) 2026/1744, in force 27 Jul 2026) reportedly gives a grace period to 2 Dec 2026 for Art. 50(2) marking. Sources disagree on its scope [S: https://fpf.org/blog/the-ai-act-implementation-timeline-what-changes-under-the-ai-omnibus/ ; https://cdp.cooley.com/digital-ai-omnibus-delays-key-deadlines-introduces-new-rules/ ; read 2026-10-26].
  - The Commission's Art. 50 guidelines (20 Jul 2026) and the code of practice on transparency of AI-generated content (10 Jun 2026) are reported but were not read. UNVERIFIED.
  - Whether a mathematics library publishes text "on matters of public interest" is not settled. math2 already discloses AI use per field, so it meets Art. 50(4) whichever way the question is decided.

### What this means for CC BY-SA 4.0 on LLM-written text

1. **Text generated purely by an LLM has, in the US, no copyright for math2 to license.** Prompts alone are not enough (Copyright Office, Part 2). Mathematical facts, definitions and short functional statements have thin protection in any case. Applying CC BY-SA 4.0 to such text is not wrong, but it may be **inoperative**. CC BY-SA 4.0 §8(a) says that the licence does not "reduce, limit, restrict, or impose conditions on any use ... that could lawfully be made without permission", and §2(a)(2) says that "where Exceptions and Limitations apply to Your use, this Public License does not apply" [P: https://creativecommons.org/licenses/by-sa/4.0/legalcode.en ; read 2026-10-26]. Thus share-alike cannot be enforced on unedited machine text.
2. **Creative Commons' own position** [P: https://creativecommons.org/2023/08/18/understanding-cc-licenses-and-generative-ai/ ; read 2026-10-26]: "The CC license you choose will apply to the creative work that you contribute to the final product, even if the portion produced by the generative AI system itself may be uncopyrightable." It also says: "We encourage the use of CC0 for those works that do not involve a significant degree of human creativity." The 2026 CC FAQ does not answer the question for outputs (https://creativecommons.org/faq/ ; read 2026-10-26).
3. **What human work adds.** Human selection, arrangement and creative modification of the corpus are protectable. So are the human-written parts. The graph structure, the curation and the edited nodes can thus carry CC BY-SA. Unedited agent text cannot, at least in the US. In other jurisdictions the answer differs and is unsettled (e.g. the UK CDPA s.9(3) "computer-generated works" rule; not researched here).
4. **The real legal risk is regurgitation, not ownership.**
   - If an LLM reproduces a protected source (a textbook, or GFDL text from Stacks or Kerodon), the third party's copyright attaches to the output.
   - The GEMA judgment held the model provider liable for such output. Whoever republishes the output is also exposed.
   - CC BY-SA text that is reproduced closely needs attribution and share-alike. This is compatible with math2 only when the attribution is recorded.
   - GFDL and NC text are incompatible with math2.
   - Thus every generated node needs a copy check against a reference corpus. The check must cover more than the cited sources, because a node that a model writes from memory may have no cited source at all (see section 3a).
5. **Provider terms do not change this picture.** Each major provider assigns to the customer whatever rights it has in the outputs (section 4). An assignment cannot create a copyright that the law does not recognise.

## 4. Provider terms

### Hosted APIs

| Provider | Ownership of outputs | Publishing outputs | Using outputs to train other models | Source (read 2026-10-26) |
|---|---|---|---|---|
| Anthropic (Commercial Terms, effective 17 Jun 2025) | Customer "owns its Outputs". "Anthropic hereby assigns to Customer its right, title and interest (if any) in and to Outputs." "Anthropic may not train models on Customer Content from Services." | no restriction in the Commercial Terms. The AUP bans "Plagiarize or submit AI-assisted work without proper permission or attribution" and bans infringing third-party IP. | Commercial Terms D.4: may not "access the Services to build a competing product or service, including to train competing AI models". **AUP (effective 12 Nov 2026): may not "Use outputs to train an AI model (e.g., "model scraping" or "model distillation") without prior authorization from Anthropic".** | https://www.anthropic.com/legal/commercial-terms ; https://www.anthropic.com/legal/aup |
| OpenAI (Services Agreement ONLINE v.010126) | §4.1: Customer "owns all Output. OpenAI hereby assigns to Customer all OpenAI's right, title, and interest, if any, in and to Output." §4.4: "Output may not be unique, and other users may receive similar content". | no restriction found in the agreement | §3.3(e): may not, "except for a Permitted Exception, use Output to develop artificial intelligence models that compete with OpenAI's products and services". The Permitted Exceptions are (a) classifiers and embeddings that are "not distributed or made commercially available to third parties", and (b) fine-tuning on OpenAI's own services. | https://cdn.openai.com/osa/openai-services-agreement.pdf (openai.com returned 403; the CDN PDF was read with pdftotext) |
| Google (Gemini API Additional Terms, last updated 2026-04-28) | "Google won't claim ownership over that content." "Google may generate the same or similar content for others." | "You're responsible for your use of generated content, and for the use of that content by anyone you share it with." Applicable law "may require the provision of attribution to your users when returned as part of an API call". | "You may not use the Services to develop models that compete with the Services (e.g., Gemini API or Google AI Studio)." | https://ai.google.dev/gemini-api/terms |

The consequences for math2:

- **Publishing is allowed by all three providers.** None of them claims the outputs, and none forbids publication under an open licence.
- **The training clauses bind math2, and math2's downstream users, differently.**
  - math2 itself may not use Claude outputs to train or fine-tune a model without Anthropic's authorisation, under the AUP that takes effect on 12 Nov 2026. This rules out a math2 "fine-tune an open prover on our Claude-written proofs" plan unless it is authorised. The same holds for OpenAI and Google outputs in any model that "competes".
  - Third parties who later download math2 under CC BY-SA have no contract with Anthropic. Whether the AUP reaches them is a contract question, not a copyright question. The licence of math2 cannot pass a training ban on to them, and it should not try to.
- **Dataset releases need provenance.** The model id recorded for each field (06.5) lets a downstream user filter the corpus by model family when that user's own terms require it.
- **Indemnities** apply only to the customer's paid use. They are not a reason to skip copy checks.
  - Anthropic Commercial Terms K.1: Anthropic defends claims "alleging that Customer's paid use of the Services" violates third-party IP.
  - OpenAI reportedly extends its indemnity to output in its Service Terms of 12 Jun 2026 [S, not read].

### Open-weight model licences, checked on Hugging Face and GitHub on 2026-10-26

| Family (newest seen) | Licence | Extra duties on outputs or on models trained with outputs |
|---|---|---|
| Llama 4 (Scout/Maverick; no newer Meta LLM on HF) | Llama 4 Community License | **Yes.** "If you use the Llama Materials or any outputs or results of the Llama Materials to create, train, fine tune, or otherwise improve an AI model, which is distributed or made available, you shall also include "Llama" at the beginning of any such AI model name." "Built with Llama" must be displayed when such a model or derivative is distributed. Over 700M MAU needs a separate licence. The licence ends if you sue Meta claiming that Llama outputs infringe. https://raw.githubusercontent.com/meta-llama/llama-models/main/models/llama4/LICENSE |
| DeepSeek V4 / V4.1 (Flash, Pro; Sep to Oct 2026) | MIT (HF tag `license:mit`) | none. |
| DeepSeek-Prover-V2 (Apr 2025) | **conflicting.** The HF LICENSE file is MIT, but the README says "subject to the Model License" (DeepSeek Model Agreement). | The DeepSeek Model Agreement defines "Derivatives of the Model" to include any model trained on "synthetic data [generated] by the Model". Use restrictions (Attachment A) flow down to derivatives. §6: "DeepSeek claims no rights in the Output". https://raw.githubusercontent.com/deepseek-ai/DeepSeek-V3/main/LICENSE-MODEL |
| DeepSeek-Math-V2 (Nov 2025) | Apache-2.0 | none |
| Qwen3.5 (Feb 2026) | Apache-2.0 | none |
| **Qwen3.8 (Aug 2026)** | **Qwen Community License 1.0** (new in 2026) | Commercial products with more than 100M MAU or more than US$20M monthly revenue must display the model name. A "Model as a Service" or "AI Work Assistant" business needs a separate licence. "Internal Use" is exempt "provided that such Use does not make the Software, its outputs, or its underlying model capabilities available to any third party". It is unclear whether publishing a corpus of outputs is a non-internal use. https://huggingface.co/Qwen/Qwen3.8-Flash-Next-FP8/raw/main/LICENSE |
| Kimi K2 (2025) | Modified MIT | display "Kimi K2" above 100M MAU or $20M monthly revenue |
| **Kimi K3 (Jun 2026)** | Kimi K3 License | "Model as a Service" businesses above $20M revenue in 12 months need an agreement. Display "Kimi K3" above the MAU or revenue thresholds. The exemption for internal use is again tied to not making "the Software, its outputs, or its underlying capabilities available to third parties". https://huggingface.co/moonshotai/Kimi-K3/raw/main/LICENSE |
| GLM-4.6 / GLM-5 (Feb 2026) | MIT | none |
| Mistral Large 3 (Dec 2025), Leanstral 1.5 (Jul 2026) | Apache-2.0 | none |
| gpt-oss-120b/20b (Aug 2025) | Apache-2.0 plus a one-line usage policy ("comply with all applicable law") | none |
| Goedel-Prover-V2 | Apache-2.0 | none |
| Kimina-Prover-72B | MIT | none |

- In 2026, the Chinese labs moved from plain MIT or Apache to custom licences: Qwen 3.8 and Kimi K3.
- For a corpus that math2 publishes, the duties that matter are these. Llama's naming duty applies only if someone trains a distributed model on Llama outputs. The DeepSeek Model Agreement flows its restrictions down to any model trained on the outputs. The internal-use exemptions of Qwen 3.8 and Kimi K3 do not cover outputs made available to third parties, so the MaaS and display thresholds apply. math2 is far below the thresholds, but the terms should be recorded per model.
- MIT, Apache-2.0 and the gpt-oss terms add nothing.

## 5. Cost

### List prices on 2026-10-26 (USD per million tokens; official pages only)

Anthropic, from https://platform.claude.com/docs/en/about-claude/pricing (read 2026-10-26):

| Model | Input | Cache hit | Output | Batch input | Batch output |
|---|---|---|---|---|---|
| Claude Fable 5.1 | $10 | $0.25 (0.025×) | $50 | $5 | $25 |
| Claude Opus 5.5 | $4 | $0.20 (0.05×) | $20 | $2 | $10 |
| Claude Sonnet 5.5 | $2 | $0.10 (0.05×) | $10 | $1 | $5 |
| Claude Haiku 5.5, prompt ≤100k tokens | $0.10 | $0.01 | $0.50 | $0.05 | $0.25 |
| Claude Haiku 5.5, prompt >100k tokens | $0.50 | $0.05 | $2.50 | $0.25 | $1.25 |

- **Cache writes:** 1.25× input for a 5-minute TTL and 2× for 1 hour. "These multipliers stack with other pricing modifiers, including the Batch API discount".
- **Long context:** "Claude 4.6 and later models (except Claude Haiku 5.5) ... include the full 1M token context window at standard pricing."
- **Data residency:** `inference_geo: "us"` costs 1.1×.
- **Fast mode** for Opus 5.5 costs $8/$40, and it is not available with Batch.
- **Batch API** (https://platform.claude.com/docs/en/build-with-claude/batch-processing ; read 2026-10-26):
  - a 50% discount;
  - up to 100,000 requests or 256 MB per batch, "most batches completing within 1 hour", expiry at 24 h, results kept 29 days;
  - "cache hits are provided on a best-effort basis. Users typically experience cache hit rates ranging from 30 percent to 98 percent".
- The prices in spec 12.3 match this page.

OpenAI, from https://developers.openai.com/api/docs/pricing (platform.openai.com redirects there; read 2026-10-26), short context ≤272K input:

| Model | Input | Cached | Output | Batch in / cached / out |
|---|---|---|---|---|
| gpt-6-astra | $10 | $1.00 | $50 | $5 / $0.50 / $25 |
| gpt-6.1-sol | $2 | $0.10 | $10 | $1 / $0.05 / $5 |
| gpt-6-luna | $0.10 | $0.01 | $0.50 | $0.05 / $0.005 / $0.25 |

- Long context costs more: gpt-6-astra $20/$75, gpt-6.1-sol $4/$15.
- Flex rates equal the Batch rates.

Google, from https://ai.google.dev/gemini-api/docs/pricing ("Last updated 2026-10-22 UTC"; read 2026-10-26):

| Model | Input | Output | Batch |
|---|---|---|---|
| Gemini 4 Argon | $2.00 until 30 Nov 2026, then **$4.00** from 1 Dec 2026 | $10.00, then **$20.00** | 50% |
| Gemini 3.8 Flash | $0.75 until 31 Dec 2026, then $1.50 | $3.75, then $7.50 | 50% |
| Gemini 3.5 Flash-Lite | $0.30 | $2.50 | 50% |

Google's prices for cached input were not read.

### An estimate for one million nodes

The token counts below are **assumptions, not measurements**. Calibrate them on the first 1,000 real runs (the cost per merged output in 12.9). Tokenizers differ between providers, so the same text costs a different number of tokens.

- **Generation call:** 10k input, of which 8k is a cached shared prefix (system prompt, style guide, neighbourhood) and 2k is unique. 6k output, including thinking.
- **Three check calls** (grounding or judge, critic, back-translation or consistency), each with 8k input (2k cached, 6k unique) and 2k output.
- All calls use Batch. Cache reads are assumed to hit.

| Configuration | $ per node | $ per 1M nodes |
|---|---|---|
| all Claude Haiku 5.5 | 0.004 | ~$4k |
| all Claude Sonnet 5.5 (≈ all gpt-6.1-sol) | 0.081 | ~$81k |
| all Claude Opus 5.5 | 0.161 | ~$161k |
| all Claude Fable 5.1 (≈ all gpt-6-astra) | 0.40 | ~$400k |
| generate on Opus 5.5; check with 2 Sonnet 5.5 calls and 1 gpt-6.1-sol call (cross-family) | 0.113 | ~$113k |
| generate on Fable 5.1; checks as above | 0.209 | ~$209k |
| all Sonnet 5.5, zero cache hits | 0.094 | ~$94k |
| all Gemini 4 Argon, before / after 1 Dec 2026 (no cache price assumed) | 0.094 / 0.188 | ~$94k / ~$188k |

- **Divide by the acceptance rate.** At a 50% first-pass acceptance rate, the cost doubles.
- **Output tokens dominate:** 70 to 85% of the cost in every configuration. The thinking budget is the lever that matters most, not the size of the context.
- **Formal work is not in this table.** A Lean statement with its F1 battery, and above all a Lean proof, costs orders of magnitude more per item (section 2). Price it separately.
- At one million nodes, informal generation costs tens to hundreds of thousands of dollars. The cost is much smaller than the review capacity that math2 would need for the output (10.6), and much smaller than formal proof.

### 5.1 Model facts that bear on scale

Source: https://platform.claude.com/docs/en/about-claude/models/overview [P].

- **Model ids:**
  - `claude-fable-5-1`
  - `claude-opus-5-5`
  - `claude-sonnet-5-5`
  - `claude-haiku-5-5`
- **Context and output:** a 1M-token context window, and up to 128K output tokens per request. On the Batch API, output can go to 300K with the beta header `output-300k-2026-03-24`.
- **Training cutoff:** reliable knowledge and training data go to **June 2026**. Mathematics published after that date is unknown to the models unless it is retrieved into the prompt. This bears on explore and novelty checks (16.5 item 5): a model's "this is new" is worth nothing for results after its cutoff.
- **Retirement:** each model is retired no sooner than 1 Sep, 22 Sep, 28 Sep and 7 Oct 2027 respectively. A million-node campaign that runs past those dates will cross a model change. This is a further reason to record exact model ids (12.3) and to keep `m2 prov quarantine` working.
- **Size rule of thumb:** 1M tokens is about 555k words, or about 2.5M characters.
  - So a node of 1,500 words of prose is about 2.7k tokens, before LaTeX. LaTeX tokenizes worse than prose.
  - The token assumptions in 5 are plausible, but they still need calibration.

## 6. Community norms on LLM text (short)

- **Wikipedia.** Speedy-deletion criterion G15 for unreviewed LLM-generated pages was adopted in August 2025, and later renamed "Unambiguously LLM-generated pages". https://en.wikipedia.org/wiki/Wikipedia:Speedy_deletion [S, the policy page as summarised; the adoption discussion is in the talk archives]. WikiProject AI Cleanup exists to find and fix such text (https://en.wikipedia.org/wiki/Wikipedia:WikiProject_AI_Cleanup) [S].
  - Wikipedia is a source that math2 imports under CC BY-SA (16.6). math2 must not push unreviewed agent text back into it (16.5 "Upstream etiquette" already forbids bulk writes).
- **arXiv.** On 31 Oct 2025, arXiv said that review articles and position papers in the CS category must have been peer-reviewed before submission, citing the flood of LLM-written submissions. https://blog.arxiv.org/?p=1740 [S, read via summary].
- **Grokipedia** (section 2.4) is the large-scale counter-example: 56% of its articles carry Wikipedia's CC footer, and no error rate is published.

These norms agree with spec 16.5. They add one point: **communities sanction unreviewed machine text in bulk, not the use of AI as such.** math2's protection is the marker plus human confirmation (P04). It should not ship unconfirmed prose as if it were reviewed.

## 7. What the spec should require because of this

The numbers in brackets point to the evidence above. Each item names the spec section it changes.

### 7.1 Quality and checking (spec 10, 12, 15)

1. **No LLM verdict is evidence of correctness for an informal proof (12.6, 16.5 item 4).**
   - Evidence: LLM judges inflate scores up to 20x [1.2]. The best step judges accept about 21% of wrong steps, and weaker judges 56 to 90% [1.2].
   - Requirement: agent-written informal proofs stay E1, and a judge pass is recorded as triage only. The spec should say this explicitly for judges, as it already does for proofs.
2. **No Haiku-tier or weak open-model judge for proofs or statements (12.3).**
   - Evidence: Hard2Verify finds that "weaker verifiers cannot catch errors". Claude Sonnet 4-class judges flag only 43.7% of wrong steps [1.2].
   - Requirement: limit tier 0 to triage, classification and dedup pre-filters. Proof and statement judging starts at the tier that the run records show reaches a measured true-negative rate, for example 75% or more on a labelled set.
   - Grayzel's result supports a cheap cross-family consensus [1.2]. Treat that as something to measure, not to assume.
3. **Judges vote across families; accept by unanimity, reject by majority (10.4 check 9, 12.6).**
   - Evidence: a unanimous vote gives the best precision, and a majority vote the best recall [1.2, 1.5]. Cross-model agreement is the best citation check that has been measured [1.5].
   - Requirement: record each judge's verdict and model id in the run record. Disagreement goes to a human, and is never settled by a tiebreak call.
4. **An anti-sycophancy check in prove and formalize (10.4, 12.4).**
   - Evidence: strong models "prove" false statements 29 to 97% of the time [1.1]. A "prove or disprove" framing raised the rejection rate of false statements from 18.5% to 71% [1.5].
   - Requirements:
     - (a) The prompt of every prove and formalize job is framed as "prove or disprove".
     - (b) A periodic canary: inject perturbed false statements into the queue, and measure how many the agents "prove". Publish the rate in the weekly report (12.9).
     - Check 6 of the F1 battery (cheap negation) only catches cheaply false statements. It does not catch the subtle ones that BrokenMath and BrokenArXiv use.
5. **Compiling and back-translation are not fidelity (10.4).**
   - Evidence: Kimina compiles 90% of statements but only 66% are accurate. Unsupervised autoformalization is faithful 60 to 90% of the time [2.3], and less often on graduate material.
   - Requirements:
     - Set the prior error rate for F1 at 10 to 40% until math2's own sampled re-review (10.8) measures it.
     - Do not treat F1 as "probably right" in the workspace.
6. **Measure the informal error classes that no one has measured (10.8, 17.3).**
   - Evidence: there is no published rate for wrong definitions, missing hypotheses or misnamed theorems in informal LLM text [1.3].
   - Requirement: add an informal audit beside the F2 re-review. Each month, a blind expert audit of a seeded sample of agent-written definitions and statements, classified with M-* style classes for informal text:
     - wrong definition;
     - dropped hypothesis;
     - misattributed name;
     - non-standard convention unstated.
   - Without this, the published error rates in release notes cover only formal bindings.
7. **Source records written by agents must be resolved and checked (04 `sources:`, a new rule in G or P).**
   - Evidence: 11.4 to 56.8% of generated citations are hallucinated [1.4]. Usable theorem references in research proofs run at 7% or less [1.3]. Fabrication is worst on unfamiliar topics [1.4].
   - Requirement: a hard rule. A `sources:` entry added by an agent must resolve to a real record (DOI, zbMATH, arXiv id, ISBN, or a record-plane entry), and the resolution is logged. A locator ("Thm 3.1") written by an agent counts toward E2 or E3 only after it is checked against retrievable text, or after a person confirms it.
   - Also: theorem names in prose ("by the X–Y theorem") must link to a node or carry a source. A name that resolves to nothing fails.
8. **Prefer pseudo-formal structure for informal proofs (12.4, 04).**
   - Evidence: pseudo-formalization raised error recall from 0.54 to 0.66, but still missed about 40% of errors on arXiv proofs [1.5].
   - Requirement: agent-written informal proofs use a step-structured format with each step's justification as a link or a citation. This helps the judge, but it does not change E1.
9. **Watch for checker gaming (11.3, X rules).**
   - Evidence: in FormalQualBench, agents tried elaborator-level workarounds [2.1]. Aristotle's results were flagged as "not Comparator-validated" [2.1].
   - Requirement: the comparator-pinned compile (check 10) and the axiom audit are mandatory for every accepted proof, with no exception for "trusted" provers.

### 7.2 Scale planning (spec 12.3, 12.9, 10.6)

10. **Budget review, not generation.**
    - Evidence: a million informal nodes costs about $4k to $400k in model calls, depending on the tier mix [5]. The published formal campaigns cost about $17 per declaration, or $300 per target theorem [2.1]. No large agent run has published a misformalization rate [2].
    - Requirements:
      - The spec's throughput targets (10.6) must be set against review capacity.
      - The weekly report should publish **cost per accepted node and per F2 binding**, and not only spend.
11. **Publish a fidelity rate before claiming scale.** No prior project publishes a misformalization rate for its agent-written library [2]. math2's sampled re-review (10.8) is a differentiator. The spec should require that a release with more than N agent-written bindings is not tagged until the sample has been drawn and measured.
12. **Plan for model churn.**
    - Facts: the current models retire from Sep 2027, and their knowledge stops at June 2026 [5.1].
    - Requirements:
      - Novelty checks (16.5 item 5) must retrieve, and not rely on model memory.
      - Every campaign record states the model ids, so that `m2 prov quarantine` can act on them.

### 7.3 Copying and licence (spec 15 L-rules, 16.6)

13. **Extend L04: compare with a reference corpus, not only with cited sources (15 L04).**
    - Evidence: LLMs reproduce text without being asked. Over 27% of encyclopedia-style output sits in verbatim spans of 50 characters or more. One case reproduced 1,170 characters of Wikipedia [3a.1]. Text a model writes from memory has no cited source to compare against.
    - Requirement: a new rule `L05` (S, rising to H above a hard threshold), run on every agent-written prose field. Check it against an indexed reference corpus:
      - Wikipedia math, ProofWiki, nLab and PlanetMath;
      - Math.SE and MathOverflow, under the per-post rules of 16.6;
      - **Stacks and Kerodon (GFDL)**;
      - OpenWebMath or arXiv text, for detection only;
      - any licensed textbook index.
    - Parameters to start with, and then to calibrate [3a.5]:
      - Text normalization: LaTeX normalized and formulas masked.
      - Exact matching: flag a span of 50 characters or more; hard-fail at 100 characters or more against a GFDL, NC or all-rights-reserved source; flag at 10% or more covered.
      - Near-duplicate matching: 5-gram MinHash with Jaccard 0.5 as the candidate threshold, confirmed with ROUGE-L 0.8 or more, or with difflib blocks of 20 words or more.
      - A stock-phrase allowlist.
    - Every threshold is calibrated on human-written math pages, whose median overlap is 0% [3a.1].
14. **An exact-match filter is not enough.**
    - Evidence: a 10-gram filter blocks verbatim copying while near-verbatim text gets through, and fuzzy duplicates count about 0.8 of exact ones [3a.2].
    - Requirement: L05 needs a near-duplicate stage, and a paraphrase stage that is sampled and sent to humans, not blocking.
15. **Record match spans.** For each L05 hit, record the source, URI, licence and offsets in provenance. This follows Gemini's citation metadata [3a.4]. A hit against a CC BY-SA source becomes an attributed import (L03). A hit against GFDL, NC or arXiv-default text is rewritten (L02).
16. **Anti-reproduction instructions in every prose prompt (12.8 prompts).**
    - Evidence: such a system prompt cut mean overlap by more than half in one model, but long matches remained [3a.1].
    - This is a cheap first layer, not a control.
17. **Licence the unedited agent text honestly (16.6).**
    - Evidence: in the US, purely AI-generated text has no copyright, and CC BY-SA cannot bind uses that need no permission (§8(a), §2(a)(2)) [3b]. CC itself suggests CC0 for works without significant human creativity [3b].
    - The spec should choose one of these:
      - **(a)** Keep CC BY-SA for all prose. Add a notice that the licence applies to the extent that copyright subsists, and that agent-written, unconfirmed fields may be in the public domain in some jurisdictions. Provenance (P04) shows which fields these are.
      - **(b)** Release unconfirmed agent prose under CC0 until a person edits or confirms it. It then becomes CC BY-SA.
    - Option (a) is simpler and keeps one prose zone. Option (b) is more honest, but makes licence zones depend on provenance. **Recommendation: (a) with the notice.** The collection, its selection and arrangement, and human-edited text stay under CC BY-SA in any case.
    - Text an LLM regurgitates from a CC BY-SA source keeps that source's terms whatever math2 says. This is why item 15 matters.
18. **Art. 50(4) disclosure is satisfied by provenance (16.5).** EU deployer disclosure for published AI text applies from 2 Aug 2026, unless a human reviewed the text and someone holds editorial responsibility [3b]. math2's per-field markers (P04) and named responsible persons (P03) cover both routes. The spec should say this in 16.5 item 7, and the site should carry a machine-readable AI-generated flag on unconfirmed fields.

### 7.4 Provider and model terms (spec 12.3, 20.2, 06.5)

19. **No training on hosted-model outputs without authorisation (12.3, 20.2).**
    - Facts:
      - Anthropic's AUP, in force from 12 Nov 2026, forbids using outputs to train an AI model without prior authorisation.
      - OpenAI and Google forbid using outputs to build "competing" models [4].
    - Requirement: any plan to fine-tune the open provers of tier 0′ on math2 data must exclude, or get authorisation for, fields generated by hosted models. Provenance must make that filter mechanical. Add it to the provenance tests of 06.5: "list each field produced by model family X".
20. **Record each model's licence and its output terms (12.3 price table, a new `models.reg`).**
    - Several open-weight licences have output clauses:
      - Llama 4 naming;
      - the DeepSeek Model Agreement derivatives clause, and the LICENSE file that conflicts with the README for DeepSeek-Prover-V2;
      - the Qwen 3.8 and Kimi K3 internal-use exemptions, which exclude outputs made available to third parties [4].
    - Requirement: a register of models with these fields: licence, URL, date read, output clauses, and thresholds. The orchestrator refuses a model with no entry.
    - Prefer MIT or Apache models for tier 0′: DeepSeek V4, DeepSeek-Math-V2, Goedel-Prover-V2, Kimina, Leanstral, GLM, gpt-oss.
21. **math2's licence does not pass provider training bans on to users (16.6).** A CC BY-SA licence cannot add a no-training term, and math2 should not try. The release notes can state which fields came from which provider. Downstream users with their own contracts can then filter.

### 7.5 Cost accounting (12.3, 12.9)

22. **Price table and prices in `llm.Prices` must be dated, and they must model batch, cache read, cache write (1.25x / 2x), the Haiku >100k tier, and residency (1.1x)** [5].
    - Google's Gemini 4 Argon price doubles on 1 Dec 2026, and Gemini 3.8 Flash on 1 Jan 2027 [5]. A static table goes stale within weeks.
23. **Thinking tokens are the cost lever.** Output, including thinking, is 70 to 85% of the modelled cost [5]. The weekly report should give output tokens per accepted node for each role and tier. Effort settings (12.3) should be tuned on that number.

## 8. Open doubts

1. **No measured error rate for informal LLM definitions and statements.** Section 1.3 found none. The prior in 7.1 item 5 is borrowed from formal autoformalization, and may be wrong in either direction for prose.
2. **The mathematical reproduction rate is unmeasured.**
   - The figure of over 27% at 50 characters is for encyclopedia prose in general, and was measured on 2024 models against a 2022 web snapshot.
   - Math prose may be higher, because of stereotyped wording, or lower, because formulas are masked.
   - The L05 thresholds in 7.3 are untested on mathematics.
3. **What "copying" means for mathematical statements.** Standard definitions have few reasonable wordings, and the merger doctrine and thin protection may apply. A strict L05 could flood review with false positives. Calibration on human-written pages is required before L05 can block.
4. **Copyright in AI output outside the US.**
   - The UK CDPA s.9(3) "computer-generated works" rule and EU originality law were not researched.
   - The CJEU judgment in C-250/25 (expected 2027) and the GEMA appeal may change the EU position on memorized output.
5. **Whether the Anthropic AUP's training ban reaches third parties** who download math2. Their only relation is through CC BY-SA, and not through any contract. Legal advice is needed before math2 makes any claim either way.
6. **DeepSeek-Prover-V2's licence conflict** (an MIT LICENSE file, against a README pointing to the Model Agreement). Which one governs is unclear. Ask DeepSeek, or avoid using its outputs for training.
7. **Qwen 3.8 and Kimi K3 "internal use".** It is not clear whether publishing a corpus of outputs is a use that "makes outputs available to third parties" in a way that triggers the MaaS clauses. math2 is far below the revenue thresholds, but the reading is uncertain.
8. **Art. 50(4) scope.** Whether a mathematics library publishes "matters of public interest" is unsettled. The Commission's Art. 50 guidelines (20 Jul 2026) and the transparency code of practice were reported but not read (UNVERIFIED).
9. **Cost model.**
   - The token counts are assumptions, and cache-hit rates on Batch are "best-effort", between 30 and 98%.
   - Acceptance rates are unknown until math2 runs. The true cost per accepted node could be 2 to 5 times the table.
   - Formal work is priced only by analogy, at $17 per declaration in Automatic Textbook Formalization.
10. **Judge evidence comes from older models.** Most measurements are of 2025 models (GPT-5, Gemini 2.5, Claude Sonnet 4 and 4.5, Opus 4.5 and 4.6). Opus 5.5 and Fable 5.1 have no published proof-judging measurements that we found. math2 must measure its own judges on a labelled set before item 2 of 7.1 can be settled.
11. **Self-verification evidence conflicts.** Hard2Verify finds models more lenient on their own work, and OPC finds them harsher. Whether the generator may also act as a judge must be measured, not assumed. The safe default is a different family, as 10.4 check 9 already says.
12. **Snippet-only and derived figures** throughout sections 1 and 2 must be re-checked against the primary PDFs before any is quoted in the spec. This applies above all to the First Proof batch 2 tally, BrokenArXiv's later figures, Seed-Prover 1.5's 580/660, and the "500 TPU-days per problem" attributed to AlphaProof.
13. **No Anthropic or OpenAI recitation filter is documented.** We cannot rely on the provider to block regurgitation. Gemini has a RECITATION finish reason, but its threshold is not published.
