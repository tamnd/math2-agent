# math2-agent

This repository holds the toolchain and the research agents for Math 2.0. The content is in [tamnd/math2](https://github.com/tamnd/math2).

Math 2.0 is a versioned knowledge graph of mathematics. The `m2` command reads the node files in math2. It builds the Lean library, computes the status of each node, makes the graph and checks each change with the audit rules. Later milestones add search, an API, an MCP server, a research workspace and AI research agents.

## Specification

The design is in [spec/](spec/00-README.md). It has 22 documents, 00 to 21. Issues, code and reviews refer to a section by its number, for example 04.5 or 15.3 S03.

The wiki at https://tamnd.github.io/math2 shows the spec, the milestones and each node.

## Status

The project is at milestone M1. Release v0.1.0 closes the code of M0: `m2 new`, `m2 fmt`, `m2 check`, `m2 mint` and `m2 site build` are written. In M1, `m2 audit` runs the rules T01 to T06, S01 to S04, S09, B01 to B05, G01 to G10, F01 to F07, V01, V03 and V04. The rules G04, F01 to F05 and F07 read the files in `build/` from the last `m2 build`. A rule skips when a file that it needs does not exist. The `graph` package derives the graph from the node files, with the AND-OR closure of spec 05.5, the CSR adjacency and the Parquet build format. The `lean` package drives M2Extract, a small Lean program that gives the type hash, the axioms and the formal dependencies of each bound declaration. `m2 build` runs `lake build`, runs M2Extract on each binding, replays each binding that needs it in six kernels, computes the status of each node and writes the graph, with a cache for each step. The `status` package computes the evidence level, the fidelity level and the flags of spec 04.7, and `m2 status` shows them. The report shows each other rule of spec 15.3 with the state skip and the reason "not written yet". `m2 help` shows the milestone that delivers each command. A command that is not written yet exits with code 3 and names its milestone. It does not exit with code 0, because a command that does nothing and exits with code 0 looks like a command that worked.

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
| `m2 mint` | Change the provisional ids to tags before merge, and add the tags to `registers/tags.reg`. | written |
| `m2 site build` | Build the wiki: a static site for people and agents. | written |
| `m2 build` | Build the derived data: Lean, extraction, replay, status and graph. Search comes later. | written |
| `m2 audit` | Run the audit rules and write the report to `reports/`. | written |
| `m2 status <tag>` | Show the computed status of a node and the evidence for it. | written |
| `m2 graph` | Run a graph query: closure, reverse dependencies, path or frontier. | M2 |
| `m2 ingest <source>` | Get a pinned snapshot of an external source. | M2 |
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

The steps are `lean`, `extract`, `replay`, `status` and `graph`. The `lean` step gets the Mathlib cache and runs `lake build` in `lean/`. The `extract` step runs M2Extract on each formal declaration and adapter that a node names, and on each declaration with the attribute `@[m2 "TAG"]`. It writes `build/extract.json`. The `replay` step checks each binding that passes each check of E5 except the replay, and writes `build/replay.json`. The `status` step computes the evidence level and the fidelity level of each node from the nodes, `build/extract.json`, `build/replay.json` and the reviews in `reviews/`, and writes `build/status.json`. The `graph` step writes the graph as Parquet to `build/graph/`. A tree without `lean/` has only the `status` and `graph` steps.

`build/manifest.json` records the Lean toolchain, the Mathlib commit, the hash of each checker program of the toolchain, and the input hash and output hash of each step. When the inputs of a step and its files do not change, the step does not run again. The output hash of the build covers `extract.json`, `replay.json`, `status.json` and `graph/`.

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

The output gives the evidence level, the fidelity level, the flags, each item of evidence, each Lean binding with its type hash, its axioms, its reviews and its replay, and one reason for each decision. For an E5 result, it also gives the checker versions. A tag that is not in the build exits with code 1.

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
build/    the steps of m2 build, their cache and the build manifest
check/    the fast rules of m2 check: schema, body and links
cli/      the command table
format/   the canonical form of m2 fmt and the change from aliases to tags
graph/    the graph of spec 05: edges, closure, cycles, CSR and Parquet
lean/     M2Extract, its Lake package and the Go package that drives it
node/     read node files, write the canonical form and compute the content hash
replay/   the replay of a declaration in the comparator and the independent kernels
schema/   check front matter against the node schema
site/     build the wiki
status/   the evidence level, the fidelity level and the flags of each node (spec 04.7), and the reviews
tag/      tags, provisional ids, the tag register and the alias register
uri/      m2:// URIs
spec/     the specification
```

Each directory except `cmd/` and `spec/` is a Go package that other programs can import. For example, a bot can use `github.com/tamnd/math2-agent/node` to read and format node files. The packages follow the conventions of the Go standard library, and `go doc` shows an example for each main function.

The full layout arrives with the milestones. The plan has these parts: `compute`, `ingest`, `align`, `search`, `api`, `mcp`, `orchestrator`, `agent`, `prompts/`, `images/`, `web/` and `bench/`.

## Contribute

Read [CONTRIBUTING.md](CONTRIBUTING.md). To report a security problem, read [SECURITY.md](SECURITY.md).

## Licence

Apache-2.0. Refer to [LICENSE](LICENSE).
