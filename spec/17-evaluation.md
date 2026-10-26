# 17. Evaluation

math2 makes claims at three levels. It makes claims about mathematics (the status of a node). It makes claims about its own machinery (the battery catches misformalizations; search finds the right node). It also makes claims about its agents (this prompt version proves more for less). Each claim needs a measurement, or it is a slogan. This document says what math2 measures, how it measures it, and what each measurement gates.

The code and data are in `tamnd/math2-agent/bench/`. The results go to `tamnd/math2/reports/eval/<release>/`, and the notes of each release give a summary of them.

## 17.1 Principles

1. **The kernel is the only grader that needs no evaluation.** math2 grades all other graders, which includes LLM judges, back-translation, the critic, search and extraction. Every grader has a labelled set, a measured error rate, and a version.
2. **Gate on regressions, report the levels.** Each suite has a gate: on the suite, a new version must be no worse than the old one, within noise. math2 publishes the absolute numbers, but they do not gate. A gate on absolute targets invites quiet changes to the target.
3. **Measure cost with quality.** Report every result with its cost: tokens, dollars, wall-clock time and Lean CPU time. In 2026, the same model could score 7/10 for $36 or 9/10 for $1,888. The difference came from the harness around the model (document 01). You cannot compare a score that has no cost.
4. **Report failures and the selection.** The report of a suite says how many items passed and which items failed. It also says how the items were chosen. This follows the AGMAI guideline that labs document failed attempts and how they selected problems (document 01). math2 applies this guideline to itself.
5. **Contamination is the default assumption.** Any public problem set may be in the training data of a model. Each suite has public, held-out and fresh items (17.6). The headline numbers come from the held-out and fresh items.

## 17.2 What is measured

| Area | Question | Suite | Gates |
|---|---|---|---|
| **status** | Is a node's computed status correct? | monthly sample of E2 to E4 nodes re-checked by a curator against their evidence (17.8); F-level re-review (10.8) | release notes; a measured error above the published bound blocks the release |
| **fidelity battery** | Does F1 catch misformalizations? | `bench/fidelity/` (17.3.1) | any battery change |
| **extraction** | Does ingestion extract and unfold results correctly? | `bench/extract/` (17.3.2) | ingest adapter changes |
| **alignment** | Are proposed `same_as` and `aligned` edges right? | `bench/align/` (17.3.3) | align role changes |
| **search** | Does search find the right node? | `bench/search/` (14.7) | search and embedding changes |
| **agents** | Does each role do its job better per dollar? | `bench/agents/<role>/` (17.3.4) | role and prompt versions (12.8) |
| **injection** | Do agents ignore instructions in sources? | `bench/agents/canaries/` (12.7) | every role version |
| **explanations** | Are explanations correct, grounded and at the right level? | `bench/explain/` (17.3.5) | explain role versions |
| **graph** | Do the queries meet their latency targets? | `bench/graph/` (05.6) | storage-layer changes |

## 17.3 Suites

### 17.3.1 The fidelity battery

The battery (10.4) is a classifier. Its verdict is faithful, or misformalized with a class. Its evaluation set contains labelled pairs, and each pair is an informal statement and a formal statement:

- **Seed.** Known misformalizations from the published audits. These include the defective items of ProofNet, the certified issues of the ICML 2026 benchmark audit, the fixed statements of PutnamBench, and the corrected statements of the formal-conjectures repository. Each item has its taxonomy class (10.5).
- **Synthetic.** Faithful statements from F3 nodes, mutated by one taxonomy class at a time: drop a hypothesis, swap a quantifier, change ℕ to ℤ, add a junk value. A person checks each mutation before it enters the set, because some mutations are also faithful by chance.
- **Live.** Every `not-faithful` review (10.6) enters the set, with its class.

Measured: recall per taxonomy class, and the false-alarm rate on faithful items. The report is a confusion matrix over classes. A battery change must not decrease recall on any class by more than the noise band of that class. The band is published with the matrix.

**The suite in m2.** `bench/fidelity` has the synthetic part, in the Lean core library, and it reads the live part from the reviews of a math2 tree. The seed part needs the published audits, and it comes later. The noise band is zero items, because the tactics of the battery have limits in heartbeats and give the same result on each run. Thus the test fails when one class catches one item less. The report is attached to each release of tamnd/math2-agent.

### 17.3.2 Extraction

The suite has five hundred papers across MSC codes, decades and source types (LaTeX, HTML only, OCR). Each paper is labelled by hand with its results, their kinds, their in-paper dependencies, and a reference unfolding of each result. Measured: result recall and precision; kind accuracy; dependency edge F1; unfolding fidelity. People grade unfolding fidelity on a sample, and the back-translation judge of the battery grades it on all items. The report gives errors by source type, so a regression in the OCR path cannot hide behind the LaTeX path.

### 17.3.3 Alignment

The suite has confirmed and rejected alignments from curation (09, 05.3). Measured: precision at the auto-propose threshold, and recall of known `same_as` pairs. Precision is more important here, because a wrong `same_as` edge contaminates everything downstream. The threshold is set so that precision on the held-out set is at least 0.98.

### 17.3.4 Agent roles

Each role in 12.1 has a suite of tasks with an objective outcome:

| Role | Tasks | Outcome measured |
|---|---|---|
| **prove** | held-out statements with known Lean proofs, by tier: undergraduate (Mathlib-adjacent), graduate, research (from bound libraries and formal-conjectures' solved items) | solved within budget; dollars and Lean CPU per solve; proof size against the reference |
| **formalize** | held-out nodes with F3 bindings | winning candidate equivalent to the F3 binding; battery verdict; dissent rate |
| **explore** | planted problems: known conjectures withheld from the context, known counterexamples to plausible statements | refutations found; false conjectures proposed and not caught by the falsify step; novelty check run before every claim |
| **explain** | 17.3.5 | see 17.3.5 |
| **curate, align** | 17.3.3 | see 17.3.3 |
| **bump** | replays of past Mathlib bumps from git history | share of breakages repaired automatically; repairs that change a statement's type hash (these must be zero without a flag) |
| **review-assist** | packets for bindings with known verdicts | verdict-relevant concerns surfaced (the critic's recall); length of packet |

Every role suite has three splits. **Public** items can be anywhere. **Held-out** items are never published. They are in a private repository that the tools of the agents cannot reach. **Fresh** items were made after the training cutoff of the model. Fresh items come from new Mathlib declarations, new formal-conjectures statements, new arXiv results with formal statements, and items that the maintainers write. Each model version is compared on all three splits. If public scores are much higher than fresh scores, the report gives the difference as a contamination signal.

### 17.3.5 Explanations

Explanations (12.6) get grades on three axes. **Correctness**: every mathematical claim in the explanation is grounded in a cited node, or it is marked. **Level**: an L1 explanation uses no concept outside the prerequisite set of its level. A mechanical check against the graph confirms this. **Usefulness**: people grade it. People grade a monthly sample, and a judge grades everything. math2 also publishes the agreement between the judge and the people on that sample. LLM judges are least reliable on explanations. Thus an improvement that only a judge sees does not pass the gate without a human-graded sample.

## 17.4 Run data

Every agent run is a record (11.6). Together, the records are math2's own dataset:

- **Outcome per target**: solved or not, with cost, model, prompt version and the retrieval used. The orchestrator uses this data to learn which ladder rung to start at for each kind of target (11.4).
- **Premise use**: which retrieved premises appeared in the final proof. This data trains the premise reranker (14.5).
- **Failure classes**: timeout, elaboration loop, wrong decomposition, statement mismatch, injection attempt detected. The classification is automatic, and people check a sample.
- **Cost curves**: solve rate against spend, by role, tier and model. The weekly report (12.9) plots this curve, and the budget split (11.5) is tuned on it.

The transcripts of merged runs are public (11.6), so their outcome data is public too. Each quarter, math2 also publishes a dataset of all runs, merged or not. It contains targets, outcomes and costs, but no transcripts for unmerged runs. This applies the AGMAI and Tao position on the report of negative results and resource use (document 01) to the project's own agents.

## 17.5 The novelty check

Some outputs can be described as new: an explore conjecture, a proof of an open node, a counterexample. Before that, the output goes through a novelty check, and the result of the check is attached to the node:

1. **Graph.** Search the statement plane for the statement, its negation, and its specialisations and generalisations (14).
2. **Record plane.** Search aligned and unaligned results from the literature. Use the formula, embedding and lexical channels.
3. **External indexes.** Query zbMATH Open, TheoremSearch, TheoremGraph and Matlas where their APIs allow (09.6). Also query arXiv full text of licence-permitted papers through `tamnd/arxiv`. Query MathOverflow and Math.SE through the Stack Exchange API.
4. **Problem databases.** For a problem node, check these sources: the status and comments in the erdosproblems database, the `solved` markers in the formal-conjectures repository, the comments of the OEIS entry, and the LMFDB.
5. **Verdict.** `known` (with the source), `likely-known` (close matches that a person should read), or `no-match-found`. **No verdict says "new".** `no-match-found` means that these searches found nothing. Only a person makes a statement of novelty, after they read the matches.

math2 evaluates the check like everything else. Its suite contains results that are known to be in the literature. Each result is phrased as it would come out of an agent. The suite includes the 2025 to 2026 rediscoveries (document 01). Measured: recall of `known`. Sometimes the novelty check misses a rediscovery and a person finds it later. That rediscovery then goes into the suite.

## 17.6 External benchmarks

math2 does not optimise for public benchmarks. But it runs them, for two reasons. They put the agents of math2 on a common scale with the agents of everyone else. And the fresh benchmarks are useful held-out sets. Each release reports the results with their costs. These results never replace the suites above.

| Benchmark | What it measures | 2026 state (document 01) | Use in math2 |
|---|---|---|---|
| **miniF2F** | competition maths in Lean | widely regarded as saturated | smoke test only |
| **PutnamBench** | Putnam problems in Lean, 672 items | saturated: all 672 solved, and the leaderboard ranks by cost | cost benchmark for the prove role: dollars to solve all of it |
| **formal-conjectures** (frozen evaluation subsets) | open and solved problems as Lean statements | 2,615 statements; AlphaProof Nexus solved 9 Erdős and 44 OEIS items | research-tier prove suite (solved items); explore targets inside campaigns (open items) |
| **FrontierMath Erdős** | 68 open Erdős problems in Lean, comparator-graded, $300 cap per problem | top score 3% | the hardest prove tier; reported with spend |
| **FrontierMath Tier 4** | expert final-answer problems | declared saturated in September 2026 | not used: final answers are not math2's output |
| **First Proof** | unpublished research lemmas, graded double-blind by people, with negative scoring for wrong answers | Batch 3: 9/10 at $1,888 for the best system | the model for 17.7; math2 enters when a batch fits its roles |
| **IMO-ProofBench, MathArena proof tracks** | informal proofs graded by judges plus people | judges agree with people at about 0.9 | calibration data for math2's own judges |
| **RealMath, LemmaBench** | results mined from recent arXiv papers, refreshed monthly | contamination-resistant by construction | fresh items for the formalize and prove suites, through 09 |
| **Annals Challenge** | 50 Lean statements of recent Annals theorems | missing definitions are the blocker | a definitions-first suite for formalize (10.3) |

Contamination rules for every external benchmark:

- During an evaluation, no context that an agent sees contains the items or their solutions. Search excludes them (`bench_exclude` in the run configuration), and the canary rule checks for leaks.
- The report gives results separately for items published before and after the training cutoff of each model.
- When a run solves an item, it reports whether search returned the solution of the item or a close neighbour. Thus a "solve" by retrieval is labelled as one.

## 17.7 Graders: judges and people

Many checks in math2 end in an LLM judge: back-translation (10.4), the critic, alignment, explanation grading, and the ranking in the novelty check. The 2026 evidence about judges is mixed (document 01):

- On competition proofs, the best judges agree with human graders at about 90%. This is about as often as human graders agree with each other.
- Agreement fell when the proofs came from AI pipelines that were optimised against judges. One study found a 31.7% false-positive rate.
- Judges show a "reviewer-pleasing" bias towards confident, well-formatted text.
- A minimum over several judges reduced overestimation.

Thus math2 applies four rules to every judge:

1. **Judges triage. They never certify.** No judge verdict raises the E-level or F-level of a node. Judges decide what a person examines first, and which items go to the kernel for a check.
2. **Min over a panel.** Sometimes a judge's verdict gates something (for example, F1 needs a back-translation pass). Such a gate uses at least two judges from different model families and takes the more severe verdict.
3. **Judges are evaluated on adversarial items.** The suite of every judge includes outputs from math2's own agents that people marked wrong. If a judge cannot tell a wrong proof by an agent from a right one, math2 does not use that judge to grade agents.
4. **Human grading follows First Proof.** When people grade agent output in an evaluation, they do not know the source: agent or human, and which model. They use a rubric that gives separate scores for correctness, exposition and attribution. A wrong answer scores lower than an abstention, so a confident wrong answer costs more than no answer.

## 17.8 Reporting

The notes of each release include an evaluation section that is generated from `reports/eval/<release>/`:

- the status error bounds: the share of sampled E2 to E4 nodes whose evidence did not support their level (0.5% of each level per month, re-checked by a curator who did not add the evidence), and F-level re-review disagreement (10.8);
- the battery confusion matrix and its change since the last release;
- search headline metrics against the external baselines (14.7);
- per-role solve rates, dollars per solve, and contamination gaps between public and fresh items;
- external benchmark results with costs;
- known failures: the items each suite failed, linked.

If a number got worse, the notes say so first.
