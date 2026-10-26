# Math 2.0: the corpus specification

The main spec (the files `00` to `21` in the parent directory) says how math2 holds, checks and serves mathematics. This sub-spec says how math2 gets the mathematics: each definition, theorem, proof and example that a person can find in a public source, and what models know, as nodes that people can read and machines can check, ready to go into a course, a book or an article (K2.1).

The corpus sub-spec adds to the main spec and changes it in a small number of named places. Where the two disagree, this sub-spec is the newer text, and the change is listed in "Changes to the main spec" below. The main spec files are not edited by this sub-spec. A change becomes part of them in the same pull request that implements it.

This sub-spec was written on 2026-10-26. Its evidence base is K01, which reflects the sources as of that date.

## Documents

| # | Document | What it settles |
|---|---|---|
| 00 | README (this file) | index, reading order, settled corpus decisions, conventions, the rules that the corpus adds |
| 01 | [Research](01-research.md) | the method, the findings that change the plan, the corrections to the main spec, what could not be verified |
| 01a | [Part A: reference works](01a-reference-works.md) | encyclopedias, wikis, databases, problem lists and Q&A sites: sizes, licences, channels |
| 01b | [Part B: literature and books](01b-literature-and-books.md) | literature indexes, full text, extraction projects, open textbooks, syllabi, theorem lists |
| 01c | [Part C: formal libraries](01c-formal-libraries.md) | formal libraries, translation between systems, formal and informal datasets |
| 01d | [Part D: model generation](01d-model-generation.md) | error rates, projects at scale, memorization, the law on model output, provider terms, cost |
| 01e | [Part E: representation and packaging](01e-representation-and-packaging.md) | formats, levels of explanation, exercises, book and course formats, notation |
| 02 | [The goal](02-the-goal.md) | the goal in one sentence, items, writing levels W0 to W3, formal outcomes, what the corpus must not do, the reference scale |
| 03 | [Identifiers](03-identifiers.md) | tags of ten characters, the check character, sharding, one space for nodes and inventory rows |
| 04 | [The inventory](04-inventory.md) | mentions and rows, clustering, outcomes, which declarations and objects become nodes, measuring what is missing |
| 05 | [Human sources](05-human-sources.md) | the source document, licence reading, public domain, reference works, textbooks, literature, curricula, exercise banks, adapters, import order |
| 06 | [Formal sources](06-formal-sources.md) | intake classes, the libraries, Mathlib, libraries that AI wrote, other systems, crosswalks, datasets, informalization |
| 07 | [Machine generation](07-machine-generation.md) | what models are for, the four new roles, generation from what models know, the write job, the model register, configurations, provider terms |
| 08 | [The node card](08-the-node-card.md) | the house style, front matter, the card of each kind, proofs in steps, explanations at levels, exercises, schema changes |
| 09 | [Quality](09-quality.md) | the gates before merge, sanity checks, the copy check, source confirmation, what people check, sample audits, reader reports |
| 10 | [Structure and notation](10-structure-and-notation.md) | the taxonomy, curricula, the notation register, conventions, names and languages |
| 11 | [Packaging](11-packaging.md) | the volume file, linearization, editorial text, outputs, the pack check, licence, editions, the Lean companion |
| 12 | [Pipeline and operations](12-pipeline-and-operations.md) | the stages of an item, jobs, priority, batches, review capacity, cost, measurement, commands and libraries |
| 13 | [Law and ethics](13-legal-and-ethics.md) | the three zones, copyright in model text, copies, human sources, the EU AI Act, provider terms, disclosure, questions for a lawyer |
| 14 | [Roadmap](14-roadmap.md) | phases 0 to 5 with entry and exit criteria, what runs all the time, the people that the phases need |
| 15 | [Open questions](15-open-questions.md) | OQ01 to OQ60, each with the way to settle it and the phase that it blocks |

## Reading order

- **Overview:** K02, then K14, then the settled decisions below.
- **Before an importer is written:** K05 or K06, then K04, K13 and K15.
- **Before agents write nodes:** K07, K08 and K09, then K12.
- **Before a volume is built:** K08, K10 and K11.
- **Reviewers of the design:** K01 first, then K09, K13 and K15. K01 is the evidence; K09, K13 and K15 are where the design could be wrong.

## How the corpus relates to the main spec

The corpus uses the objects of the main spec and adds no new box to its architecture:

| The corpus needs | The main spec gives | The corpus adds |
|---|---|---|
| a place for each item | nodes and kinds (spec 04) | the inventory as a register of rows (K04), the exercise node (K8.7) |
| sources | the record plane and licence classes (spec 09) | the source document, licence reading, the public-domain rule (K05), intake classes (K06) |
| writers | agent roles and run records (spec 11, 12) | four new roles, the model register, generation from what models know (K07) |
| checks | the audit and the status levels (spec 15) | the copy check, source confirmation, sample audits, new rules (K09) |
| readers | the workspace and explanations at levels (spec 13) | the node card (K08), the notation register (K10), volumes and the pack check (K11) |

## Settled corpus decisions

These decisions are settled. A change to one of them is a change to this file and to the document that it names, in one commit, with the reason.

1. **Tags have ten characters.** Nine random characters and one check character, from Crockford base32. The tags of six characters that exist are frozen and stay valid (K03).
2. **Nodes and inventory rows share one identifier space.** A row identifier is never a node tag (K3.6, rule T08).
3. **The inventory is the denominator.** The corpus is complete when each row has an outcome. The open count is the number of rows with no outcome, and its target is zero (K04).
4. **No row from models alone.** A mention that a model gave becomes a row only when a source that people wrote confirms it (rule I03).
5. **The licence class decides what the corpus takes from a source.** Only `open` text enters the prose zone. `share-alike-nc`, `gfdl` and `record` give records, locators and links. `verbatim` gives short quotations only (K5.1, K13.4).
6. **The licence is read at the source.** An aggregator does not change a licence. The licence is read again at each snapshot, and the reading has a date and a person (K5.2.1).
7. **Public domain means the US and the EU.** A work is `open` as public domain only if it is in the public domain in both (K5.2.3).
8. **Each formal library has one intake class:** `bind`, `crosswalk`, `link`, `evaluate` or `exclude` (K6.2). Benchmarks evaluate roles; they are not sources (K6.8).
9. **Models write; they never decide what is true.** Models never raise a status, and a judge is of a family other than the writer's (K07, rule X09). This keeps settled decision 3 of the main spec.
10. **The copy check runs against a reference corpus,** not only against the cited sources. No restricted text is in the context of a write job (K9.4, K7.5).
11. **The prose zone stays CC BY-SA 4.0,** with a notice that the licence applies only where copyright subsists. The provenance says which sections software wrote (K13.2). This keeps settled decision 17 of the main spec.
12. **The exercise is a node.** Its text is in the prose zone. Its Lean statement and its reference solution are in the code zone, Apache-2.0 (K8.7, K13.1).
13. **One notation register.** Each macro has one meaning in its scope, and the LaTeX macros and the KaTeX map are generated from the register (K10.4).
14. **A volume adds no mathematics.** Each definition, statement, proof and example in a volume is a node, and glue states no mathematics (K11).
15. **Review capacity sets the speed.** Agent work is capped by what people can review, not by the cost of the models (K12.5).
16. **Phases have entry and exit criteria, not dates** (K14).

## Conventions

- **Numbering.** Corpus documents are K01 to K15, and their sections are K2.1, K2.2 and so on. A reference to "spec 09" or "spec 9.3" is to the main spec. A reference to "settled decision 17" is to the list in the main README.
- **Rules.** A rule has an id in the families of the main spec 15, and **H** (hard: the audit fails) or **S** (soft: the audit reports). The rules in this sub-spec are proposed. They become part of the audit when `m2 audit` implements them, and the main spec 15 lists them in the same pull request.
- **Not settled.** Where the sources disagree or a fact is not known, the text says "not settled" or names the question in K15, and it takes the safe course (K15.1).
- **Facts and dates.** Each fact from the research has a URL and the date of reading in K01. A figure that a later reading changes is changed in the corpus document and in the part of K01, in the same commit (K1.6).
- **Style.** The text follows ASD-STE100 Simplified Technical English.

## Rules that the corpus adds

| Rule | H/S | Checks | Where |
|---|---|---|---|
| T06 | H | changed: a tag has six or ten characters | K3.8 |
| T08 | H | the tag register and the inventory register have no value in common | K3.6, K3.7 |
| T09 | H | each tag of ten characters has a valid check character | K3.7 |
| T10 | H | no tag of six characters is minted after the cut-over commit | K3.7 |
| I01 | H | each `inventory` value in a node points to a row whose outcome is `node` or `member` with the tag of that node | K4.9 |
| I02 | H | each row with the outcome `node` or `member` points to a live node, or to a retired node with a successor | K4.9 |
| I03 | H | no row has only mentions from models | K4.9 |
| I04 | H | each outcome that a rule gave names a rule in `schema/outcomes.v1.yaml` | K4.9 |
| I05 | S | a `deferred` row whose date has passed | K4.9 |
| I06 | S | an MSC code at two digits whose estimated recall is below 90% | K4.9 |
| L05 | H | each agent-written body has a clean copy check result. Soft until the calibration is done | K9.9 |
| L06 | H | each item from a source with a licence for each item records the licence of the item | K13.9 |
| L07 | S | a node with a takedown report older than two working days that is not quarantined | K13.9 |
| L08 | H | each source document has the required fields, and each mention names a source | K5.14 |
| L09 | H | a source with a choice of licences has an `election` | K5.14 |
| L10 | H | a public-domain source of the `open` class is public domain in the US and the EU | K5.14 |
| L11 | H | a mention with text comes from a snapshot whose licence was read on or before the snapshot date | K5.14 |
| L12 | S | a source whose licence was last read more than twelve months ago | K5.14 |
| F10 | H | each mention from a library records the library, the pin, the declaration, the intake class and the licence of the item | K6.12 |
| F11 | H | no section holds text from a library of class `link`, `evaluate` or `exclude`, or from an item whose licence is not open | K6.12 |
| F12 | H | each library source document has `authored`, and each mention copies it | K6.12 |
| F13 | S | a binding to an `ai` or `mixed` declaration whose F2 review does not cover the definitions that its statement uses | K6.12 |
| P05 | H | each source that an agent added resolves to a record, and the record matches the claim | K9.9 |
| P06 | S | a source with `locator_unconfirmed` older than 180 days | K9.9 |
| X05 | H | an agent-written statement or proof has the results of the sanity checks in its run record | K9.9 |
| X06 | S | a stratum whose error rate is near its threshold | K9.9 |
| X07 | S | a theorem that a body names that resolves to no node and has no source | K9.9 |
| X08 | H | each field that an agent wrote names a model in the model register | K7.10 |
| X09 | H | each judge verdict is from a model of a family other than the writer's | K7.10 |
| N01 | H | each macro in a body has a register entry in scope, or the node defines it | K10.7 |
| N02 | H | two entries with the same glyph in overlapping scopes are the same macro, or list each other in `conflicts` | K10.7 |
| N03 | S | a curriculum puts a member before one of its prerequisites | K10.7 |
| N04 | S | a node uses a non-default option of a convention and does not say so | K10.7 |
| N05 | S | a concept with no `specializes` edge and no topic other than its MSC code | K10.7 |
| N06 | H | each macro has one `denotes`, and the macro files are the output of the register | K10.7 |
| N07 | S | a symbol in a `notation` field that no register entry has as `introduced_by` | K10.7 |

The pack check of a volume (PC1 to PC10, K11.6) is not an audit rule. It runs when a volume is built, and a volume that fails it is not published.

## Changes to the main spec

| Main spec | Change | Where |
|---|---|---|
| 00, 04.3, 04.4, 04.5, 05.1, 06.4, 06.7, 15.3, 20.1 | tags of ten characters, the shard key, the inventory register; rules T08 to T10 | K3.7 |
| 04.9 | the new edges, sections and fields of node.v1; the exercise kind in node.v2 | K8.9 |
| 1.7, 9.2, 9.3, 9.6, 16.6 | figures and licences of sources, including Kerodon as `record` | K1.4, K5.13 |
| 1.7, 9.4 | figures of formal libraries | K6.11 |
| 9.1 | adapters are `importer/<source>` | K5.11 |
| 12 | the roles `enumerate`, `write`, `informalize` and `exercise` | K7.3 |
| 15 | the rules above | this file |
| 16.6 | the zone of the Lean part of an exercise is the code zone | K13.1 |
