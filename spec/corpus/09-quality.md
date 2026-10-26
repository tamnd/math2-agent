# K09. Quality: how the corpus stays correct

The corpus is written at a scale that no group of people can read in full. Most first drafts come from importers and models. This document says which checks each piece of text goes through before it merges and after it merges, how the corpus measures its own error rate, and what happens when a check finds a problem.

The main spec already has the hard part: status is computed (spec 04.7), only the kernel and the certificate checkers referee E4 and E5 (settled decision 3), and fidelity above F1 needs people (spec 10.6). This document does not change these rules. It adds the checks that a corpus of millions of nodes needs on top of them.

## K9.1 Four properties

A node is good when it has four properties. Each property has its own checks, and no check covers two properties:

| Property | Question | Checked by |
|---|---|---|
| **true** | Is the statement true, and does the proof prove it? | the E levels: kernel, certificates, refereed and textbook sources (spec 04.7). Before that: the checks of K9.3, which only triage |
| **faithful** | Does the node say what its sources say, and does its formal binding say what the node says? | source confirmation (K9.5), the F levels (spec 10) |
| **readable** | Does the node follow its card and the house style? | the card findings and the style linter (K08), the writing level (K2.4) |
| **ours to publish** | Is each byte of text allowed under the prose licence? | the licence gate (spec 9.3), the copy check (K9.4) |

The reports give a separate number for each property. A good score in one property never hides a bad score in another.

## K9.2 The gates before merge

Each pull request that adds or changes a body goes through these gates in this order. A gate that fails stops the pull request. The failure has a reason, and the reason goes back to the job (K12) as output.

| Gate | Checks | Applies to | Hard or soft |
|---|---|---|---|
| G1. Schema and body | `m2 check`: rules S01 to S04 and B01 to B03 | each change | hard |
| G2. Card | the required sections of the target writing level (K8.4) | each change | hard for the level that the job asked for, soft otherwise |
| G3. Style | the linter of K8.2 | each body | soft below W3, hard for a W3 job |
| G4. Copy check | K9.4 | each body that an agent wrote, and each body from an importer of share-alike text | hard |
| G5. Sources | K9.5 | each source that the change adds | hard for a source that an agent proposed |
| G6. Sanity | K9.3 | each statement and proof that an agent wrote | soft: a failure opens a counterexample job and blocks only auto-merge |
| G7. Formal | rules F01 to F09 | each change to a binding | hard (spec 15.3) |
| G8. Review capacity | the cap of spec 11.5 | each agent pull request | hard: the pull request waits, it is not closed |

The gates run in the agent loop before the pull request opens (spec 12.2), and again in CI. An agent sees the result of each gate as a tool result, so it can repair its draft in the same run.

## K9.3 Sanity checks on statements and proofs

These checks catch false statements and broken proofs before a person spends time on them. **They never raise an E level and they never count as a review.** A pass means "no problem was found", and nothing more.

1. **Small cases.** When a statement quantifies over integers, finite groups, graphs, matrices of small size, or any family that the computational engine (spec 08) can enumerate, the explore role writes a test and runs it for small cases. A failure is a counterexample candidate. It opens a computation node and a job for a person, and it blocks the merge of the statement. Most false statements that models write fail at small cases.
2. **Formal round trip.** When the formalize role can bind the statement, the F1 battery of spec 10.4 runs. A back-translation that differs from the informal text shows an error in one of the two. The finding names the difference.
3. **Independent check by other models.** Models of other families read the statement or the proof one step at a time (K8.5) and report each step that they cannot follow. They get the node, the linked nodes and the sources, and not the run that wrote the node. The evidence on model judges sets the rules (K01, K7.6):
   - A judge verdict is triage, never evidence. Judges gave proofs scores up to 20 times higher than human graders did, and the best step judges still accept about one wrong step in five.
   - A judge model must have a measured rate of rejecting wrong steps of 75% or more on a labelled set of math2 before it can judge proofs or statements. Tier 0 models do not judge proofs.
   - The check passes only when each judge passes the node. When the judges disagree, the node goes to a person, and no extra model call breaks the tie.
   - Each verdict and its model id go in the run record. Agreement is used only for triage (settled decision 3).
4. **Hypothesis check.** For each hypothesis of a statement, the check asks whether a known counterexample shows that the hypothesis is needed. When a hypothesis is not needed, the statement is still true, but the node gets a remark: either the statement is weaker than it can be, or a source states it that way for a reason that the node must give.
5. **Prove or disprove.** A job that proves or formalizes a statement asks the model to prove it or to disprove it, not to prove it. Models "prove" false statements often, and this framing made one model reject false statements almost four times as often (K01). Each month, the queue gets canaries: false statements made by small changes to true ones. The weekly report gives the share of canaries that each configuration "proved", and a share above 5% stops the configuration.
6. **Names of theorems.** A theorem that the prose names ("by the theorem of X and Y") links to a node or has a source. Models invent theorems that match what they want to prove, so a name that resolves to nothing is a finding (rule X07).
7. **Proof dependencies.** Each `uses` edge of a proof points to a node at E2 or better, or the proof gets the `sorry_in_closure` flag (spec 04.7). An agent proof that uses an E1 node can merge, but it shows the flag.

## K9.4 The copy check

Models reproduce text that they saw in training, even when nobody asks them to. In one study, more than 27% of the text that models wrote in the style of an encyclopedia was in verbatim runs of 50 characters or more, and one model reproduced 1,170 characters of a Wikipedia article (K01). For mathematics the risk is higher, because standard proofs appear in many sources with almost the same words. The corpus must not publish text that the licence gate refuses (K2.6, rule 3). So each body that an agent wrote goes through a copy check before it merges.

- **Compare with a reference corpus, not only with the sources of the node.** A model that writes from memory cites nothing, so a check against the cited sources finds nothing. The reference corpus holds the mathematical pages of Wikipedia, ProofWiki, the nLab and PlanetMath, the dumps of Math.StackExchange and MathOverflow, the Stacks Project (GFDL), Kerodon (no licence, so `record`), the open textbooks of K05, and, for detection only, OpenWebMath and the arXiv text that the record plane can lawfully read (spec 9.3). Each text in it has its licence class. For sources that math2 cannot read, the check cannot run, and K9.6 covers the gap.
- **Normalization.** Lowercase, whitespace collapsed, LaTeX macros made canonical. Formulas are masked: each formula becomes a placeholder, so that a standard formula does not count as a copy. A separate pass compares the structure of formulas, so that a renamed variable does not hide a copied proof.
- **Stage 1, exact runs.** An index of suffixes over the reference corpus finds each run of the body that is also in a reference text. A run of 50 characters or more is a flag. A run of 100 characters or more in one source, or flagged runs that cover 10% or more of a section, is a **match**.
- **Stage 2, near copies.** A filter for exact runs alone gives false safety: in the studies, small edits pass it, and a near copy counts almost as much as an exact one (K01). So each paragraph also goes through MinHash on word 5-grams. A candidate pair with an estimated Jaccard similarity of 0.5 or more is a match if it also has a common block of 20 words or more after alignment, or a ROUGE-L of 0.8 or more.
- **Stage 3, paraphrases.** A sample of paragraphs, with their nearest reference paragraphs by embedding, goes to people. This stage does not block a merge. It measures what the first two stages miss, for the sample audit (K9.7).
- **What a match does.** A match with a source of class `record`, `verbatim`, `share-alike-nc` or `gfdl` blocks the merge, and the agent gets the locations to rewrite. A match with an `open` source is allowed when the licence of the source is compatible, and the node then gets the attribution of rule L03 in its provenance. Each match records the source, its URI, its licence and the offsets, in the run record.
- **Standard phrases.** Some text is the same in each source because there is only one way to say it, for example "let $G$ be a group". A list of standard phrases, kept by curators, does not count towards a match. A phrase gets onto the list only by a curator decision.
- **Calibration before blocking.** The thresholds above are a start, and nobody has tested them on mathematics. Before L05 blocks merges, the copy check runs over pages that people wrote, compared with each other: Wikipedia against ProofWiki, and open textbooks against each other. The median overlap of human text is 0% in the studies. The thresholds are set so that the rate of false matches on human text is under 1%, and K9.7 tunes them on samples after that.
- **A first layer in the prompt.** Each prompt for prose tells the model to write in its own words and not to reproduce a source. In a study, this cut the overlap by half or more, but long copies still came through. It is a cheap first layer and not a control.

Rule L04 of spec 15.3 is soft for text from people. The corpus adds a hard rule for text from agents:

| Rule | H/S | Checks |
|---|---|---|
| L05 | H | each agent-written body has a copy check result in its run record, and the result has no blocking match. Until the calibration is done, L05 is soft |

## K9.5 Source confirmation

Models invent references that look correct. A wrong reference is worse than none, because readers trust it. So an agent cannot add a source by itself:

1. **The record must exist.** A source is a record in the record plane: a DOI that Crossref resolves, a zbMATH id, an arXiv id, an ISBN with a catalogue record, or a record of an open textbook (K05). An agent proposes a source as an identifier. If the identifier does not resolve, the proposal is dropped, and the run record counts it as a fabricated source.
2. **The record must match.** The title, the authors and the year of the record must match what the agent claimed. A mismatch is a fabricated source too.
3. **The locator must be checked.** A locator says where in the source the item is: a theorem number, a section, a page. When the record plane can read the text, a check finds the locator and compares the item with the node. When it cannot read the text, the source has the flag `locator_unconfirmed`, and only a person can remove the flag.
4. **E2 and E3 need confirmed locators.** The build counts a source towards E2 or E3 (spec 04.7) only when its locator is confirmed. A node can show an unconfirmed source as further reading.

The rate of fabricated sources for each model and prompt version goes into the weekly agent report (spec 12.9). A rate above 2% stops source proposals from that configuration until a person looks at it.

| Rule | H/S | Checks |
|---|---|---|
| P05 | H | each source that an agent added resolves to a record in the record plane, and the record matches the claim |
| P06 | S | a source with `locator_unconfirmed` older than 180 days |

## K9.6 What people check

People are the scarce resource (settled decision 20). The corpus uses them only where a machine cannot check:

| Check | Done by | Why a machine cannot do it |
|---|---|---|
| fidelity of a formal statement (F2, F3) | reviewers, with `m2 review` (spec 16.3) | settled decision 4 |
| confirmation of an agent-written section (rule P04) | curators and area editors | the section claims to explain; only a reader can say whether it does |
| an unconfirmed locator | anyone with access to the source | the record plane cannot read most of the literature |
| a sanity finding (K9.3) | area editors | a finding can be a false alarm, and only a person can decide |
| a copy match that the agent cannot fix | curators | the decision whether a phrase is standard |

The review-assist role (spec 12.1) makes a packet for each check, with the evidence in one place. Each check has a target time, and spec 17 measures it. A person can confirm many sections in one session, but each confirmation is recorded for each section, not for the session.

## K9.7 Sample audits and published error rates

Gates and reviews catch errors one at a time. They cannot say how many errors remain. The sample audit can.

- **The strata.** Each month, the audit draws a random sample of merged nodes in each stratum. A stratum is a combination of kind, writer (person, importer, agent role), model and prompt version, and layer (K2.7). A new configuration gets its own stratum from its first merge.
- **The sample size.** With no errors in *n* samples, the upper end of the 95% confidence interval for the error rate is about 3/*n*. So 300 samples with no errors show a rate below 1%. The audit uses 100 samples in each stratum each month as a minimum, and more where the rate is near a threshold. The report uses the Wilson interval when the sample has errors.
- **The audit is blind.** Auditors do not see who or what wrote the node. They check the four properties of K9.1 against a written checklist for each kind.
- **Error classes for prose.** No study has measured how often models write a wrong definition or drop a hypothesis in informal text (K01). The corpus must measure it. The checklist classifies each error in a body: wrong definition, dropped hypothesis, statement stronger than its source, wrong name or attribution, convention not stated, step that does not follow, wrong source. The classes sit next to the misformalization classes of spec 10.5, so that the release notes give error rates for prose and not only for bindings.
- **The thresholds.** For each stratum, the error rate of each property must stay under a threshold: 1% for a false statement, 2% for an unfaithful statement or a wrong source, 5% for a style or card error. These numbers are a start, and a change to them is a decision that the release notes record.
- **Above the threshold.** When the upper end of the interval for a stratum passes its threshold, the configuration stops. Its unconfirmed output goes to quarantine by a generated pull request, as in the withdrawal test of spec 06.5. A person decides whether to fix, rerun or retire the output.
- **The publication.** Each release publishes the error rates of each stratum, with the sample sizes and the intervals, including the strata that failed. This is rule 6 of K2.6.
- **Small strata.** A stratum with fewer than 1,000 merges in a month pools its samples over three months. A stratum with fewer than 100 merges in three months is audited in full.

The sample audit costs reviewer time. At 100 samples in each of 20 strata, and 5 minutes for each sample, it costs about 170 hours each month. K12.5 counts this cost in the review budget first, before the agents get their share. When review time is short, the corpus runs fewer configurations. It does not take smaller samples, because a rate that nobody measured is not a rate.

## K9.8 Reports from readers

Each page of the workspace has a "report a problem" link. A report becomes an issue with the tag of the node and the property that the reader thinks is wrong. A report on a node at E5 that says the statement is false goes first in the queue, because it is a report on the fidelity of the binding or on the checker.

A confirmed report is an error that the sample audit did not catch. The report counts it in the stratum of the node, so that the published rates include it.

## K9.9 The rules that this document adds

| Rule | H/S | Checks | Section |
|---|---|---|---|
| L05 | H | copy check result present and clean for agent-written bodies | K9.4 |
| P05 | H | agent-added sources resolve and match | K9.5 |
| P06 | S | unconfirmed locators older than 180 days | K9.5 |
| X05 | H | an agent-written statement or proof has the results of the sanity checks of K9.3 in its run record | K9.3 |
| X06 | S | a stratum whose error rate is near its threshold | K9.7 |
| X07 | S | a theorem that a body names and that resolves to no node and has no source | K9.3 |

With the rules of K03 (T08 to T10) and K04, the catalogue of spec 15.3 grows from 65 rules. Each new rule needs a fixture pair, as spec 15.3 says.
