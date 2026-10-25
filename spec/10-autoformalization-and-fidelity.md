# 10. Autoformalization and statement fidelity

The kernel answers one question perfectly: is this Lean term a proof of this Lean type? It cannot answer the question that matters: does this Lean type say what the mathematician meant? That second question is **fidelity**. In 2026 it is the weakest link in all formal mathematics, with AI or without it. This document tells how to change fidelity from a hope into a property that we measure, review and maintain.

## 10.1 The evidence that this is the bottleneck

Document 01 has the sources. In brief:

- An audit in 2025 found that about **a third of the Lean 4 port of ProofNet** was misformalized.
- An ICML 2026 audit of five formal benchmarks found **4,833 issues**. Of these, **398 were mechanically certified**: counterexamples to stated theorems, vacuous statements, and unsound axioms.
- The statements of PutnamBench needed **27 fixes** from one prover team and 2 more from another before people could call the benchmark saturated.
- In 2025, an AI "solution" to an Erdős problem got much attention. It proved a weaker variant, because the problem as written did not have a hypothesis.
- The 2026 Navier–Stokes claim came with a Lean proof. The dispute about it was about the gap between the formal statement and the physical problem.
- The 2026 Annals Challenge put Lean statements of 50 recent Annals theorems into Mathlib. It found that the main blocker was **missing definitions**, not missing proofs.
- In the largest 2026 formalizations, people fixed or cross-checked the top-level statement by hand, against a curated library. In the Fermat run, agents cross-checked the intermediate statements of each other, and that found false ones early.

The lesson that math2 takes: **autoformalization of statements is now cheap and usually right. "Usually" is not good enough for a library that other work will build on.** Thus the pipeline has two halves. A cheap machine half, F1, finds most errors. A human half, F2 and F3, is made fast enough that people can review the important statements.

## 10.2 The formalize pipeline

The formalize role (12.1) runs this pipeline for one node. `m2 autoform <tag>` runs it from the command line.

```
1. context      node statement, its uses closure (definitions!), sources' locators,
                bound neighbours and their Lean statements, Mathlib conventions
2. definitions  every concept the statement uses must already be bound, or is formalized first (10.3)
3. candidates   k candidate Lean statements from ≥ 2 model families, each elaborated in a Lean session
4. battery      the F1 battery (10.4) on every candidate that elaborates
5. select       candidates that pass; clustered by type hash and by proved equivalence;
                the largest agreeing cluster wins; disagreement is itself recorded
6. propose      PR: binding at F0 with the winning statement, the battery report,
                the dissenting candidates, and a review packet (10.6)
```

Two design points:

- **Agreement among independent candidates is evidence.** Five candidates from two model families can elaborate to the same type hash, or to types proved equivalent in a Lean session. That is stronger than the back-translation of one candidate. If the candidates split into clusters, the split usually points directly at the ambiguity: which convention for $\mathbb{N}$, whether "ring" includes a unit, whether a sum is over a finite set. The PR reports it.
- **Search before writing.** Step 1 first searches for an existing declaration with type search (14.3) and embedding search. If Mathlib already states it, the binding goes to Mathlib, and the formalize run becomes an align run.

## 10.3 Definitions before statements

A statement can only be as faithful as the definitions that it uses. A faithful statement about the wrong notion of "regular" is wrong. Thus formalize works from the bottom up over the `uses` closure:

1. Each concept or definition node in the closure must have a binding at F2 or higher before a statement that uses it can go higher than F1. The fidelity queue enforces this review order (11.4 `unlock`).
2. If Mathlib has no definition, the formalize role proposes one in `Math2/`, together with **sanity lemmas**. These show that the definition agrees with the examples of the textbook and with known equivalent characterisations. The proof of the sanity lemmas is a prove job. A definition with proved sanity lemmas is much easier to review.
3. If Mathlib has several candidate definitions (bundled and unbundled, `Set` and `Finset`, different universe levels), the binding names the selected one, and the remarks of the node say why.

## 10.4 The F1 battery

F1 means that a candidate has passed each check that needs no human. The battery report records each check. Each check has an evaluation set with a measured detection rate (17.3).

| # | Check | How | Catches |
|---|---|---|---|
| 1 | **elaborates** | the Lean session at the pinned toolchain | syntax and type errors |
| 2 | **back-translation** | a model that has not seen the informal statement translates the Lean statement into English; a judge compares the two, sentence by sentence, with a structured verdict per hypothesis and conclusion | dropped or added hypotheses, wrong quantifier order, wrong conclusion |
| 3 | **constant audit** | every constant in the elaborated statement is mapped to a node through bindings, and that node must be in the informal statement's `uses` closure | the right words with the wrong definitions (e.g. `Nat.sub` truncation, `Real.sqrt` on negatives, `0⁻¹ = 0`) |
| 4 | **hypotheses satisfiable** | a Lean search, then an SMT search, for an instance satisfying all the hypotheses, starting from example nodes in the graph | vacuous statements: contradictory hypotheses make anything provable |
| 5 | **conclusion not trivial** | automation (`simp`, `decide`, `grind`, `aesop`) attempts the statement with a short timeout; success is flagged for review, not failed | statements that are trivially true by a junk-value convention |
| 6 | **negation not provable cheaply** | automation attempts the negation | statements that are false |
| 7 | **examples and counterexamples** | the statement is instantiated on the graph's `example` and `counterexample` nodes for its concepts and checked with `decide` or `plausible`-style testing | statements that fail on the textbook's own examples, or "prove" the counterexample |
| 8 | **neighbour consistency** | if the node `specializes` or `generalizes` a bound node, a Lean session tries to derive one formal statement from the other | inconsistent formalizations across the graph |
| 9 | **critic** | an independent model, from a different family from the generator where possible, reads the informal statement, the Lean statement and checks 2–8, and lists concerns with severity | everything else; its output goes to the reviewer, it is not a verdict |
| 10 | **pinned-statement compile** | the statement alone compiles as a comparator challenge module against Mathlib and `Math2` | statements that depend on the proof file's local context |

A candidate is F1 if checks 1, 3, 4, 6, 7, 8 and 10 pass, and checks 2 and 9 raise no high-severity concern. Check 5 never blocks F1. Sometimes a trivial truth is the correct statement. But the review packet always shows a trivial truth.

Checks 4, 5 and 6 are also rule `F09`. This rule runs on each F1+ binding at each release, because a Mathlib bump can make a statement trivial or vacuous and not change its text.

## 10.5 A taxonomy of misformalization

Each class has a name, a detector and an example. Reviews and audit samples get labels from these classes. Thus we can measure the error rate for each class (17.3), and we can improve the battery where it is weak.

| Class | Description | Primary detector | Example |
|---|---|---|---|
| `M-hyp-missing` | a hypothesis of the informal statement is absent | back-translation; examples | "every continuous function attains its maximum", stated without compactness |
| `M-hyp-extra` | a hypothesis is added | back-translation; critic | the statement is proved for finite groups only when the source says groups |
| `M-quant` | quantifier order or scope wrong | back-translation | ∀ε ∃δ against ∃δ ∀ε |
| `M-vacuous` | hypotheses unsatisfiable | check 4 | `n > 0 → n < 0 → P` |
| `M-junk` | a junk value makes it trivially true or false | constant audit; check 5 | division by zero, natural subtraction, `sqrt` of a negative, `sSup` of an unbounded set |
| `M-def` | a constant means something different from the informal notion | constant audit; definitions-first | Mathlib's `Nat.Prime` against a source that counts 1 as prime |
| `M-type` | wrong carrier: ℕ for ℤ, `Fin n` off by one, real for complex | back-translation; examples | a sum over `Finset.range n` meant to be 1..n |
| `M-conv` | a convention mismatch on an otherwise correct statement | critic; review | ℕ including 0; `⊆` against `⊂`; graph with or without loops |
| `M-weak` | the formal statement is a special case | neighbour consistency; review | the Erdős #124 pattern |
| `M-strong` | the formal statement is stronger and false | negation check; examples | dropping "for sufficiently large n" |
| `M-context` | the statement depends on local context, notation or instances in the proof file | check 10 | a local `instance` that changes `≤` |
| `M-tamper` | the proof environment redefines or shadows something in the statement | pinned statement plus comparator (12.2) | redefining a norm so that every element has norm zero |

## 10.6 Human review: F2 and F3

### What a reviewer signs

A fidelity review says: *I have read the informal statement with hash h₁, and the Lean statement with elaborated type hash h₂, together with the definitions of every constant it uses, and the Lean statement says what the informal statement says, up to the conventions recorded on the node.* The review records:

- the reviewer, the date, h₁ and h₂;
- a verdict: `faithful`, `faithful-with-conventions` (listed), or `not-faithful` (with a taxonomy class);
- free-text notes.

A `not-faithful` review has the same value as a positive one. It sends the binding back to formalize with the class and the notes.

### The review packet

Review-assist builds a packet for each binding in the fidelity queue. `m2 review <tag>` renders it:

- the informal statement and the Lean statement side by side, the Lean one pretty-printed both with and without notation;
- the back-translation, and the judge's per-clause verdicts;
- each constant in the statement, expanded to its definition and its bound node, with that node's fidelity level;
- the battery report, the critic's concerns, the dissenting candidate clusters and how they differ;
- for a re-review after a bump, the old and new types as a diff.

Review-assist never signs. An agent cannot be a reviewer. A reviewer cannot review a binding that came from an agent run that the reviewer started (rule `V03`).

### Independence

F3 needs one of two things:

- **two independent reviews**: different reviewers, neither of them the binding's author, sharing no affiliation (rule `V02`);
- **one review plus an independent formalization proved equivalent**. The second formalization comes from a different run, of a different model family, given no access to the first candidate. The equivalence is proved in Lean.

The second route is cheaper, and it becomes cheaper each year. It is also the route that scales. The target for each node in the undergraduate core is F3 by cross-formalization (document 02).

### Throughput targets

For undergraduate-level statements, a review takes 3 minutes. Thus one reviewer-hour gives 20 F2 reviews. The undergraduate core (about 25,000 statements and definitions, document 18) then needs about 1,250 reviewer-hours for F2. That is a summer of work for twenty people, or a year of light work for a hundred. Research-level statements take much longer, at about 15 minutes or more. For this reason the fidelity queue is ranked by `unlock`. Also for this reason, the Annals-Challenge-style approach is the correct order: it formalizes statements before anyone tries to prove them. A reviewed statement has more value than an unreviewed proof.

## 10.7 Statements without proofs

Many of the most useful formal bindings in math2 will have no proof for a long time. These are research-level theorems, open conjectures, and results whose proofs depend on large theories that Mathlib does not have. Such a binding is `match: partial` until a proof exists. It is still worth having:

- it can be reviewed to F2 and F3, which makes it a trustworthy target for anyone, human or agent, who wants to prove it;
- it gives search a formal type to match against (14.3);
- for conjectures, it is the precondition for any AI claim of a solution to be checkable. A solution counts only against the pinned, reviewed statement. That is the lesson of every 2025 to 2026 episode in which an AI proved the wrong version.

The formal-conjectures repository and the Annals Challenge are the two largest collections of formal statements without proofs. math2 aligns to both (document 09). If their maintainers have reviewed an aligned statement, math2 records it as a review and names the source.

## 10.8 Measuring fidelity

Fidelity levels are claims too, and math2 measures how reliable they are:

- **Sampled re-review.** Each month, a different reviewer does a blind re-review of 1% of F2 bindings and 0.5% of F3 bindings. We adjudicate and publish the disagreements. The measured error rate per level, by area, is a headline number in the notes of each release.
- **Battery evaluation.** We evaluate each check in 10.4 against a labelled set of known misformalizations. The set starts from the published benchmark audits and grows with each `not-faithful` review. We publish detection rates per class (17.3).
- **Downstream evidence.** Sometimes the proof of a statement needs a hypothesis that the informal statement does not have. Sometimes the proof is suspiciously short. In both cases, the statement goes back to the fidelity queue with a flag. A suspiciously short proof means that the measured size is much smaller than that of comparable proofs.

If F2 means "probably right", math2 should be able to say how probable.
