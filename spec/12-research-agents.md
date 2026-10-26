# 12. Research agents

The top box of the diagram says "Formalize, Prove, Explore, Explain". Readers see these four agent roles. Four more roles work behind them and keep the graph healthy: curate, align, bump and review-assist. All eight roles are in `tamnd/math2-agent/agent/`. They run under the orchestrator (document 11). They change content only through pull requests.

The design comes from one finding of 2026. Document 01 gives this finding with its sources. **A minimal agent loop on a strong general model, with a strict verifier, matches or beats bespoke provers.** A plain agent on a general model saturated PutnamBench for about $111. The baseline agent of AlphaProof Nexus replicated the Erdős results of AlphaProof Nexus. The Fermat formalization ran on a generic DAG-of-statements harness. Thus math2 does not build a prover. It builds the things that a prover needs and cannot supply itself:

- a pinned, sandboxed, metered Lean environment;
- retrieval over the whole graph;
- a dependency structure in which a lemma is a first-class node;
- a verifier that nobody can talk past;
- a record of everything that was tried.

Then math2 uses the model that does best on the evaluations in document 17.

## 12.1 Roles and what they may touch

Each role has a bot account, a tool profile and a **field permission set**. Rule `X01` checks each agent PR against that set. Any change outside the set fails the audit, even if the change is correct.

| Role | Does | May create | May edit | Auto-merge |
|---|---|---|---|---|
| **formalize** | writes Lean statements for nodes, runs the F1 battery (10.4) | bindings at F0; `Math2/` statement declarations; `Conjectures/` entries | its own unmerged bindings | never |
| **prove** | finds formal proofs of bound statements | proof nodes; lemma nodes (statement plus binding); `Math2/` proofs | `formal[].decl` of nodes it proves | yes, if the target binding is already F2 or above and the new lemmas are internal to the proof (10.6) |
| **explore** | experiments, conjectures, counterexample searches | computation nodes; conjecture and counterexample nodes, which are always E1 or E4 by construction | none | never |
| **explain** | writes levelled explanations, examples and glosses | explanation sections marked `by: agent`; example nodes | its own unconfirmed explanations | never |
| **curate** | turns extracted paper results into nodes, proposes dedup merges, fixes graph health findings | statement and concept nodes from record plane results; `same` proposals | `uses`, `about`, `msc`, names and aliases | never |
| **align** | proposes crosswalk edges | `same_as`, `close_to`, `aligned` edges | its own unconfirmed alignments | `same_as` to Mathlib, only when the type hash of the bound declaration is identical |
| **bump** | moves to new Lean and Mathlib versions | the bump branch; rebindings | `formal[].decl`, `formal[].commit` | renames taken from Mathlib deprecation data |
| **review-assist** | prepares review packets for people | review packets in `reports/review/` (not content) | nothing in `nodes/` or `reviews/` | not applicable |

No role may write a status, a review, a register entry or a retirement (rules `S03`, `X04`, `V03`). An agent can give the reasons to retire a node, but only a curator can retire it.

A run writes only in its own work tree, through the `workspace` package. Its tools read and write files by their paths in the tree, and they cannot reach a file outside it. `agent.Permit` checks the changes of the run against the set of its role before the run opens a pull request. The sets of formalize and prove are written. The sets of the other roles come with their roles, and rule `X01` (#63) will use the same check.

## 12.2 The common loop

All roles run the same loop. Only the tools, the prompts and the stop condition are different.

```
job ← orchestrator
context ← API: the target node, its neighbourhood (depth ≤ 2), its sources, its formal status,
           prior runs on this target (summaries), the role's prompt at its pinned version
loop:
    model turn (tools available per role profile)
    tool calls → API / Lean session / compute sandbox      # every result cached by hash
    check budget; check for stuck (no progress in k turns)
until: goal predicate holds | budget exhausted | agent reports stuck with reason
diff ← workspace changes
validate diff: m2 check, role field permissions, local audit on touched shards
open PR (agent/<role>/<run-id>) with run summary | or record failure with reason
```

These decisions are intentional:

- **Tools, not bash.** The agent sees typed tools: `get`, `search`, `premises`, `lean_*`, `compute`, `propose_*`. It does not get a shell. In 2026, agents used a shell to find and exploit kernel and harness bugs (document 01). The tool surface is the MCP surface of document 11. Thus an external MCP client with the same permissions can do all that a math2 agent can do.
  The package `agent/tools` has the first of these tools. `tools.API` makes a tool from each read method, with the names and the parameters of the MCP server. `read_file` and `write_file` work in the work tree of the run, and a write that the role may not make fails and leaves the file as it was. `lean_check` elaborates a Lean file of the tree, or a scratch file, and its time counts in the Lean session seconds of the budget.
- **One target, one closure.** The context comes from the graph neighbourhood of the target, not from a general search over everything. Thus runs are reproducible and cheap to cache.
- **The verifier's verdict is the only verdict.** When an agent claims success, it must produce a diff that passes the audit. The system records the agent's own assessment and never uses it.
- **Failures with reasons are outputs.** "Statement false at n=7, witness attached" closes a prove job and attaches a counterexample job. "Needs a lemma that is not in Mathlib: X" creates a node for X.

The `agent` package runs this loop. A turn makes progress when it calls a tool with an input that the run did not call before, and the run caches the result of each call by the tool and the input. The tool `report_stuck` is in each profile, and the model calls it with the reason. The goal predicate is a function of the role that reads the tree and the build, never the text of the model. Each model turn has a limit of output tokens from the role and from the budget that is left after the most that the input of the turn can cost. When the goal does not hold, the predicate also gives the reason in one sentence, and the next message to the model starts with this reason. `agent.OnChanges` makes a goal that runs its check only when the changes of the work tree are different from the changes of the last check, because a check can build the library. `tools.Profile` makes the tools of a role from the tool names in its `role.yaml`, and a name with no tool is an error.

### Statement pinning

In 2026, the worst failure mode was a correct proof of a different statement. The causes were edited context, redefined notions, a weakened hypothesis or an exploited checker. Thus each role that writes Lean works against a **pinned statement**:

- A statement with no proof is a definition of a proposition, `def S : Prop`, because rule `F06` refuses `sorry` (07.2). When the job starts, the pin records `S` and the hash of each declaration of math2 in its closure. For a definition, this is the hash of the value. For a theorem, it is the hash of the type.
- The proof is accepted only if it is a theorem of type `S`, its axioms are a subset of the allowed set (rule `F03`), and each hash of the closure is unchanged.
- The agent's Lean files must not modify, shadow or re-declare anything in the closure of the target. The check runs on the elaborated environment, not on the source text.
- Before merge, a Comparator-style check confirms that the statement is identical to the pinned one. This check builds against Mathlib and `Math2` only. Then the external checkers replay the proof (07.7).

## 12.3 Models

The model is a configuration value, never a dependency in code. Each run records the exact model ids that it called. The `llm` package has one interface. Its first client is the Anthropic client. It has adapters for local open-weight prover models served behind gRPC (document 20.2). The client of the Claude models is in `llm/claude`, on the official Go SDK. It streams each request, caches the system prompt, the tools and the prefix of the conversation, and keeps the thinking blocks as the service returned them. `llm/llmtest` has a scripted model, thus the tests of the roles call no model service. `llm.Cost` gives the price of a usage from a table of prices, and `llm.Prices` is the table above.

| Tier | Model | Price in/out per MTok | Used for |
|---|---|---|---|
| 0 | `claude-haiku-5-5` | $0.10 / $0.50 | triage, classification, dedup judge pre-filter, first cheap proof attempts, the F1 critics' easy checks |
| 0′ | open prover models (e.g. Leanstral, DeepSeek-Prover, Goedel-Prover families) | self-hosted GPU | high-sample-count tactic and whole-proof attempts in the portfolio |
| 1 | `claude-sonnet-5-5` | $2 / $10 | bulk: curate, align, explain L1–L2, routine formalization, most prove attempts |
| 2 | `claude-opus-5-5` | $4 / $20 | default for hard roles: research-level formalization, decomposition planning, explain L3–L4, explore |
| 3 | `claude-fable-5-1` | $10 / $50 | the top of the escalation ladder: hardest proofs, cross-checking research-level statements, explore campaigns on open problems |

How math2 uses the API:

- **Adaptive thinking** on each role that does much reasoning. The **effort** setting increases up the escalation ladder: medium by default, then high, then max at the top. math2 never sets a fixed thinking-token budget. Current models reject those parameters. Thus the token budget of the run is in the job (11.5). The API's task budgets enforce it where they are supported, and `max_tokens` enforces it in all cases.
- **Prompt caching** for the parts of the context that all runs of a role share: the system prompt, the tool definitions, the role's style guide and the Mathlib conventions. There are thousands of runs each day, so this is the largest single cost saving.
- **Message Batches** at half price for work that no person waits for: curate, align, the dedup judge, explanation backfill and back-translation in the F1 battery.
- **Compaction** for long prove and explore runs. Thus a run can go past one context window and keep its record. The run record keeps the full transcript without compaction.
- **The orchestrator never forces a tool choice.** The current models reject a forced `tool_choice`. The roles are designed so that the model selects its tools.
- **Managed Agents** is an option for the roles that need only the read API and MCP (explain, curate, align). These roles run as hosted sessions connected to the MCP server of math2. Prove and formalize run in the loop of math2, because math2 must pin, sandbox and meter their Lean sessions (11.3).

The table is a starting point. The run records tune the **escalation ladder** of each role (11.4) each week. A role moves to a cheaper default when the acceptance rate of the cheaper tier on recent comparable targets is within a set margin of the more expensive tier. The report (12.9) publishes the acceptance and cost for each tier that justify each move.

Open-weight models are important for two reasons other than price. First, they let the portfolio draw thousands of different samples at marginal cost. Second, they keep the prove pipeline of math2 in operation if hosted access changes. In 2026, the gap between open and frontier models in formal proof became small. In research-level informal proof, the gap stayed large (document 01). math2 uses open models where they work and does not pretend that they work in other places.

## 12.4 Formalize and prove

Document 10 gives the full specification of the formalize role, because for that role the fidelity battery is more important than the agent. This section is about prove.

A **prove** job has a target node with a binding at F1 or above. Before F2, a proof is still useful because it tests the statement. But a proof of an unreviewed statement never auto-merges. The loop:

1. **Read the informal proofs.** If the node has a `proof` node, the agent works from it. If the node has only sources, the agent reads the aligned paper results (only text that the licence permits, 09.3). First, the model writes an informal proof sketch. The run record keeps this sketch.
2. **Decompose into the graph.** The agent writes the lemmas of the sketch as provisional nodes, each with a Lean statement. The largest formalizations of 2026 used this DAG-of-statements pattern (Prove2Me for Fermat, Gauss for sphere packing, the textbook project). There is one difference: the DAG of math2 is the knowledge graph itself. If a lemma is general, it becomes a reusable node. Dedup (05.4) finds lemmas that the graph already has. Before the agent proves a lemma, it searches for it with `search_formal` and `premises` over Mathlib and the graph.
3. **Cross-check the lemma statements.** A second, independent model call checks the statement of each lemma. The check asks three questions: is the lemma true, is it what the sketch needs, and is it degenerate? It uses the same degenerate checks as F1 (10.4). In the Fermat run, this check found false statements early. When a false lemma is found late, all of its subtree is wasted work.
4. **Prove leaves by portfolio.** Each leaf gets attempts with several strategies in parallel:
   - automation first: `simp`, `omega`, `decide`, `aesop`, `grind`, and the Lean hammer;
   - then sampled whole proofs from a cheap tier;
   - then tactic-level search in a Lean session with retrieval;
   - then an expensive tier.
   The first success wins.
5. **Recurse on failure.** If a leaf resists, the agent decomposes it more or sends it back up with a reason. If the reason is "this lemma looks false", the counterexample tools of the explore role try to refute it.
6. **Assemble and check.** The proof must build against the pinned toolchain with no `sorry`. Its axioms must be in the allowed set, and the type hash of the target must not change. The external replay and the comparator check must pass. Only then does the PR open.
7. **Clean up.** The system records the proof size and the build time. A proof is marked `needs-golf` if it is more than 10× the size of comparable Mathlib proofs, or if it takes longer than a set time to elaborate. It gets a follow-up job to make it shorter. The main complaint about the landmark formalizations of 2026 was AI-written proofs that work but are not maintainable. math2 must maintain its proofs through each Mathlib bump.

The packages `agent/prove` and `agent/formalize` have the goal and the first message of each role. These are the first parts of the loop above. A prove run writes the theorem `T : S`, moves the attribute `@[m2 "TAG"]` from `S` to `T`, and changes the `decl` of the binding to `T`. Its goal holds when the changes are in the field permissions of prove, the library builds, and the pin accepts `T`. A formalize run adds Lean files and appends bindings. Its goal holds when the changes are in the field permissions of formalize, the library builds, each new declaration has the right kind for the node, it uses only the standard axioms, and the degenerate checks of 10.4 do not find a problem. The decomposition, the cross-check and the portfolio of steps 2 to 4 are not written yet.

**What prove does not do.** It does not prove anything with `native_decide` unless the job permits it (rule `F04` records it in all cases). It does not add axioms. It does not edit the statement of the target. If the statement is wrong, it says so and stops.

## 12.5 Explore

Explore is the role that is nearest to research, and it needs the most restraint. It does experiments, finds patterns, searches for constructions and counterexamples, and proposes conjectures. Its tools are the computational engine (document 08), the SMT and SAT search tools, and evolutionary program search for constructions, in the AlphaEvolve and PatternBoost style (document 01). In this search, the model writes programs that generate candidates, a scorer ranks them, and the best programs are mutated.

Explore obeys four rules. Each rule comes from a failure in 2025 to 2026:

1. **Falsify before filing.** When the agent finds a pattern up to n, it checks the pattern to a larger n. It also checks the pattern against OEIS, LMFDB and the graph. Only then does the pattern become a conjecture node. Most patterns fail here, and that is the purpose of the rule.
2. **Look up before claiming.** No explore output gets the label new until the novelty check (17.5) has run. The check searches the record plane, zbMATH Open, arXiv full text where the licence permits, and MathOverflow. The search and its result are attached to the node. Without this step, an AI result is a rediscovery until someone shows that it is not. The Erdős episode of 2025 and the Aletheia sweep showed that this is the usual case. In the Aletheia sweep, 9 of 13 addressed problems already had solutions in the literature, and only 6.5% of graded candidates were meaningfully correct.
3. **Verifiable or E1.** If a construction or counterexample can be checked (a witness, a certificate), the agent files it with its check, and it can get to E4 or E5. An informal argument stays at E1, however convincing it is. The workspace shows it as a claim.
4. **People decide what is interesting.** Explore runs inside a project or campaign that a person defined. It does not select open problems for itself. This is a policy, not a technical limit. In 2026, there was a debate about mass "harvesting" of open problems by AI (document 01). math2 should not decide this debate by default.

## 12.6 Explain

Explain writes the explanations that make the graph readable. It writes the four levels of document 04 into nodes, with examples and non-examples. It also answers questions in the "Ask about this node" panel of the workspace (13.4).

- **Grounded.** Each factual claim in an explanation must cite a node by tag. A grounding check runs after the text is generated. A second model call takes each sentence and the cited node, and gives the sentence one of three labels: supported, unsupported or contradicted. Unsupported sentences are removed or marked. Contradicted sentences fail the run.
- **Levelled.** The level is a contract about prerequisites. An L1 explanation may use only nodes in the curriculum topic for its level (`topics/curricula/`). The check is mechanical: each cited or linked node must be in the allowed set.
- **Marked.** Explanations are `by: agent` until a person confirms them (rule `P04`). To confirm is to read and approve in `m2 review`. The system records the person who confirms.
- **Not a proof.** An explanation can describe a proof. It is never a replacement for the proof node.

## 12.7 Untrusted text and prompt injection

Agents read text that math2 did not write: paper text, MathOverflow posts, Wikipedia, OEIS comments and docstrings. Any of this text can contain instructions. The defences are in the structure, not in advice:

- **Separation.** Source text goes only inside tool results, as data blocks marked with their source and licence. It is never put in a system prompt, a tool description or the instructions of a role.
- **Least privilege.** The tool profile of each role is the minimum that it needs. Explain cannot write Lean. Curate cannot run compute. No role has network access other than the math2 API and the model API.
- **Narrow outputs.** The output is a diff that must pass `X01` (field permissions) and the audit. An injected instruction can make an agent edit something outside its role. The result is a failed run, not a merged change.
- **Canaries.** The evaluation suite (document 17) has source documents with planted instructions. If a prompt or model version obeys them, it fails its release gate.
- **Quarantine.** If someone finds an injection in the wild, the run records identify each run that read the poisoned source. One command quarantines all of these runs (06.5).

## 12.8 Versioning roles and prompts

The behaviour of a role depends on its prompts, its tool profile, its model configuration and its code. These four have one version together, the **role version** (semver), in `prompts/<role>/`:

```
prompts/prove/
  role.yaml          version: 3.4.0, tools, model ladder, effort per tier, budgets
  system.md          the system prompt
  style/             Mathlib style conventions given to the model
  examples/          worked examples (each a real merged run, referenced by run id)
  CHANGELOG.md
```

- The package `prompts` embeds the role directories, and `agent.LoadRole` reads one. The first versions are formalize 0.1.0 and prove 0.1.0. They have no examples yet, because there is no merged run yet.
- Each run records the role version and the hashes of each prompt file that it loaded.
- A change to any file bumps the version. A minor bump must pass the evaluation suite of the role (17.3), with results no worse than the previous version. A major bump also needs a shadow week. In that week, the new version runs with the old one on the same jobs, and the system uses only the output of the old one.
- Provenance can be queried by role version. Thus "everything prove 3.2.x produced" is one command, and quarantine can use it as a target (06.5).

## 12.9 The weekly agent report

Each Monday, the orchestrator writes `reports/agents/<yyyy>-W<ww>.md` and its JSON twin. The report is public. People use it to decide if the agents are worth their budget:

- **Volume:** jobs started, finished, succeeded, failed and stuck, per role.
- **Acceptance:** PRs opened, merged, closed or changed before merge, per role and model tier. "Changed before merge" counts the fixes that a person made. This count tells more about quality than the merge count does.
- **Cost:** dollars per merged output per role, by area (MSC) and by tier, and the trend. One example is the cost of an E5 proof of a lemma at a given depth in a given area. Nobody else publishes this number, and math2 can.
- **Audit sample:** the 5% sample of auto-merged PRs (16.2), with the errors found and the error rate of the role against the 1% threshold.
- **Incidents:** quarantines, injection detections, reward-hacking attempts that statement pinning caught (with the run ids), and stale reviews that bumps caused.
- **Ladder changes:** the tier and effort changes made that week to the escalation ladder of each role, and the measurements behind them.
