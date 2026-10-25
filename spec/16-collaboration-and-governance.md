# 16. Collaboration and governance

People and agents write math2 together, and everyone can read it. This document defines who may change what and how changes are reviewed. It also defines how credit and disclosure work, how math2 relates to the communities whose work it links to, and how the content is licensed.

## 16.1 Roles

| Role | Who | Can |
|---|---|---|
| reader | anyone | read, search, run examples, use the read-only API and MCP |
| contributor | anyone with a GitHub identity | open PRs (nodes, edits, alignments, Lean code), comment, claim tasks |
| reviewer | a contributor listed in `registers/reviewers.reg` for one or more areas | sign fidelity reviews (F2/F3) in those areas; approve PRs in those areas |
| curator | a reviewer with merge rights in an area | merge PRs in the area; resolve dedup queues; retire nodes |
| maintainer | a small group | merge anything; change schema, rules, budgets, roles; cut releases |
| agent role | a bot account per role | open PRs within the role's field permissions (12.1); never review, never merge except under auto-merge rules |

The curators of an area give reviewer status for that area. An area is an MSC two-digit class or a narrower topic. The basis for the decision is the reviews that the person has already done. A reviewer does not need an academic position. Mathlib's reviewer and maintainer system is based on contributions, not on credentials, and math2 uses the same model. But a reviewer must have a real identity: an ORCID or a long-standing GitHub account. The independence rule for F3 (rule `V02`) cannot work with anonymous reviewers.

## 16.2 How changes land

Every change is a pull request against `tamnd/math2`. The gate before a merge depends on the kind of change:

| Change | Gate |
|---|---|
| typo or prose fix in a body | `m2 check` green; any contributor approval |
| new node or new edge, E1 evidence only | audit green; one curator approval |
| new formal binding (F0) | audit green, Lean build green; one curator approval |
| fidelity review (F2/F3) | the review itself is the gate; one reviewer signs; for F3, a second independent reviewer or an equivalence proof |
| retire, merge or split nodes | two curator approvals; release notes entry |
| agent PR, role with auto-merge (12.1) | audit green; role-specific checks green; sampled human audit afterwards |
| agent PR, any other role | as for the same change by a person, plus the agent label |
| schema, rule, budget, role changes | two maintainer approvals; an entry in document 21 if it settles an open question |

**Auto-merge** applies only to changes whose correctness is entirely mechanical. One example is a Mathlib rename that comes from deprecation data. Another example is a formal proof added to a node whose statement binding is already F2 or above. For these changes, the kernel and the replay are the whole check. Even then, people audit a sample of 5% of the auto-merged PRs of each role each week. The agent report (12.9) publishes the findings of the sample. If the sampled error rate for a role is more than 1%, auto-merge stops for that role until the cause is fixed.

## 16.3 Curation at scale

Curators cannot curate a graph of all mathematics if they approve PRs one at a time. Curators work through **queues**. Each queue has a rank order, so that one hour of a curator's time has the most effect:

- **dedup queue:** candidate `same` pairs (05.4), ranked by how much of the graph the merge would connect;
- **alignment queue:** agent-proposed `same_as` edges, grouped by external system and ranked by centrality;
- **fidelity queue:** bindings at F1 that wait for a human review, ranked by `unlock` (11.4). A review of a statement that 300 nodes depend on is worth more than a review of a leaf;
- **extraction queue:** paper results that the extractor could not align, sampled by topic;
- **retirement queue:** nodes flagged by graph health rules (orphans, hubs that are too small, conflicting `same_as`).

The reviewer's tool, `m2 review`, makes a fidelity review fast. It shows the informal statement and the elaborated Lean statement side by side. It also shows the back-translation, the results of the degenerate-statement check, and the critics' notes (document 10.4). It shows the definition of every non-trivial constant in the Lean type, and each definition expands on demand. The target median time per review is 3 minutes for undergraduate-level statements and less than 15 minutes for research-level statements. Document 17 measures this.

## 16.4 Credit

Work on math2 must count as scholarly output. If it does not, the people who can review research-level fidelity will not give their time to it. Thus:

- **Every release is citable** with a DOI (06.2). Every node at a release is citable as `m2://TAG@RELEASE`.
- **Contributor statements.** `m2 prov credit <person> --from --to` produces a CRediT-taxonomy statement. It lists, with links, the nodes that the person wrote, formalised, reviewed or curated. Its format is for direct use in a CV or a grant report.
- **Reviews are signed and public.** A fidelity review is a small, real and checkable piece of mathematical work. It carries the reviewer's name.
- **Projects publish.** A project that reaches its goal (13.6) can publish a release note as a paper. The author list comes from provenance, and the project's members edit it. A standard paragraph discloses the AI contributions, and provenance also generates this paragraph (16.5).

## 16.5 AI contribution: disclosure and norms

The norms that formed from 2024 to 2026 are converging. They come from journal and arXiv policies and from statements by mathematical societies. They also come from the public practice of Terence Tao and other researchers who work with AI tools openly (document 01 has the sources). The norms agree on three points. AI tools may be used. The humans who sign are responsible for the result. The disclosure of the use must be specific enough that a reader can judge it. math2 puts these points into the data and does not depend on good intentions:

1. **Every agent-produced byte is marked** by provenance (06.5). The workspace shows the marker on prose that an agent wrote and a person has not confirmed (rule `P04`).
2. **Agents are not authors.** They are tools that act for a named person, project or schedule (rule `P03`). The person or project is responsible.
3. **Formal proofs found by agents** need no disclosure beyond provenance for their correctness, because the kernel checked them. But they do need disclosure for credit and for the record. If an agent run is the first to prove a theorem, the history tab of the theorem says so, with the run id.
4. **Informal proofs by agents stay E1** (04.7) until someone formalises, certifies or publishes them. The workspace never shows them as more than claims.
5. **Novelty claims are checked against the literature first.** Before anyone describes an agent's result as "new" in any place, the explore agent runs a literature search. It searches the record plane, zbMATH, arXiv full text where the licence allows, and MathOverflow. math2 attaches the search and its result. Several of the 2025 "AI solved an open problem" announcements were rediscoveries of known results. math2 is designed so that it does not repeat that error (document 01, 17.5).
6. **Withdrawal is mechanical** (06.5). Sometimes a model or prompt version is found to give systematically wrong output. Then one command quarantines all of its unconfirmed output.
7. **Disclosure meets the 2026 guidelines by construction.** The AGMAI guidelines of September 2026 ask labs and authors to publish the model, prompts, summarized reasoning, time and compute cost. They also ask for documentation of failed attempts and of selection criteria. They ask for persistent identifiers in lab-independent repositories, and for machine-readable links between informal and formal statements. Run records (11.6) and bindings (07) already hold all of these. `m2 prov disclose <tag|project>` renders them as a disclosure paragraph. For any formal result, it also renders them as a Palomar-compatible `formalization.yaml`. Thus you can register a math2 result on Palomar, or cite it in a journal that requires an AI declaration (the LMS, Elsevier). Nobody has to reconstruct what happened.
8. **The human role is recorded with a taxonomy.** Each agent-assisted contribution states what the people did. It uses categories adapted from the erdosproblems AI-contributions wiki: the agent solved it with no literature found; the agent solved it and literature was later found; the agent built on literature; people and agents worked together; and literature search, formalization, rewriting or computation only. The project page and the disclosure paragraph show these categories. One "AI-assisted" label in place of these categories hides exactly the information that a reader needs.
9. **Participation without AI is first-class.** Some mathematicians have chosen not to work with commercial AI systems (the Association for Human Mathematics, 2026). Some venues now require human-written expositions. A project can declare itself `agents: none`, and then the orchestrator refuses jobs in it. Any reader can filter the workspace to show only human-authored prose. The credit statement of a person (16.4) separates the work that they did themselves from the work that they directed. math2 should be a place where both kinds of mathematician can work side by side.

### Upstream etiquette

math2 links into communities that have their own norms. The most important are Mathlib, Wikidata, nLab, OEIS and the Stacks Project. The rules are:

- **Upstreaming to Mathlib is by people, in Mathlib's process.** If math2 Lean code is worth upstreaming, a person offers it as Mathlib PRs and guides them through review. Agents may prepare the branch. Agents do not open PRs on Mathlib or on any other external project, unless that project has asked for it.
- **No bulk writes to external wikis without agreement.** Writes to Wikidata, MaRDI and similar systems follow the bot policies of those systems. This means approved bot accounts, rate limits, and community discussion first.
- **Respect the load.** Ingestion uses dumps and published bulk channels. It does not crawl live sites. It follows the pace that each source states (document 09). This is the same discipline that `tamnd/arxiv` applies to arXiv.

## 16.6 Licensing

The content has three licence zones. The zones let math2 take in the sources that matter, and they let others reuse what math2 produces:

| Zone | Licence | Why |
|---|---|---|
| node prose (bodies, explanations, names) | **CC BY-SA 4.0** | compatible with importing from Wikipedia, zbMATH Open, OEIS, LMFDB, ProofWiki and individual MathOverflow/Math.SE posts, which are all BY-SA family (3.0 material can be adapted under 4.0); also compatible with our own text being reused by those communities |
| Lean code (`lean/`) and computation code (`compute/`) | **Apache-2.0** | identical to Mathlib's licence, so code can move upstream with nothing in the way |
| metadata (front matter facts, edges, registers, manifests, the derived graph) | **CC0** | facts about mathematics should be free of conditions; also matches arXiv metadata and Wikidata |

The `L` audit rules enforce the licence rules (document 15). In particular:

- **GFDL sources are linked, never copied.** The Stacks Project and Kerodon are GFDL. GFDL text cannot go into CC BY-SA content, and CC BY-SA text cannot go into GFDL content. math2 links to Stacks and Kerodon tags with `same_as` and writes its own statements. This is permitted, because a mathematical fact is not copyrightable. Rule `L04` compares node text with the linked source to find accidental copies.
- **arXiv default-licence papers give metadata and structure, not text.** The `tamnd/arxiv` licence gate applies unchanged. math2 can record that a paper states a result (a record plane entry with a locator). This does not require it to republish the paper's wording.
- **MSC2020 descriptions are CC BY-NC-SA.** The codes are identifiers, and math2 uses them freely. The descriptive text of each class has a non-commercial clause that CC BY-SA content cannot absorb. Thus math2 never stores this text in `tamnd/math2`. The site shows it from a pinned snapshot, with attribution.
- **Stack Exchange text is imported per post, never from the dumps.** Posts are CC BY-SA (2.5, 3.0 or 4.0, depending on the date). But since 2024, the official dumps are behind click-through terms that forbid LLM training. At least one dump contained planted watermark entries. math2 gets individual posts through the public API, and records the licence and the author of each post. It stores records and short quotations, not bulk text.
- **Mathlib docstrings** are Apache-2.0. You can use one as a node body, with attribution in provenance. The CC BY-SA node body then includes it under BY-SA, and Apache-2.0 permits this.

## 16.7 Governance and sustainability

- **Decisions are written down.** Settled decisions are in document 00. Open decisions are in document 21. A change to either document is a PR with a rationale. If the change reopens a settled decision, it needs a measurement. The other specs in this series use the same rule.
- **Budgets are public.** math2 commits the monthly agent budget, its split by role, and what it bought (12.9) in `reports/agents/`.
- **The project could outlive its funding.** The content is in git and archived to Software Heritage. It is citable by DOI, and you can rebuild it from pinned images (document 06). If agent funding stops, math2 becomes a static, versioned and checkable library that people maintain at human pace. That outcome is smaller, but it is not a failure.
- **Funding and institutions.** Document 01 lists the 2025 to 2026 programmes that fund this kind of infrastructure. They include AI-for-math funds, DARPA's expMath, national research data initiatives such as MaRDI, and the Lean FRO's model of a focused research organisation. In the milestone structure of math2 (document 19), each milestone is a deliverable that a funder can check.
