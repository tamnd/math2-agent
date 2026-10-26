# 11. The Mathematical API and the agent orchestrator

In the diagram, one box is between the Research Workspace and the Research Agents: "Mathematical API and Agent Orchestrator". This box has two roles in one process:

- the **Mathematical API** is the only way for anything outside the build to read the knowledge graph or to ask for work. This includes a person's browser, an agent and an external tool;
- the **Orchestrator** changes requests for work into agent runs. It schedules the runs, enforces budgets and records everything. It changes the results into pull requests.

Both are in `tamnd/math2-agent` (`api`, `mcp`, `orchestrator`). `m2 serve` and `m2 orchestrate` serve them.

## 11.1 Principles

1. **Read is free, write is a pull request.** The API serves reads from the derived store of a release, or of `main`. Every write becomes a branch and a pull request against `tamnd/math2`. This is true for a write from a person in the workspace and for a write from an agent. The API has no other write path. Thus the audit (document 15) is the only gate, and git is the only history.
2. **One surface, three encodings.** The API uses HTTP+JSON for browsers and scripts, gRPC for the agents' tool calls inside the cluster, and MCP for any model client. One interface definition generates all three: `api/m2.proto` with HTTP annotations. Thus the three encodings cannot drift apart.
3. **Every response says what it is about.** It carries the release or commit that served it, the tag's content hash and the evidence summary. An agent that reasons from an API response can cite exactly what it saw.
4. **The API never returns a status it did not compute.** It serves the status that the build computed (document 04.7). It does not pass through any statement about truth from an external system.

## 11.2 The read surface

The methods are in groups by purpose. Each method is also an `m2` subcommand and an MCP tool with the same name.

### Resolve and fetch

| Method | Input | Output |
|---|---|---|
| `resolve` | any identifier: tag, alias, `m2://` URI, Lean name, QID, A-number, Stacks tag, arXiv result tag, DOI | tag(s) with match type, or "not held" plus the external system |
| `get` | tag[@release] | node: front matter, body, computed status, evidence summary |
| `get_formal` | tag | bindings, with the elaborated statement pretty-printed, axioms, replay result |
| `get_source` | tag | sources with locators and aligned paper results |
| `history` | tag | revisions with provenance, binding changes, review history |

### Navigate

| Method | Input | Output |
|---|---|---|
| `neighbours` | tag, edge types, depth ≤ 3 | typed subgraph |
| `closure` | tag, strategy (min-depth / min-size), stop set | AND–OR closure with trust profile (05.5) |
| `rdeps` | tag | everything whose every proof depends on it |
| `path` | from, to, edge filter | prerequisite path (05.6 Q5) |
| `frontier` | topic, kind, evidence filter | ranked frontier lists (Q6, Q7) |
| `topic` | MSC code or topic tag | the topic page data: members by kind and level, coverage |
| `diff` | topic, release₁, release₂ | changes (Q10) |

### Search

| Method | Input | Output |
|---|---|---|
| `search` | free text (natural language and/or LaTeX) | ranked nodes with snippets, scores and channel attribution (document 14) |
| `search_formal` | a Lean type pattern, or a Lean goal | bound nodes and Mathlib declarations, Loogle-style (14.3) |
| `search_formula` | LaTeX formula | formula-structure matches (14.2) |
| `similar` | tag | nearest nodes in embedding space, with the judge label when one is cached |
| `premises` | Lean goal state + file context | ranked candidate lemmas for a proof step (14.5) |

### Check and compute

These methods do work, but they do not write content. They return their results. The results are also cached by hash.

| Method | Input | Output |
|---|---|---|
| `lean_check` | Lean source snippet + imports at the pinned toolchain | elaboration result, messages, goals, axioms |
| `lean_goal` | snippet + position | goal state (used by provers via REPL sessions, 11.3) |
| `fidelity_check` | tag + candidate Lean statement | the F1 battery: back-translation, degenerate checks, critics (document 10.4) |
| `compute` | computation spec (image, entry, inputs) | output, hash, certificate check result (document 08) |
| `counterexample` | Lean statement or informal claim + domain hints | SMT/SAT/brute-force search result |

### Propose (writes, via pull request)

| Method | Effect |
|---|---|
| `propose_node` | open a PR with a new node (provisional id) |
| `propose_edit` | open a PR editing allowed fields of a node |
| `propose_binding` | open a PR adding a formal binding plus its Lean code |
| `propose_alignment` | open a PR adding `same_as`/`close_to`/`aligned` edges |
| `request_review` | file a review task for a binding (document 10.6) |
| `request_work` | file an orchestrator job (11.4) |

People use the same propose methods from the workspace. The PR that results is attributed to the person, and the workspace is named as the tool.

## 11.3 Lean sessions

A proof is interactive work: try a tactic, look at the new goals, then try a different tactic. One HTTP call for each attempt with a full re-elaboration is much too slow. Thus the API gives **Lean sessions**:

- A session is a Lean REPL process (the community `repl`, or Pantograph-style goal-level access). It starts from the pinned toolchain with Mathlib and `Math2` imported. The process is snapshotted after the imports. Thus a session costs milliseconds to start, not the 10 to 30 s that it takes to load Mathlib.
- A session holds a proof state tree. The calls are `run_tactic(state_id, tactic) → new states | error`, `goal(state_id)`, `branch(state_id)` and `close()`.
- Sessions run in the sandbox (document 08.6), with a memory cap and a timeout for each call. When a tactic times out, only its own call stops.
- Each runner has a session pool of a set size. The orchestrator charges session-seconds to the budget of the run.

The `lean-lsp-mcp` and Kimina-server style tools show the same interface (document 01). math2 uses its own interface because it must pin it to the release toolchain, sandbox it and meter it. Because of the MCP surface, any agent that can use MCP can prove things against math2 directly. This includes Claude Code with no math2-specific code.

## 11.4 The orchestrator: jobs

A **job** is a unit of work with a goal, a target and a budget. Jobs come from four places:

1. **People**, through the workspace or `request_work` ("formalise the statement of 7K3QX2", "find a counterexample to this").
2. **Standing queues** that the orchestrator computes from the graph: the frontier lists (Q6, Q7, Q12). Examples: each E3 statement with an F2 formal statement and no formal proof becomes a `prove` job. Each node with no explanation at L2 becomes an `explain` job. Each unconfirmed alignment older than 30 days becomes a `align` job.
3. **Events**: a Mathlib bump breaks bindings (→ `bump` jobs), a review becomes stale (→ `review-assist` jobs), a new arXiv batch arrives (→ `extract` and `align` jobs).
4. **Campaigns**: a project that a person defines, with a target set. Examples are "the undergraduate core list to F2" and "the Erdős problems in additive combinatorics to formal statements". A campaign is a topic node plus a goal predicate. Human research projects use the agents through campaigns (document 16.3).

Each job has this record:

```yaml
job: j-2026-11-02-41a7
role: prove
target: 7K3QX2
goal: "binding formal[0] reaches E5"
priority: 0.73            # computed (11.5)
budget: {usd: 4.00, wall: 2h, lean_session_s: 3600, compute_cpu_s: 7200}
on_behalf_of: campaign:ug-core   # or a person, or schedule:standing-prove
requested_by: orcid:0000-0002-1825-0097
constraints: {models: [claude-opus-5-5, claude-sonnet-5-5], no_native_decide: true}
```

### Priority

Priority is a score, not a decision by a person. Its formula has versions, as a prompt has:

```
priority = w1 · centrality(target)          # PageRank on the uses graph, so foundational nodes first
         + w2 · demand(target)              # workspace views, citations, campaign membership
         + w3 · unlock(target)              # how many frontier items this would move a level
         + w4 · tractability(target, role)  # learned: predicted success from past runs on similar targets
         − w5 · attempts(target, role)      # diminishing returns after failed attempts
```

`unlock` is the most important term. For example, 200 E1 statements use a lemma, and each one needs only that lemma. A proof of that lemma has more value than a proof of any one of the 200 statements. The closure structure in document 05.5 lets the orchestrator compute this.

### Scheduling

- Jobs go into one queue for each role. The queues share fairly across campaigns, so that one project cannot use all of the budget.
- **Portfolio scheduling.** A hard target gets several cheap attempts with different strategies before one expensive attempt. The strategies use different models, different retrieval and different decompositions. This is the method that worked in the provers of 2025 to 2026 (document 01): many different samples, with a verifier as the filter, are better than one long attempt.
- **Escalation ladder.** A job starts at the cheapest configuration that has some recorded success on similar targets. If the job fails, it moves up the ladder: Haiku 5.5 or a small open prover, then Sonnet 5.5, then Opus 5.5 at higher effort, then Fable 5.1 at high or max effort. Each role has its own ladder. The run records tune the ladder (document 12.9).
- **Deduplication.** If the target and goal of a job match a running job, the orchestrator merges the new job into the running job. Sometimes a job's target had an attempt in the last N days with the same configuration, and the attempt failed. The orchestrator skips that job until something changes in the neighbourhood of the target.

## 11.5 Budgets

Money and compute are the real limits, so they are first-class:

- **Global:** a monthly budget, divided by role in `orchestrator.yaml`. People review this file each month and commit it.
- **Per campaign:** the owner of the campaign sets it. The global allocation is its cap.
- **Per job:** the priority tier sets it, and the orchestrator enforces it. Model calls carry token budgets. These are the API's task budgets where the model supports them, and hard `max_tokens` in all cases. The sandbox meters Lean time and compute time.
- **Review capacity:** money is not the only limit. Each role has a cap on PRs per week. The cap is a multiple of the review throughput for that kind of PR over the previous four weeks. Thus agents cannot go faster than the people who must check their work (document 21, Q-11).
- **Stopping:** a job stops at success, when the budget is used up, or when the agent reports that it is stuck and gives a reason. A stuck report with a reason is valuable output. "This statement appears false at n=7, here is the computation" is a result.

Each job records its spend: tokens in and out for each model, cached tokens, session-seconds, CPU-seconds and dollars. The weekly agent report (document 12.9) uses these records. The report shows what a formal proof costs, by area and by difficulty. At this time, the field does not publish this number.

## 11.6 Run records

Each agent run makes a run record in the artefact store. The run id is the key. The record contains:

- the job record and the resolved configuration: role version, prompt version, model ids, tool versions, image digests;
- the full transcript: every model request and response (thinking blocks as the API returned them), every tool call and result;
- the tactic tree of every Lean session;
- the outcome: success with the diff, failure with the reason, or a partial result;
- the spend.

Run records complete the provenance chain in document 06.5. They are also the training and evaluation data that improve the agents (document 17.4). The system keeps them as document 06.6 specifies. Their summaries are public. When the output of a run was merged, the full transcript is public. When the output was not merged, the system keeps the full transcript but does not publish it. By default, math2 does not publish a failed attempt, but it uses the attempt to learn.

## 11.7 Security and abuse

- Read endpoints are public and have rate limits. Write endpoints need a GitHub identity, and the PR opens under that identity.
- `lean_check`, `compute` and Lean sessions run arbitrary code: Lean metaprograms can do arbitrary IO at elaboration time. Thus they run in the sandbox, with no network, a read-only toolchain and quotas for each identity. Anonymous use is permitted only for `lean_check` on small snippets, with a strict timeout.
- MCP tools that write have a mark that says so. The read-only MCP profile disables them. That profile is the default for third-party agent clients.
- Text from sources (paper text, MathOverflow posts) is data. It goes into agent contexts with an untrusted mark. It is never put in a system prompt, and it is never concatenated into a tool description (document 12.7).

## 11.8 The first version

The first version of the read surface is in the `api` package, and `m2 serve` serves it. It has these limits:

- It serves the last build of one working tree, not a release. A request for `tag@release` gives an error.
- `get_source`, `history`, `topic` and `diff` come with the provenance of document 06 and the topic pages of document 13. The check, compute and propose methods come with the orchestrator.
- The one interface definition is the table of methods in the `api` package. The HTTP handler and the MCP server read the table. `api.Proto` makes `api/m2.proto` from it, with the HTTP annotations, and `buf` makes the Go code in `api/m2pb` from that file (tamnd/math2-agent#126). A test checks that `api/m2.proto` is the file that the table makes, a test checks that the code in `api/m2pb` agrees with the table, and a test checks that each gRPC call of the contract gives the HTTP answer. Thus the three encodings cannot drift apart. The gRPC service is `m2.v1.ReadService`, and each rpc gives one `Response` message with the result as a `google.protobuf.Value`, as each HTTP call gives the same JSON envelope. `m2 serve -grpc ADDR` serves it with the same rate limit for each IP address.
- The rate limit is a token bucket for each IP address of a peer, in memory. Behind a proxy or a CDN, the handler must read the address that the proxy sets.
- The MCP server is in the `mcp` package. It has the read-only profile only, with one tool for each read method, and `m2 serve` serves it over streamable HTTP at `/mcp` and over stdio. It uses the Go SDK of the Model Context Protocol.
- `similar` uses the embedding channel with the text of the node, and it has no judge label yet.
