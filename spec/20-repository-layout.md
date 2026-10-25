# 20. Repository layout

There are two repositories. They use the same split as `tamnd/arxiv` and `tamnd/arxiv-reader`:

- `tamnd/math2` is the **content**. It is the thing that must be right.
- `tamnd/math2-agent` is the **toolchain and the agents** that build, check, grow and serve the content. Its binary is `m2`.

Content never depends on the toolchain's code. A person with a text editor, `lake` and Python can read, edit and check the core of `tamnd/math2`. For this, they never need to install `m2`. The full audit needs `m2`, in the same way as the full audit of `tamnd/arxiv` needs `ax`.

## 20.1 `tamnd/math2`

```
math2/
  README.md                      what it is, the counts table (generated), how to check it
  LICENCE                        CC BY-SA 4.0 (prose), Apache-2.0 (code), CC0 (metadata). See 16.6
  CONTRIBUTING.md                how people and agents contribute; disclosure; review
  CITATION.cff                   how to cite a release and a node

  schema/
    node.v1.json                 JSON Schema for node front matter, per kind
    review.v1.json
    computation.v1.json
    manifest.v1.json
    edges.md                     the 23 edge types (document 05.2), generated from the schema

  macros/
    m2.sty                       the only LaTeX macros a body may use, KaTeX/MathJax compatible
    m2.katex.json                the same macros for KaTeX

  nodes/
    7K/7K3QX2.md                 one file per node, sharded by the first two tag characters
    ...

  compute/
    3MZ8QD/main.gp               code for computation nodes, next to the tag
    3MZ8QD/Check.lean

  reviews/
    7K/7K3QX2.yaml               fidelity reviews, bound to statement and type hashes

  prov/
    7K/7K3QX2.jsonl              provenance, one line per field change

  registers/
    tags.reg  aliases.reg  retired.reg  bindings.reg  reviewers.reg  crosswalk.reg

  queues/
    align/wikidata.jsonl         proposed same_as, close_to and aligned edges, one file for each external system (16.3)
    dedup.jsonl                  proposed duplicate pairs (05.4)

  lean/                          the math2 Lean library (a Lake package)
    lakefile.toml                requires mathlib at the pinned commit
    lean-toolchain               the pinned Lean release
    Math2.lean
    Math2/
      Meta/Attr.lean             @[m2 "TAG"] attribute, the binding marker
      Meta/Match.lean            adapters: stating that a Mathlib decl matches a node up to a recorded transformation
      Algebra/ …                 statements and proofs math2 holds that Mathlib does not (yet)
      NumberTheory/ …
      Conjectures/ …             formal statements of open conjectures (Prop-valued defs, never axioms)
      Hypotheses/ …              the only place `axiom` may appear: named hypotheses such as GRH, for conditional results. Each is a node
      Cert/ …                    certificate checkers proved sound (LRAT, Pratt, interval)
    test/                        golden tests for the attribute and the adapters

  topics/
    msc2020.yaml                 MSC2020 codes → topic tags, with our own short labels. The official class descriptions are CC BY-NC-SA, so they are
                                 not stored here; the site fetches them from a pinned snapshot and shows them with attribution (16.6)
    curricula/                   reading lists (undergraduate core, qualifying exams). Each one is a topic with ordered members
    skeletons/                   the C2 skeleton of each top-level MSC2020 class, one file NN.yaml for each class (18.3)

  manifests/
    releases/2026.11.yaml        release manifests (document 06.2)
    releases/2026.11.levels      the evidence and fidelity level of each live node at the release
    releases/2026.11.md          the release notes: a summary by a person, and the sections that m2 release notes writes
    sources.yaml                 every external snapshot used: URL, hash, date, licence (document 09)
    images.yaml                  computational images by digest
    ingest/                      per-source import manifests
    coverage/expected.yaml       the pinned number of documents in each MSC2020 class, the denominator of C0 (18.2)

  reports/
    audit/                       committed audit reports (document 15)
    bump/                        one report for each Mathlib bump: the breaks by kind, the time to green, the stale reviews (07.9)
    graph/                       graph health
    coverage/                    per-MSC coverage tables (document 18)
    releases/                    the full pre-release report per release
    restore/                     one record for each quarterly restore drill (06.6, rule R04)
    agents/                      weekly agent acceptance and cost reports (document 12.9)
    eval/                        per-release evaluation reports (document 17.8)

  .github/
    workflows/                   check.yml (schema+body+Lean build on PR), audit.yml (full on main), bump.yml (weekly Mathlib bump), release.yml
    scripts/check_nodes.py       the Python-only check that needs no m2: schema, tags, links, sections
```

### The Lean package

`lean/` is an ordinary Lake package named `Math2`. It depends on Mathlib at the pinned commit. It holds only what Mathlib does not have. A node bound to a Mathlib declaration has no Lean code in math2 at all. It has only the binding in its front matter.

Sometimes math2 holds a theorem and Mathlib gains it later. Then the bump agent proposes two changes: the binding moves to Mathlib, and the math2 copy becomes deprecated (document 07.9). The aim is that `Math2/` gets smaller in each area as Mathlib grows. The aim is also that math2's Lean contributions move upstream (document 16.5).

`@[m2 "7K3QX2"]` on a declaration is the binding seen from the Lean side. The build checks that the attribute and the front matter agree (rule `F01`). The attribute is the only Lean metaprogramming in the content repository. Everything else that must inspect Lean (dependency extraction, type hashing, axiom audit) is in the toolchain.

## 20.2 `tamnd/math2-agent`

It is a Go module for the toolchain and the agents. It also has a small Lean package for extraction and a web front end for the workspace.

```
math2-agent/
  README.md
  go.mod                         module github.com/tamnd/math2-agent
  cmd/
    m2/                          the binary

  check/                         the fast rules of m2 check: S01 to S04, B01 to B03, G01 (15.3)
  cli/                           the command table and the flags of each command
  format/                        m2 fmt: canonical front matter, aliases to tags (04.4, 04.5)
  node/                          parse, canonicalise, hash node files (document 04)
  schema/                        schema validation, migrations
  tag/                           minting, provisional id rewrite, registers
  graph/                         derive edges, closure (AND–OR), CSR, Parquet writer (document 05)
  store/                         SQLite + DuckDB + CSR + vector serving store (05.7)
  build/                         the build graph: nodes → Lean → compute → graph → search → site
  bump/                          the Mathlib bump: pins, breaks, renames and the bump report (07.9)
  compute/                       image runner, sandbox, certificate dispatch (document 08)
  ingest/                        source adapters (document 09), one subpackage per source
    arxiv/  zbmath/  crossref/  oeis/  lmfdb/  wikidata/  mardi/  nlab/  stacks/  proofwiki/  mathlib/  leanlibs/
    afp/  palomar/  blueprint/  erdos/  formalconj/  mo/  objects/  theoremidx/
  extract/                       statement extraction from papers (09.5)
  align/                         dedup and crosswalk (05.3, 05.4)
  autoform/                      statement autoformalization and fidelity pipeline (document 10)
  search/                        lexical, formula, embedding, type-directed search (document 14)
  audit/                         one file per rule, with fixtures (document 15)
  prov/                          provenance writing, PROV export, quarantine (06.5)
  release/                       release manifest, notes, archive (06.2)
  api/                           the Mathematical API: HTTP+JSON, gRPC (document 11)
  mcp/                           the MCP server over the API
  orchestrator/                  job queue, scheduler, budgets, run records (document 11)
  agent/                         the research agents (document 12)
    formalize/  prove/  explore/  explain/  curate/  align/  bump/  review-assist/
  llm/                           model clients (Anthropic SDK first; adapters for local prover models)
  sandbox/                       gVisor/Firecracker wrappers shared by compute and agents
  uri/                           m2:// parser and the ant driver
  site/                          the wiki: a static site for people and agents

  lean/                          drive Lake, the Lean REPL and the extraction package; axiom audit; replay (document 07)
    M2Extract/                   Lean package: dependency graph, elaborated-type hash, axioms, statement pretty-printing, degenerate-statement checks
    M2Extract/lakefile.toml      depends on Mathlib and on tamnd/math2's Math2 package at the pinned commits

  prompts/                       versioned prompts per agent role; each change bumps the role version (12.8)
  images/                        Nix flakes for every computational image (08.1)
  web/                           the research workspace front end (document 13)
  bench/
    graph/                       Q1 to Q12 latency benchmarks (05.6)
    search/                      retrieval benchmarks (14.7)
    fidelity/                    the F1 battery suite (17.3.1)
    extract/                     extraction and unfolding (17.3.2)
    align/                       alignment precision and recall (17.3.3)
    agents/                      per-role suites (public and fresh splits; held-out items live in a private repository) and canaries (17.3.4, 12.7)
    explain/                     explanation grading (17.3.5)
    novelty/                     the novelty check's recall suite (17.5)
    external/                    runners for external benchmarks (17.6)
    prov/                        provenance tests (06.5)
  migrate/                       schema migrations (04.9)
  docs/                          command reference, generated
  spec/                          this specification (documents 00 to 21)
```

The module has no `internal/` directory. Each Go directory is a package that other programs can import, for example a bot that reads node files with `node` and checks them with `schema`. Thus each package follows the conventions of the Go standard library. It has a package comment, a doc comment on each exported name and testable examples. It exports only what other programs need. A change to an exported name is a breaking change, and the release notes say so.

### The command set

Each command names the milestone in which it arrives (document 19). If the milestone of a command has not arrived, the command exits non-zero and says which milestone delivers it. It does not pretend to succeed. That is the `ax` convention.

| Command | What it does | Milestone |
|---|---|---|
| `m2 new <kind>` | scaffold a node with a provisional id | M0 |
| `m2 fmt` | canonicalise front matter, rewrite aliases to tags | M0 |
| `m2 check` | schema, body, links on the working tree (fast, no Lean) | M0 |
| `m2 mint [-commit C]` | change provisional ids to tags before merge, and add them to `registers/tags.reg` | M0 |
| `m2 site build` | the static wiki for GitHub Pages: HTML for people, JSON, Markdown and `llms.txt` for agents | M0 |
| `m2 build` | full build: Lean, extraction, status, graph, search | M1 |
| `m2 audit [-plane X] [-rule R] [-changed] [-release R]` | the audit (document 15) | M1 |
| `m2 status <tag>` | the computed status and the evidence behind it | M1 |
| `m2 graph closure|rdeps|path|frontier <tag>` | graph queries Q3 to Q7 | M2 |
| `m2 ingest <source> [--snapshot]` | pull a pinned snapshot of a source (document 09) | M2 |
| `m2 align <source>` | propose crosswalk edges | M2 |
| `m2 search <query>` | the search engine (document 14) | M3 |
| `m2 serve [--addr]` | the API, the MCP server and the workspace | M3 |
| `m2 compute run|verify <tag>` | the computational engine (document 08) | M3 |
| `m2 review <tag>` | the reviewer's tool: side-by-side informal and formal, back-translation, checks, sign | M4 |
| `m2 autoform <tag>` | statement autoformalization for a node | M4 |
| `m2 agent run <role> [--target T] [--budget B]` | one agent run | M5 |
| `m2 orchestrate` | the scheduler daemon | M5 |
| `m2 bump` | Mathlib/Lean bump branch (document 07.9) | M2 |
| `m2 prov show|quarantine|credit|disclose` | provenance operations; `disclose` renders a disclosure paragraph and `formalization.yaml` (16.5) | M3 |
| `m2 release cut|notes|verify` | releases | M2 |

### Why Go for the toolchain

The rest of the `tamnd` toolchain is Go: `ax`, `ant`, and the `*-cli` family. `m2 ingest` links the packages of that family as libraries. Go builds one static binary and cross-compiles easily. It has the official Anthropic SDK, DuckDB and SQLite bindings, and good sandbox control. Lean-specific work happens in Lean (`lean/M2Extract`), driven over the Lean REPL protocol. Python appears only inside computational images and in the no-dependency check script in `tamnd/math2`.

One exception may come later: a Python sidecar for local open-weight prover models (document 12.3). The reason is that the inference stacks of these models are in Python. If the sidecar exists, it sits behind a gRPC interface in `llm`. Nothing else in the toolchain knows about it.

## 20.3 How the two repositories move together

- `tamnd/math2-agent` is versioned by semver. `tamnd/math2` is versioned by calendar (document 06.2).
- Each math2 release names the `m2` version that built it. CI in `tamnd/math2` uses the `m2` version pinned in `manifests/releases/next.yaml`. A person bumps that version in a PR.
- A schema change lands in `math2-agent` first, behind a flag. Then a migration PR lands in `math2`. Then the flag is removed. The two repositories never change in a single step.
- Agents open PRs against `tamnd/math2` from a bot account (document 16.2), with branch names `agent/<role>/<run-id>`. A person merges them, or for some roles a rule-limited auto-merge does (document 12.1).
