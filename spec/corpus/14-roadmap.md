# K14. Roadmap

This document puts the corpus programme in phases. Each phase has a goal, the work, and an exit criterion that the build can check. A phase starts when the phase before it meets its exit criterion, except where the table says that two phases overlap. The phases come after the milestones of spec 19 that build the system. They use the system, and they do not replace a milestone.

Each phase that runs models needs a budget that a person approved (K12.6). No phase publishes a dataset or a DOI without approval (spec 06.2). The roadmap gives no dates, because the speed is set by review capacity (K12.5), and review capacity is not known yet.

## K14.1 The state on the day of this spec

`tamnd/math2` holds four nodes (the concept *Group*, Lagrange's theorem, and a computation with its node), the registers, the schemas of node.v1, the curriculum seed and the skeletons of 60 MSC classes in `topics/skeletons/`. The toolchain `tamnd/math2-agent` has the build, the audit of spec 15, the site, the agent framework with fakes, and the orchestrator. No corpus package of K12.8 exists yet. No model has written content for math2.

## K14.2 Phase 0: the foundations

**Goal.** The toolchain can hold, check and measure a corpus before the corpus exists.

**Work, in this order.** Each item is one pull request, or one in each repository:

1. Tags of ten characters (K3.8): the `tag` package, the schema patterns, `m2 migrate shard3`, rules T06 and T08 to T10, the Lean attribute and the resolver. In `tamnd/math2`: the migration, the cut-over commit and the frozen legacy set.
2. The minor version of node.v1 with the fields and the edges of K8.9, and `schema/cards.v1.yaml`. The `card` and `style` packages, and the writing level in the build.
3. The record plane layout of K4.8: `records/mentions/`, `records/inventory/`, `registers/inventory/`. The `inventory` package, the open count, and rules I01 to I06.
4. The notation register (K10.4), seeded from `macros/m2.sty`, with the build that writes `m2.sty` and `m2.katex.json` from it, and rules N01 to N07.
5. The `copycheck`, `sources` and `sample` packages, and rules L05, P05, P06, X05 and X06 (K9.9).
6. `m2 corpus status`, `queue` and `plan` (K12.8). These commands call no model.
7. The first `pack` with the HTML output, and `m2 pack -check` (K11). LaTeX and PreTeXt follow in phase 2.

**Exit.** Each rule of K9.9, K4.9 and K10.7 has a fixture pair, and the audit runs them on `tamnd/math2`. `m2 corpus status` prints the open count of an empty inventory. `m2 pack -check` passes on a volume made of the nodes that exist.

## K14.3 Phase 1: the inventory of sources that are safe and open

**Goal.** A first inventory, made only from sources whose data math2 can hold with no doubt about the licence, and no model.

**Work.**

1. Importers for the CC0 and permissive sources first: Wikidata items about mathematics, the declarations and the module documentation of Mathlib (Apache-2.0), the OEIS names (K4.6), and the curricula that are published as open data (K10.3).
2. Importers of records and locators only, for sources of the `record` and `gfdl` classes: tables of contents and indexes, where the source allows it (K05).
3. Clustering passes 1 and 2 (K4.3), which need no model. The gold set of K4.3 for the first areas.
4. The capture and recapture estimates of K4.7 for each MSC code at two digits.

**Exit.** Each mention has a row or is in the `cluster` queue. The report gives the estimated recall for each MSC code at two digits. The licence class of each mention is recorded, and rule L02 is clean.

## K14.4 Phase 2: the pilot area

**Goal.** One area, end to end, so that each number in K12 is measured and not estimated.

**The area.** Undergraduate group theory: MSC 20 at the level of a first course. The reasons: math2 already has two of its nodes, Mathlib covers it well, the textbook and wiki sources are many, and the statements are short enough that small-case checks (K9.3) work.

**Work.**

1. The inventory of the area from all sources of K05 and K06, and from the enumerate role, with the clustering passes 3 and 4.
2. An outcome for each row.
3. Nodes to W2 for each row with the outcome `node`, by importers of `open` text and by the write, explain, exercise and informalize roles, under an approved budget.
4. The formal track for each statement: bound, F1, and F2 for the core list.
5. The sample audit of K9.7 for each stratum of the pilot.
6. The first volume: *Groups: a first course*, packed in each output format of K11.

**Exit.**

- The open count of the area is zero.
- Each node of the area is at W2 or better, and each statement has a formal outcome (K2.5).
- The sample audit gives an error rate under the thresholds of K9.7 for each stratum, with its interval. Strata above a threshold are stopped, and their output is fixed or removed.
- The measured cost for each stage replaces the estimates of K12.6 in the spec.
- `m2 pack -check` passes on the volume, and a teacher who is not in the project has read it and reported the result.

If the pilot fails its exit, the corpus programme stops and the spec changes before anything grows. A failed pilot is a result, and the report says why.

## K14.5 Phase 3: the textbook layer

**Goal.** The textbook layer of K2.7: an open count of zero, W2 for each node, and E2 or better for each statement.

**Work.** Area by area, in the order of the curricula of K10.3 and the priority of K12.3. The undergraduate core comes first (spec 18.3), then the graduate core, then the rest of the textbook layer. Each area repeats the work of the pilot with the measured numbers. A standard volume for each area of the undergraduate core goes to W3.

**Exit for each area.** The exit of the pilot. **Exit for the layer.** Each area has met its exit, and the inventory recall (K4.7) is at 90% or better for each MSC code at two digits.

## K14.6 Phase 4: the formal layer

**Goal.** The formal layer of K2.7: each declaration that a person would cite (K4.5) is a node with a binding.

This phase overlaps with phase 3. It starts when the importers of K06 exist, and it shares the review budget with phase 3 by the order of K12.5.

**Work.** For each library of K06: the declarations that K4.5 selects, the informalize role for the ones with no human mention, the alignment of the ones that a textbook row already has, and F1 for each binding.

**Exit.** Each row of the formal layer has an outcome. Each node of the layer has a binding at F1 or better.

## K14.7 Phase 5: the research layer

**Goal.** The research layer of K2.7: an open count of zero in the record plane.

**Work.** The extraction and the alignment of spec 09, area by area, in the order of spec 18.5. Each result gets the outcome `aligned` or `record`, or it becomes a node when the graph needs it.

**Exit.** Each result of the record plane has an outcome.

## K14.8 What runs all the time

From phase 2 on, these run each month: the sample audit (K9.7), the reports of K12.7, the estimates of recall (K4.7), and the review of the thresholds of K9.7. From phase 3 on, a new source that enters scope goes through the held-out check of K4.7 before its mentions are added.

## K14.9 The people that the phases need

| Phase | People | For |
|---|---|---|
| 0 | maintainers of the toolchain | the code |
| 1 | one curator for each source class | the licence decisions, the gold set |
| 2 | an area editor for group theory, 5 to 10 reviewers, one outside teacher | the pilot |
| 3 | an area editor for each area of the undergraduate core, about 100 reviewers | K12.5 |
| 4 | reviewers who know each formal library | F2 |
| 5 | area editors in the research areas | the close cases of the alignment |

The recruitment of reviewers is work of the project, not a side effect. Spec 16 says how reviewers get credit.
