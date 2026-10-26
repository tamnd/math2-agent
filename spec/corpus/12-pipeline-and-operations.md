# K12. Pipeline and operations

This document says how the corpus is produced: the stages that an item goes through, the jobs and the queues, the limits on speed, the cost, the commands and the libraries. It uses the orchestrator of spec 11.4 and 11.5 and the roles of spec 12.1 and K7.3. It adds no new write path. Each change is a pull request, and the audit is the gate (settled decision 7).

## K12.1 The stages of an item

An item goes from "a source mentions it" to "a complete node" in stages. Each stage has a goal predicate that the build can compute, so a job knows when it is done.

| Stage | Goal predicate | Done by | Section |
|---|---|---|---|
| 1. Listed | the item is a mention in a source catalogue, or in a list that a model made | importers (K05, K06), the enumerate role (K7.3) | K4.2 |
| 2. Inventoried | the mention belongs to an inventory row | clustering (K4.3), the dedup judge, curators for close cases | K4.3 |
| 3. Decided | the row has an outcome: a node, a member of a hub, an `aligned` record, or a reason why it is not a node | the curate role proposes, the rules of K4.4 decide, curators decide the rest | K4.4 |
| 4. Stub | a node at W0 | the curate role, importers | K2.4 |
| 5. Stated | W1 | importers of `open` text, the write role, the informalize role for formal-first items | K08 |
| 6. Usable | W2 | the write, explain and exercise roles; the prove and formalize roles in parallel | K08 |
| 7. Complete | W3 | the same roles, and people who confirm sections | K08 |

The formal track runs next to stages 5 to 7. It does not wait for them:

| Formal stage | Goal | Done by |
|---|---|---|
| bound | a binding at F0 | formalize, or align for items that a library already has |
| checked | F1 | the F1 battery (spec 10.4) |
| reviewed | F2 or F3 | people (spec 10.6) |
| proved | E5 | prove, or an import from a library with a checked proof |

An item that a formal library has (K06) starts on the formal track at "bound" and reaches stage 5 through the informalize role. An item from a textbook starts at stage 5 and reaches the formal track through formalize. Both paths end at the same node.

## K12.2 Jobs

The corpus adds standing queues to the four sources of jobs in spec 11.4. Each queue is computed from the graph and the inventory on each build:

| Queue | Holds | Role | Goal |
|---|---|---|---|
| `enumerate` | each topic and each MSC code whose inventory has fewer rows than its estimate (K4.7) | enumerate | the model lists each item of the area that it knows, with a source for each one |
| `cluster` | the mentions with no row, and the candidate pairs above the similarity threshold | dedup judge, then curators | each mention belongs to one row |
| `decide` | the rows with no outcome | curate | the open count goes down |
| `state` | the nodes at W0 | write, informalize | W1 |
| `complete` | the nodes below the writing level of their volume (K11) or their layer (K2.7) | write, explain, exercise | the next writing level |
| `confirm` | the agent-written sections with no confirmation | review-assist, then people | rule P04 is clean |
| `sanity` | the agent-written statements with no result of K9.3 | explore | X05 is clean |
| `source` | the sources with `locator_unconfirmed` | review-assist, then people | P06 is clean |
| `bind`, `prove` | the queues of spec 11.4 | formalize, prove | spec 12.4 |

A job of the corpus has the record of spec 11.4. Its goal names a stage, for example `goal: "node reaches W2"` or `goal: "row 9DK2RX7Q4H has an outcome"`.

## K12.3 Priority

The priority formula of spec 11.4 applies. The corpus adds two terms, and the formula keeps its version like a prompt:

```
priority = w1 · centrality(target)
         + w2 · demand(target)
         + w3 · unlock(target)
         + w4 · tractability(target, role)
         − w5 · attempts(target, role)
         + w6 · spread(target)        # the number of independent sources that hold the item
         + w7 · curriculum(target)    # the item is in a curriculum or a standard volume (K10, K11)
```

`spread` puts first the items that many sources hold. An item that eight textbooks and two libraries hold is almost surely standard and is cheap to state well. An item that one source holds needs more care, and it can wait. `curriculum` puts first the items that a course or a book needs, so that the first volumes can be packed early (K14).

The order of work in spec 18.5 still applies: breadth first on the record plane, depth first on the statement plane. In the corpus, this means that the inventory of each area is built first, and then each area is written in depth, in the order of the curricula.

## K12.4 Batches

Most corpus work has no person waiting for it. So it runs in batches:

- **Model batches.** The write, explain, exercise, informalize and enumerate roles, and the dedup judge, use Message Batches at half price (spec 12.3). A batch holds jobs of one role and one topic, so that the cached part of the context (the system prompt, the card, the style guide, the notation register of the topic) is the same for each job.
- **Pull request batches.** One pull request for each node would give millions of pull requests. A batch pull request holds up to 50 nodes of one topic, from one role and one configuration. The audit checks each node in it. A reviewer can merge a part of it: the nodes that pass, with the others sent back to the queue with their reasons.
- **Small diffs.** A batch pull request changes only the files of its nodes and their provenance logs. It never changes a register by hand. The build writes the registers.

## K12.5 Review capacity is the limit

Settled decision 20 caps agent pull requests at a multiple of the measured review throughput. In the corpus, this is the main limit on speed, not money and not model speed. So the plan starts from the review hours.

Each month, the review hours go first to the work that keeps the measurements honest, and then to the work that moves items:

| Order | Use | Example cost |
|---|---|---|
| 1 | the sample audit (K9.7) | about 170 hours at 20 strata |
| 2 | reader reports (K9.8) and quarantine decisions | measured |
| 3 | F2 reviews in the order of `unlock` (spec 10.6) | 3 minutes for each undergraduate statement |
| 4 | confirmation of agent-written sections (rule P04) | 1 to 2 minutes for each section, by estimate |
| 5 | decisions on close cases of the inventory (K4.3, K4.4) | 1 minute for each case, by estimate |
| 6 | locator confirmation (K9.5) | 2 minutes for each source, by estimate |

The agents then get pull request capacity in proportion to the review hours that remain, by spec 11.5. A role that makes work that nobody can review in time does not run.

An example with round numbers. 100 active reviewers give 10 hours each month: 1,000 hours. The sample audit and the reports take 250 hours. Of the 750 hours that remain, 400 go to F2 reviews (8,000 statements) and 350 go to confirmation (about 14,000 sections). At this rate, F2 for the 500,000 statements of the textbook layer takes about five years, and W3 for 1.5 million nodes takes longer. This is why W2 is the target of the textbook layer and W3 is the target of the standard volumes only (K2.7, K11). It is also why the corpus must recruit reviewers (K14) and make each review fast (spec 16.3).

## K12.6 The cost of the models

The model cost is small next to the cost of people, but it is not zero, and the plan must say what it is. The numbers below are estimates for planning, with the prices of spec 12.3 and batch pricing. They assume that the cached part of each context is read from the cache. The run records replace them with measured numbers after the first thousand jobs of each role. K7.8 gives the cost of each configuration of models for one node.

| Stage | Tier | Tokens in, tokens out, for each item | Cost for each item |
|---|---|---|---|
| enumerate (for each item listed) | 1 | 0.5k, 0.1k | under $0.001 |
| dedup judge (for each candidate pair) | 0 | 2k, 0.1k | under $0.001 |
| state: W0 to W1 | 1 | 10k, 1.5k | about $0.02 |
| usable: W1 to W2 (proof, examples, L2, the second-model check of K9.3) | 1, and 2 for hard items | 30k, 6k | about $0.06 at tier 1, $0.12 at tier 2 |
| complete: W2 to W3 (other levels, history, exercises) | 1 and 2 | 30k, 8k | about $0.07 to $0.14 |
| formalize a statement to F1 | 1 to 3 | varies by orders of magnitude | $0.50 to $5, by spec 17 measurements when they exist |

For the textbook layer of 1.5 million nodes at W2, the writing cost is about $0.10 for each node, or about $150,000. Retries and escalation can double it. Formalization of 500,000 statements costs $250,000 to $2.5 million, and it is the largest model cost. The copy check (K9.4) and the sanity checks by computation run on math2's own machines.

**No model run starts without a budget that a person approved.** The global monthly budget of spec 11.5 is the only source of money. `m2 corpus plan` (K12.8) estimates the cost of a queue before anyone approves it.

## K12.7 Measurement

The corpus has its own section in the weekly agent report (spec 12.9) and in each release manifest:

- the open count, for each layer and each MSC code, and its change since the last report;
- the number of nodes at each writing level, for each layer;
- the formal outcomes of K2.5;
- the error rates of K9.7, with their intervals;
- the review hours used, by use (K12.5), and the queue lengths;
- the model spend, by role and stage, and the cost of each item that reached a new stage;
- the rate of fabricated sources (K9.5) and of copy matches (K9.4), by configuration.

A number that goes the wrong way is in the report with the others. The release notes account for each change in the counts (rule R03).

## K12.8 Commands and libraries

The corpus adds two command groups to `m2`. Each command is a thin layer over a library package. The packages follow the conventions of the toolchain: one package for each concept, no `internal/` directory, exported types with documentation and a testable example, and tests that use fakes for each model and each network service.

| Command | Does |
|---|---|
| `m2 corpus import -source NAME` | runs the importer of a source (K05, K06) into mentions in the record plane |
| `m2 corpus cluster` | groups mentions into inventory rows, and writes the close cases to the `cluster` queue |
| `m2 corpus status [-layer L] [-msc CODE]` | prints the open count, the writing levels and the formal outcomes |
| `m2 corpus queue NAME` | prints a standing queue in priority order |
| `m2 corpus plan NAME -n N` | estimates the cost and the review hours of the first N jobs of a queue. It does not call a model |
| `m2 corpus run NAME -n N -budget USD` | submits the jobs to the orchestrator under an approved budget |
| `m2 corpus sample -month YYYY-MM` | draws the sample of K9.7 and writes the audit packets |
| `m2 pack VOLUME` and `m2 pack -check` | K11 |

| Package | Holds |
|---|---|
| `inventory` | mentions, rows, outcomes and the open count (K04) |
| `cluster` | the grouping of mentions, with the similarity and the judge behind interfaces |
| `card` | the cards of K08, read from `schema/cards.v1.yaml`, and the writing level |
| `style` | the linter of K8.2 |
| `copycheck` | the shingles, the fingerprints and the match rules of K9.4 |
| `sources` | the confirmation of K9.5, with Crossref, zbMATH and the catalogues behind interfaces |
| `sample` | the strata, the draw and the intervals of K9.7 |
| `pack` | the volumes and the outputs of K11 |
| `importer/<source>` | one package for each source adapter of K05 and K06 |

## K12.9 Scale of the repository

The textbook and formal layers add 1 to 2 million node files, with their provenance logs. That is in the range of spec 06.1 and 06.7. The research layer is in the record plane, and its inventory rows are in the record plane too (K4.8). When the repository passes the trigger of spec 06.1 (10 GB packed or 5 million files), the record plane and the inventory of the research layer move to `tamnd/math2-records` together.

Agent branches are short. A batch branch lives until its pull request merges or closes, and the orchestrator deletes it then. Contributors who work on one area use a sparse checkout of the shards that they need. `m2` works on a sparse checkout, except for the commands that need the full graph (`m2 build`, `m2 audit`), which run in CI.
