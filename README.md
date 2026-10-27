# math2-agent

This repository holds the toolchain and the research agents for Math 2.0. The content is in [tamnd/math2](https://github.com/tamnd/math2).

Math 2.0 is a versioned knowledge graph of mathematics. The `m2` command reads the node files in math2. It builds the Lean library, computes the status of each node, makes the graph and checks each change with the audit rules. Later milestones add search, an API, an MCP server, a research workspace and AI research agents.

## Specification

The design is in [spec/](spec/00-README.md). It has 22 documents, 00 to 21. Issues, code and reviews refer to a section by its number, for example 04.5 or 15.3 S03.

The wiki at https://tamnd.github.io/math2 shows the spec, the milestones and each node.

## Status

The project is at milestone M3. Each release v0.N.0 closes the code of one or more milestones:

- Release v0.1.0 closes the code of M0: `m2 new`, `m2 fmt`, `m2 check`, `m2 mint` and `m2 site build` are written.
- Release v0.2.0 closes the code of M1 and M2.

The code of M1 is the trust core. `m2 audit` runs the rules T01 to T06, S01 to S04, S09, B01 to B05, G01 to G10, F01 to F09, V01 to V04, R01 and R02. The rules G04, F01 to F05 and F07 to F09 read the files in `build/` from the last `m2 build`. A rule skips when a file that it needs does not exist. The `graph` package derives the graph from the node files, with the AND-OR closure of spec 05.5, the CSR adjacency and the Parquet build format. The `lean` package drives M2Extract, a small Lean program that gives the type hash, the axioms, the formal dependencies and the deprecation of each bound declaration. It also runs the checks for degenerate statements of spec 10.4 on each bound theorem. `m2 build` runs `lake build`, runs M2Extract on each binding, replays each binding that needs it in six kernels, computes the status of each node and writes the graph, with a cache for each step. The `status` package computes the evidence level, the fidelity level and the flags of spec 04.7, and `m2 status` shows them. A review counts only when its reviewer has a record in `registers/reviewers.reg` and did not write the binding. F3 needs two reviewers with no shared affiliation. The `release` package makes the release manifest of spec 06.2 from a tree and its last build. It rebuilds the graph of a release from its commit (rule R01), and it rebuilds the `.olean` files and lists each file that is different (rule R02).

The code of M2 connects the graph to the world. The `store` package puts the graph and the status of each node into one SQLite file, and `m2 graph` runs the neighbourhood, closure, reverse dependency, learning path and frontier queries on it. The `synth` package and `bench/graph` measure these queries on a synthetic graph. The `ingest` package and `m2 ingest` get pinned snapshots of external sources, apply the licence gate of spec 09.3 and write the records. The adapters of arXiv, zbMATH Open, Mathlib, the OEIS, Wikidata, the LMFDB, the Erdős problems, formal-conjectures, Palomar, the Stacks Project, PlanetMath, ProofWiki, Wikipedia, the nLab, the open textbooks and the open curricula are written. The rules L01 to L04 check the licences. Rule A01 checks that each `same_as` id has the syntax of its system, with the `crosswalk` package. The `align` package and `m2 align` propose `same_as`, `close_to` and `aligned` edges to the records of a source, and a person accepts or rejects each proposal. The rules A02 to A04 check these edges over time. `bench/align` measures the precision and the recall of the aligner on labelled pairs. The `bump` package and `m2 bump` move the tree to a newer Mathlib release on a new branch. `m2 release` cuts a release, writes its notes and verifies it, and the rules R03 to R05 check the notes, the restore drills and the pins. The `msc` package reads the topic map of MSC2020, and the `coverage` package and `m2 coverage` write the coverage report of each code.

Some acceptance items of M2 need time or people, and they stay open: three monthly bumps in a row, the releases of April to June 2027, the full harvests of zbMATH Open and arXiv for C0, and a test of the curation guide by a new curator.

The code of M3 opens the graph to readers. The `search` package writes the search index in `m2 build`, and `m2 search` runs the identifier resolver and the lexical, formula, formal and embedding channels, with the premise search of spec 14.5. `bench/search` measures the quality of search on labelled queries. The `api` package and `m2 serve` give the read methods of spec 11.2 over HTTP with JSON and over gRPC, and the `mcp` package gives the same methods as MCP tools. The `sandbox` package runs a program with no network and with limits, and the `compute` package and `m2 compute` run the computations in it. The rules C01 to C05 check the computations and their runs. The `prov` package reads and writes the provenance log of each node (spec 04.8), and `m2 prov` shows, records, credits, quarantines and confirms the lines. The rules P01 to P04 check the logs and the agent runs. The `ingest/arxivresults` adapter reads the results of arXiv papers from the content plane of tamnd/arxiv, and `bench/extract` measures it.

The code of M5 is the research engine, and it is in progress. The `llm` package is the one interface to a language model (spec 12.3), `llm/claude` is the client of the Claude models, and `llm/llmtest` has a scripted model for tests. The `agent` package runs a role with the common loop of spec 12.2: it loads the role at its version, gives the model only the typed tools of the profile of the role, checks the budget and the progress at each turn, and stops when the goal predicate holds, when the budget is used up, when the model makes no progress, or when the model reports that it is stuck. `agent.WriteRecord` writes the full run record and its summary for the provenance log (spec 11.6). The `workspace` package is the work tree of one run, and `agent.Permit` checks its changes against the field permissions of the role. `agent/tools` has the typed tools of the roles: the read methods of the API, the files of the work tree and the Lean check, and `tools.Profile` gives the tools of a role from its `role.yaml`. The `prompts` package has the eight roles of spec 12.1 the enumerate, write, informalize and exercise roles of corpus K7.3, and the understand role of corpus arxiv AX07. `agent/formalize`, `agent/prove`, `agent/explore`, `agent/explain`, `agent/curate`, `agent/align`, `agent/bump`, `agent/reviewassist`, `agent/enumerate`, `agent/write`, `agent/informalize`, `agent/exercise` and `agent/understand` have the goal predicate and the first message of each role. A statement with no proof is a definition of a proposition, `def S : Prop`, and a prove run writes a theorem of type `S`. The goal of prove checks that the proof has no `sorry`, uses only the standard axioms, and leaves the statement and its closure unchanged. The goal of explain checks that each claim of the explanation cites a node, that the cited nodes are in the curricula of the level, and that a second model call labels each sentence as supported by the cited nodes. The goal of curate checks that a new node from a record names the locator of the record, does not copy its text, and states the same result by the verdict of a judge call. The goal of align uses the same judge on each crosswalk edge that the run proposes, and it lets the run bind a node to Mathlib only when the Mathlib declaration has the type hash of the Math2 binding. The goal of bump checks that each Lean binding of the node resolves in the pinned Mathlib and is not deprecated, and that the run changed only bindings that were broken. The goal of review-assist checks that the review packet of the node in `reports/review/` has the packet of the last build, and that each concern of the critic names a binding, a class of spec 10.5 and a reason. An enumerate job asks a model for the items of one cell, an MSC code at five digits and a level such as `54D30/undergraduate`, that the inventory does not have. The tool `list_items` adds each item as a mention of the source `model-FAMILY` in `records/mentions/`, with its sources as cites that the clustering does not trust, and the goal checks that the run added a mention and changed nothing else. A write job asks a model for the sections, the uses, the examples and the proofs that the card of the target asks for at the next writing level (corpus K6), and the goal runs the gates of corpus K9.2 on the result. An informalize job asks a model for the node of a Mathlib declaration, in words and in the house style, with an exact binding to the declaration. An exercise job asks a model for exercises of one node, and a model of another family solves each one before a person sees it. An understand job is one pass of tier T1 on one arXiv paper, such as `arxiv:2401.00001/classify`, and it reads the analysis plane of a checkout of tamnd/arxiv that `-arxiv` gives. The pass `classify` gives the item kind of each object that the extractor could not place and of each remark and problem, and a confidence below 0.7 sends the item to the queue `classify-review`. The pass `concepts` gives the MSC2020 codes and the concepts of each item, with a Wikidata item or a name with the flag `new-concept`. The tool writes one record for each item to `records/understand/arxiv/PASS/`, and the record holds no text of the paper other than the terms that a definition of an open paper defines. The goal checks that each item of the pass has a record of the run. The goal of explore checks that the run filed a conjecture or a counterexample with the evidence of a computation, that each new computation verifies when it runs again in the sandbox, and that the output does not call itself new before the novelty check has run. The run record keeps the novelty check of each output. `agent.Meter` counts the spend of these calls in the budget of the run. No test calls a model service. The `novelty` package is the novelty check of spec 17.5. It searches the graph, the record plane, zbMATH Open, arXiv, MathOverflow and the OEIS for a claim, a judge compares the claim with the best matches, and the verdict is `known`, `likely-known` or `no-match-found`. No verdict says that a claim is new.

The corpus work of phase P0 is in progress (the corpus spec is in `spec/corpus/`). A new tag has ten characters: nine random characters and a check character. The three tags of six characters of math2 are a frozen legacy set. The tree is sharded by the first three characters of the tag. The `inventory` package holds the mentions, the rows and the open count, and `m2 corpus` imports, counts, queues and plans the work. The rules T08 to T10 check the tags: no value is both a tag and an inventory row, each long tag has a valid check character, and no tag of six characters is added to `registers/tags.reg` after the cut-over commit that `registers/README.md` names. The rules I01 to I06 check the inventory. The `card` package reads the node cards of corpus K08 from `schema/cards.v1.yaml` and computes the writing level W0 to W3 of each node, with the work list for the next level. The writing step of `m2 build` writes them to `build/writing.json`. The formal step of `m2 build` gives each statement, definition, conjecture and problem its formal outcome of corpus K2.5 (bound, proved, pending, blocked, out_of_reach or none) and writes `build/formal.json`. The outcome comes from the levels and the bindings in `build/status.json`, and from the decisions in `registers/formal.reg`. The wiki shows the level and the work list on the page of each node, and the page `levels/` counts the nodes at each level and lists the items that the nodes need most. A site with no build computes the levels from the tree. The `style` package checks the bodies against the house style of corpus K8.2, with the rules Y01 to Y04: the length of each sentence, the filler words that hide a step, "we" in the sections Statement and Definition, and references to place. `m2 check -style` runs them. A style finding is a warning, and it is an error for a node at W3 in `build/writing.json`. The `notation` package reads the notation register of corpus K10.4 in `notation/`, one file for each topic and `notation/global.yaml` for the symbols of all areas, and the register of conventions in `notation/conventions.yaml`. `m2 notation build` writes `macros/m2.sty` and `macros/m2.katex.json` from the register, and `m2 notation seed` makes a first register from `macros/m2.sty`. The rules N01 to N07 check the scope of each macro in a body, the glyphs that two entries share, the order of each curriculum, the conventions of each node, the place of each concept, the meaning of each macro with the macro files, and the home of each new symbol.

The audit report shows each other rule of spec 15.3 with the state skip and the reason "not written yet". `m2 help` shows the milestone that delivers each command. A command that is not written yet exits with code 3 and names its milestone. It does not exit with code 0, because a command that does nothing and exits with code 0 looks like a command that worked.

| Milestone | Dates | Result |
|---|---|---|
| [M0, the skeleton](https://github.com/tamnd/math2-agent/milestone/1) | Nov to Dec 2026 | Both repositories, the node schema, `m2 new`, `m2 fmt`, `m2 check` and the public wiki. |
| [M1, the seed and the trust core](https://github.com/tamnd/math2-agent/milestone/2) | Jan to Mar 2027 | About 2,000 seed nodes, `m2 build`, `m2 audit`, the checker stack and release 2027.03. |
| [M2, the graph meets the world](https://github.com/tamnd/math2-agent/milestone/3) | Apr to Jun 2027 | External sources, alignment, the monthly Mathlib bump and monthly releases. |
| [M3, open to readers](https://github.com/tamnd/math2-agent/milestone/4) | Jul to Sep 2027 | Search, the read API, the MCP server, the workspace and the computational engine. |
| [M4, fidelity at scale](https://github.com/tamnd/math2-agent/milestone/5) | Oct to Dec 2027 | The review tool, statement autoformalization and write access through the API. |
| [M5, the research engine](https://github.com/tamnd/math2-agent/milestone/6) | Jan to Jun 2028 | The research agents, the orchestrator, budgets and the weekly agent report. |

Each milestone has a tracking issue with the label `milestone`. The tracking issue has the exit criteria and a list of sub-issues.

## Install

You need Go 1.27 or later.

```sh
go install github.com/tamnd/math2-agent/cmd/m2@latest
```

You can also download a binary from the [releases page](https://github.com/tamnd/math2-agent/releases). Each archive has a build provenance attestation. To verify an archive, run this command:

```sh
gh attestation verify m2_v0.1.0_linux_amd64.tar.gz --repo tamnd/math2-agent
```

## Commands

| Command | What it does | Milestone |
|---|---|---|
| `m2 new <kind>` | Make a node file with a provisional id. | written |
| `m2 fmt` | Make front matter canonical and change aliases to tags. | written |
| `m2 check` | Check schema, body and links on the working tree. This does not use Lean. | written |
| `m2 mint` | Change the provisional ids to tags before merge, and add the tags to `registers/tags.reg`. `m2 mint -seed` gives tags that start with `0` to the nodes of the M1 seed. | written |
| `m2 migrate shard3` | Move each file of `nodes/`, `prov/` and `reviews/`, and the code of each computation in `compute/`, to the shard of the first three characters of its tag (corpus spec K3.5). A second run moves nothing. | written |
| `m2 migrate node.v2` | Move the tree to schema node.v2 (corpus spec K8.9). Each node.v1 exercise becomes a node of the kind `exercise`, with the fields `about`, `difficulty`, `math_group` and `check` and the sections Exercise, Hint, Answer and Solution. The tag does not change. Each moved field keeps its author in the provenance log. The command writes `schema/node.v2.json` and removes `schema/node.v1.json`. Until the move, the toolchain accepts both versions, and rule S09 flags each node.v1 exercise. A second run moves nothing. | written |
| `m2 notation build` | Write `macros/m2.sty` and `macros/m2.katex.json` from the notation register in `notation/`. `-check` changes no file and exits with code 1 when a file is not the output of the register. | written |
| `m2 notation seed` | Make a first register, `notation/global.yaml`, from `macros/m2.sty`. It does not change a register that is there. | written |
| `m2 site build` | Build the wiki: a static site for people and agents. | written |
| `m2 site check` | Test the wiki against the size, MathML, no-JavaScript, figure and offline targets. | written |
| `m2 site serve` | Preview the wiki with gzip, as GitHub Pages serves it. | written |
| `m2 site archive` | Write the wiki as a zip file that works offline. | written |
| `m2 pack <volume>` | Assemble a volume of `volumes/` into an edition: the order of the nodes, the numbers, the glue, the HTML page, the LaTeX and PreTeXt files and the lock file. `-check` runs the pack checks PC1 to PC10 and writes no file. | written |
| `m2 build` | Build the derived data: Lean, extraction, replay, the fidelity battery, status, graph, store and the search index. | written |
| `m2 audit` | Run the audit rules and write the report to `reports/`. | written |
| `m2 status <tag>` | Show the computed status of a node and the evidence for it. | written |
| `m2 graph` | Run a graph query: neighbourhood, closure, reverse dependencies, path or frontier. | written |
| `m2 ingest <source>` | Get a pinned snapshot of an external source and write its records. `-snapshot HASH` uses a recorded snapshot again. `-mentions` also writes the mentions. | written |
| `m2 wp extract [title...]` | Fetch Wikipedia articles to the content plane of the Wikipedia corpus (tamnd/wikipedia-math), and write their objects and their census to the analysis plane. With no title, extract again each page of the content plane. `m2 wp scope`, `fetch`, `census`, `refresh` and `gold` arrive in the later milestones of the Wikipedia roadmap. | written |
| `m2 align <source>` | Propose crosswalk edges to an external source. `m2 align queue`, `accept`, `reject` and `confirm` show the proposals and record the decision of a person. | written |
| `m2 bump` | Make a branch that moves Lean and Mathlib to the next release, rebind the renamed declarations and write the bump report. | written |
| `m2 coverage` | Write the coverage of each MSC2020 code at the levels C0 to C4 to `reports/coverage/`. `-count` asks zbMATH Open for the number of documents in each class. | written |
| `m2 release` | Cut a release with its manifest, levels and notes, write the release notes, or rebuild a release from its manifest. | written |
| `m2 search <query>` | Search the knowledge graph with the identifier resolver and the lexical, formula, formal and embedding channels. `m2 search -premises <goal>` ranks the Lean declarations that can help to prove a goal. | written |
| `m2 serve` | Serve the read API over HTTP with JSON and over gRPC, and the MCP server over HTTP or stdio, from the last build. | written |
| `m2 compute run\|verify <tag>` | Run a computation in the sandbox, or verify its output and its certificate. | written |
| `m2 prov` | Show, record, credit, quarantine, confirm and disclose provenance. | written |
| `m2 review <tag>` | Show the review packet of a node, and write a fidelity review of a Lean binding. | written |
| `m2 review queue` | List the bindings that wait for a review, with the nodes that more nodes use first. | written |
| `m2 review sample`, `m2 review measure` | Draw the monthly re-review sample, and measure the disagreement of the re-reviews. | written |
| `m2 review export [reviewer]` | List the reviews of each reviewer, for a CV or a grant report. | written |
| `m2 autoform <tag>` | Run the formalize role several times on a node, one candidate for each rung of `-rungs`, run the battery on each candidate, put the candidates that pass into groups by the hash of their statement, and commit the first candidate of the largest group on a branch, with its provenance lines. The report in `autoform/` of the store lists the candidates, the groups, the battery of the winner, the dissenting candidates and the definitions in the uses closure that have no Lean binding. `-pr` pushes the branch and opens the pull request with `gh`. | written |
| `m2 agent run align\|bump\|curate\|enumerate\|exercise\|explain\|explore\|formalize\|informalize\|prove\|review-assist\|understand\|write` | Do one agent run of one of the thirteen roles in a new work tree, and write its run record. | written |
| `m2 agent report` | Write the weekly agent report of the runs: volume, acceptance, cost and incidents. | written |
| `m2 corpus report` | Print the cost, the acceptance rate and the review hours of each stage of the corpus, from the run records, the provenance logs, the reviews and the inventory. | written |
| `m2 agent dataset` | Write the dataset of the runs of a quarter, merged or not, with no transcripts and no personal data. | written |
| `m2 orchestrate` | Run the jobs of the agent roles from a queue, with budgets and the escalation ladder. | written |

## The wiki

`m2 site build` makes the wiki at https://tamnd.github.io/math2 from a math2 working tree and the spec. The math2 repository runs it on each push to `main` and each night, and GitHub Pages serves the result.

```sh
go run ./cmd/m2 site build -content ../math2 -spec spec -out _site -url http://localhost:8000/math2
go run ./cmd/m2 site check -url http://localhost:8000/math2 _site
go run ./cmd/m2 site serve _site
```

Each node gets a page at `wiki/<slug>/` with an infobox, its links and its backlinks. The slug comes from the name, for example `wiki/lagranges-theorem/`. Two nodes with the same name get the kind or the tag as a suffix. The short link `node/<TAG>/` does not change when the name changes, and it sends the reader to the page. The build renders the math to HTML and MathML with KaTeX and the macros of math2, so a reader can read each node with JavaScript off. The site serves the KaTeX style sheet and fonts itself, so it loads nothing from a CDN. Each page also has a copy for programs: `node/<TAG>.md`, `api/node/<TAG>.json`, `api/nodes.json`, `llms.txt` and `llms-full.txt`. The pages Contribute and Curation come from CONTRIBUTING.md and CURATION.md in the math2 tree. A build with no `-date` flag gives the same bytes each time. Cloudflare Pages takes at most 20,000 files in one deployment, so `-cloudflare` writes no page for each short link. In place of those pages, it writes `api/links.json`, a `_worker.js` that serves `node/<TAG>/` from that map, and a `_routes.json` that runs the worker only for `node/*`.

Each page also shows what its node needs and what depends on it. The closure of a node is a layered figure, with the foundations at the top, and an ordered reading list with the same nodes. A topic page shows a blueprint: the members of the topic and each node that they need. The Frontier page lists the statements with no proof and the open conjectures and problems. These views come from the graph, so they need no build.

Give `-build DIR` to show the evidence level, the fidelity and the flags of each node from the store of a build in DIR. The bar on each box of a figure shows the evidence level in colour, and the box also gives the level as text. With no `-build` flag, a statement page says that the site has no build data.

`m2 site serve` serves the site at http://localhost:8000/math2 by default. Give the same `-url` to the build and to the server.

## The offline archive

`m2 site archive` writes a built site to a zip file that a reader can open from disk, with no network and no server. Each release of math2 attaches this archive, and Pages publishes the same build.

```sh
go run ./cmd/m2 site archive -url https://tamnd.github.io/math2 -o math2-2026.11-site.zip _site
```

Give the URL that the site was built with. In each page, each link into the site becomes a relative link, and a link to a directory names its index.html. A browser does not let a page on disk read `api/search.json`, so the archive also has the index as `assets/search.js`, and search works offline. Two archives of the same site are the same bytes.

## The site targets

Spec 13.7 sets these targets for each page of the wiki:

- A page takes less than 50 KB on the network before math fonts, with gzip.
- Each formula has MathML.
- A reader can read each page with JavaScript off.
- Each figure has a list with the same data.
- A page loads files from the site only, so an archive of the site works offline.
- Each page passes each accessibility audit of Lighthouse.
- The first paint and the largest paint come in less than 1 s on a mid-range phone on 4G.

`m2 site check` tests the first five targets on each page of a built site. It prints the heaviest page and each problem, and exits with code 1 when a page misses a target. The last two targets need a browser. The CI job `site targets` builds the site of the fixtures, runs `m2 site check`, and runs Lighthouse CI on one page of each template through `m2 site serve`. It puts the report in the summary of the run. Each release has the same report as `site-report.md`. See [.github/lighthouse](.github/lighthouse/README.md) for the profile and for a local run.

## Volumes

A volume is a book that `m2 pack` assembles from the nodes of the tree (corpus K11). Its file `volumes/<id>.yaml` gives the targets, the levels that each node must have, the choice of a definition for a concept with two or more, the conventions, the glue and the formats. `m2 pack <volume>` takes the closure of the targets, puts the nodes in an order where no node comes before a node that it uses, groups them into chapters and sections by their MSC codes and the curricula, and numbers the items. It writes the edition to `build/volumes/<id>/` and the lock file to `volumes/<id>.lock`. The lock file pins each node to a release and to the hash of its file, so the same lock builds the same edition again.

The HTML edition is one page with KaTeX and no JavaScript. Each item has its tag as its anchor. A link to a node in the volume goes to its item, and a link to a node outside the volume is a citation to its page on the wiki. The page has the conventions, the table of notation, the cited nodes and the attribution of each node and each glue text. A statement below evidence level E2 shows as a claim.

The formats `latex` and `pretext` write more files next to the page (corpus K11.4). `latex/` has `main.tex`, a copy of `macros/m2.sty` and `main.bib` with the sources of the nodes. Each item is an amsthm environment, the book kinds use the class `book` or `report`, and an article or a survey uses the class `article`. For an article, `latex/` is an arXiv tree with a `00README.json` that names pdflatex and TeX Live 2025. `pretext/main.ptx` is one PreTeXt file, where each kind of node is the nearest PreTeXt element and the table of notation is a `<notation-list/>`. Each edition has a list of sources and an attribution page that gives the licence CC BY-SA 4.0 of the prose, then the people, then the agent runs (corpus K11.7). The other formats, such as `jats` and `typst`, have no back end yet, and PC9 fails for them.

| Check | What it checks |
|---|---|
| PC1 | Each node of the volume has the writing level and the evidence level of the volume. |
| PC2 | No item comes before a node that it uses, and no proof uses an item after its statement. |
| PC3 | Each node that an item uses is in the volume or in `assumes`. |
| PC4 | No two macros of the volume print the same glyph for two meanings, unless the volume chooses a variant. |
| PC5 | Each node uses the option of the volume for each convention, or the volume names the node in `differ`. |
| PC6 | Each link of the volume ends at a node. |
| PC7 | The glue has no mathematics and has a writer, and a person confirmed the glue of an agent. |
| PC8 | The nodes of the volume pass the provenance rules L02 and L03. |
| PC9 | Each format builds the same bytes two times, and the lock file of a pinned release is up to date. |
| PC10 | Each section has the exercises that the volume asks for. |

Each check is hard. A volume that fails one does not build an edition.

## The build

`m2 build` makes the derived data of a math2 working tree in `build/`. Run it in the root of the tree. It needs elan for the Lean steps.

```sh
m2 build                # all the steps
m2 build -only lean     # only lake build
m2 build -json          # write the manifest as JSON
m2 build -no-sandbox    # run the checkers without the sandbox, for example on macOS
```

The steps are `lean`, `extract`, `replay`, `battery`, `status`, `graph`, `store` and `search`. The `lean` step gets the Mathlib cache and runs `lake build` in `lean/`. The `extract` step runs M2Extract on each formal declaration and adapter that a node names, and on each declaration with the attribute `@[m2 "TAG"]`. It writes `build/extract.json`. The `replay` step checks each binding that passes each check of E5 except the replay, and writes `build/replay.json`. The `battery` step runs the machine checks of spec 10.4 on each Lean binding and writes `build/battery.json`. The `status` step computes the evidence level and the fidelity level of each node from the nodes, `build/extract.json`, `build/replay.json`, `build/battery.json` and the reviews in `reviews/`, and writes `build/status.json`. The `graph` step writes the graph as Parquet to `build/graph/`. The `store` step writes the graph, the status of each node and the aliases to the SQLite file `build/store.db`, for `m2 graph`. The `search` step writes the search index to `build/search.db`, for `m2 search`. A tree without `lean/` has only the `status`, `graph`, `store` and `search` steps.

`build/manifest.json` records the Lean toolchain, the Mathlib commit, the hash of each checker program of the toolchain, and the input hash and output hash of each step. When the inputs of a step and its files do not change, the step does not run again. The output hash of the build covers `extract.json`, `replay.json`, `battery.json`, `status.json`, `graph/`, `store.db` and `search.db`.

## Computations

A computation is a node of kind `computation` (spec 08.2). Its code is in `compute/SHARD/TAG/` of the content tree, with the shard of the node file, for example `compute/AAY/AAYEQE/`. The code of a new computation is in `compute/~/~ID/` until the merge gives it a tag. Its record names the image by digest, the entry point, the inputs, the seed, the budgets, the hash of the output and, for a computation that can reach E4, a certificate.

```sh
m2 compute run 3MZ8QD       # run it in the sandbox
m2 compute verify 3MZ8QD    # run it, and check the output and the certificate
m2 compute verify -all      # each computation of the tree
```

The `compute` package reads the record and the image manifest `manifests/images.yaml`. A record names an image by the digest of one architecture, and the runner runs the digest of the same image for its own architecture. The program reads `compute/SHARD/TAG/` at `/work`, gets the inputs as JSON in `M2_INPUTS` and the seed in `M2_SEED`, and writes a certificate to `/out/certificate`. The output hash is the BLAKE3 hash of the standard output.

The run writes `run.json`, `stdout` and `stderr` to `build/compute/TAG/ARCH/`. A verified run exits with the code 0 in time, and its output has the hash of the record. When the record has a certificate, the certificate has the hash of the record, and the fixed checker of its kind (spec 08.8) accepts it. `compute.Checkers` is the table of the fixed checkers. A change to it needs the review of a person.

The rules C01 to C05 of `m2 audit` check the computations:

| Rule | It checks |
|---|---|
| C01 | the image of each record is in the manifest, the entry point exists in `compute/SHARD/TAG/`, and the record is valid |
| C02 | each E4 candidate has a run on amd64 and a run on arm64, the two runs give the same output and the same certificate, and these are the hashes of the record |
| C03 | the image and the checker image of each record are the current digests of the manifest. This rule is soft, because an old digest stays valid, and the next release runs the computation again |
| C04 | the certificate kind is known, the checker image is the fixed checker of the kind, and the checker program exists |
| C05 | each run in `build/compute/` had no network, and its logs name no model API |

C02 and C05 read the runs in `build/compute/`. C02 skips when the directory does not exist. An E4 candidate is a computation with a certificate that is not an experiment.

`m2 build` reads the same runs for the status. A computation is E1. It is E4 when it has a certificate, it is not an experiment, and the verified runs on amd64 and on arm64 agree with each other and with the record. The page of a computation on the site has a "Run this" panel. It gives the command that runs the record again, the image, the entry point, the inputs, the seed and the recorded hashes, and it shows the runs of the build next to the recorded output hash.

## The sandbox

The `sandbox` package runs a program in a container image with the isolation of spec 08.6. The image is named by its digest. The program runs with no network, on a read-only image, as the user `nobody`, with no capabilities. It reads its inputs at `/work`, which is read-only, and writes only to `/scratch` and `/tmp`, which are small tmpfs mounts. The run has limits on the CPU, the memory, the processes, the wall clock and the output. When the Docker daemon has the gVisor runtime `runsc`, `Docker.Detect` selects it.

The `sandbox` job of CI runs a probe on x86-64 and on arm64, with the default runtime and with gVisor. The probe tries to open a TCP socket and a UDP socket, to resolve a name, and to write outside the scratch area. Each try must fail. To run the probe on your machine:

```sh
M2_DOCKER=1 go test -run TestDocker -v ./sandbox
M2_DOCKER=1 M2_DOCKER_RUNTIME=runsc go test -run TestDocker -v ./sandbox
```

The test builds the probe as a static binary in an image `FROM scratch`, so it pulls no image.

## The replay of a proof

The `replay` package checks a formal declaration again, outside the process that elaborated it (spec 07.7). It writes a challenge module that states the declaration again from its record, with `sorry` as the proof. It exports the challenge and the module of the declaration with `leanexport`, and runs `lake comparator --paranoid` on the two exports. The declaration passes when it has the statement of the challenge, uses only the standard axioms, and each kernel of the toolchain accepts it. The kernels are `leanchecker`, `leanchecker-paranoid`, `lean4lean`, `nanoda_bin`, `con-leche` and `con-ron`. A check takes about 20 seconds for a declaration that imports Mathlib.

The comparator runs the checkers in a `bwrap` sandbox, and `bwrap` needs Linux. On macOS, `m2 build -no-sandbox` runs the checkers without the sandbox. A replay without the sandbox does not give E5, and `m2 status` gives the reason. E5 also needs two kernels that accept the proof. Two builds of the same commit give the same output hash (rule R01).

## The F1 battery

The `battery` package runs the ten machine checks of spec 10.4 on each Lean binding. Each check gives pass, fail, flag, open, skip or not-run, and a fail has a class of the taxonomy of spec 10.5, for example `M-junk` or `M-strong`. The constant audit maps each constant of the statement to the nodes that bind it, and it flags the junk values of Mathlib, such as natural subtraction and division by zero. The examples check and the neighbour check ask Lean for a short tactic proof of `A → B` or of `A → ¬B`. A binding is F1 when no blocking check fails or stays open. The back-translation and the critic need models, and the build does not run them yet, so no binding is F1 yet.

## Review a binding

`m2 review` shows the review packet of a node from the last `m2 build` (spec 10.6). The packet gives the informal statement and the Lean statement side by side, the Lean statement without notation, each constant of the statement with its type and the nodes that bind it, the report of the F1 battery and the reviews of the binding. With `-json`, the command writes the packet as JSON.

```sh
m2 review 3C53TD
m2 review -decl Math2.Match.lagrange -json 3C53TD
m2 review -reviewer github:octocat -verdict faithful -dry-run 3C53TD
m2 review -reviewer github:octocat -verdict faithful-with-conventions -convention "the order is Nat.card" -commit 3C53TD
m2 review -reviewer orcid:0000-0002-1825-0097 -verdict not-faithful -class M-hyp-extra -notes "the Lean statement adds that H is normal" 3C53TD
```

With `-verdict`, the command writes a review. It records the statement hash and the type hash of the binding, gives the review the next id of the node, adds it to `reviews/XX/TAG.yaml` and adds the id to `fidelity_reviews` of the binding. The reviewer is `orcid:` and an ORCID iD, or `github:` and a GitHub login, and `$M2_REVIEWER` gives the default. An agent cannot review, and the author of a binding cannot review it. The verdict `faithful-with-conventions` needs one `-convention` or more, and `not-faithful` needs a class of spec 10.5. The command refuses a review when the statement changed after the last build. `-dry-run` checks the review and changes no file. `-commit` commits the two files with `git commit -S`, so the commit has the signature of the reviewer. The tool does not sign for the reviewer. The packet does not yet show the candidate statements of autoformalization or the change of a type after a bump.

`m2 review queue` lists the fidelity queue of spec 16.3: the Lean bindings at F1 that wait for a first review. The nodes that more nodes use come first, because a review of such a node unlocks more of the graph (spec 11.4). With `-reviewer`, the list leaves out the bindings that the reviewer wrote or reviewed. `-area` keeps the nodes with an MSC2020 code that starts with the code, and `-second` lists the bindings at F2 that wait for a second review. The command shows 20 bindings, and `-n` changes the number.

```sh
m2 review queue -reviewer github:octocat -area 20
m2 review queue -second -n 0 -json
```

Each month, a different reviewer does a blind re-review of a sample of the reviewed bindings (spec 10.8). `m2 review sample` draws 1% of the bindings at F2 and 0.5% of the bindings at F3, at least one of each level. The draw depends only on the seed, the month and the bindings, thus anyone can draw the same sample again with the seed. Each binding goes to a registered reviewer who did not write the binding and did not review it, with a reviewer of the area of the node first. `-write` adds the sample to `registers/samples.reg` of the tree. The reviewer runs `m2 review -blind TAG`, which shows the packet without the reviews of the binding, and writes the review as usual. The review files are public, thus the reviewer must not open the review file of the node before the re-review. `m2 review measure` finds the re-review of each sample and shows the share of the re-reviews that disagree, for each level, in all areas and in each top-level MSC2020 class.

```sh
m2 review sample -month 2026-11 -write
m2 review -blind 3C53TD
m2 review measure 2026-11
```

A review is signed and public work, and the reviewer can cite it (spec 16.4). `m2 review export` lists the reviews of each reviewer, the newest first, with the node, the declaration and the verdict of each one. Give a reviewer to get only their reviews, and `-json` for JSON. The wiki has the same list for each reviewer at a stable link, `reviewers/SLUG/`, where the slug is the id with a hyphen for the colon. The JSON copy is at `api/reviewers/SLUG.json`, and `api/reviewers.json` lists the reviewers.

```sh
m2 review export github:octocat
m2 review export -json
```

## The status of a node

`m2 status` shows the status of a node from the last `m2 build`. It reads `build/status.json`. Give a tag, an alias or an m2:// URI. A URI of the form `m2://inv/<id>` names an inventory row, not a node. The wiki shows it on the card of the row, at `inventory/row/<prefix>/#<id>`.

```sh
m2 status 3C53TD
m2 status lagrange-theorem-groups
m2 status m2://lean/Subgroup.card_subgroup_dvd_card
m2 status -json 3C53TD
```

The output gives the evidence level, the fidelity level, the flags, the statement hash, each item of evidence, each Lean binding with its type hash, its axioms, its reviews and its replay, and one reason for each decision. For an E5 result, it also gives the checker versions. A tag that is not in the build exits with code 1.

## Graph queries

`m2 graph` runs a query on `build/store.db` from the last `m2 build`. Give each node as a tag, an alias or an m2:// URI. Put the flags before the nodes. Each query takes `-json`.

```sh
m2 graph neighbourhood D2SB01           # the nodes and the edges at most 2 edges from the node (Q2)
m2 graph closure 3C53TD                 # the best alternative, its trust profile and its axioms (Q3)
m2 graph closure -measure size 3C53TD   # the alternative with few nodes, not the least depth
m2 graph rdeps m2://concept/group       # the nodes that need the node, and the nodes that use it (Q4)
m2 graph path group 3C53TD              # the shortest learning path through definitions (Q5)
m2 graph frontier -msc 11               # the formal statements with no kernel proof (Q6)
m2 graph frontier -open -topic T0NT01   # the open problems and the statements that bound them (Q7)
```

`m2 graph neighbourhood` takes `-hops N`. `m2 graph frontier` also takes `-evidence E1`. A node that `needs` the node loses each alternative when the node is false. A node that `uses` the node uses it in at least one alternative. The learning path goes from `from` to `to` through the uses edges that end at a definition and the defines edges.

## Search

`m2 search` runs a query on `build/search.db` from the last `m2 build` (spec 14). A query can be words, a LaTeX formula, a Lean type or goal, an identifier, or a mix of these. Put the flags before the query.

```sh
m2 search finite subgroup of the multiplicative group of a field
m2 search '\sum_{d \mid n} \varphi(d) = n'
m2 search 'Subgroup _ → Finite _ → IsCyclic _'
m2 search 'IsCyclic, Units, Field'
m2 search Q180967                        # a Wikidata item, an alias, a tag, a Lean name or an m2:// URI
m2 search -kind statement,concept -k 20 group
m2 search -open -channel lexical,embedding prime gaps
m2 search -premises -tag 3C53TD 'G : Type
inst : Group G
⊢ IsCyclic G'
```

The identifier resolver runs first. A node that it finds comes first. Then four channels run, and reciprocal rank fusion combines their lists:

- **lexical**: BM25 in SQLite FTS5 over the names, the aliases, the statements and the other sections.
- **formula**: each formula in `$...$` or `\(...\)`, or a query with `^`, `_`, `=` or `\`, becomes an operator tree. The channel finds candidates by the paths from leaf to root, and ranks them by the size of the largest common subtree, with a consistent change of the names of the variables. Thus `|Y| = |H|^{q-1}` finds `|X| = |G|^{p-1}`.
- **formal**: a Lean type pattern, a goal or a list of constants is matched against the head of the conclusion, the heads of the hypotheses and the constants of each extracted declaration. A declaration that no node binds comes back as `lean:DECL`, with no node yet.
- **embedding**: the cosine between the query and the name and statement, the Lean statements and the explanations of each node. The vectors are int8. The default embedder runs on the local machine and needs no model. The index never gives out the vectors, only the neighbours and their scores.

Each result shows the channels that found it and its rank in each one, for example `lexical#1 formula#3`. Statements and concepts rank a little above proofs, and E5, E3 and E2 above E1. With `-open`, the open problems and the weak evidence rank first. A retired node folds into its live successor. `-json` writes the results as JSON.

`m2 search -premises` ranks the Lean declarations that can help to prove a goal (spec 14.5). It fuses the formal channel, the declarations that the proofs of near declarations use, the declarations of the nodes that the node of `-tag` uses, and the embedding similarity. The learned reranker comes with the run records of the agents.

## Serve

`m2 serve` serves the read API (spec 11.2) from the last `m2 build`. It reads `build/store.db`, `build/search.db` and the node files of the tree. A write is a pull request against tamnd/math2, and `m2 serve -propose` serves the write methods (see below).

```
m2 serve -addr localhost:8080
curl 'http://localhost:8080/v1/get?ref=Q529ES'
curl 'http://localhost:8080/v1/search?q=Lagrange&k=5'
curl -d '{"from": "DSG7KN", "to": "Q529ES"}' http://localhost:8080/v1/path
```

`GET /v1` lists the methods and their parameters. A method takes its parameters in the query of a GET request, or as a JSON object in the body of a POST request. The methods are `resolve`, `get`, `status`, `get_formal`, `neighbours`, `closure`, `rdeps`, `path`, `frontier`, `search`, `search_formal`, `search_formula`, `similar` and `premises`. Each response gives the version of the API, the method, the commit of the tree, the output hash of the build, and the content hash and the evidence level of each node in the result. An error gives `{"error": {"code": ..., "message": ...}}` with the HTTP status of its code: `invalid` is 400, `not_found` is 404, `rate_limited` is 429 and `internal_error` is 500. Each client can send `-rate` requests each second, and `-burst` requests at once. A client over the limit gets 429 with a `Retry-After` header. The contract test in `api/` keeps the answer of each method in `api/testdata/contract/`.

The MCP server (spec 11.2 and 11.7) has one tool for each method, with the same name and the same parameters. A tool gives the response of its method as structured content and as JSON text. Each tool only reads, and it has the read-only annotation. `m2 serve` serves MCP over streamable HTTP at `/mcp`, behind the same rate limit, and `m2 serve -stdio` serves it on stdin and stdout. To use it in Claude Code:

```
claude mcp add math2 -- m2 serve -stdio -root ~/math2
claude mcp add --transport http math2 http://localhost:8080/mcp
```

The gRPC service `m2.v1.ReadService` (spec 11.2) has one rpc for each method, for example `GetFormal` for `get_formal`. `api/m2.proto` defines it, and the table of methods in `api/methods.go` makes that file, so that the three encodings agree. `m2 serve -grpc localhost:9090` serves it next to HTTP, with the same rate limit for each client and with server reflection. The rpc gives the response of the HTTP API, with the result as a `google.protobuf.Value`. An error has the gRPC code of its code: `InvalidArgument`, `NotFound`, `ResourceExhausted` or `Internal`. The package `api/m2pb` has the Go client. After a change to the table, `make proto` makes `api/m2.proto` and `api/m2pb` again with `buf`, `protoc-gen-go` and `protoc-gen-go-grpc`.

```
m2 serve -grpc localhost:9090
grpcurl -plaintext -d '{"ref": "Q529ES"}' localhost:9090 m2.v1.ReadService/Status
```

### Writes

`m2 serve -propose` also serves the write methods of spec 11.2 over HTTP: `propose_node`, `propose_edit`, `propose_binding` and `propose_alignment`. Each write opens a pull request against tamnd/math2 (`-repo`) under the identity of the caller, thus the root must be a git clone of that repository. A write is a POST request with the header `Authorization: Bearer TOKEN`, where TOKEN is a GitHub token of the caller with the scope to fork the repository and to open a pull request. The server does not keep the token.

```
m2 serve -propose -root ~/math2
curl -H "Authorization: Bearer $GITHUB_TOKEN" -d '{"ref": "TAG", "edge": "same_as", "system": "wikidata", "id": "QID", "message": "Why the two are the same."}' http://localhost:8080/v1/propose_alignment
```

The `propose` package does the work. It makes the change from the tree, adds a provenance line for each changed field under the identity `github:LOGIN`, and runs the audit of `m2 audit -changed` on the change in a new git worktree. A change that fails a hard rule gives 400 with the findings, and no pull request opens. A change that passes goes to a new branch `m2-api/METHOD/TAG-ID`, in the fork of the caller or in the repository when the caller is its owner. The answer gives the number and the URL of the pull request, the files, the audit and the cap of the method. A new node gets a provisional id in `nodes/~/`, and the mint workflow gives it a tag when the pull request merges. A binding starts with no review, so it is F1 at most.

Each write method has a cap on its pull requests (spec 11.5). Seven days can have twice the pull requests of the method that were closed or merged in a week, over the last four weeks, and at least 10. A write over the cap gets 429. A write with no token, or with a token that GitHub does not accept, gets 401.

## Ingest

`m2 ingest` gets a snapshot of an external source through its bulk channel, records the snapshot in `manifests/sources.yaml` and writes the records to `build/records/<source>.jsonl`. The import manifest `manifests/ingest/<source>.yaml` names the snapshot, the adapter and the hash of the records. Thus two imports of the same snapshot give the same bytes, and anyone can check it.

```sh
m2 ingest arxiv                        # a new snapshot from the bulk channel of the source
m2 ingest arxiv -snapshot blake3:...   # the recorded snapshot again, from the cache or from its URL
m2 ingest zbmath -url 'https://oai.zbmath.org/v1/?verb=ListRecords&metadataPrefix=oai_zb_preview&set=11'
m2 ingest mathlib                      # the declarations of Mathlib at the commit that lean/lake-manifest.json pins
m2 ingest lmfdb -url 'https://www.lmfdb.org/api/nf_fields/?degree=i2&disc_abs=py{"$lte":1000}'
m2 ingest wikidata -mentions           # also write the mentions of the inventory
```

With `-mentions`, `m2 ingest` also writes one mention for each record to `records/mentions/<source>/`, as `m2 corpus import` does, but it does not group the mentions into rows. A mention has the snapshot, the locator, the native id, the kind, the name, the MSC codes, the class and the importer (corpus K4.1). It holds text only when the class of the record allows it and the licence is not share-alike. A mention of a source with a licence for each item has the licence of the item. Two imports of the same snapshot write the same mentions.

These sources have an adapter now:

| Source | Package | Bulk channel | Licence | What a record keeps |
|---|---|---|---|---|
| `afp` | `ingest/afp` | the git archive of isabelle-prover/mirror-afp-devel, the mirror of the Archive of Formal Proofs, at a pinned commit. Fetch refuses an archive of another commit | BSD-3-Clause or LGPL-2.1, for each entry | each entry: the title, the topics, the date, the names of the authors, and the abstract as plain text for a BSD entry only. Each named lemma, theorem, corollary and proposition, and each definition, function, inductive definition, datatype, typedef, record, locale and class of the theories: the entry, the theory, the locale and the name as the id, the keyword, and the line at the commit as the URL. No text. |
| `arxiv` | `ingest/arxiv` | the metadata plane of tamnd/arxiv, one JSONL file for each month | CC0 metadata; each paper has the licence of its last version | the mathematics papers only: title, authors, categories, MSC codes, DOI and the `ax://` locator. No text. |
| `arxiv_results` | `ingest/arxivresults` | the git archive of tamnd/arxiv, the content plane `content/en/` | each result has the licence of its paper version; the papers with the access class record are skipped | the theorems, lemmas, propositions, corollaries, definitions, conjectures, problems and examples of the mathematics papers: the kind and role of spec 04.2, the heading as the title, the `ax://paper/<id>#<tag>` locator, the proofs, the dependencies inside the paper, the section, the source type and the MSC codes. The text of the statement only when the licence lets math2 store it. |
| `erdosproblems` | `ingest/erdosproblems` | the git archive of teorth/erdosproblems, `data/problems.yaml` | Apache 2.0 | each problem: the number, the status and its date, the formal status, the prize, the OEIS sequences, the tags, the short name and Paul Erdős as the person who posed it (rule G06). No statement text, because the data file has none. |
| `formal_conjectures` | `ingest/formalconjectures` | the git archive of google-deepmind/formal-conjectures, the Lean files under `FormalConjectures/` | Apache 2.0 | each theorem with a category attribute, except the category test: the full Lean name, the docstring as the text, the category, the AMS classes, the Lean statement, the link to a formal proof, the title and the links of the module docstring, and the Erdős, OEIS, MathOverflow and Wikipedia ids that the links name. The URL is the line of the theorem at the commit of the snapshot. |
| `lmfdb` | `ingest/lmfdb` | a URL of the API of the LMFDB, with a table and filters. The default is the elliptic curves over Q of conductor at most 100 | CC BY-SA 4.0 | each object: the path of its page as the id, such as `EllipticCurve/Q/11.a2`, the label as the title and the table as the kind. The adapter knows the tables `ec_curvedata` and `nf_fields`. |
| `mathcomp` | `ingest/mathcomp` | the git archive of math-comp/math-comp at the pinned commit of mathcomp-2.6.0. Fetch refuses an archive of another commit | CeCILL-B: record only | each Lemma, Theorem, Corollary, Fact, Remark and Proposition, and each Definition, Fixpoint, Inductive, Variant, Record and Structure, also in the forms of Hierarchy Builder: the full Rocq name as the id, the keyword, the logical path of the file and the sections. The URL is the line at the commit. No text. |
| `mathlib_100`, `mathlib_1000` | `ingest/curricula` | the raw file `docs/100.yaml` or `docs/1000.yaml` of mathlib4, at a commit | Apache 2.0 | each theorem of the list: the number or the Wikidata key as the id, the title, the declarations, the status, the authors, the date, the other formalizations and the note. The URL is the entry on the page of the list. |
| `mathlib_undergrad`, `mathlib_overview` | `ingest/curricula` | the raw file `docs/undergrad.yaml` or `docs/overview.yaml` of mathlib4, at a commit | Apache 2.0 | each item of the list: the sections and the name as the id, the name as the title, the sections, the place in the list, and the declaration or the link that the list gives. The locator is the file and the line. |
| `mathlib` | `ingest/mathlib` | M2Extract in the Lake project `lean/` of the tree, at the pinned commit of Mathlib | Apache 2.0 | each declaration of Mathlib: kind, module, the hash of the canonical type, the constants that the value uses, the docstring as text and the link to the documentation. |
| `nlab` | `ingest/nlab` | the git archive of ncatlab/nlab-content, the `name` and `content.md` of each page | none stated: record only | each page: the name as the id and the title, the directory of the page as the locator, the names of the pages that it links to and its other names from its `!redirects` lines. The URL is the page on the site. The source of the page gives the fingerprints of rule L02, and the licence gate drops the text. |
| `oeis` | `ingest/oeis` | `https://oeis.org/names.gz`, the names of all sequences | CC BY-SA 4.0 | each sequence: the A-number and the name as the title. No other text. |
| `palomar` | `ingest/palomar` | the listing of the Palomar registry API, `order=registered&trust=all` | CC0 | each registered result at the newest version of its entry: the entry id, the title, the abstract as the text, the authors, the arXiv categories, the MSC classes, the Lean names of the theorems, the repository and commit that the registry checked, the trust level and the time of registration. |
| `planetmath` | `ingest/planetmath` | the git archive of one repository of github.com/planetmath, one for each MSC class. The default is `20_Group_theory_and_generalizations` | CC BY-SA 3.0 | each entry: the canonical name as the id, the type as the kind, the title, the MSC codes, the authors for the attribution, the defined terms, the synonyms, the related entries, the date of the last change and the file. The URL is the page of the entry. The text is the LaTeX of the body, and the inventory keeps no text because the licence is share-alike. A node names an entry in `same_as` under `planetmath`. |
| `proofwiki` | `ingest/proofwiki` | the daily XML dump of ProofWiki, `https://proofwiki.org/xmldump/latest.xml.gz` | CC BY-SA 3.0, elected from CC BY-SA 3.0 or the GFDL | each result, proof, definition and axiom: the full title as the id, the kind from the namespace or the first heading, the title without the namespace, the plain text of the statement for the copy check, the categories as crosswalk data, the page that a proof proves, and the revision, its date and its author. The inventory keeps no text, because the licence is share-alike. |
| `setmm` | `ingest/setmm` | the raw file `set.mm` of metamath/set.mm at a pinned commit. Fetch refuses a URL of another commit | CC0 | each theorem, axiom and definition: the label as the id and the title, the kind, the assertion, the hypotheses, the section, the contributor and the date, and whether new usage is discouraged. The text is the comment of the statement. The URL is the page of the label on the Metamath site. |
| `stacks` | `ingest/stacks` | the git archive of stacks/stacks-project, `tags/tags`, `chapters.tex` and the LaTeX of the chapters | GFDL 1.2 or later: link only | each tag: the kind, the title of the part, chapter, section or named statement, the label, the chapter, the section and the file and line. The URL is the page of the tag. The text of the statement and its proof gives the fingerprints of rule L02, and the licence gate drops the text. |
| `open_logic`, `hott_book`, `lebl_ra`, `lebl_diffyqs`, `lebl_scv`, `napkin`, `levin_dmoi`, `applied_combinatorics`, `ernst_ibl`, `austin_ula`, `active_calculus` | `ingest/textbook` | the git archive of the repository of each book at a pinned commit. Fetch refuses an archive of another commit, because a person reads the licence again before the pin moves (corpus K5.4.4) | CC BY 4.0, CC BY-SA 3.0 or CC BY-SA 4.0, as each book states it. Lebl's books elect CC BY-SA 4.0, Levin is pinned at the 3rd edition, and Napkin gives its text only. Hefferon waits for a person, because its repository names CC BY-SA 2.5 | each definition, statement, conjecture, proof, example and exercise in the LaTeX or PreTeXt source: the path and the label as the id, the kind from the name of the environment, the title, the file and line, the label, the section, the statement that a proof proves and the items that it refers to. The URL is the line at the commit. The text is the source of the statement, and the inventory keeps it only for a book that is not share-alike. |
| `thousand_plus` | `ingest/curricula` | the git archive of 1000-plus/1000-plus.github.io, the files `_thm/<id>.md` | Unlicense | each theorem: the Wikidata item and the letter as the id, the title, the MSC class, the Wikipedia pages, the systems with a formal proof or a formal statement, and the Mathlib declarations. The URL is the row of the theorem in the list on the site. |
| `wikidata` | `ingest/wikidata` | the SPARQL service of Wikidata, one query for each crosswalk property | CC0 | each item with an id in nLab, MathWorld, ProofWiki, the Encyclopedia of Mathematics, the MSC, the OEIS, Mathlib, the LMFDB knowls, the House of Graphs or the Erdős problems: the QID, the English label as the title and the ids in the other systems. |
| `wikipedia` | `ingest/wikipedia` | the multistream XML dump of the English Wikipedia, `https://dumps.wikimedia.org/enwiki/latest/enwiki-latest-pages-articles-multistream.xml.bz2`. The adapter reads the index of the dump and gets with range requests only the streams that have the articles that the tree names, in `same_as`, `close_to` or the mentions, and the targets of their redirects | CC BY-SA 4.0 | each article that the tree names: the title as the id, the permanent link to the revision as the locator, the plain text of the lead and the body for the copy check, the short description, the categories as crosswalk data, the redirects to the article, and the revision, its date and its author. The inventory keeps no text, because the licence is share-alike. |
| `zbmath` | `ingest/zbmath` | the OAI-PMH interface of zbMATH Open, `oai_zb_preview` | CC BY-SA 4.0 | each document: title, authors, MSC codes, Zbl number, DOI, the source as the locator and the ids of the references. No review text. |

The formal-conjectures adapter reads the Lean files as text and does not build them, because the project pins its own Mathlib. It skips comments and strings, follows the namespaces and takes each theorem and lemma that has a category attribute. The snapshot of 25 October 2026 (commit 49ba26b) gave 4,652 statements: 1,602 open research problems, 2,816 solved research problems and 234 textbook results. 2,773 of them name an Erdős problem.

The Wikipedia import of the dump of 1 October 2026 gave 2,163 articles for the 2,408 titles that the tree names, as a snapshot of 45 MB in place of the dump of 27 GB. A title that is a redirect gives the article of its target, and 230 of the articles have such titles. The adapter skips the disambiguation pages and the titles that the dump does not have.

A formal library has the kind `library` in its source document, with an intake class (corpus K6.2): `bind`, `crosswalk`, `link`, `evaluate` or `exclude`, and the reason in `intake_reason`. The field `authored` gives `human`, `ai` or `mixed`, and a Lean library gives its root modules in `modules`. Each mention of a library records the commit of the snapshot as `pin`, the intake class, the field `authored` and the licence of the item. Rule F10 checks the intake class and the mentions, and rule F12 checks `authored`. Rule F11 finds the text of a library with the class `link`, `evaluate` or `exclude`, or of an item whose licence is not open, in a node body. Rule F13 reports a binding to a declaration of a library that AI wrote when its statement uses a definition of the same library and no review of the binding lists that definition in `covers`. The Mathlib and formal-conjectures adapters give `bind` and `human`. The set.mm, MathComp and AFP adapters give `crosswalk` and `human`, because only Lean 4 libraries give bindings in v1, and their records have the kind `statement` or `definition` of the inventory. Each pins one commit, and a new pin waits for a person to read the licence again. At their pins, set.mm gives 49,727 records (48,130 theorems, 1,471 definitions and 126 axioms), each with its comment as the text, under CC0. MathComp gives 19,677 records (15,834 statements and 3,843 definitions) with no text, because corpus K6.6 does not name CeCILL-B for the prose zone. The AFP gives 1,043 entries (1,017 under BSD and 26 under LGPL) and 365,305 declarations (296,118 statements and 69,187 definitions). The adapter skips a theory that does not lex, because no session can load it: at the pin, one theory of Pi_Calculus has a quote with no end.

The Palomar adapter follows the cursor of the listing with one request each second. When the registry changes during a fetch, the API answers 409 Conflict and the adapter starts again from the first page. When the listing abbreviates an entry, the adapter gets the complete record of the entry. The snapshot of 25 October 2026 (revision 517) gave 760 results in 43 seconds. 117 of them name a directory in their repository, and 85 entries have more than one version.

The Stacks adapter reads the LaTeX as text, one line after the other, and joins each tag in `tags/tags` to its label. It does not compute the numbers of the book, such as Lemma 26.4.2, because the numbers come from the LaTeX build and the adapter cannot check them. The snapshot of 25 October 2026 (commit a04446e) gave 21,446 tags in less than 5 seconds: 12,587 lemmas, 3,299 sections, 1,721 definitions, 688 equations, 449 examples, 440 items, 386 exercises, 330 propositions, 214 theorems, 124 situations, 116 chapters, 35 subsections, 9 parts, 1,043 remarks and 5 lists of remarks. The records keep no text, only the fingerprints. With these records, rule L02 passes on the nodes of math2 and fails on a node that copies the statement of tag 01IS.

The nLab states no licence, so the nLab adapter gives records with no text: names, locators and the link graph (corpus K5.1). The mention of a page of the nLab has the same name as the nLab id of Wikidata, so the inventory joins the two. `m2 ingest -mentions` and `inventory.WriteMentions` refuse a mention of the record or the gfdl class that holds text, and rule L01 reports one in the tree. The Stacks Project tags give mentions of the gfdl class in the same way. Kerodon, the Open Problem Garden and the Manifold Atlas have no adapter yet, because they have no bulk channel, or their licence is not settled (corpus K5.3).

The curricula adapters read the lists of corpus K5.8 as data, not as text (K10.3). Each item that a list names is a mention, with the list as its source, so the inventory knows which items the curricula ask for. A mention of a list keeps the Mathlib declarations and the Wikidata item of the item as its ids, so the inventory joins it to the mention of the declaration and to the Wikidata item. The 1000+ theorems project gives a Wikipedia page only when the link names the whole page, because a link to a section names a page about something larger. At the commit 2b2e967 of mathlib4 and the commit 8e04b97 of the 1000+ theorems project, the adapters gave 566 items of undergraduate mathematics (396 with a declaration), 512 items of the overview, 100 theorems (78 formalized and 1 with only the statement) and 1,199 entries of the 1000+ list in Mathlib, and 1,200 theorems of the 1000+ project (227 with a formal proof in some system). Wiedijk's own list, the syllabi and the tables of contents of textbooks have no adapter yet, because they have no licence and the corpus takes only facts from them.

`m2 corpus import -plan corpus/plan.yaml` is the command of the import pipeline (corpus K12.1). The plan lists the importers in order. Each import has a `source` with an adapter, and it can pin the bulk channel with a `url`, for example a raw file at a commit, or pin a `snapshot` of the catalogue with its hash. The command runs each importer as `m2 ingest` does, one after the other, writes the mentions, groups them into rows and prints a summary: for each source the snapshot, the records, the mentions and the new mentions, then the new rows and the open count before and after the run. `-summary FILE` also writes the summary in Markdown, for the body of the pull request. Each adapter keeps the rate limit of its source, and each request has the User-Agent of the toolchain. When the sources give the same bytes again, the run uses the snapshot of the catalogue again, and the tree does not change.

`m2 corpus import` and `m2 corpus cluster` group the mentions into rows with the first two passes of corpus K4.3. Pass 1 joins two mentions with the same identifier, or with identifiers that a crosswalk links. The hash pass joins two mentions of one kind and one block with the same content hash, which is one item that two papers copy (corpus arxiv AX8.2). The block is the MSC codes at two digits (AX8.1). An arXiv object with a body shorter than 40 characters, with no labels and no space, gets no hash, because many papers have the same short body. Pass 2 joins two statements of a formal library with the same canonical type hash, because Lean then proves each from the other with the identity. Pass 2 never joins definitions. A row that a pass merged into another row names the pass as its rule, `identifiers`, `hash` or `equivalence`. A new row of arXiv mentions gets the outcome `record` by the rule `research-default` or `research-exercise`, or `out_of_scope` by the rule `research-context` (AX8.6). A row of results from arXiv results, Palomar or TheoremSearch gets `record` by the rule `research-results`, and a row of papers from zbMATH Open or the arXiv metadata gets `record` by the rule `research-paper` (spec 9.8). When a live node has an aligned edge to a mention of a research row, or to its locator, the row gets the outcome `aligned` by the rule `research-aligned`, with the tags of the nodes as its target. When the edges are gone, the row gets its record rule again. `m2 corpus cluster` reads the aligned edges from the nodes of the tree, and `m2 corpus status` gives the open count of each layer, because corpus K14.7 asks that the open count of the research layer goes down each month. A row of the kind unknown stays open until the pass classify gives its kind. Then the formal rules of corpus K4.5 apply to each row of a formal declaration that is open or `formal_only`. A row stays open as a node candidate when one of four tests selects it: `formal-human` (a mention from a human source is in the row), `formal-main` (a list of the library such as the 100 theorems names it, the module documentation names it as a main result, or it is a statement of Formal Conjectures), `formal-used` (a node uses it in a uses edge that a person wrote) or `formal-asked` (a person asks for it). The node outcome of the row then names the test. Each other row gets `formal_only` by the rule `formal-only`, and a `formal_only` row opens again when a test selects it later. An outcome that a person gave does not change. The command does not read the module documentation, the uses edges or the requests yet, so only the mentions of a row select it now. The queue `align` of `m2 corpus queue` lists the nodes of the areas that a project or a campaign asks for, when the area has research results that are open or records. `-asked 11,05` names the areas by their MSC codes at two digits. With no area asked, the queue is empty, because spec 18.5 builds the research core of an area only on demand. `m2 corpus cluster` reads only the mentions, so two runs on the same mentions give the same rows. Equivalences that need a real Lean proof are not in pass 2 yet.

```
m2 corpus cluster                    # group the mentions into rows again
m2 corpus cluster -source arxiv      # also the name pass and reports/inventory-arxiv.md
m2 corpus gold                       # the precision and the recall of each pass
m2 corpus gold -write                # also write reports/inventory/clustering.json
```

With `-source arxiv`, `m2 corpus cluster` also runs the name pass of AX8.2. An arXiv mention whose name is the only name of a row of another source in its block, such as "Banach fixed point theorem", gets a candidate in `reports/inventory/arxiv-names.jsonl`. The pass does not join the two, because the name of a variant can be the name of the textbook result, and the dedup judge must decide. `-msc CODE` limits the name pass to one block. The command also writes `reports/inventory-arxiv.md` with the counts of AX8.7 for each MSC code at two digits and each year of submission.

`m2 corpus plan understand -tier T1` estimates the cost of one tier of the passes of AX7.8 on the arXiv mentions, with the numbers of AX13.3. It calls no model. T1 counts each paper, T2 each paper with an `open` or `share-alike-nc` version, T3 each statement and definition whose row has a spread of 2 or more or holds more than one paper, and T4 each proof of a statement of T3. `-msc CODE` counts only one area. The high end of the cost doubles the count for retries and escalation. No pass runs without a budget that a person approved.

The gold set of the clustering is in `records/gold/clustering.jsonl`. Each line is one decision of a curator: two mentions `a` and `b`, whether they are one item (`same`), the layer of the pair, the curator (`by`) and the date (`on`). `m2 corpus gold` runs the passes on the mentions alone and gives the precision and the recall of each pass, for each layer and for all layers. The passes are cumulative, so the score of `hash` is the score of the identifiers and the hashes together, and the score of `equivalence` is the score of all three. A false merge counts against the precision, and a false split counts against the recall. A pair with a mention that is not in the inventory is missing, and it is not in the score.

`m2 corpus recall` estimates the recall of the inventory for each MSC code at two digits (corpus K4.7). Two sources of an area that are independent give a capture-recapture estimate: if the sources have a and b rows of the area, and m rows are in both, the area has about (a+1)(b+1)/(m+1) - 1 items (the Chapman form). The recall is the count of rows of the area divided by that estimate, with a 95% interval. The sources of one family, for example mathlib and its lists, are not independent, so the command uses one pair of families. The default pair is the pair with the most rows in both. Each pair is in the JSON output. Rows that are merged, split or out of scope do not count. `-write` writes the estimates to `reports/inventory/recall.json`. The rule I06 gives a finding for each area with an estimated recall below 90%. The rule is soft, because the estimate is only as good as the independence of the two sources.

`m2 corpus index` is the index term check of corpus K4.7. The index of a standard textbook lists the terms that the book defines, and each term must match a row. The terms of a book are in `records/index/<book>.jsonl`, one term and its locator on each line, for example `{"term":"group, abelian","locator":"p. 3"}`. A term matches a row when its name, or the name or an alias of a mention of the row, is the same name: "group, abelian" and "group!abelian" are "abelian group", and a part in parentheses can be left out. Rows that are merged, split or out of scope do not count. The command prints the match rate of each book, and `-missing` prints the terms with no row, which are candidate gaps. `-write` writes the rates to `reports/inventory/index.json`.

`m2 corpus report` measures the stages of the pilot (corpus K12.5 to K12.7). The stage of a run comes from the goal of its job, for example "node reaches W2" gives the stage usable, and else from its role. For each stage, the report gives the jobs, the runs, the accepted runs, the spend, the cost for each job and for each accepted run, and the planned cost of K12.6 next to them. A stage has measured numbers after 1,000 jobs. A review act is a fidelity review, a confirmation by a person in a provenance log, or a decision of a person on an inventory row. A fidelity review records its time when the reviewer gives `m2 review -minutes N`. The other acts record no time, so their hours use the minutes of K12.5 and the report says that they are estimates. `-from` and `-to` give the window, and `-json` prints JSON. The command writes no file and calls no model.

The arxiv_results adapter does not extract results again. The extractor of tamnd/arxiv marks each object of a paper with an attribute block, such as `{#thm-1 .statement tag=3KH2 env=theorem}`, and the adapter reads these blocks. The extent of a result runs to the next block that is not an equation, as in the objects package of tamnd/arxiv-reader. A proof belongs to the result that its heading links to, or to the last result before it. The dependencies are the links to other results, and to the equations in them, from the statement and its proofs. The adapter does not read references in plain words, and it does not unfold results, because unfolding needs a model. The main branch of tamnd/arxiv at commit c818ec0, of 14 September 2026, has one paper, 2312.00752. It is in cs.LG, so it gives no results yet.

When the bulk channel is a git repository, the snapshot is the git archive that GitHub gives for the branch, and the catalogue records the commit from the archive. An adapter uses `ingest.FetchArchive` and `ingest.WalkArchive` for this. A snapshot that has more than one file, such as an OAI-PMH harvest, is the files one after the other, compressed with gzip. The zbMATH adapter fails when the number of records is not the size of the list that zbMATH Open gives. The full zbMATH harvest has more than four million documents, so give a set to harvest one MSC class.

The Mathlib adapter downloads nothing. It runs M2Extract on the .olean files of the tree, so run `m2 build` first. The catalogue records the commit of Mathlib with the snapshot. When the tree moves to a new tag of Mathlib, run `m2 ingest mathlib` again to get the records of the new tag. At the commit 56a0058 of Mathlib, for Lean 4.35.0, the adapter gives 358,102 records in less than 5 minutes on a laptop. Of these, 90,326 have a docstring. The snapshot is 50 MB.

The Wikidata adapter gives the crosswalk that Wikidata has now. On 2026-10-25 it gave 12,302 items in less than 30 seconds. MathWorld has the most ids (6,840) and nLab is next (4,348). Only 115 items have a Mathlib id. The adapter does not ask for the items with a defining formula, because most of them are single numbers and the query does not end in the 60 seconds that the service gives it.

The LMFDB adapter gets the labels one page of 100 after the other, with a pause of 2 seconds between the pages. The LMFDB asks automated clients to use its bulk downloads for large slices, and it answers with a captcha when it gets too many requests. Thus give a filter that selects a small slice. When the answer is not JSON, the adapter stops with an error.

Each source has an access class: `open`, `share-alike-nc`, `verbatim`, `gfdl` or `record` (spec 09.3). The licence gate stores the text of an item only when its class is `open`. For the other classes it keeps the record, the locator and the fingerprints of the text. The fingerprints cannot give the text back. The audit uses them to find copied text:

| Rule | What it checks |
|---|---|
| L01 | Each source names a licence and a class that agrees with it. Each import manifest names a recorded snapshot. The records agree with the manifest and store only the text that their class allows. No mention holds text of a class that does not allow it, such as record or gfdl, or text under a share-alike licence. |
| L02 | No node body copies text from a source that is not `open`. |
| L03 | Each share-alike source has an attribution line, and a node that uses its text names the item in `sources` or `same_as`. |
| L04 | A node body that is similar to an item that it cites. A person checks it. |
| L05 | Each body that an agent wrote has a copy check result in the summary of its run, and no match of the result is with a source that is not `open`. L05 is soft until the calibration of corpus K9.4. |
| L06 | Each item of a source with `per_item_licence: true` records its own licence. |
| L08 | Each source document has the fields of corpus K5.2: `licence`, `class`, `licence_read`, `authored`, `channel`, a snapshot, `attribution` and `approved_by`. Each mention names a source with a complete document. |
| L09 | A source with a choice of licences, such as `CC-BY-SA-4.0 OR CC-BY-NC-SA-4.0`, has an `election` that is one of the choices, and the elected licence gives the class. |
| L10 | A source of the `open` class with the licence `LicenseRef-public-domain` has `public_domain` with `us: true`, `eu: true` and a basis. |
| L11 | A mention with text comes from a snapshot whose licence was read on or before the date of the snapshot. A snapshot can give its own `licence_read` date. |
| L12 | A source whose licence was last read more than twelve months before the HEAD commit. Read the licence again at the source. |

L02 to L04 need the records in `build/records/`. When they do not exist, the rules skip. L05 needs the run summaries in `build/runs/`. The package `copycheck` is the copy check of corpus K9.4. It compares a body with a reference corpus, not only with the sources of the node: stage 1 finds the exact runs of 50 normalized characters or more, and stage 2 finds the near copies of each paragraph with MinHash on word 5-grams. The check masks each formula and each standard phrase of a list that a curator keeps. Each source has an adapter in the package `ingest/<source>`. An adapter whose bulk channel has more than one file also implements `ingest.Fetcher`. To add a source, write the adapter, register it with `ingest.Register`, import its package in `cmd/m2`, and write the source document in `manifests/sources.yaml`. Read the licence at the source itself, not at a catalogue, and record where, when and by whom in `licence_read`. A maintainer approves the entry and names themself in `approved_by`.

### The items of arXiv papers

The arXiv importer (corpus arxiv AX05) does not use `build/records/`. It reads the analysis plane of a checkout of tamnd/arxiv and writes the mentions directly:

```
$ m2 corpus import -source arxiv -corpus ../arxiv
arxiv: 14519 mentions from 406 papers, 4745 attached, 0 retired, 41 restated prints, 0 objects with no tag
rows: 9774, new 9774, with new mentions 0, merged 0, waiting for a source 0, joined by equal formal statements 0, attached with no row 522, changed by the formal rules 0
```

The mention id is `ax://paper/<id>#<tag>`, with the tag of the object in the analysis plane. A statement, a definition, an example, a counterexample, a conjecture, a problem, an exercise and an object of the kind unknown each get a mention. A remark, a hypothesis and the other objects do not. A proof is attached to what it proves, and a claim inside a proof is attached to the proof. An attached mention is not a row. It is in the field `proofs` or `local` of the row of its target. A proof with no target has `attached_to: unknown`. A restated item is one mention, and the other prints are in its field `also`. A mention whose object is gone in the latest version gets `retired`.

The importer copies no text of a paper. The field `text_ref` names the object record, and `hash` is its content hash. A version of the class `record` gives no name and no `text_ref`. Run `ax tags analysis -all` on the checkout first, because an object with no tag gets no mention.

Two rules check the arXiv mentions. D05 reads the checkout again and checks that each object of an item kind in the latest version of each paper has a mention, that each proof has `attached_to`, and that each attached mention names a mention that exists. It needs the checkout, so it skips when `-arxiv` is not given. D06 checks that a mention of the class `record` holds no text: it has no name, text, `text_ref`, aliases or cites, its locators have the form `<id>v<n>#<local>`, its label, number, environment and head are short tokens, its hash is a sha256 hash, and the locator of each cited result is a short reference. Both rules are hard.

```
$ m2 audit -rule D05,D06 -arxiv ../arxiv
```

## The Wikipedia corpus

The Wikipedia corpus is the repository tamnd/wikipedia-math. It has the manifests in `manifests/wikipedia/` (the template table, the heading table and the waivers), the content plane in `content/wikitext/` (one wikitext file for each article, with a header that gives the revision, the history link and the licence) and the analysis plane in `analysis/` (the objects and the census of each revision). The `wpcorpus` package reads and writes these files, and it extracts the objects with the parser and the math converter of tamnd/wikipedia-cli.

`m2 wp extract` fetches the current revision of each article that it names, writes the content file, and writes the objects file and the census file. `-list FILE` gives one title on each line. With no title, the command extracts again each page of the content plane, and it does not use the network. `-page ID` and `-type T` select the pages. The census counts each cue of an item: an item template, an item heading, a heading that names a result, an inline head, a proof block, a glossary entry, a list entry and a table row. Each cue must give an object, or match a waiver with a reason.

```
$ m2 wp extract -corpus ../wikipedia-math "Sylow theorems" "Coset"
$ m2 wp extract -corpus ../wikipedia-math -list pilot.txt
$ m2 wp extract -corpus ../wikipedia-math            # extract the content plane again
```

The Y rules check the corpus. They read the checkout that `-wikipedia` names, and they skip when it is not given. Y02 checks that each revision of the content plane has a census record that names the extractor version and the revision, and an objects file. Y03 checks that each cue of each census has an object, or a waiver of `waivers.yaml` with a reason. These two rules are hard. Y08 lists the templates that the template table does not map, Y09 lists the headings in 5 or more articles that the heading table does not map, and Y10 lists the formulas that `wiki/texvc` does not convert. These three rules are soft. The other Y rules come in the later milestones of the Wikipedia roadmap. `-group Y` runs only the Y rules, and `-hard` runs only the hard rules.

```
$ m2 audit -wikipedia ../wikipedia-math -group Y -hard
```

## Alignment

`m2 align <source>` compares each node with the records of a source and proposes edges. Run `m2 ingest <source>` first. The aligner does not change a node. It writes the proposals to `queues/align/<system>.jsonl`, and a person decides on each one.

```sh
m2 ingest wikidata
m2 align wikidata                     # write the proposals to queues/align/wikidata.jsonl
m2 align wikidata -n                  # show the proposals and do not write them
m2 align queue                        # all proposals, the node with the most dependents first
m2 align accept -by github:octocat 3f9a0c12b7de
m2 align accept -by github:octocat -note "The record is the finite case only." 8e01d2c4a9b3
m2 align reject -by github:octocat 5c7e2b90d1fa
m2 align confirm -by orcid:0000-0002-1825-0097 3C53TD wikidata
```

The aligner gives a score from 0 to 1 to each candidate record. It uses four channels:

| Channel | What it compares |
|---|---|
| id | The node and the record have the same id in a third system, or the record names the Mathlib declaration of the node. The Wikidata records connect the systems, thus a node with an nLab id finds its OEIS sequence through the Wikidata item that has both ids. When only one record matches by id, its score is 1. |
| name | The names and the aliases, in lowercase and without accents. An equal name gives 1. Otherwise the score is the share of the rare words that the two names have in common. |
| formula | The formulas in `$...$`, without space and braces. This channel counts only when both texts have formulas. |
| embedding | The cosine of the vectors of the two texts. The default embedder `hash/1` hashes the words, the word pairs and the character trigrams. It is in the binary, and it calls no service. A program can give a different `align.Embedder`. |

A node gets a `same_as` proposal when the best record has a score of at least 0.9 (`-threshold`), the next record is at least 0.05 lower (`-margin`), and the node has no value for the system yet. Each other record with a score of at least 0.75 (`-close`) gives a `close_to` proposal. When two nodes match one record, the aligner proposes `close_to` for both, and it proposes in `queues/dedup.jsonl` that the two nodes are one node. A record of arXiv or zbMATH Open is a paper, and it gives an `aligned` proposal with the locator of the paper. The aligner skips retired nodes, the edges that the nodes have already and the edges that a person rejected.

`m2 align accept` writes the edge into the node in canonical form, and it removes the proposal from the queue. A `close_to` edge needs a note that tells how the node and the record differ. An `aligned` edge needs `-match`: `exact`, `stronger`, `weaker`, `variant` or `partial`. An accepted duplicate does not change the nodes, because a curator merges or retires the nodes in a separate pull request. Only a person decides. The `-by` flag takes `orcid:` and an ORCID iD, or `github:` and a GitHub login, and the command refuses an agent or a bot. Each decision is one line of `registers/crosswalk.reg`, with the hash of the record at the time of the decision. `m2 align confirm` records that a person checked a `same_as` value that was in the node already.

| Rule | What it checks |
|---|---|
| A02 | A `same_as` value that an agent wrote, as the provenance log in `prov/` shows, has the decision of a person within 90 days. |
| A03 | Each `same_as` value is still in the latest records of its system. When a person decided on the value, the record still has the hash that it had at the time of the decision. The rule reads `build/records/`, and it does not use the network. |
| A04 | No duplicate proposal in `queues/dedup.jsonl` waits for more than 30 days. |

On the nodes of math2 and the Wikidata records of 25 October 2026, the aligner proposes two `same_as` edges: Q505798 for Lagrange's theorem, through its Mathlib declaration, and Q83478 for the node Group. Both are correct. The other sources give no proposals for the seed nodes.

## Provenance

Each node has a provenance log in `prov/<shard>/<tag>.jsonl` (spec 04.8). Each line of the log is one change to one field: the field, the operation, the agent or the person, and the time. A line of an agent also names its run, and it can name the model and the prompt version. A person is `orcid:` and an ORCID iD, or `github:` and a GitHub login. An agent is a role at a version, for example `m2-agent/formalize@0.9.1`. The log only grows. The fields are the top-level keys of the front matter, `body` for the text before the first section, `body.<section>` for each section, `body.explanations.L1` to `L4` for each explanation, and `*` for the whole node.

```sh
m2 prov record -by github:octocat     # write a line for each field that changed since HEAD
m2 prov record -agent m2-agent/explain@1.2.0 -run r-2026-10-26-1 -model claude-opus-5-5
m2 prov show 3C53TD                   # the lines of a node, and the fields in quarantine
m2 prov credit -from 2027-01-01 -to 2027-12-31 orcid:0000-0002-1825-0097
m2 prov quarantine -model claude-opus-5-5 -prompt explain/7
m2 prov quarantine -model claude-opus-5-5 -prompt explain/7 -by github:octocat
m2 prov confirm -by github:octocat 3C53TD body.explanations.L1
m2 prov disclose 3C53TD               # the disclosure paragraph of a node
m2 prov disclose -yaml 3C53TD > formalization.yaml
```

`m2 prov record` compares the node files with a revision, `HEAD` by default, and it skips a field that has a new line since that revision already. `m2 prov credit` lists the changes of a person in the CRediT roles, with the changes of the agent runs that worked on behalf of the person. `m2 prov quarantine` lists the fields that an agent wrote with a model and a prompt version, and that no person changed or confirmed since (spec 06.5). With `-by`, it puts the fields in quarantine. A person clears a field when they change it or run `m2 prov confirm`. `m2 align accept` and `m2 align confirm` write the lines of their decisions. `m2 mint` moves the log of a provisional id to its tag.

The site shows the history of each node from its log. It names the fields that an agent wrote and that no person checked yet. It hides each field in quarantine on the page, in the Markdown copy and in the JSON view, and the headings stay with a notice.

The summary of each agent run is in `build/runs/<id>.json`, with the agent, the prompt version, the models, the tools, the images, the person or schedule that the run worked on behalf of, and the outcome. It can also give the nodes that the job named, the wall time in seconds and the spend in US dollars.

`m2 prov disclose` writes the disclosure of a node from its log, the run summaries and the last build (spec 16.5). The paragraph names the people, and for each agent run it gives the model, the prompt, the person or schedule, the outcome, the time and the spend. It also counts the runs on the node that did not succeed, gives the kind of agent contribution, names the fields that no person checked and the fields in quarantine, and gives the evidence level, the fidelity level and the reviewers of each binding. When the record does not have a value, the paragraph says so. With `-yaml`, the command writes the `formalization.yaml` of the node in the form v0.4 of the [Mathlib Initiative](https://github.com/mathlib-initiative/formalization.yaml), so that a person can register the binding in the Palomar registry. The file makes the node a source, makes the repository a thin wrapper of Mathlib at its commit when each binding is in Mathlib, gives one automation method for the people and one for each agent run, and maps the node to each declaration in `alignment`. A node with no formal binding has no such file. A project disclosure needs the project data of the workspace, so it comes later.

| Rule | What it checks |
|---|---|
| P01 | Each change to a node in the pull request has a line in its log, each new line is valid, and no line of the log changed. |
| P02 | Each line of an agent names a run that exists, with the same agent and a model of the run. |
| P03 | Each run worked on behalf of a person, a schedule or the project. |
| P04 | An explanation that the node marks as written by a person was not written last by an agent. |
| P05 | Each source that an agent added resolves to a record in `build/records/`, and the title, the authors and the year of the record match what the run claimed. A source that resolves to no record is fabricated. |
| P06 | A source has the flag `locator_unconfirmed` for more than 180 days, to the date of the HEAD commit. |

P02, P03 and P05 need the run summaries in `build/runs/`, and P05 also needs the records. When they do not exist, the rules skip. The package `sources` is the source confirmation of corpus K9.5. The flag `locator_unconfirmed` of a source gives the date when the source got it, and only a person removes it.

## Agent runs

`m2 agent run` does one run of the formalize, the prove (spec 12.4), the explore (spec 12.5), the explain (spec 12.6), the curate, the align, the bump or the review-assist role (spec 12.1), or the write, informalize and exercise roles (corpus K7.3), on one target:

```sh
m2 agent run -target 7K3QX2 -on-behalf-of github:octocat -budget 2 -sandbox ghcr.io/tamnd/m2-lean@sha256:... prove
```

The command needs the last build of the tree for the read API, and `ANTHROPIC_API_KEY` for the Claude API. To use the command-line clients of your subscriptions instead, set `M2_LLM_COMMAND` to the clients in order of preference, for example `codex,claude` (package `llm/command`). A client gets the request as one prompt and answers with a JSON object of text and tool calls. It runs in an empty directory with no tools of its own and without the API keys of the environment. When a client hits its usage limit, the next client takes the request. It makes a new git work tree at `HEAD` in the temporary directory, or in `-work`. It links `lean/.lake/packages` of the tree into the work tree, so that the run uses the Mathlib build that is there. The role, the prompts and the model ladder come from `prompts/<role>/`, and `-rung` selects the rung of the ladder. A job file in YAML (spec 11.4) can give the job with `-job`. The person, the project or the schedule on whose behalf the run works comes from `-on-behalf-of` or `$M2_ON_BEHALF_OF`.

The model sees only the tools of the role. It cannot use a shell. The run stops when the goal of the role holds, when the budget is used up, when the model makes no progress, or when it reports that it is stuck. For prove, a statement with no proof is a definition `def S : Prop`. The run must write a theorem of type `S`, move the attribute `@[m2 "TAG"]` to it, and change the `decl` of the binding. The goal checks that the theorem has no `sorry`, uses only the standard axioms, uses `native_decide` only with `-native-decide`, and leaves `S` and its closure unchanged.

A Lean file can run programs when Lean elaborates it, and the model writes the Lean files of the run. Thus the command runs lake, Lean and M2Extract in the sandbox (spec 11.7) when `-sandbox` or `$M2_LEAN_IMAGE` gives an image by digest. Each call is a container with no network, 4 CPUs and 8 GiB of memory. The directory `lean/` of the work tree is writable at `/out`, and the Lake packages of the tree are read-only at `/work`. The image must have lake, the Lean toolchain of `lean/lean-toolchain` and M2Extract at `/usr/local/bin/m2extract`. The command refuses to run with no image. `-no-sandbox` runs the Lean programs on the host, and you use it only for a run that you watch. The image `m2-lean` is not built yet (see `images/README.md`).

An explain run writes no Lean, thus it needs no sandbox and no Lake packages:

```sh
m2 agent run -target 7K3QX2 -on-behalf-of github:octocat -budget 1 explain
```

The run writes the explanations of the target under headings such as `### L2 agent`, and it can add nodes of kind `example`. It cannot change an explanation that a person wrote or confirmed. Each claim must cite a node with a link `[[text|TAG]]`. An L1 explanation can link only to the target, to the nodes that the run adds and to the nodes of the year 1 courses in `topics/curricula/`. An L2 explanation can also use the year 2 courses. Then the model of the rung labels each sentence with the cited nodes. The run continues while a sentence is unsupported, and it fails when a sentence is contradicted.

A curate run also writes no Lean. Its target is a node, or a record of `m2 ingest` in the form `source:id`:

```sh
m2 agent run -target arxiv_results:2312.00752#3KH2 -on-behalf-of github:octocat -budget 1 curate
```

For a record, the run reads the record from `build/records/` of the tree with the tool `record`, and it adds a statement or a concept node in `nodes/~/` in its own words. The node must pass `m2 check`, name the locator of the record in its `sources`, and not copy the text of the record. Then the model of the rung judges whether the node states the same result as the record, and the run continues until it does. For a node, the run can change the fields `uses`, `about`, `msc`, `name` and `aliases`, and propose a duplicate in `queues/dedup.jsonl` with the tool `propose_same`. A person merges the nodes.

An align run writes no Lean either. Its target is a node:

```sh
m2 agent run -target 3C53TD -on-behalf-of github:octocat -budget 1 align
```

The run reads the records of the other systems with the tool `record`, and it proposes the edges `same_as`, `close_to` and `aligned` with the tool `propose_edge`. The tool writes each proposal into `queues/align/<system>.jsonl`, by `m2-agent/align@<version>`, and a person accepts or rejects it with `m2 align accept` or `m2 align reject`. The tool refuses an edge that a person rejected, and only a paper of arXiv or zbMATH Open gets `aligned`. The model of the rung judges each new proposal: `same_as` needs the verdict same, and the other edges need a verdict that is not different. A node meets Mathlib through its formal binding and not through `same_as`. Thus the run can also append a Mathlib binding to the target, but only when the type hash of the declaration in `build/records/mathlib.jsonl` is the type hash of a Math2 binding of the node in the last build, with the same match. `align.AutoMerge` reports such a change, which is the only change of align that can merge with no review.

A bump run comes after `m2 bump` (see "The Mathlib bump"). Its target is a node with a Lean binding that does not resolve or is deprecated with the new pins:

```sh
m2 agent run -target 3C53TD -on-behalf-of github:octocat -budget 1 bump
```

The run finds the declaration that states the same result, and it changes `formal[].decl` of the binding with the tool `rebind`. It writes no Lean and changes no other field. The goal holds when each changed binding was broken before the run, a new declaration has the kind and the type hash of an old declaration that resolves, and each Lean binding of the node resolves and is not deprecated. A rebind to another type hash needs `-type-change`. The Lean environment of the run imports Mathlib and Math2, because a binding can name any declaration of Mathlib. `bump.AutoMerge` reports a run whose rebinds all take the replacement that the deprecation names and keep the type hash, and such a run can merge with no review. A person reviews each other rebind and records it in `registers/bindings.reg`, because no role writes a register.

A review-assist run helps the person who reviews a binding (spec 10.6). It writes no Lean, and it needs the last build of the tree:

```sh
m2 agent run -target 3C53TD -on-behalf-of github:octocat -budget 0.5 review-assist
```

The run reads the review packet of the node, as `m2 review -blind` shows it, and acts as the critic of spec 10.4. It writes the packet and its concerns to `reports/review/<tag>.md` with the tool `write_packet`. Each concern names a binding of the packet, a class of spec 10.5 and a reason, for example "`Math2.Algebra.lagrange` M-junk: Nat.card G is 0 when G is infinite". The packet has at most 10 concerns, and it has no reviews, thus a blind re-review can use it. The run writes no other file. It never gives a verdict and never signs, and the person reviews with `m2 review`.

A write run writes the text of a node for the card of its kind (corpus K6). It writes no Lean:

```sh
m2 agent run -target 3C53TD -goal "the W2 sections" -on-behalf-of github:octocat -budget 1 write
```

The level is the writing level W1, W2 or W3 that the goal names. Without one, it is the lowest level at which the node lacks a section, a use, an example or a proof. The brief lists the items of the card that are missing at that level. The run can change the sections of the target other than Explanations and History, its notation and its uses (it only adds), and it can add example and proof nodes of the target in `nodes/~/`. It changes a section that is there only when the provenance log shows that write wrote it and no person confirmed it. The goal holds when `m2 check` finds no problem in the changed nodes, the card of the target has no missing item at the level, each Proof section is in numbered steps, and each Definition section names its term in bold. At W3, the house style must find no new problem in the text. The copy check needs a reference corpus, thus it runs in CI and not in the run.

An informalize run writes the informal text of a Mathlib declaration. Its target is the declaration in the form `mathlib:DECL`:

```sh
m2 agent run -target mathlib:Subgroup.card_subgroup_dvd_card -on-behalf-of github:octocat -budget 0.5 informalize
```

The brief gives the type of the declaration from M2Extract, its docstring from the record plane, the nodes of the constants in its type, and the conventions of `notation/conventions.yaml` for these nodes. A theorem gets a statement node and a definition or a structure gets a definition node, with the binding `{system: lean4, library: mathlib, decl: DECL, match: exact}`. A definition can also add the concept node that it defines. When a node binds the declaration already, the run changes only its sections, its name, its aliases, its notation and its uses (it only adds), and only the sections that informalize wrote. The goal holds when `m2 check` finds no problem, one node binds the declaration, the text does not use the Lean name, each constant of the type that has a node is in `uses`, a Definition section names its term in bold, and the node names no option of a convention other than its default, because Mathlib uses the default of math2. The F1 battery and the faithfulness check run in the build, not in the run.

An exercise run writes exercises for a node (corpus K8.7). It writes no Lean:

```sh
m2 agent run -target 3C53TD -goal "three exercises" -on-behalf-of github:octocat -budget 0.5 exercise
```

The number of exercises is the first number of the goal, and it is 1 when the goal has none. The role writes an exercise in `nodes/~/` in the form of the schema of the tree. In a tree at node.v2, or in a tree with no schema file, an exercise is a node of the kind `exercise` with the fields `about`, `difficulty`, `math_group` and `check` and the sections Exercise, Hint, Answer and Solution (corpus K8.7). In a tree that has `schema/node.v1.json`, an exercise is an example node whose Remarks section opens with the line of the role, for example `Role: exercise. About: 3C53TD. Difficulty: 2. Group: B. Check: exact.`, and has the headings Hint, Answer and Solution (corpus K2.3). The Example section is the task, and `m2 migrate node.v2` later moves the exercise to the kind `exercise`. The brief gives the form of the tree, and the goal accepts both forms. The group is A, B or C from the MATH taxonomy, and the check is exact, numeric, choice, cas or none. The run can change only the sections, the name, the aliases and the uses of an exercise that it wrote, and also `about`, `difficulty`, `math_group` and `check` of an exercise of the kind `exercise`. It does not write the field `external`. The goal holds when `m2 check` and the house style find no problem, each exercise is about the target and has it in its uses, each one has a task, an answer unless a person checks it and a solution in numbered steps, and the run added the exercises that the job asks for. Then a model of another family solves each task from the task alone. An exercise whose answer the solver does not reach, or reaches in another form, goes to a person in `queues/exercises.jsonl`. The model register has no model of another family yet, thus each exercise goes to a person. The check lean comes later, because the role writes no Lean yet.

An explore run works for a project that a person defined, thus `-on-behalf-of` must name a project (spec 12.5, rule 4). It writes no Lean, and it runs its programs with `m2 compute` in the Docker sandbox:

```sh
m2 agent run -target 4N9QX2 -goal "test the claim for small n" -on-behalf-of project:fermat -budget 3 explore
```

The run writes a program in `compute/<id>/` and a computation node for it in `nodes/~/`, and it runs the program with the tool `run_computation`. The images are the images of `compute/images.yaml` for the architecture of the machine. The runs go to the directory next to the work tree, with the suffix `.compute`, and their CPU seconds count in the budget of the run. Then the run files a conjecture or a counterexample in `nodes/~/` that cites the computation. The goal holds when each new computation verifies: it runs again, its output hash is the hash in the node, and the checker of its certificate accepts it. A counterexample refutes its target through the computation, and a conjecture links to it. An output that calls itself new, for example with the words "a new result" or "for the first time", fails the goal. The tool `lookup` runs the novelty check of spec 17.5 on a claim before the run files it. The goal runs the check again on each output, and the run record keeps the result in `novelty`. The searches are the graph, from the search index of the last build, and the record plane. `-web` also searches zbMATH Open, arXiv, MathOverflow and the OEIS, with the User-Agent `m2/<version> (+https://github.com/tamnd/math2-agent)`. The verdict is never "new". A person reads the matches.

The full run record goes to `records/<id>.json` and its summary to `runs/<id>.json` in `-store`, which is `build/` by default. Thus the rules P02 and P03 read the summary. The summary has the spend and the budget of the run, for rule X02. The rules X01 to X04 check each agent pull request: its changes are in the field permissions of the role (X01), its runs stayed in their budgets (X02), and no agent wrote a status field or a review (X04). X03 reports a role and a model whose runs are seldom accepted. The summary also has the results of the gates of corpus K9.2 that the run ran before the pull request: the copy check of each body (`copy`, rule L05), the sources that the run proposed with their title, authors and year (`sources`, rule P05), and the sanity checks of each statement and proof (`sanity`). X05 checks that each statement and proof that an agent wrote has a result of each sanity check of corpus K9.3: pass, fail or n/a. X07 reports a theorem that a body of an agent names, such as "the theorem of X and Y", when no node has the name and no source of the node names it. X06 reports a stratum of the sample audit whose error rate is near its threshold or above it. The package `sample` draws the sample audits of corpus K9.7 into `registers/audits.reg` and gives the error rates, with the upper end of the 95% interval: 3/n with no errors, and the Wilson interval with errors. The command prints the outcome, the spend and the changed files, and it leaves the work tree for a person to see. The exit code is 0 when the goal holds, and 1 when it does not. The command does not open a pull request yet.

The model register of corpus K7.7 is `models/models.yaml`, and the package `models` reads it. Each model that a rung can name has an entry: the exact id, the family, the provider and the terms that apply, with their URL, the date that a person read them and the clauses on outputs. The register has the four Claude models. The terms of Anthropic that a person read on 2026-10-26 say that the customer owns its outputs, and from 2026-11-12 its usage policy forbids the use of outputs to train an AI model without its authorization. A run refuses a model with no entry. X08 checks that each provenance line of an agent names a model with an entry. X09 checks that each judge verdict of the sanity check `judges` is from a model of a family other than the family of the writer. The register has no model of another family yet. Thus a run that records a judge verdict fails X09 until a person reads the terms of another provider and adds its models.

`m2 orchestrate` runs the jobs in a queue (spec 11.4 and 11.5). A job is a YAML file in `queue/` of the store, in the form of `-job`. The budgets are in `orchestrator.yaml` in the tree:

```yaml
roles: {prove: 200, formalize: 100}         # the monthly budget of each role in dollars
on_behalf_of: {project:ug-core: 50}         # the monthly cap of a project or a schedule
retry: 168h                                 # do not try a failed target, goal and rung again for this time
workers: 2                                  # the most runs at the same time
```

Each run gets the least of the money that is left to its job, to its role and to its project in the month, and the run cannot spend more. A job starts on the cheapest rung of the ladder with a recorded success, and each failed run moves it one rung up. A job with the target and the goal of a job in the queue merges into it. The orchestrator takes the roles in turn, and in a role it takes first the project that spent the least in the month. The past spend and failures come from the run records in the store, thus a restart counts them again. A job that ends goes to `queue/done/<id>.json` with its outcome. The work tree of a run that reaches the goal stays in `-work`, and the orchestrator removes the others. With `-once`, the command stops when no job can start. Without it, the command reads the queue again after each `-interval`. The exit code is 1 when a run spent more than its budget. The orchestrator runs with no person to watch it, thus give it `-sandbox` in the same way as `m2 agent run`.

A major bump of a role version needs a shadow week (spec 12.8). Put the files of the candidate in a directory, in the form of `prompts/<role>/`, and name it in `orchestrator.yaml` with `shadow: {prove: /path/to/candidate}`. A relative path is in the tree. The version of the candidate must not be the version of the role. After each run of a job of the role, the candidate runs the same job on the same rung with the same budget, in its own work tree, which the orchestrator always removes. The system uses none of its output. The record of a shadow run has `shadow: true`, and it has no summary in `runs/`, thus the audit rules and the disclosures do not count it. The spend of a shadow run counts in the monthly budget of the role, and not in the budget of the job or in the cap of its project. A shadow run does not move the ladder and does not end a job. The weekly agent report compares the candidate with the current version on the same jobs.

`m2 agent report` writes the weekly agent report (spec 12.9) to `reports/agents/<yyyy>-W<ww>.md` and its JSON twin, and prints the Markdown, or the JSON with `-json`. The week is the last full ISO week, or `-week 2026-W43`. The report counts the runs that started in the week, from the run records in `-store`. It gives the jobs, the runs, the successes, the failures, the stuck runs and the partial runs of each role, the acceptance of each role and model, and the cost of each role, model and MSC area for each accepted run. The trend of the cost comes from the report of the week before. A run is accepted when a provenance line on the branch names it. The incidents are the quarantines of the week, the runs in which the pinned statement caught a change to its closure, and the runs that spent more than their budget. A role in its shadow week has its own table, and the other tables do not count the shadow runs. The runs do not open pull requests yet, thus the report has no counts of pull requests and no audit sample, and it lists them as not measured.

`m2 agent dataset` writes the dataset of the runs of a quarter (spec 17.4) as JSON Lines, to the standard output or to `-o FILE`. The quarter is the last full quarter, or `-quarter 2026-Q4`. Each line is one run that started in the quarter, merged or not: the job, the role at its version, the model of the rung and the models that answered, the outcome and its reason, the number of turns, the budget, the spend, and the verdict of the novelty check of each output. A run is merged when a provenance line on the branch names it. The dataset has no transcripts, no system prompt and no brief. A job on behalf of a person gives only `person`, and the person who requested the job is not in the dataset. The goal and the reason lose their email addresses, API keys, tokens and home directories. The dataset is not published yet, because its licence and its DOI need a decision of the maintainers.

## Projects

A project is a campaign with people attached (spec 13.6). It is the file `projects/<id>.yaml` in the math2 tree:

```yaml
name: The Fermat numbers
goal: File a counterexample for each known factor of a Fermat number.
topic: T0PC1X        # the topic node of the goal
level: E4            # the evidence level that the members of the topic must reach, or no level
budget: 40           # the most US dollars that the agent runs of the project can spend in a month
roles: [explore]     # the agent roles that work for the project
members:
  - {id: github:octocat, role: owner}
  - {id: orcid:0000-0002-1825-0097, role: member}
publications:        # the works in which the people of the project published its work
  - {title: Factors of Fermat numbers, doi: 10.1234/example, year: 2027}
```

The id of the project is the name of the file, and a job works for the project when it runs on behalf of `project:<id>`. `m2 agent run` refuses such a job when the tree has no file for the project, when the role is not in `roles`, or when the person who requested the job is not a member. `m2 orchestrate` adds the budget of each project to the caps of `on_behalf_of`, and a lower cap in `orchestrator.yaml` stays. Each project needs an owner, and the members review the file as they review a node. `m2 check` on the whole tree also reads the projects, and it fails on a project that is not valid or whose topic is not a topic node of the tree.

The wiki has a page for each project at `projects/<id>/`, with a JSON copy at `api/projects/<id>.json`. The page shows the goal, the blueprint of the topic, the tasks, the members, the publications, the spend of each month and the run log. The tasks are the statements with no proof and the open conjectures and problems of the blueprint, and with `-build`, the members of the topic below the level of the goal. Give `m2 site build -store DIR` to show the spend and the runs from the run records in DIR. A shadow run is not a run of its project. The log does not show the pull requests, the reviews and the discussions yet.

## The Mathlib bump

`m2 bump` moves the tree to the newest release tag of Mathlib (spec 07.9). It never picks a release candidate. The command needs a git working tree with no changes, and it does these steps:

1. It builds the tree with the old pins, if the last build is not of the pinned Mathlib commit. This build gives the type hash of each binding before the bump.
2. It makes the branch `bump/<tag>`. It writes the tag into `lean/lakefile.toml` and the toolchain of the tag into `lean/lean-toolchain`, and it runs `lake update mathlib`.
3. It builds the tree with the new pins and puts each change of a binding into one kind: `deprecated` (the `@[deprecated]` attribute names the new declaration), `renamed` (the old declaration is gone, and one declaration has its type hash), `refactored` (the type hash changed), `lost` (no declaration or more than one has the type hash) or `broken` (Lean reports an error in the Math2 code).
4. It writes the new names of the deprecated and renamed declarations into the node files, and records each rename and refactor in `registers/bindings.reg`. A rename keeps the author of the binding. Then it builds again and runs the audit.
5. It writes the report to `reports/bump/<date>.md` and commits it on the branch. The report counts the breaks by kind, gives the time to green, lists the nodes whose fidelity level went down, and lists the Math2 declarations that Mathlib now has with the same type hash.

The exit code is 0 when the build and the audit pass with the new pins. It is 1 when they do not. The branch and the report are there in both cases, thus a workflow can open a pull request for each bump. `m2 bump -master` builds Mathlib master as an early warning. Do not merge such a branch. `m2 bump -n` shows the tag of the bump and stops.

A lost declaration has no rename that `m2 bump` can apply. An agent run of the bump role (`m2 agent run bump`, see "Agent runs") finds the new declaration of such a binding.

## Releases

A release of math2 has a name, `YYYY.MM` for the monthly release or `YYYY.MM.N` for a patch release (spec 06.2). It has three files in `manifests/releases/` and a git tag:

| File | What it has |
|---|---|
| `YYYY.MM.yaml` | The manifest: the commit, the pins, the hash of each artefact, the counts and the rebuild check. |
| `YYYY.MM.levels` | The evidence level and the fidelity level of each live node. The notes of the next release read it, thus they need no build of an old commit. |
| `YYYY.MM.md` | The release notes. A person writes the summary. `m2 release notes` writes the other sections. |

A cut has two runs of `m2 release cut`:

1. The first run builds the tree, writes the three files and commits them. The name is the month of today, or the next patch release when the month has a release. The manifest must pin one stable Lean toolchain and one Mathlib commit.
2. A person writes the summary of the notes. On a second machine, `m2 release verify -oleans DIR -record RUNNER NAME` rebuilds the graph and the `.olean` files from the commit and writes the result into the manifest. With `-against DIR`, it also lists each `.olean` file that is different, and a person writes the reason of each one.
3. The second run checks that only the files of the release changed after the commit of the manifest, runs the full audit, and makes the tag when the audit passes. Push the tag with `git push origin NAME`.

`m2 release notes FROM..TO` writes the notes of TO again and keeps its summary. Without FROM, it uses the release before TO. The notes have these sections: the counts that changed with the cause of each change, the topics whose number of nodes changed, the nodes that moved evidence level, the conjectures and problems that a statement resolves now, the retired nodes with their successors, and the bindings that a Mathlib rename or refactor changed.

| Rule | What it checks |
|---|---|
| R01 | The graph rebuilds from the commit of the manifest to the same hash. |
| R02 | The `.olean` files rebuild to the same hash, or the manifest gives the reason of each different file. |
| R03 | The counts table of the notes gives each count that changed since the release before, with the same numbers and a cause. |
| R04 | The last restore drill in `reports/restore/` passed, and it is not older than one quarter at the last release. |
| R05 | The manifest pins one stable Lean toolchain and one full Mathlib commit, the Lake project of the commit has the same pins, and the last bump report to that commit is green. |

## Coverage

The `msc` package reads MSC2020 codes and the topic map `topics/msc2020.yaml` of the math2 tree. The map gives each of the 6,603 codes a short label in our own words, and it can give the tag of the topic node of a code. The official descriptions of MSC2020 are under CC BY-NC-SA, thus the map does not have them (spec 16.6).

`m2 coverage` measures the coverage of each code at the five levels of spec 18.1. It reads the nodes, the status of the last `m2 build` and the records in `build/records/`. A node or a record counts for each of its codes and for each code above them. The command writes these files:

| File | What it has |
|---|---|
| `reports/coverage/NAME.parquet` | One row for each code: the records, the documents, the nodes by kind and by level, and C0 to C4. |
| `reports/coverage/NAME.md` | The summary of the 63 classes. |
| `reports/coverage/NAME/NN.md` | The summary of the class NN and of its areas. |

The name is `next`, or the name of a release with `-name`. C0 is the share of the documents of zbMATH Open in a class that have a record. The number of documents comes from `manifests/coverage/expected.yaml`. `m2 coverage -count` writes this file with one request to the REST interface of zbMATH Open for each class, with 2 seconds between two requests. C1 is 0, because M2 extracts no results. C2 to C4 have no value until the cores of spec 18.3 are in `topics/cores/`.

## Benchmarks

`bench/graph` measures the latency of the queries Q1 to Q7 on a synthetic graph from the `synth` package, and compares the 95th percentile with the targets of spec 05.6. The test runs with 20,000 nodes in CI. The `bench` workflow runs it each week with 1 million nodes and puts the table in the summary of the run. To run it at reference scale (5 million nodes), use a machine with at least 24 GB of memory and 6 GB of free disk:

```sh
go test ./bench/graph -run TestLatency -nodes 5000000 -timeout 2h -v -report bench.md
```

On a Mac with 10 cores and 24 GB, with 5,001,118 nodes and 16,225,859 edges, each query meets its target:

| Query | | p50 | p95 | target p95 |
|---|---|---|---|---|
| Q1 | resolve an identifier | 0.15 ms | 1.2 ms | 5 ms |
| Q2 | neighbourhood, 2 hops | 5.5 ms | 10 ms | 20 ms |
| Q3 | dependency closure (median 682,066 nodes) | 24 ms | 33 ms | 100 ms |
| Q4 | reverse dependencies | 1.0 ms | 132 ms | 200 ms |
| Q5 | prerequisite path | 3.4 ms | 12 ms | 100 ms |
| Q6 | status slice | 94 ms | 230 ms | 500 ms |
| Q7 | open-problem frontier | 8.7 ms | 144 ms | 200 ms |

The store file has 2.9 GB. Open takes 37 seconds, and the heap after Open has 2.6 GB. The store keeps in memory only the uses, proves and defines edges, the kind and the evidence level of each node, and the axioms of the bindings. The other data stays in SQLite.

`bench/align` measures the precision and the recall of `m2 align` on labelled pairs (spec 17.3.3). Each line of `bench/align/pairs.tsv` is an item of one source, a record of a second source, and the label `same` or `different`. The file has the ids only, and no text of a source. The first pairs come from the exact matches that the sources state: the OEIS, Mathlib and Erdős problem ids of the Wikidata items, and the Erdős problem of each main theorem of formal-conjectures. The decisions of people in `registers/crosswalk.reg` of math2 add more pairs. The evaluation turns off the id channel, because the ids are the labels. The hash of the item puts each pair in the dev split or the test split. Only the threshold is fitted, on the dev split. Without flags the test runs on small records in `testdata`. To run it on the records of the sources:

```sh
go test ./bench/align -root DIR -v -report align.md
```

DIR is a tree with the records from `m2 ingest` in `build/records`. On the records of 2026-10-26, with 1,361 pairs, the test split at the default threshold 0.90 gives:

| Sources | Items | Proposals | Correct | Wrong | Precision | Recall |
|---|---|---|---|---|---|---|
| formal_conjectures → erdosproblems | 621 | 603 | 603 | 0 | 1.000 | 0.971 |
| mathlib → wikidata | 96 | 36 | 33 | 3 | 0.917 | 0.344 |
| wikidata → erdosproblems | 3 | 1 | 1 | 0 | 1.000 | 0.333 |
| wikidata → oeis | 350 | 47 | 44 | 3 | 0.936 | 0.126 |
| all | 1,070 | 687 | 681 | 6 | 0.991 | 0.636 |

The precision of all the pairs is 0.991, thus the default threshold meets the target of 0.98. The precision of mathlib → wikidata and of wikidata → oeis is less than 0.98. Most wrong proposals in the dev split are a near name of a different thing: Stirling numbers of the first kind has more than one OEIS sequence, and the Mathlib `PowerSeries` is the formal power series, not the power series. A threshold does not remove these errors, because their score is 1. The fitted threshold is 0.83. On the test split it gives the precision 0.972, thus the default stays at 0.90. The pooled dev split is mostly formal-conjectures pairs, and these do not tell the threshold for the other sources.

The pairs do not cover all the sources yet. The Stacks project, the LMFDB, zbMATH, arXiv and Palomar state no exact matches that the bench can read: Wikidata has no Stacks property, the adapter does not read the zbMATH property, the LMFDB knowl values are not object ids, and the snapshot has no arXiv records for the Palomar papers. Pairs for these sources come from the decisions of people in `crosswalk.reg`.

`bench/extract` measures the extraction of results from papers (spec 17.3.2). It reads a content plane with the arxiv_results adapter and compares the records with `labels.tsv`, where each line is a result id, its kind and role, its dependencies and the source type of its paper. The report gives the result recall and precision, the kind accuracy and the precision, recall and F1 of the dependency edges, for each source type. Without flags the test runs on four small papers in `testdata`, which we wrote ourselves. They have the hard cases on purpose: a proof in an appendix, a proof with no link, an equation inside a result, a custom environment, a reference in plain words, prose after a statement and a result that the extractor did not mark. On these 19 labelled results:

```sh
go test ./bench/extract -v -report extract.md
```

| Source type | Labelled | Extracted | Recall | Precision | Kind accuracy | Edge precision | Edge recall | Edge F1 |
|---|---|---|---|---|---|---|---|---|
| native | 4 | 2 | 0.50 | 1.00 | 1.00 | - | 0.00 | 0.00 |
| render | 5 | 5 | 1.00 | 1.00 | 1.00 | 0.60 | 1.00 | 0.75 |
| source | 10 | 10 | 1.00 | 1.00 | 0.90 | 1.00 | 0.92 | 0.96 |
| all | 19 | 17 | 0.89 | 1.00 | 0.94 | 0.88 | 0.88 | 0.88 |

The suite is small and we wrote it, so these numbers show the behaviour of the adapter on known cases, and they are not an estimate for arXiv. The misses are the results that the extractor did not mark or marked as a remark, a custom environment with no role, a reference in plain words, and links in the prose after a statement or in a question. Spec 17.3.2 asks for five hundred papers labelled by hand. To run the test on a checkout of tamnd/arxiv with labels by id:

```sh
go test ./bench/extract -root DIR -labels FILE -v -report extract.md
```

Unfolding fidelity is not measured yet, because the adapter does not unfold results.

`bench/search` measures the quality of `m2 search` and of the premise search (spec 14.7). Each line of `bench/search/queries.tsv` is a query and the node or the Lean declaration that it must find. The concept queries describe a theorem in other words than the node. The formula queries give a formula of a node with other names for the variables. The formal queries give a Lean type, a goal or a list of constants. The paper queries are a short title and abstract. We wrote all the queries and the Lean records in `testdata/records.tsv` ourselves, and no tuning used them. The test also makes an identifier query from each `same_as` id, and a premise query from each record that uses other records. Each benchmark runs with all the channels and with each channel alone. `baseline.tsv` keeps the headline metric of each benchmark, and the test fails when a metric is more than 0.02 below it. On the 117 nodes of `testdata/nodes/valid` and 18 records:

```
go test ./bench/search -v -report search.md
```

| Benchmark | Queries | Metric | m2 | lexical | formula | formal | embedding |
|---|---|---|---|---|---|---|---|
| concept | 50 | MRR | 0.679 | 0.680 | 0.000 | 0.000 | 0.620 |
| concept | 50 | R@10 | 0.840 | 0.820 | 0.000 | 0.000 | 0.860 |
| formula | 16 | nDCG@10 | 0.977 | 0.116 | 1.000 | 0.000 | 0.338 |
| formal | 15 | R@1 | 0.867 | 0.000 | 0.000 | 1.000 | 0.000 |
| formal | 15 | R@10 | 1.000 | 0.000 | 0.000 | 1.000 | 0.000 |
| paper | 16 | R@10 | 0.938 | 0.875 | 0.000 | 0.000 | 0.938 |
| identifier | 18 | R@1 | 1.000 | 0.000 | 0.000 | 0.000 | 0.000 |
| premise | 7 | R@10 | 1.000 | | | | |

The fused search does not beat the best channel alone yet. On the concept and paper queries it is equal to the best channel, within one query. On the formula and formal queries the fusion moves an exact match down by one or two ranks, because the lexical and embedding channels add other nodes. The concept misses are paraphrases with no word in common with the node, such as "the derivative equals the slope of the secant line" for the mean value theorem. The hash embedder cannot find these. A learned embedder, behind the same interface, is the next step. The test does not run LeanSearch, Moogle, Loogle or the other tools of spec 14.7, because it would send each query to the service of the tool.

`bench/novelty` measures the recall of the novelty check (spec 17.5). Each item of `bench/novelty/items.yaml` is a known result in the words of an agent output, with the matches that make it known: a node of `testdata/nodes/valid`, or a paper of `records.yaml`. Some items have no match, and the check must not call them known. We wrote the items and the papers ourselves, and the ids of the papers are not ids of zbMATH Open. The searches are the graph and the record plane. Without flags, an oracle judge gives `same` to a match of the item, thus the recall is the share of the known items whose match is in the three best matches of a search. `baseline.tsv` keeps it, and the test fails when the recall is more than 0.02 below it. With `-live`, the judge of explore judges, which needs `ANTHROPIC_API_KEY` and spends money:

```sh
go test ./bench/novelty -v -report novelty.md
go test ./bench/novelty -live -rung 0 -v -report novelty.md
```

On the suite, the check finds 18 of the 22 known items, a recall of 0.818. The misses are results whose words are far from the node or the paper, for example Fermat's little theorem as "$a^p - a$ is divisible by $p$". The searches of the web are not in the suite, because a run sends each query to the service.

`bench/fidelity` measures the F1 battery as a classifier (spec 17.3.1). Each line of `bench/fidelity/items.tsv` is an informal statement and a label: `faithful`, or the class of the fault of its formal statement. The formal statements are in `bench/fidelity/lean/Suite.lean`, and they use only the Lean core library, thus the suite does not need Mathlib. We wrote the items, and no person checked the labels yet. The test runs the battery on each item, as `m2 build` does. A check that fails catches the item, and a check that flags with a class flags it. The report is a confusion matrix, with the recall of each class and the false-alarm rate on the faithful items. `baseline.tsv` has the number of items that the battery catches in each class, and the test fails when a class catches fewer, or when the battery fails a faithful item. Each release has the report as `fidelity-report.md`.

```sh
go test ./bench/fidelity -v -report fidelity.md
go test ./bench/fidelity -tree DIR -v -report fidelity.md
```

With `-tree`, the items are the reviews of a math2 tree, with the reports of its last build. On the suite, without the models of the checks 2 and 9:

| Label | Items | Caught | Recall | Recall with flags |
|---|---|---|---|---|
| faithful | 14 | 0 | | |
| M-hyp-missing | 6 | 1 | 0.17 | 0.67 |
| M-hyp-extra | 5 | 0 | 0.00 | 1.00 |
| M-quant | 4 | 0 | 0.00 | 0.00 |
| M-vacuous | 5 | 5 | 1.00 | 1.00 |
| M-junk | 3 | 0 | 0.00 | 1.00 |
| M-def | 3 | 2 | 0.67 | 0.67 |
| M-type | 4 | 0 | 0.00 | 0.75 |
| M-conv | 3 | 2 | 0.67 | 1.00 |
| M-weak | 4 | 0 | 0.00 | 1.00 |
| M-strong | 5 | 1 | 0.20 | 0.20 |

The machine checks catch the vacuous statements, and the false statements whose negation a tactic proves. They do not catch a wrong quantifier, and they catch no false statement that needs a counterexample, such as n² < 2ⁿ. The flags come from the constant audit, which names natural subtraction and division, and from check 5, which flags a statement that a tactic proves. Check 5 also flags 10 of the 14 faithful items, thus a flag tells a reviewer where to look, and it is not a verdict. The other faults need the back-translation and the critic. No class has 30 items yet, thus the target of a recall of 0.9 does not apply yet.

`bench/agents` is the suite of the formalize and prove roles (spec 17.3.4). Each line of `bench/agents/items.tsv` is a target node and its role. A prove item has a pinned statement and a reference proof. A formalize item has a node with no binding and a reference formal statement. The 11 public items use only the Lean core library, and we wrote them. The test builds one work tree with all the items and runs each item in it with real Lean. Without flags, a scripted model writes the reference of each item, and the goal of the role must hold. Then it writes wrong attempts: a proof with `sorry`, a proof that changes the statement, a proof of another statement, and a false formal statement. The goal must refuse each of them with the right reason. No model service is called. With `-live`, the test runs the roles with the Claude models of the rung `-rung` of each ladder and reports the items solved, the turns and the dollars. This needs `ANTHROPIC_API_KEY` and spends money. With `M2_LLM_COMMAND` set, the live run uses the clients of the subscriptions, and the dollars are the prices of the Claude API for the same tokens.

```sh
go test ./bench/agents -v
go test ./bench/agents -live -rung 0 -v -report agents.md
M2_LLM_COMMAND=codex,claude go test ./bench/agents -live -rung 0 -v -report agents.md
```

`bench/agents/explain` and `bench/agents/curate` are the suites of the explain and the curate roles. They need no Lean, thus they also run with `-short`. Each item of `items.yaml` is in our own words. An explain item is a target node, a level and a reference explanation, and a curate item is a record of the source `bench` and its reference node. Without flags, a scripted model writes the reference, the scripted check or judge passes it, and the goal must hold. Then the model writes wrong attempts, and the goal must refuse each of them with the right reason. For explain, these are an explanation with no citation, one that cites a node outside the curricula of its level, one at the wrong level, and the wrong explanations of the items with the labels of the check. For curate, these are a node that copies the record, a node with no source at the locator of the record, a node of the wrong kind, and the wrong nodes of the items with the verdict of the judge. With `-live`, the test runs the role on each item, and then it grades the check or the judge alone on the reference and the wrong attempts of each item. This needs `ANTHROPIC_API_KEY` and spends money.

```sh
go test ./bench/agents/explain ./bench/agents/curate -v
go test ./bench/agents/curate -live -rung 0 -v -report curate.md
```

`bench/agents/enumerate` is the suite of the enumerate role, and it needs no Lean. Each item is a cell, the names that the inventory has for it, and reference items that a model of the cell must know, each with a source that resolves. Without flags, a scripted model lists the names that the inventory has, which the tool refuses, and then the reference items, which the goal accepts. With `-live`, the report gives the share of the reference items that the model listed, and the share of the listed items with a source.

```sh
go test ./bench/agents/enumerate -v
go test ./bench/agents/enumerate -live -rung 0 -v -report enumerate.md
```

`bench/agents/exercise` is the suite of the exercise role, and it needs no Lean. Each item is a target node and reference exercises for it, with the answer that a solver must reach. Without flags, the suite runs each item in a tree at node.v1 and in a tree at node.v2, and in each tree a scripted model writes an exercise with no solution in steps, which the goal refuses, and then the reference exercises, which the goal accepts. A scripted solver solves the first exercise and not the others, and the queue must have each exercise that it does not solve. With `-live`, the report gives the exercises of each target with their groups and checks, and the spend.

```sh
go test ./bench/agents/exercise -v
go test ./bench/agents/exercise -live -rung 0 -v -report exercise.md
```

`bench/agents/write` is the suite of the write role, and it needs no Lean. Each item is a work tree with a target node, the goal of the job, and the reference writes that take the target to the level of the goal. These writes are the sections, examples, non-examples and proofs that the card of the kind of the target asks for. Without flags, a scripted model makes each reference write. The last write first has the wrong form of the item, such as a proof that is not in steps or a definition with no term in bold, and the goal must refuse it with the reason of the item. Then the last write is as it is, and the goal must hold. With `-live`, the report gives the level of each job, the outcome, the turns, the files that the run wrote, and the spend.

```sh
go test ./bench/agents/write -v
go test ./bench/agents/write -live -rung 0 -v -report write.md
```

`bench/agents/informalize` is the suite of the informalize role. It needs no Lean, because a fake Lean environment gives the record of the declaration of each item. Each item is a Mathlib declaration with a short form of its type that we wrote, a work tree with the nodes that bind the constants of the type, and the reference writes that give the declaration its node. Without flags, a scripted model makes each reference write. The last write first has the wrong form of the item, such as a Lean name as the name of the node, a definition with no term in bold, or a node whose uses do not have the node of a constant, and the goal must refuse it with the reason of the item. Then the last write is as it is, and the goal must hold. With `-live`, the report gives the outcome, the turns, the nodes that the run added with their kinds and names, and the spend.

```sh
go test ./bench/agents/informalize -v
go test ./bench/agents/informalize -live -rung 0 -v -report informalize.md
```

`bench/agents/reviewassist` is the suite of the review-assist role, and it needs no Lean either. Each item is a node with one Lean binding, a Lean record that we wrote by hand, and the verdict of a review. A not-faithful item has the class of spec 10.5 that the critic must name. Without flags, a scripted model writes the packet with the reference concern, and the goal must hold. Then the goal or the tool must refuse each wrong attempt: no packet, a concern of a binding that the packet does not have, a class that spec 10.5 does not have, too many concerns, and a write of the node. With `-live`, the report gives the recall of the critic, which is the share of the not-faithful items whose packet names the class of the review, the concerns on the faithful items, and the length of the packets.

```sh
go test ./bench/agents/reviewassist -v
```

`bench/agents/align` is the suite of the align role, and it needs no Lean. Its tree has nodes with a Math2 binding, records of Wikidata, arXiv and Mathlib that we wrote, the type hashes of the Math2 bindings, and an edge that a person rejected. Each item is a target and its reference change: an edge to a record with the verdict of the judge, or a Mathlib binding with the type hash of the Math2 binding. Without flags, a scripted model makes the reference change, and the goal must hold. Then the goal or the tool must refuse each wrong attempt: no change, same_as to Mathlib, same_as to a paper, aligned to a record that is not a paper, an edge that a person rejected, a Mathlib binding of another type, and edges whose verdict is not the same. With `-live`, the role runs once on each target, and the report gives the precision and the recall of its changes and the share of the right verdicts of the judge.

```sh
go test ./bench/agents/align -v
```

`bench/agents/bump` is the suite of the bump role, and it needs no Lean. Each item is a replay of a bump: the Lean bindings of a node before the bump, a pinned Mathlib after the bump that the file gives, the rebinds that repair the node, and whether the repair can merge with no review. The public items are made up. Without flags, a scripted model makes the rebinds, the goal must hold, and `bump.AutoMerge` must give the value of the item. A rebind to another type hash needs `type_change`, thus its item must fail without it. Then the goal or the tool must refuse each wrong attempt: no rebind, a write of the node, the same declaration, another kind or type, a declaration that does not resolve, a binding that is not broken, and a repair of only one of two broken bindings. With `-live`, the report gives the share of the breakages that the role repaired, the share that can merge with no review, and the repairs that change a type hash with no `type_change`, which must be zero.

```sh
go test ./bench/agents/bump -v
```

`bench/agents/explore` is the suite of the explore role, and it needs no Lean. Its items are planted problems: each target is a plausible statement that is false, such as "the Fermat numbers are prime", and each item has the program that finds the counterexample, the output of the program, and the reference counterexample. Without flags, the sandbox is a fake that gives the output of the item, and no container runs. A scripted model writes the computation, runs it, looks up the claim and files the counterexample, and the goal must hold. Then the goal or the tool must refuse each wrong attempt: no output, an output that says that it is new, a conjecture that no computation checks, a counterexample with no computation, a wrong output hash, and a change of the target. With `-live`, the computations run in docker, thus the run also needs the manifest of the images with `-images`. The report gives the refutations found, the conjectures that the runs filed for a person to read, and the share of the runs that looked up each claim before they filed it.

```sh
go test ./bench/agents/explore -v
go test ./bench/agents/explore -live -images ../math2/manifests/images.yaml -rung 0 -v -report explore.md
```

## Rules that the toolchain keeps

1. The files in tamnd/math2 are the truth. Each database, index and site comes from these files, and you can make it again from them.
2. The build computes the status of each node. No person and no agent writes a status.
3. Only the Lean kernel and the certificate checkers can give the levels E4 and E5. A model can propose a proof. A model cannot accept a proof.
4. A formal statement must agree with its informal statement. This agreement is a separate level, F0 to F3. Only a person can give F2 or F3.
5. Each write is a pull request against tamnd/math2. The audit is the gate.
6. Each agent run has a run record. Each run has a person or a schedule that is responsible for it.
7. Each release pins the Lean toolchain, the Mathlib tag, the container images, the model ids and the prompt versions.

## Build and test

```sh
make build    # writes bin/m2
make test     # runs the tests with the race detector
make lint     # runs golangci-lint
```

CI runs the same checks on each pull request. It also runs govulncheck, CodeQL, OpenSSF Scorecard, a smoke test of the binary, actionlint and zizmor.

## Layout

```
cmd/m2/   the m2 binary
align/    the aligner: candidates, scores, the curation queues and the register of decisions (spec 05.3, 05.4 and 16.3)
api/      the API of spec 11.2: the read methods, the write methods, the HTTP handler and the rate limit
audit/    the audit rules of spec 15, with a fixture pair for each rule
battery/  the F1 battery: the ten machine checks of the fidelity of a Lean binding (spec 10.4)
bench/    the benchmarks: bench/graph measures the graph queries of spec 05.6, bench/align measures the precision of the aligner, bench/extract measures the extraction of results from papers, bench/search measures the quality of search, bench/novelty measures the recall of the novelty check, and bench/fidelity measures the F1 battery
bump/     the Mathlib bump: the pins, the breaks of a bump, the renames and the bump report (spec 07.9)
build/    the steps of m2 build, their cache and the build manifest
check/    the fast rules of m2 check: schema, body and links
copycheck/ the copy check of corpus K9.4: the exact runs, the near copies and the standard phrases
crosswalk/ the syntax of the ids of each external system of same_as (spec 05.3)
cli/      the command table
format/   the canonical form of m2 fmt and the change from aliases to tags
graph/    the graph of spec 05: edges, closure, cycles, CSR and Parquet
katex/    render TeX to HTML and MathML with KaTeX in pure Go, with the style sheet and the fonts of the same release
ingest/   the adapters of the external sources, the licence gate, the snapshots and the import manifests (spec 09)
lean/     M2Extract, its Lake package and the Go package that drives it
mcp/      the MCP server: a read-only tool for each method of the API (spec 11.2 and 11.7)
models/   the model register of corpus K7.7: the family, the provider and the terms of each model that a run can call
pack/     assemble a volume into an edition: the closure, the order, the numbers, the HTML, the lock file and the pack checks (corpus K11)
notation/ the notation register and the register of conventions (corpus K10.4 and K10.5), and the macro files that the register gives
markup/   render the Markdown of a body to HTML, with a resolver for node links and math
node/     read node files, write the canonical form and compute the content hash
propose/  the writes of spec 11.1: the change of each write method, the audit gate, the cap and the pull request on GitHub
project/  the projects of the tree: the goal, the budget, the roles and the members (spec 13.6)
prov/     the provenance logs, the agent runs, the credit of a person and the quarantine of a model (spec 04.8 and 06.5)
release/  the release manifest, the levels and the notes of spec 06.2, and the rebuild checks R01 and R02
replay/   the replay of a declaration in the comparator and the independent kernels
review/   the review packets of spec 10.6, the writing of fidelity reviews and the export of the reviews of each reviewer (spec 16.4)
sample/   the sample audits of corpus K9.7: the draw, the register and the error rates of each stratum
schema/   check front matter against the node schema
search/   the search index: the identifier resolver, the four channels, the fusion and the premise selection (spec 14)
sources/  the source confirmation of corpus K9.5: the lookup of a claimed source in a catalogue and the comparison with its record
site/     build the wiki
store/    the serving store: the graph and the status of each node in SQLite, and the queries of spec 05.6
status/   the evidence level, the fidelity level and the flags of each node (spec 04.7), and the reviews
synth/    synthetic graphs for the benchmarks
tag/      tags, provisional ids, the tag register and the alias register
uri/      m2:// URIs
spec/     the specification
```

Each directory except `cmd/`, `bench/` and `spec/` is a Go package that other programs can import. For example, a bot can use `github.com/tamnd/math2-agent/node` to read and format node files. The packages follow the conventions of the Go standard library, and `go doc` shows an example for each main function.

The full layout arrives with the milestones. The plan has these parts: `compute`, `api`, `mcp`, `orchestrator`, `agent`, `prompts/`, `images/` and `web/`.

## Contribute

Read [CONTRIBUTING.md](CONTRIBUTING.md). To report a security problem, read [SECURITY.md](SECURITY.md).

## Licence

Apache-2.0. Refer to [LICENSE](LICENSE).
