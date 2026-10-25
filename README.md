# math2-agent

This repository holds the toolchain and the research agents for Math 2.0. The content is in [tamnd/math2](https://github.com/tamnd/math2).

Math 2.0 is a versioned knowledge graph of mathematics. The `m2` command reads the node files in math2. It builds the Lean library, computes the status of each node, makes the graph and checks each change with the audit rules. Later milestones add search, an API, an MCP server, a research workspace and AI research agents.

## Specification

The design is in [spec/](spec/00-README.md). It has 22 documents, 00 to 21. Issues, code and reviews refer to a section by its number, for example 04.5 or 15.3 S03.

The wiki at https://tamnd.github.io/math2 shows the spec, the milestones and each node.

## Status

The project is at milestone M1. Release v0.1.0 closes the code of M0: `m2 new`, `m2 fmt`, `m2 check`, `m2 mint` and `m2 site build` are written. In M1, `m2 audit` runs the rules T01 to T06, S01 to S04, S09, B01 to B05, G01 to G10, F01 to F09, V01 to V04, R01 and R02. The rules G04, F01 to F05 and F07 to F09 read the files in `build/` from the last `m2 build`. A rule skips when a file that it needs does not exist. The `graph` package derives the graph from the node files, with the AND-OR closure of spec 05.5, the CSR adjacency and the Parquet build format. The `lean` package drives M2Extract, a small Lean program that gives the type hash, the axioms, the formal dependencies and the deprecation of each bound declaration. It also runs the checks for degenerate statements of spec 10.4 on each bound theorem. `m2 build` runs `lake build`, runs M2Extract on each binding, replays each binding that needs it in six kernels, computes the status of each node and writes the graph, with a cache for each step. The `status` package computes the evidence level, the fidelity level and the flags of spec 04.7, and `m2 status` shows them. A review counts only when its reviewer has a record in `registers/reviewers.reg` and did not write the binding. F3 needs two reviewers with no shared affiliation. The `release` package makes the release manifest of spec 06.2 from a tree and its last build. It rebuilds the graph of a release from its commit (rule R01), and it rebuilds the `.olean` files and lists each file that is different (rule R02). The command `m2 release` comes in M2. The first work of M2 is written: the `store` package puts the graph and the status of each node into one SQLite file, and `m2 graph` runs the neighbourhood, closure, reverse dependency, learning path and frontier queries on it. The `synth` package and `bench/graph` measure these queries on a synthetic graph. The `ingest` package and `m2 ingest` get pinned snapshots of external sources, apply the licence gate of spec 09.3 and write the records. The adapters of arXiv, zbMATH Open, Mathlib and the OEIS are written. The rules L01 to L04 check the licences. Rule A01 checks that each `same_as` id has the syntax of its system, with the `crosswalk` package. The report shows each other rule of spec 15.3 with the state skip and the reason "not written yet". `m2 help` shows the milestone that delivers each command. A command that is not written yet exits with code 3 and names its milestone. It does not exit with code 0, because a command that does nothing and exits with code 0 looks like a command that worked.

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
| `m2 build` | Build the derived data: Lean, extraction, replay, status, graph and store. Search comes later. | written |
| `m2 audit` | Run the audit rules and write the report to `reports/`. | written |
| `m2 status <tag>` | Show the computed status of a node and the evidence for it. | written |
| `m2 graph` | Run a graph query: neighbourhood, closure, reverse dependencies, path or frontier. | written |
| `m2 ingest <source>` | Get a pinned snapshot of an external source and write its records. `-snapshot HASH` uses a recorded snapshot again. | written |
| `m2 align <source>` | Propose crosswalk edges to an external source. | M2 |
| `m2 bump` | Make a branch that moves Lean and Mathlib to the next release. | M2 |
| `m2 release` | Cut, describe or verify a release. | M2 |
| `m2 search <query>` | Search the knowledge graph. | M3 |
| `m2 serve` | Serve the API, the MCP server and the workspace. | M3 |
| `m2 compute` | Run or verify a computation. | M3 |
| `m2 prov` | Show or change provenance. | M3 |
| `m2 review <tag>` | Review a formal statement against its informal statement. | M4 |
| `m2 autoform <tag>` | Make a formal statement for a node. | M4 |
| `m2 agent run <role>` | Do one agent run. | M5 |
| `m2 orchestrate` | Run the job scheduler. | M5 |

## The wiki

`m2 site build` makes the wiki at https://tamnd.github.io/math2 from a math2 working tree and the spec. The math2 repository runs it on each push to `main` and each night, and GitHub Pages serves the result.

```sh
go run ./cmd/m2 site build -content ../math2 -spec spec -out _site
python3 -m http.server -d _site 8000
```

Each node gets a page with an infobox, its links and its backlinks. KaTeX renders the math in the browser with the macros of math2. Each page also has a copy for programs: `node/<TAG>.md`, `api/node/<TAG>.json`, `api/nodes.json`, `llms.txt` and `llms-full.txt`. A build with no `-date` flag gives the same bytes each time.

To preview the site with the local server above, set `-url http://localhost:8000`.

## The build

`m2 build` makes the derived data of a math2 working tree in `build/`. Run it in the root of the tree. It needs elan for the Lean steps.

```sh
m2 build                # all the steps
m2 build -only lean     # only lake build
m2 build -json          # write the manifest as JSON
m2 build -no-sandbox    # run the checkers without the sandbox, for example on macOS
```

The steps are `lean`, `extract`, `replay`, `status`, `graph` and `store`. The `lean` step gets the Mathlib cache and runs `lake build` in `lean/`. The `extract` step runs M2Extract on each formal declaration and adapter that a node names, and on each declaration with the attribute `@[m2 "TAG"]`. It writes `build/extract.json`. The `replay` step checks each binding that passes each check of E5 except the replay, and writes `build/replay.json`. The `status` step computes the evidence level and the fidelity level of each node from the nodes, `build/extract.json`, `build/replay.json` and the reviews in `reviews/`, and writes `build/status.json`. The `graph` step writes the graph as Parquet to `build/graph/`. The `store` step writes the graph, the status of each node and the aliases to the SQLite file `build/store.db`, for `m2 graph`. A tree without `lean/` has only the `status`, `graph` and `store` steps.

`build/manifest.json` records the Lean toolchain, the Mathlib commit, the hash of each checker program of the toolchain, and the input hash and output hash of each step. When the inputs of a step and its files do not change, the step does not run again. The output hash of the build covers `extract.json`, `replay.json`, `status.json`, `graph/` and `store.db`.

## The replay of a proof

The `replay` package checks a formal declaration again, outside the process that elaborated it (spec 07.7). It writes a challenge module that states the declaration again from its record, with `sorry` as the proof. It exports the challenge and the module of the declaration with `leanexport`, and runs `lake comparator --paranoid` on the two exports. The declaration passes when it has the statement of the challenge, uses only the standard axioms, and each kernel of the toolchain accepts it. The kernels are `leanchecker`, `leanchecker-paranoid`, `lean4lean`, `nanoda_bin`, `con-leche` and `con-ron`. A check takes about 20 seconds for a declaration that imports Mathlib.

The comparator runs the checkers in a `bwrap` sandbox, and `bwrap` needs Linux. On macOS, `m2 build -no-sandbox` runs the checkers without the sandbox. A replay without the sandbox does not give E5, and `m2 status` gives the reason. E5 also needs two kernels that accept the proof. Two builds of the same commit give the same output hash (rule R01).

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

## Ingest

`m2 ingest` gets a snapshot of an external source through its bulk channel, records the snapshot in `manifests/sources.yaml` and writes the records to `build/records/<source>.jsonl`. The import manifest `manifests/ingest/<source>.yaml` names the snapshot, the adapter and the hash of the records. Thus two imports of the same snapshot give the same bytes, and anyone can check it.

```sh
m2 ingest arxiv                        # a new snapshot from the bulk channel of the source
m2 ingest arxiv -snapshot blake3:...   # the recorded snapshot again, from the cache or from its URL
m2 ingest zbmath -url 'https://oai.zbmath.org/v1/?verb=ListRecords&metadataPrefix=oai_zb_preview&set=11'
m2 ingest mathlib                      # the declarations of Mathlib at the commit that lean/lake-manifest.json pins
```

These sources have an adapter now:

| Source | Package | Bulk channel | Licence | What a record keeps |
|---|---|---|---|---|
| `arxiv` | `ingest/arxiv` | the metadata plane of tamnd/arxiv, one JSONL file for each month | CC0 metadata; each paper has the licence of its last version | the mathematics papers only: title, authors, categories, MSC codes, DOI and the `ax://` locator. No text. |
| `mathlib` | `ingest/mathlib` | M2Extract in the Lake project `lean/` of the tree, at the pinned commit of Mathlib | Apache 2.0 | each declaration of Mathlib: kind, module, the hash of the canonical type, the constants that the value uses, the docstring as text and the link to the documentation. |
| `oeis` | `ingest/oeis` | `https://oeis.org/names.gz`, the names of all sequences | CC BY-SA 4.0 | each sequence: the A-number and the name as the title. No other text. |
| `zbmath` | `ingest/zbmath` | the OAI-PMH interface of zbMATH Open, `oai_zb_preview` | CC BY-SA 4.0 | each document: title, authors, MSC codes, Zbl number, DOI, the source as the locator and the ids of the references. No review text. |

A snapshot that has more than one file, such as an OAI-PMH harvest, is the files one after the other, compressed with gzip. The zbMATH adapter fails when the number of records is not the size of the list that zbMATH Open gives. The full zbMATH harvest has more than four million documents, so give a set to harvest one MSC class.

The Mathlib adapter downloads nothing. It runs M2Extract on the .olean files of the tree, so run `m2 build` first. The catalogue records the commit of Mathlib with the snapshot. When the tree moves to a new tag of Mathlib, run `m2 ingest mathlib` again to get the records of the new tag. At the commit 56a0058 of Mathlib, for Lean 4.35.0, the adapter gives 358,102 records in less than 5 minutes on a laptop. Of these, 90,326 have a docstring. The snapshot is 50 MB.

Each source has an access class: `open`, `share-alike-nc`, `verbatim`, `gfdl` or `record` (spec 09.3). The licence gate stores the text of an item only when its class is `open`. For the other classes it keeps the record, the locator and the fingerprints of the text. The fingerprints cannot give the text back. The audit uses them to find copied text:

| Rule | What it checks |
|---|---|
| L01 | Each source names a licence and a class that agrees with it. Each import manifest names a recorded snapshot. The records agree with the manifest and store only the text that their class allows. |
| L02 | No node body copies text from a source that is not `open`. |
| L03 | Each share-alike source has an attribution line, and a node that uses its text names the item in `sources` or `same_as`. |
| L04 | A node body that is similar to an item that it cites. A person checks it. |

L02 to L04 need the records in `build/records/`. When they do not exist, the rules skip. Each source has an adapter in the package `ingest/<source>`. An adapter whose bulk channel has more than one file also implements `ingest.Fetcher`. To add a source, write the adapter, register it with `ingest.Register`, import its package in `cmd/m2`, and ask a maintainer to approve the class in `manifests/sources.yaml`.

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
audit/    the audit rules of spec 15, with a fixture pair for each rule
bench/    the benchmarks: bench/graph measures the graph queries of spec 05.6
build/    the steps of m2 build, their cache and the build manifest
check/    the fast rules of m2 check: schema, body and links
crosswalk/ the syntax of the ids of each external system of same_as (spec 05.3)
cli/      the command table
format/   the canonical form of m2 fmt and the change from aliases to tags
graph/    the graph of spec 05: edges, closure, cycles, CSR and Parquet
ingest/   the adapters of the external sources, the licence gate, the snapshots and the import manifests (spec 09)
lean/     M2Extract, its Lake package and the Go package that drives it
node/     read node files, write the canonical form and compute the content hash
release/  the release manifest of spec 06.2 and the rebuild checks R01 and R02
replay/   the replay of a declaration in the comparator and the independent kernels
schema/   check front matter against the node schema
site/     build the wiki
store/    the serving store: the graph and the status of each node in SQLite, and the queries of spec 05.6
status/   the evidence level, the fidelity level and the flags of each node (spec 04.7), and the reviews
synth/    synthetic graphs for the benchmarks
tag/      tags, provisional ids, the tag register and the alias register
uri/      m2:// URIs
spec/     the specification
```

Each directory except `cmd/`, `bench/` and `spec/` is a Go package that other programs can import. For example, a bot can use `github.com/tamnd/math2-agent/node` to read and format node files. The packages follow the conventions of the Go standard library, and `go doc` shows an example for each main function.

The full layout arrives with the milestones. The plan has these parts: `compute`, `align`, `search`, `api`, `mcp`, `orchestrator`, `agent`, `prompts/`, `images/` and `web/`.

## Contribute

Read [CONTRIBUTING.md](CONTRIBUTING.md). To report a security problem, read [SECURITY.md](SECURITY.md).

## Licence

Apache-2.0. Refer to [LICENSE](LICENSE).
