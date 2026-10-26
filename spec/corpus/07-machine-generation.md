# K07. Machine generation: what models write, and how

The goal asks for the mathematics that people wrote down, and also for what models know (K2.1). Models are the only way to write a million nodes with the people that the project can have. They are also the largest source of errors, of copies and of invented sources. This document says what models do in the corpus, what they must not do, and how each piece of model output is controlled.

The main spec already sets the frame. Models never raise a status (settled decision 3). Each change is a pull request that the audit checks (settled decision 7). Agent pull requests are capped by review capacity (settled decision 20). Each run records the exact model ids, the role version and the prompt hashes (spec 12.3, 12.8). This document does not change these rules.

## K7.1 What models are for

| Use | Role | Output | Trusted as |
|---|---|---|---|
| list the items of an area | enumerate (K7.3) | mentions in the record plane | a mention source. A row never comes from model mentions alone (rule I03) |
| write the sections of a node | write, explain | bodies at W1 to W3 | text by an agent until a person confirms it (rule P04) |
| write the informal text of a formal declaration | informalize | statement and concept nodes | text by an agent. The binding is checked by the F1 battery, and F2 needs a person |
| write exercises | exercise | exercise nodes (K8.7) | text by an agent. An answer with a machine check is checked by the machine |
| decide, align, formalize, prove, explore | the roles of spec 12.1 | as in spec 12 | as in spec 12 |
| check other model output | judges (K7.6) | verdicts in the run record | triage only |

Models do **not**:

- decide that a statement is true. Only the kernel, the certificate checkers and refereed or textbook sources give E levels (spec 04.7);
- add a source that does not resolve to a record (rule P05);
- give the outcome of an inventory row by themselves when the rules of K4.4 say that a curator decides;
- get the text of a source whose licence class is not `open` in their context (K7.5);
- write a register, a review or a status (spec 12.1).

## K7.2 What the evidence says

The research of K01 (part D) measured model output in mathematics from 2024 to 2026. Eight results set the controls of this document:

1. Model judges gave proofs scores up to 20 times higher than human graders did.
2. The best judges of single proof steps still accept about 21% of wrong steps. Weaker judges accept 56% to 90%.
3. Strong models "proved" false statements of competition level 29% to 70% of the time. At research level, they rejected fewer than 40% of false statements.
4. A correct final answer is not a correct proof. For one model, only 59.5% of correct answers had a correct proof.
5. From 11% to 57% of the citations that models generate do not exist. A citation that more than three models give is real 95.6% of the time.
6. Statements that models formalize with no person in the loop are faithful 60% to 90% of the time. A statement that compiles is not a faithful statement: one system compiled 90% and was accurate on 66%.
7. More than 27% of the text that models write in the style of an encyclopedia is in verbatim runs of 50 characters or more from public text.
8. No study measures how often models write a wrong definition, drop a hypothesis or give a theorem the wrong name in informal text. The corpus must measure this itself (K9.7).

Most of these numbers come from models of 2025. Nobody has published the same measures for the current models. So the corpus measures its own configurations before it trusts them, and the numbers above are the prior.

## K7.3 The four new roles

The corpus adds four roles to the eight of spec 12.1. Each has a bot account, a tool profile and a field permission set, and rule X01 checks each pull request against it.

| Role | Does | May create | May edit | Auto-merge |
|---|---|---|---|---|
| **enumerate** | lists the items of an area that the model knows, each with its kind, a one-line statement, its aliases and one or more source identifiers | mentions in `records/mentions/model-<family>/` (K4.8) | none | yes, into the record plane only. A mention changes no node |
| **write** | writes the sections of a node that its card asks for at the target writing level (K8.4): the statement, the definition, the proof in steps, the examples and non-examples | sections marked `by: agent`; example nodes; informal proof nodes | its own unconfirmed sections | never |
| **informalize** | writes the informal text of a formal declaration that no person described: the name, the statement, the summary | statement and concept nodes with a binding at F0 to the declaration; sections marked `by: agent` | its own unconfirmed sections | never |
| **exercise** | writes exercises for a node, with a hint, an answer, a solution and a check (K8.7) | exercise nodes (node.v2), or example nodes with the role `exercise` before node.v2 | its own unconfirmed exercises | never |

The roles of spec 12.1 do this work in the corpus:

- **curate** proposes the outcome of each row in the `decide` queue (K4.4);
- **explain** writes the levels L1 to L4 (K8.6), with the grounding check of spec 12.6;
- **formalize** and **prove** run the formal track (K12.1);
- **explore** runs the sanity checks of K9.3;
- **align** proposes the `aligned` outcomes and the crosswalks to Wikidata, Mathlib and the other libraries (K06);
- **review-assist** makes the packets for each check that people do (K9.6).

## K7.4 Generation from what models know

The enumerate role asks models what mathematics they know. It is how the corpus finds the items that no catalogue lists: the lemma that each textbook proves and no index names, the standard counterexample, the folklore result.

**The unit of work.** One job asks one model about one cell: an MSC code at five digits and one level (school, undergraduate, graduate, research). There are about 6,000 codes and 4 levels, so a full pass is about 24,000 jobs for each model.

**The prompt.** The job gives the model the code, the level, the names of the rows that the inventory already has for the cell, and the schema of a mention (K4.1). It asks for each item that is not on the list: its kind, its common name and aliases, a statement of one sentence, its prerequisites by name, and the sources where it appears, as identifiers (an ISBN with a theorem number, a DOI, an arXiv id, a Wikipedia title). It asks the model to say "unsure" when it is unsure, and never to give a source that it cannot name exactly.

**Rounds until saturation.** A cell gets a new round with the new list, until a round adds fewer than 2% new rows after clustering (K4.3), for two rounds in a row and for models of two families.

**More than one family.** The models of one family share their training data, so their lists are not independent. The job runs on at least two families: Claude, and one open-weight family with a licence that adds no duty on outputs (K7.9). An item that only one model lists, with no source that resolves, goes to the `cluster` queue as a mention and waits for a human source.

**What enumeration cannot measure.** Capture and recapture between models overstates recall, because the models have seen the same text. So model lists count as one capture occasion in the estimates of K4.7, and the held-out human source is the test of recall.

**The knowledge cutoff.** The current Claude models know mathematics up to June 2026. A model cannot list what was published after its cutoff, and its claim that a result is new means nothing. The research layer comes from the record plane (spec 09), not from what models remember.

## K7.5 How a write job runs

A write job takes one node to the next writing level. It runs the common loop of spec 12.2, with this context and these limits:

**The context.**

- the card of the kind at the target level (K8.4) and the house style (K8.2), in the cached part of the prompt;
- the notation register of the scope of the node and the conventions register (K10.4, K10.5), also cached;
- the node, its neighbourhood to depth 2, and the inventory row with all its mentions (K4.1);
- the text of the sources of the row, **only** when the source has the licence class `open` and its licence is compatible with the prose licence (CC0, CC BY, CC BY-SA, public domain). The node then carries the attribution of rule L03;
- for sources of the classes `share-alike-nc`, `gfdl`, `verbatim` and `record`: the identifier and the locator only. Their text never goes into the context of a model that writes prose for math2. A model that rewrites a GFDL proof that it can see makes a derived work, and the licence of math2 cannot take it.

**The instructions.**

- Write in your own words. Do not reproduce a source. This cuts copying, but it does not stop it (K9.4).
- Write each proof as numbered steps, with the reason for each step as a link to a node or a source (K8.5). The structure helps the judges, but it does not change the E level.
- For a statement, prove it or disprove it (K9.3). Report a hypothesis that you think is missing or not needed.
- Use only the macros of the register. Declare each new symbol in the `notation` field.
- Mark a source that you cannot name exactly as unsure, or leave it out.

**The output.** The diff adds sections marked `by: agent`, with the role version and the model id of each field (spec 04.8). The gates of K9.2 run in the loop before the pull request opens, so the model can repair its draft in the same run.

**The informalize role.** It gets the declaration, its type, its docstring when one exists, the module documentation, the declarations that it uses, and their informal text when math2 has it. The output is a statement in the house style, with the conventions of the declaration stated (K10.5). Then the F1 battery runs on the pair, as on any other binding. No published study measures the error rate of informalizations of Mathlib (K01, part C). So informalize is a stratum of its own in the sample audit from its first merge (K9.7).

**The exercise role.** It writes exercises for a node, with the fields of K8.7. It uses a model of another family to solve each exercise from the statement alone. An exercise whose answer the solver does not reach, or reaches in a different form, goes to a person. An exercise with `check: lean` must compile with `sorry`, and its reference solution must compile. Banks of exercises with a non-commercial licence (K05) are links only, and their text is not in the context.

## K7.6 Checks on model output

Each piece of model output goes through the gates of K9.2. The judges of K9.3 follow these rules, because the evidence of K7.2 says that one judge is not enough:

1. **A judge verdict is triage.** It decides what a person sees first. It never raises an E level, an F level or a writing level.
2. **The judge is of another family.** The model that wrote a node does not judge it. A check names the family of the writer and the family of each judge, and rule X09 checks that they differ.
3. **Judges are measured first.** A judge configuration judges proofs or statements only after it has a measured rate of rejecting wrong steps of 75% or more on a labelled set of math2. The set holds wrong proofs that people found, and canaries (K9.3). Tier 0 models do triage, classification and the first pass of the dedup judge, and they do not judge proofs.
4. **Accept by all, send by any.** A node passes the judges when each judge passes it. When one judge fails it, the node goes to a person with the reports of all judges. No extra call breaks a tie.
5. **Canaries each month.** The queues get false statements made by small changes to true ones. The weekly report gives the share that each configuration "proved", and a share above 5% stops the configuration (K9.3).
6. **Agents game checkers.** In a published benchmark, agents tried to work around the elaborator. So the pinned statement, the Comparator check and the axiom audit of spec 12.2 apply to each accepted proof, from each prover, with no exception.

## K7.7 Provenance and the model register

- **Each field has its writer.** The provenance of spec 04.8 records the role version, the model id and the run id of each field that an agent wrote. A person who confirms a field adds a confirmation and does not replace the record.
- **A register of models.** `models/models.yaml` in `tamnd/math2-agent` holds an entry for each model that the orchestrator can call: the exact id, the family, the provider, the licence or the terms with their URL and the date that a person read them, the clauses on outputs (for example a duty to name the model, or a ban on training other models with the outputs), and the retirement date when one is known. The orchestrator refuses a model with no entry. Each release manifest of `tamnd/math2` includes the entries of the models that wrote its fields.
- **Filters by model.** `m2 prov list -family F` lists each field that a model of a family wrote. A user whose own terms need it can remove those fields. The corpus uses the same filter for the rule of K7.9 on training.
- **Model change.** The current Claude models retire from September 2027. A campaign of a million nodes will cross a model change. A new model is a new configuration, with its own stratum in the sample audit (K9.7), and `m2 prov quarantine` can act on the output of one model.

## K7.8 Configurations and cost

The cost estimate of K12.6 is for each stage. This table, from K01 part D, is for each configuration of one node with one generation call and three check calls, at batch prices with cache hits. The token counts are assumptions that the first thousand runs replace.

| Configuration | For each node | For one million nodes |
|---|---|---|
| all calls on Haiku 5.5 | $0.004 | about $4,000 |
| all calls on Sonnet 5.5 | $0.081 | about $81,000 |
| all calls on Opus 5.5 | $0.161 | about $161,000 |
| all calls on Fable 5.1 | $0.40 | about $400,000 |
| write on Opus 5.5, check with two Sonnet 5.5 calls and one call to a model of another family | $0.113 | about $113,000 |

- **Divide by the acceptance rate.** When half of the drafts pass, the cost doubles. Until the pilot measures the rate (K14.4), the plan uses two to five times the table.
- **Output tokens are 70% to 85% of the cost**, and thinking tokens are output tokens. The effort setting is the lever that matters. The weekly report gives the output tokens for each accepted node, for each role and tier.
- **The all-Haiku row is not a real option** for proofs and statements, because tier 0 does not judge proofs (K7.6). It is the cost of enumeration and triage.
- **Formal work costs much more.** A published campaign of textbook formalization cost about $17 for each Lean declaration and about $300 for each target theorem (K01). K12.6 prices it apart.
- **Prices change.** The price table of the `llm` package has the date of each price, and it models batch, cache reads, cache writes and the long-context tiers. A table with no date is a finding of the weekly report.

The money is not the limit. The limit is the review hours (K12.5). The plan gives the cost of each accepted node and of each F2 binding, not only the spend.

## K7.9 Terms of the providers

K13 gives the law. This section gives the rules for the operations:

- **Publishing is allowed.** The terms of Anthropic, OpenAI and Google that a person read on 2026-10-26 give the customer the rights in the outputs that the provider has, and none of them stops the publication of outputs under an open licence.
- **No training on hosted outputs without authorization.** From 12 November 2026, the usage policy of Anthropic forbids the use of outputs to train an AI model without its prior authorization. OpenAI and Google forbid the use of outputs to build models that compete with theirs. So the corpus does not fine-tune a model (for example an open prover of tier 0′) on fields that a hosted model wrote, unless the provider authorized it in writing. The filter of K7.7 makes this mechanical.
- **Open-weight models with duties on outputs.** Some licences add duties on outputs or on models trained with them: the naming duty of Llama 4, the clause of the DeepSeek Model Agreement on models trained with its synthetic data, and the conditions of the Qwen 3.8 and Kimi K3 licences for outputs made available to others. The model register records each clause. For roles that write content, the corpus prefers models with MIT or Apache-2.0 licences and no clause on outputs, for example DeepSeek V4, Qwen3.5, GLM-5, Mistral, Leanstral, gpt-oss, Goedel-Prover-V2 and Kimina-Prover.
- **The licence of math2 adds no term.** CC BY-SA cannot carry a ban on training, and math2 does not try to add one. The release notes say which fields came from which provider, so that users with their own contracts can filter.
- **No model run without approval.** Each run that calls a model needs a budget that a person approved (K12.6). Tests use fakes for each model.

## K7.10 Rules

| Rule | H/S | Checks |
|---|---|---|
| X08 | H | each field that an agent wrote names a model that has an entry in the model register |
| X09 | H | each judge verdict in a check of K9.3 is from a model of a family other than the family of the writer |

With rule I03 (K4.9), L05 (K9.4) and P05 (K9.5), these rules are the hard limits on model output.
