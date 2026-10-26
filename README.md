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

The code of M2 connects the graph to the world. The `store` package puts the graph and the status of each node into one SQLite file, and `m2 graph` runs the neighbourhood, closure, reverse dependency, learning path and frontier queries on it. The `synth` package and `bench/graph` measure these queries on a synthetic graph. The `ingest` package and `m2 ingest` get pinned snapshots of external sources, apply the licence gate of spec 09.3 and write the records. The adapters of arXiv, zbMATH Open, Mathlib, the OEIS, Wikidata, the LMFDB, the Erdős problems, formal-conjectures, Palomar and the Stacks Project are written. The rules L01 to L04 check the licences. Rule A01 checks that each `same_as` id has the syntax of its system, with the `crosswalk` package. The `align` package and `m2 align` propose `same_as`, `close_to` and `aligned` edges to the records of a source, and a person accepts or rejects each proposal. The rules A02 to A04 check these edges over time. `bench/align` measures the precision and the recall of the aligner on labelled pairs. The `bump` package and `m2 bump` move the tree to a newer Mathlib release on a new branch. `m2 release` cuts a release, writes its notes and verifies it, and the rules R03 to R05 check the notes, the restore drills and the pins. The `msc` package reads the topic map of MSC2020, and the `coverage` package and `m2 coverage` write the coverage report of each code.

Some acceptance items of M2 need time or people, and they stay open: three monthly bumps in a row, the releases of April to June 2027, the full harvests of zbMATH Open and arXiv for C0, and a test of the curation guide by a new curator.

The code of M3 opens the graph to readers. The `search` package writes the search index in `m2 build`, and `m2 search` runs the identifier resolver and the lexical, formula, formal and embedding channels, with the premise search of spec 14.5. `bench/search` measures the quality of search on labelled queries. The `api` package and `m2 serve` give the read methods of spec 11.2 over HTTP with JSON and over gRPC, and the `mcp` package gives the same methods as MCP tools. The `sandbox` package runs a program with no network and with limits, and the `compute` package and `m2 compute` run the computations in it. The rules C01 to C05 check the computations and their runs. The `prov` package reads and writes the provenance log of each node (spec 04.8), and `m2 prov` shows, records, credits, quarantines and confirms the lines. The rules P01 to P04 check the logs and the agent runs. The `ingest/arxivresults` adapter reads the results of arXiv papers from the content plane of tamnd/arxiv, and `bench/extract` measures it.

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
| `m2 site build` | Build the wiki: a static site for people and agents. | written |
| `m2 site check` | Test the wiki against the size, MathML, no-JavaScript, figure and offline targets. | written |
| `m2 site serve` | Preview the wiki with gzip, as GitHub Pages serves it. | written |
| `m2 site archive` | Write the wiki as a zip file that works offline. | written |
| `m2 build` | Build the derived data: Lean, extraction, replay, the fidelity battery, status, graph, store and the search index. | written |
| `m2 audit` | Run the audit rules and write the report to `reports/`. | written |
| `m2 status <tag>` | Show the computed status of a node and the evidence for it. | written |
| `m2 graph` | Run a graph query: neighbourhood, closure, reverse dependencies, path or frontier. | written |
| `m2 ingest <source>` | Get a pinned snapshot of an external source and write its records. `-snapshot HASH` uses a recorded snapshot again. | written |
| `m2 align <source>` | Propose crosswalk edges to an external source. `m2 align queue`, `accept`, `reject` and `confirm` show the proposals and record the decision of a person. | written |
| `m2 bump` | Make a branch that moves Lean and Mathlib to the next release, rebind the renamed declarations and write the bump report. | written |
| `m2 coverage` | Write the coverage of each MSC2020 code at the levels C0 to C4 to `reports/coverage/`. `-count` asks zbMATH Open for the number of documents in each class. | written |
| `m2 release` | Cut a release with its manifest, levels and notes, write the release notes, or rebuild a release from its manifest. | written |
| `m2 search <query>` | Search the knowledge graph with the identifier resolver and the lexical, formula, formal and embedding channels. `m2 search -premises <goal>` ranks the Lean declarations that can help to prove a goal. | written |
| `m2 serve` | Serve the read API over HTTP with JSON and over gRPC, and the MCP server over HTTP or stdio, from the last build. | written |
| `m2 compute run\|verify <tag>` | Run a computation in the sandbox, or verify its output and its certificate. | written |
| `m2 prov` | Show, record, credit, quarantine, confirm and disclose provenance. | written |
| `m2 review <tag>` | Show the review packet of a node, and write a fidelity review of a Lean binding. | written |
| `m2 autoform <tag>` | Make a formal statement for a node. | M4 |
| `m2 agent run <role>` | Do one agent run. | M5 |
| `m2 orchestrate` | Run the job scheduler. | M5 |

## The wiki

`m2 site build` makes the wiki at https://tamnd.github.io/math2 from a math2 working tree and the spec. The math2 repository runs it on each push to `main` and each night, and GitHub Pages serves the result.

```sh
go run ./cmd/m2 site build -content ../math2 -spec spec -out _site -url http://localhost:8000/math2
go run ./cmd/m2 site check -url http://localhost:8000/math2 _site
go run ./cmd/m2 site serve _site
```

Each node gets a page with an infobox, its links and its backlinks. The build renders the math to HTML and MathML with KaTeX and the macros of math2, so a reader can read each node with JavaScript off. The site serves the KaTeX style sheet and fonts itself, so it loads nothing from a CDN. Each page also has a copy for programs: `node/<TAG>.md`, `api/node/<TAG>.json`, `api/nodes.json`, `llms.txt` and `llms-full.txt`. The pages Contribute and Curation come from CONTRIBUTING.md and CURATION.md in the math2 tree. A build with no `-date` flag gives the same bytes each time.

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

A computation is a node of kind `computation` (spec 08.2). Its code is in `compute/TAG/` of the content tree. Its record names the image by digest, the entry point, the inputs, the seed, the budgets, the hash of the output and, for a computation that can reach E4, a certificate.

```sh
m2 compute run 3MZ8QD       # run it in the sandbox
m2 compute verify 3MZ8QD    # run it, and check the output and the certificate
m2 compute verify -all      # each computation of the tree
```

The `compute` package reads the record and the image manifest `manifests/images.yaml`. A record names an image by the digest of one architecture, and the runner runs the digest of the same image for its own architecture. The program reads `compute/TAG/` at `/work`, gets the inputs as JSON in `M2_INPUTS` and the seed in `M2_SEED`, and writes a certificate to `/out/certificate`. The output hash is the BLAKE3 hash of the standard output.

The run writes `run.json`, `stdout` and `stderr` to `build/compute/TAG/ARCH/`. A verified run exits with the code 0 in time, and its output has the hash of the record. When the record has a certificate, the certificate has the hash of the record, and the fixed checker of its kind (spec 08.8) accepts it. `compute.Checkers` is the table of the fixed checkers. A change to it needs the review of a person.

The rules C01 to C05 of `m2 audit` check the computations:

| Rule | It checks |
|---|---|
| C01 | the image of each record is in the manifest, the entry point exists in `compute/TAG/`, and the record is valid |
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

## The status of a node

`m2 status` shows the status of a node from the last `m2 build`. It reads `build/status.json`. Give a tag, an alias or an m2:// URI.

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
```

These sources have an adapter now:

| Source | Package | Bulk channel | Licence | What a record keeps |
|---|---|---|---|---|
| `arxiv` | `ingest/arxiv` | the metadata plane of tamnd/arxiv, one JSONL file for each month | CC0 metadata; each paper has the licence of its last version | the mathematics papers only: title, authors, categories, MSC codes, DOI and the `ax://` locator. No text. |
| `arxiv_results` | `ingest/arxivresults` | the git archive of tamnd/arxiv, the content plane `content/en/` | each result has the licence of its paper version; the papers with the access class record are skipped | the theorems, lemmas, propositions, corollaries, definitions, conjectures, problems and examples of the mathematics papers: the kind and role of spec 04.2, the heading as the title, the `ax://paper/<id>#<tag>` locator, the proofs, the dependencies inside the paper, the section, the source type and the MSC codes. The text of the statement only when the licence lets math2 store it. |
| `erdosproblems` | `ingest/erdosproblems` | the git archive of teorth/erdosproblems, `data/problems.yaml` | Apache 2.0 | each problem: the number, the status and its date, the formal status, the prize, the OEIS sequences, the tags, the short name and Paul Erdős as the person who posed it (rule G06). No statement text, because the data file has none. |
| `formal_conjectures` | `ingest/formalconjectures` | the git archive of google-deepmind/formal-conjectures, the Lean files under `FormalConjectures/` | Apache 2.0 | each theorem with a category attribute, except the category test: the full Lean name, the docstring as the text, the category, the AMS classes, the Lean statement, the link to a formal proof, the title and the links of the module docstring, and the Erdős, OEIS, MathOverflow and Wikipedia ids that the links name. The URL is the line of the theorem at the commit of the snapshot. |
| `lmfdb` | `ingest/lmfdb` | a URL of the API of the LMFDB, with a table and filters. The default is the elliptic curves over Q of conductor at most 100 | CC BY-SA 4.0 | each object: the path of its page as the id, such as `EllipticCurve/Q/11.a2`, the label as the title and the table as the kind. The adapter knows the tables `ec_curvedata` and `nf_fields`. |
| `mathlib` | `ingest/mathlib` | M2Extract in the Lake project `lean/` of the tree, at the pinned commit of Mathlib | Apache 2.0 | each declaration of Mathlib: kind, module, the hash of the canonical type, the constants that the value uses, the docstring as text and the link to the documentation. |
| `oeis` | `ingest/oeis` | `https://oeis.org/names.gz`, the names of all sequences | CC BY-SA 4.0 | each sequence: the A-number and the name as the title. No other text. |
| `palomar` | `ingest/palomar` | the listing of the Palomar registry API, `order=registered&trust=all` | CC0 | each registered result at the newest version of its entry: the entry id, the title, the abstract as the text, the authors, the arXiv categories, the MSC classes, the Lean names of the theorems, the repository and commit that the registry checked, the trust level and the time of registration. |
| `stacks` | `ingest/stacks` | the git archive of stacks/stacks-project, `tags/tags`, `chapters.tex` and the LaTeX of the chapters | GFDL 1.2 or later: link only | each tag: the kind, the title of the part, chapter, section or named statement, the label, the chapter, the section and the file and line. The URL is the page of the tag. The text of the statement and its proof gives the fingerprints of rule L02, and the licence gate drops the text. |
| `wikidata` | `ingest/wikidata` | the SPARQL service of Wikidata, one query for each crosswalk property | CC0 | each item with an id in nLab, MathWorld, ProofWiki, the Encyclopedia of Mathematics, the MSC, the OEIS, Mathlib, the LMFDB knowls, the House of Graphs or the Erdős problems: the QID, the English label as the title and the ids in the other systems. |
| `zbmath` | `ingest/zbmath` | the OAI-PMH interface of zbMATH Open, `oai_zb_preview` | CC BY-SA 4.0 | each document: title, authors, MSC codes, Zbl number, DOI, the source as the locator and the ids of the references. No review text. |

The formal-conjectures adapter reads the Lean files as text and does not build them, because the project pins its own Mathlib. It skips comments and strings, follows the namespaces and takes each theorem and lemma that has a category attribute. The snapshot of 25 October 2026 (commit 49ba26b) gave 4,652 statements: 1,602 open research problems, 2,816 solved research problems and 234 textbook results. 2,773 of them name an Erdős problem.

The Palomar adapter follows the cursor of the listing with one request each second. When the registry changes during a fetch, the API answers 409 Conflict and the adapter starts again from the first page. When the listing abbreviates an entry, the adapter gets the complete record of the entry. The snapshot of 25 October 2026 (revision 517) gave 760 results in 43 seconds. 117 of them name a directory in their repository, and 85 entries have more than one version.

The Stacks adapter reads the LaTeX as text, one line after the other, and joins each tag in `tags/tags` to its label. It does not compute the numbers of the book, such as Lemma 26.4.2, because the numbers come from the LaTeX build and the adapter cannot check them. The snapshot of 25 October 2026 (commit a04446e) gave 21,446 tags in less than 5 seconds: 12,587 lemmas, 3,299 sections, 1,721 definitions, 688 equations, 449 examples, 440 items, 386 exercises, 330 propositions, 214 theorems, 124 situations, 116 chapters, 35 subsections, 9 parts, 1,043 remarks and 5 lists of remarks. The records keep no text, only the fingerprints. With these records, rule L02 passes on the nodes of math2 and fails on a node that copies the statement of tag 01IS.

The arxiv_results adapter does not extract results again. The extractor of tamnd/arxiv marks each object of a paper with an attribute block, such as `{#thm-1 .statement tag=3KH2 env=theorem}`, and the adapter reads these blocks. The extent of a result runs to the next block that is not an equation, as in the objects package of tamnd/arxiv-reader. A proof belongs to the result that its heading links to, or to the last result before it. The dependencies are the links to other results, and to the equations in them, from the statement and its proofs. The adapter does not read references in plain words, and it does not unfold results, because unfolding needs a model. The main branch of tamnd/arxiv at commit c818ec0, of 14 September 2026, has one paper, 2312.00752. It is in cs.LG, so it gives no results yet.

When the bulk channel is a git repository, the snapshot is the git archive that GitHub gives for the branch, and the catalogue records the commit from the archive. An adapter uses `ingest.FetchArchive` and `ingest.WalkArchive` for this. A snapshot that has more than one file, such as an OAI-PMH harvest, is the files one after the other, compressed with gzip. The zbMATH adapter fails when the number of records is not the size of the list that zbMATH Open gives. The full zbMATH harvest has more than four million documents, so give a set to harvest one MSC class.

The Mathlib adapter downloads nothing. It runs M2Extract on the .olean files of the tree, so run `m2 build` first. The catalogue records the commit of Mathlib with the snapshot. When the tree moves to a new tag of Mathlib, run `m2 ingest mathlib` again to get the records of the new tag. At the commit 56a0058 of Mathlib, for Lean 4.35.0, the adapter gives 358,102 records in less than 5 minutes on a laptop. Of these, 90,326 have a docstring. The snapshot is 50 MB.

The Wikidata adapter gives the crosswalk that Wikidata has now. On 2026-10-25 it gave 12,302 items in less than 30 seconds. MathWorld has the most ids (6,840) and nLab is next (4,348). Only 115 items have a Mathlib id. The adapter does not ask for the items with a defining formula, because most of them are single numbers and the query does not end in the 60 seconds that the service gives it.

The LMFDB adapter gets the labels one page of 100 after the other, with a pause of 2 seconds between the pages. The LMFDB asks automated clients to use its bulk downloads for large slices, and it answers with a captcha when it gets too many requests. Thus give a filter that selects a small slice. When the answer is not JSON, the adapter stops with an error.

Each source has an access class: `open`, `share-alike-nc`, `verbatim`, `gfdl` or `record` (spec 09.3). The licence gate stores the text of an item only when its class is `open`. For the other classes it keeps the record, the locator and the fingerprints of the text. The fingerprints cannot give the text back. The audit uses them to find copied text:

| Rule | What it checks |
|---|---|
| L01 | Each source names a licence and a class that agrees with it. Each import manifest names a recorded snapshot. The records agree with the manifest and store only the text that their class allows. |
| L02 | No node body copies text from a source that is not `open`. |
| L03 | Each share-alike source has an attribution line, and a node that uses its text names the item in `sources` or `same_as`. |
| L04 | A node body that is similar to an item that it cites. A person checks it. |

L02 to L04 need the records in `build/records/`. When they do not exist, the rules skip. Each source has an adapter in the package `ingest/<source>`. An adapter whose bulk channel has more than one file also implements `ingest.Fetcher`. To add a source, write the adapter, register it with `ingest.Register`, import its package in `cmd/m2`, and ask a maintainer to approve the class in `manifests/sources.yaml`.

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

P02 and P03 need the run summaries in `build/runs/`. When they do not exist, the rules skip.

## The Mathlib bump

`m2 bump` moves the tree to the newest release tag of Mathlib (spec 07.9). It never picks a release candidate. The command needs a git working tree with no changes, and it does these steps:

1. It builds the tree with the old pins, if the last build is not of the pinned Mathlib commit. This build gives the type hash of each binding before the bump.
2. It makes the branch `bump/<tag>`. It writes the tag into `lean/lakefile.toml` and the toolchain of the tag into `lean/lean-toolchain`, and it runs `lake update mathlib`.
3. It builds the tree with the new pins and puts each change of a binding into one kind: `deprecated` (the `@[deprecated]` attribute names the new declaration), `renamed` (the old declaration is gone, and one declaration has its type hash), `refactored` (the type hash changed), `lost` (no declaration or more than one has the type hash) or `broken` (Lean reports an error in the Math2 code).
4. It writes the new names of the deprecated and renamed declarations into the node files, and records each rename and refactor in `registers/bindings.reg`. A rename keeps the author of the binding. Then it builds again and runs the audit.
5. It writes the report to `reports/bump/<date>.md` and commits it on the branch. The report counts the breaks by kind, gives the time to green, lists the nodes whose fidelity level went down, and lists the Math2 declarations that Mathlib now has with the same type hash.

The exit code is 0 when the build and the audit pass with the new pins. It is 1 when they do not. The branch and the report are there in both cases, thus a workflow can open a pull request for each bump. `m2 bump -master` builds Mathlib master as an early warning. Do not merge such a branch. `m2 bump -n` shows the tag of the bump and stops.

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
bench/    the benchmarks: bench/graph measures the graph queries of spec 05.6, bench/align measures the precision of the aligner, bench/extract measures the extraction of results from papers, bench/search measures the quality of search, and bench/fidelity measures the F1 battery
bump/     the Mathlib bump: the pins, the breaks of a bump, the renames and the bump report (spec 07.9)
build/    the steps of m2 build, their cache and the build manifest
check/    the fast rules of m2 check: schema, body and links
crosswalk/ the syntax of the ids of each external system of same_as (spec 05.3)
cli/      the command table
format/   the canonical form of m2 fmt and the change from aliases to tags
graph/    the graph of spec 05: edges, closure, cycles, CSR and Parquet
katex/    render TeX to HTML and MathML with KaTeX in pure Go, with the style sheet and the fonts of the same release
ingest/   the adapters of the external sources, the licence gate, the snapshots and the import manifests (spec 09)
lean/     M2Extract, its Lake package and the Go package that drives it
mcp/      the MCP server: a read-only tool for each method of the API (spec 11.2 and 11.7)
node/     read node files, write the canonical form and compute the content hash
propose/  the writes of spec 11.1: the change of each write method, the audit gate, the cap and the pull request on GitHub
prov/     the provenance logs, the agent runs, the credit of a person and the quarantine of a model (spec 04.8 and 06.5)
release/  the release manifest, the levels and the notes of spec 06.2, and the rebuild checks R01 and R02
replay/   the replay of a declaration in the comparator and the independent kernels
review/   the review packets of spec 10.6 and the writing of fidelity reviews
schema/   check front matter against the node schema
search/   the search index: the identifier resolver, the four channels, the fusion and the premise selection (spec 14)
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
