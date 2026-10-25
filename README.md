# math2-agent

This repository holds the toolchain and the research agents for Math 2.0. The content is in [tamnd/math2](https://github.com/tamnd/math2).

Math 2.0 is a versioned knowledge graph of mathematics. The `m2` command reads the node files in math2. It builds the Lean library, computes the status of each node, makes the graph and checks each change with the audit rules. Later milestones add search, an API, an MCP server, a research workspace and AI research agents.

## Specification

The design is in [spec/](spec/00-README.md). It has 22 documents, 00 to 21. Issues, code and reviews refer to a section by its number, for example 04.5 or 15.3 S03.

The wiki at https://tamnd.github.io/math2 shows the spec, the milestones and each node.

## Status

The project is at milestone M0. `m2 site build` is written. `m2 help` shows the milestone that delivers each command. A command that is not written yet exits with code 3 and names its milestone. It does not exit with code 0, because a command that does nothing and exits with code 0 looks like a command that worked.

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
| `m2 build` | Build everything: Lean, extraction, status, graph and search. | M1 |
| `m2 audit` | Run the audit rules. | M1 |
| `m2 status <tag>` | Show the computed status of a node and the evidence for it. | M1 |
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
check/    the fast rules of m2 check: schema, body and links
cli/      the command table
format/   the canonical form of m2 fmt and the change from aliases to tags
node/     read node files, write the canonical form and compute the content hash
schema/   check front matter against the node schema
site/     build the wiki
tag/      tags, provisional ids, the tag register and the alias register
uri/      m2:// URIs
spec/     the specification
```

Each directory except `cmd/` and `spec/` is a Go package that other programs can import. For example, a bot can use `github.com/tamnd/math2-agent/node` to read and format node files. The packages follow the conventions of the Go standard library, and `go doc` shows an example for each main function.

The full layout arrives with the milestones. The plan has these parts: `graph`, `lean`, `compute`, `ingest`, `align`, `search`, `audit`, `api`, `mcp`, `orchestrator`, `agent`, `lean/M2Extract`, `prompts/`, `images/`, `web/` and `bench/`.

## Contribute

Read [CONTRIBUTING.md](CONTRIBUTING.md). To report a security problem, read [SECURITY.md](SECURITY.md).

## Licence

Apache-2.0. Refer to [LICENSE](LICENSE).
