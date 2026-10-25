# 13. The research workspace: search, explore, explain, collaborate

The workspace is the bottom-left box of the diagram. People use math2 through it. It reads from the knowledge graph through the API. It writes back only through the orchestrator, as jobs and pull requests. There are three front ends, and each one is complete for its job:

- a **static site**, `m2 site`, published with each release. It is fast, people can cite it and archive it, and it runs no server code. It is for reading.
- a **live web app**, `m2 serve`, which adds search, Lean sessions, computations, collaboration and agent requests.
- a **terminal UI**, `m2 tui`, for people who work in a terminal, and for scripts. It uses the same navigation model as `ant tui`.

Everything that the web app shows is also available through the API and MCP (document 11). The workspace must not have a private capability.

## 13.1 The page model

Each node shows as a page. The page has a fixed structure, so that a reader learns it one time:

```
┌─────────────────────────────────────────────────────────────────────────┐
│ Lagrange's theorem                                   m2://7K3QX2@2026.11 │
│ statement · theorem · 20D60                                              │
│ Proved in Lean (Mathlib). Statement reviewed by 2 people. Classical.     │  ← evidence phrase (15.5)
├─────────────────────────────────────────────────────────────────────────┤
│ Statement                                                                │
│   Let G be a finite group and H a subgroup of G. Then |H| divides |G|.   │
│                                                                          │
│ [Formal] [Proofs (2)] [Uses] [Used by (1,204)] [Sources (37)] [History]  │  ← tabs, each a query
│                                                                          │
│ Explanations   L1 · L2 · L3 · L4                                         │
│ Examples       ℤ/6, S₃, … (executable)                                   │
│ Related        converse fails: A₄ (counterexample) · Sylow theorems · …  │
├─────────────────────────────────────────────────────────────────────────┤
│ Open work on this node:  explanation L1 missing · 1 alignment to confirm │  ← jobs, claimable
└─────────────────────────────────────────────────────────────────────────┘
```

- The **Formal** tab shows the bound Lean statement, pretty-printed and also as source. It has links to the Mathlib documentation at the pinned commit. It shows the axioms, the replay results, and the reviews with their hashes. If the binding is `stronger`, `weaker` or `variant`, the tab shows the adapter and explains the difference in words.
- The **Proofs** tab shows each proof node: an informal proof, formal or not, its method tags and its own dependency closure. A reader can compare proofs side by side by what they use ("proof 1 uses choice, proof 2 does not").
- **Uses** and **Used by** are the closure views (05.5). They can show as a list, a layered DAG, or a blueprint-style dependency graph coloured by evidence level.
- **Sources** lists each aligned paper result in date order. The earliest result has a mark. Each result has a link to `ax://` for arXiv content, or to the publisher, zbMATH or MathSciNet for records.
- **History** shows revisions with provenance, each binding change that a Mathlib rename caused, and each change in evidence level, with the reason.

## 13.2 Search

The search box accepts natural language, LaTeX, a Lean expression, an identifier, or a mix of these. Document 14 describes the engine. The workspace adds three things:

- **Facets** for kind, MSC, evidence level, fidelity, "has formal statement", "has formal proof", "open", "uses choice", source date range and method.
- **Explanation of results.** Each result shows the channel that matched it (lexical, formula, embedding or formal type) and the matched span. Thus a reader can see why the result is in the list.
- **Did-you-mean across systems.** Sometimes a query matches nothing in math2 but resolves in Wikidata, nLab, Mathlib or OEIS. Then the workspace shows "not held yet", with the external link and a one-click "request this node". This button files a curate job (11.4).

## 13.3 Explore

To explore is to move through the graph with a purpose. The workspace supports the moves that mathematicians actually make:

| Move | Interface |
|---|---|
| "What do I need to understand this?" | prerequisite path (Q5) from a chosen starting concept, or from the reader's declared background (13.5), as an ordered reading list |
| "What is this used for?" | reverse dependencies, grouped by topic, ranked by centrality |
| "What is the most general form?" | the `generalizes` chain upwards, and the hub family view |
| "What is known about this open problem?" | the problem page: conjecture, bounds over time as a chart, partial results, related computations, formal statement status, who is working on it (13.6) |
| "Where are the gaps?" | the frontier view for a topic: formalised-but-unproved, refereed-but-unformalised, open-with-formal-statement. This view is the shared to-do list for people and agents |
| "What changed?" | the topic diff between releases |
| "Show me the landscape" | a topic map: concepts as nodes, `specializes`/`about` edges, laid out hierarchically and coloured by coverage, with zoom from MSC sections to individual nodes |

The client draws the graph. It uses a WebGL renderer for large neighbourhoods. It never shows more than approximately 2,000 nodes at one time. Above that number, it changes to aggregation by topic, with counts. A picture of 50,000 nodes is decoration, not navigation.

## 13.4 Explain

The explanation agent (document 12.6) writes levelled explanations into nodes. The workspace adds explanation on demand:

- **Ask about this node.** Each page has a chat panel that answers questions. The context of the panel has the node, its neighbourhood and its formal statement. Each answer cites `m2://` tags. If a statement in the answer has no cited node to support it, the panel marks it unsupported. The panel uses the explanation agent with a read-only tool profile.
- **Unfold this proof.** For a formal proof, the reader can expand any step into its sub-steps, down to single Lean tactics and the lemmas that they used. Each one has a link back to its node. The informal gloss for each step is generated, cached, and marked as generated.
- **Translate notation.** When the pointer is on a symbol, the workspace shows its definition node. A reader can change notation conventions, such as $\subset$ versus $\subseteq$, or conventions for $\mathbb{N}$, where the concept records the convention.
- **Languages.** The workspace can show explanations and statement prose in the languages that `tamnd/arxiv` already supports (English, Vietnamese, Simplified and Traditional Chinese, Japanese). It uses the same glossary and translation pipeline. Translations have a mark that says they are translations. The English text (or the text in the original language) stays canonical. Formal statements are never translated, because they do not depend on a language.

## 13.5 The reader's profile

Anyone can read without an account. A reader who signs in (GitHub or ORCID) can declare a **background**: the set of concepts and statements that they already know. A curriculum topic can be the start of a background ("finished a first course in algebra"), and the reader can change it. Prerequisite paths and explanation levels then start from the background. The workspace stores the profile, not `tamnd/math2`. The profile is private, and the reader can export it or delete it.

## 13.6 Collaborate

Collaboration is the area where math2 is most different from a static library. Its design follows the three ways that mathematical collaboration actually works in 2026 (document 01): Polymath-style open projects, blueprint-driven formalisation projects (FLT, PFR, the Equational Theories Project), and small groups of humans who work with AI tools.

### Projects

A **project** is a campaign (11.4) with people attached. It has:

- a **goal**: a set of target nodes and a target evidence or fidelity level, or an open problem with "find a proof or a counterexample";
- a **blueprint**: the closure of the targets, computed live and coloured by status. The graph generates it, and nobody writes it by hand. This is the main improvement over `leanblueprint`. If a project already has a blueprint, `m2 ingest blueprint` imports it as nodes and edges (document 09.4);
- **tasks**: the frontier items inside the closure, each claimable;
- **members**: people and agent roles, with permissions;
- a **log**: every PR, review, agent run and discussion thread that touched the project.

### Tasks and claims

Any frontier item can be a task. A person claims a task for a time window (default 14 days, renewable). Everyone can see the claim, so two people do not do the same work. Agents examine claims before they take a job. A claim by a person stops agent work on that target, unless the person invites agent work ("let agents try the cheap attempts first"). This follows the norm that large formalisation projects agreed on in 2024 to 2026: claim visibly, release promptly.

### Discussion

Each node and each project has threads. The workspace database stores threads, not git. If a thread decided something, it must end with a pull request that writes the decision into a node or a review. Discussion is not knowledge until it is in the graph. For projects that already use Zulip, threads can be mirrored to Zulip streams. The Lean community discusses on Zulip, and math2 should join that discussion, not compete with it.

### Working with agents in the open

A project member can request agent work inside the budget of the project. Agent runs show in the log with their transcripts (11.6). Anyone in the project can read how a proof was found, and see which attempts failed and why. This is intentional. In 2025 to 2026, there were episodes in which AI "solved" problems that were already in the literature, or solved a misstated version. People found each of these problems because someone could see what was actually done (document 01). Openness is the cheapest verification method available.

### Credit

Provenance (06.5) attributes each contribution. The project page shows contributions by person and by agent role. Each agent contribution names the person or project that directed it. `m2 prov credit` exports a CRediT-style statement for any person and period (document 16.4).

## 13.7 Performance and accessibility targets

- Static node pages: less than 50 KB transferred before math fonts, and the first meaningful paint in less than 1 s on a mid-range phone on 4G.
- The server renders math to HTML+MathML at build time (KaTeX). MathML is always present for screen readers. The client never needs JavaScript to read a node.
- Each interactive view (graph, chart) has an equivalent table or list.
- The static site is fully usable offline as a downloaded archive. Each release publishes one archive. Thus people can read math2 without a network, and this is important for many of the mathematicians in the world.
