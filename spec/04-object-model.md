# 04. The object model

This document defines what math2 stores. It gives the kinds of node, the permanent name of a node, the format of a node file on disk, how the status of a node is computed, and what a node must not say about itself. All other documents use these definitions. If a later document seems to disagree with this one, this document is correct and the other document has a bug.

## 4.1 The unit is a node

A node is one piece of mathematics that someone may want to cite, check, compute with or explain on its own. Its size is about that of a Stacks Project tag, or of a Mathlib declaration with its docstring. It is not the size of a paper or a chapter.

A node has five parts:

1. a **permanent tag**, which is its identity (4.3);
2. a **kind**, from a closed list (4.2);
3. an **informal body** in constrained Markdown with LaTeX (4.5);
4. **typed edges** to other nodes and to external records (document 05);
5. **evidence**: formal bindings, proofs, computations, sources and reviews. The status of the node is computed from this evidence and is never written by hand (4.7).

A node is not a page. The workspace renders pages from nodes and their neighbourhoods (document 13). A node is also not a Lean declaration. A Lean declaration is evidence that a binding attaches to a node (document 07). The separation of these three things is the most important decision in this document. Mathlib has about a quarter of a million theorems. Most of them are API lemmas that nobody would cite by name. The literature of mathematics has millions of results, and most of them are not formalised. A system whose unit is the Lean declaration cannot represent the second set. A system whose unit is the page cannot be checked. The node is between the two.

## 4.2 Kinds

The list of kinds is closed. A new kind changes the schema version (4.9) and needs an entry in document 21.

| Kind | What it is | Typical evidence | Example |
|---|---|---|---|
| `concept` | A mathematical notion, independent of any one definition of it | definitions, equivalence statements | compact topological space |
| `definition` | One specific definition of a concept | formal binding to a `def`/`structure`/`class` | compact iff every open cover has a finite subcover |
| `statement` | A proposition claimed true. Its `role` is `theorem`, `lemma`, `proposition`, `corollary`, `fact` or `axiom` | proofs, formal binding, computation, sources | Lagrange's theorem |
| `conjecture` | A proposition that is open, or that was posed as open | sources, partial results, computational evidence, formal statement | Goldbach's conjecture |
| `problem` | A question that is not a single proposition: classify, compute, decide, find the best constant | sources, bounds as linked statements | Erdős problem #N, Hadwiger–Nelson |
| `example` | A specific instance that illustrates a concept | computation, formal `example` | $\mathbb{Z}/6$ is cyclic but not simple |
| `counterexample` | A specific instance that refutes a statement or shows a hypothesis is needed | computation, formal proof of the negation | Sorgenfrey plane is not normal |
| `object` | A named mathematical object with an identity of its own | external identifiers (LMFDB label, OEIS A-number, GAP id) | the Monster group, $E_8$ lattice, A000045 |
| `proof` | One argument for one statement. It is informal, formal, or both | a body, a formal binding, `uses` edges | the proof of Lagrange via cosets |
| `computation` | An executable, reproducible computation and its recorded result | an environment, inputs, outputs, a certificate | checking Goldbach's conjecture up to $4\cdot10^{18}$ |
| `topic` | A navigational grouping. It is never a claim | MSC codes, child links | finite group theory |

Three distinctions are intentional.

**Concept versus definition.** "Compact" has at least four standard definitions: open covers, the finite intersection property, nets and ultrafilters. In the literature they are one concept. In a proof assistant, one of them is the definition and the others are theorems. math2 keeps one `concept` node, one `definition` node for each definition, and `statement` nodes that say the definitions are equivalent. Navigation and search go through the concept. Checks go through the definitions. With this structure, math2 can align with nLab (organised by concept), with Mathlib (organised by definition) and with textbooks (which choose one definition and prove the others). It does not force any of them into the shape of the others.

**Statement versus proof.** A statement can have many proofs, and the proofs differ in what they use. A researcher needs this information to know whether a result survives without the axiom of choice, or without the classification of finite simple groups. So proofs are nodes, and each proof has its own `uses` edges. The dependency closure of a statement is not one set. It is the set of alternatives, one for each proof (document 05.5).

**Conjecture versus problem.** A conjecture is a proposition with a truth value. It can be formalised as a Lean `Prop`. A proof of it, or of its negation, settles it. A problem may have no single truth value ("determine $R(5,5)$"). A chain of statements settles it. DeepMind's `formal-conjectures` repository and the Erdős problems database both mix the two. math2 keeps them apart, because they are settled in different ways.

## 4.3 Tags: identity forever

Every node has a permanent tag. The idea comes directly from the Stacks Project. `tamnd/arxiv` already uses the same tag discipline for results inside a paper. Three rules apply:

1. **A tag is assigned once.**
2. **A tag is never edited.**
3. **A tag is never reused, even if its node is deleted.**

Audit rule `T01` checks the tag register against git history. It fails if a tag has ever disappeared (document 15).

### Format

A tag is six characters of Crockford base32: the digits and the uppercase letters, without `I`, `L`, `O` and `U`. That gives 32^6 tags, about 1.07 billion. A four-character tag (the Stacks size, about 1.7 million in base 36) is too small. Mathlib alone has over 400,000 declarations. The literature has tens of millions of stated results, and the record plane (document 09) can turn any of them into a node.

The first character is never `0`. Tags that begin with `0` are reserved for the bootstrap seed (milestone M1), so the first few thousand curated nodes are easy to identify. A tag never ends in a character that would make it a valid MSC code, an OEIS A-number or an arXiv identifier. With this rule, a bare tag pasted into a search box resolves to exactly one kind of thing.

Tags are random, not sequential, for two reasons. Random tags make no claim about order, importance or age. Also, two branches can mint tags without coordination. Audit rule `T02` catches collisions at merge (the chance per pair is about one in a billion).

### When a tag is minted

Tags are minted at merge to `main`, and never before. A node on a branch, in the scratch space of an agent or in a pull request has a **provisional id**. Its form is `~` followed by eight hex characters, for example `~3fa94c01`. The merge bot rewrites provisional ids to tags in the node, in every edge that points to it, and in every Lean attribute. A pull request that cites a provisional id outside its own diff fails `T03`.

This rule prevents agents from wasting tag space. Agents propose tens of thousands of candidate nodes each day (document 12), and most of them are rejected. No rejected candidate ever had a tag. So the register records only things that were accepted at some time.

### Deletion is deprecation

A node is never deleted, even if it is found to be a duplicate, a mistake or out of scope. Its file stays, its `kind` stays, and it gets:

```yaml
retired:
  date: 2027-02-14
  reason: duplicate        # duplicate | false | out-of-scope | split | merged
  successor: [7K3QX2]      # zero or more tags
  note: "Same statement as 7K3QX2 with the hypotheses reordered."
```

A retired tag still resolves. The workspace shows its successor and the reason. A citation from 2026 still resolves to something meaningful in 2036.

`split` and `merged` record a refactor of the curated graph. It can become clear that one node states two results, or that two nodes are one. The successor list keeps the history navigable.

## 4.4 URIs

math2 is the `m2://` scheme in the `ant` URI space. Other schemes in that space are `ax://`, `wikipedia://`, `oeis://` and the rest of the `tamnd/*-cli` family. So `ant get m2://7K3QX2` works in the same way as for any other record.

```
m2://7K3QX2                     the node, at the latest release
m2://7K3QX2@2026.11             the node as it was in release 2026.11
m2://7K3QX2@c4f1e9a             the node as it was at a git commit
m2://7K3QX2#proof               the statement section; #proof, #formal, #history, etc.
m2://statement/7K3QX2           a typed alias, redirects to m2://7K3QX2
m2://concept/compact-space      a slug alias, resolves through the alias table
m2://msc/20D60                  a topic view over an MSC2020 code
m2://lean/Subgroup.card_subgroup_dvd_card   the node bound to this Mathlib declaration
m2://release/2026.11            the release manifest
```

Aliases are a convenience. They are not identities. A slug can go to a different node later, for example if "compact space" is found to be ambiguous between topology and order theory. A tag cannot. Every place that stores a reference stores the tag. `m2 fmt` rewrites aliases into tags before commit (rule `T04`).

External identifiers map into `m2://` through `same_as` edges (document 05.3). `m2://ext/wikidata/Q180967` resolves to the node that has that `same_as`. If no node has it, the URI returns a "not held" record that names the external system of the identifier.

## 4.5 The node file

A node is one file, `nodes/<first two characters of the tag>/<tag>.md`. The file has YAML front matter and a Markdown body. The shard key is the first two characters of the tag. With it, each directory has fewer than about a thousand entries at a million nodes.

The full schema is `schema/node.v1.json` in `tamnd/math2` (document 20). A statement node looks like this:

```markdown
---
tag: 7K3QX2
kind: statement
role: theorem
name: Lagrange's theorem
aliases: [lagrange-theorem-groups]
msc: [20D60]
topics: [4F2M9A]                # tag of the topic node "finite groups"
uses:                           # informal dependencies of the statement itself
  - 1H8D2C                      #   definition: subgroup
  - 6PQ0WZ                      #   definition: order of a finite group
proofs: [9TX3RB, 2YB7KE]        # proof nodes, in the order shown to readers
formal:
  - system: lean4
    library: mathlib
    decl: Subgroup.card_subgroup_dvd_card
    match: exact                # exact | stronger | weaker | variant | partial
    fidelity_reviews: [r-7K3QX2-01, r-7K3QX2-02]
sources:
  - zbmath: "0000.00000"        # example only; real records carry real ids
    locator: "Thm 3.1"
  - book: "isbn:9780471433347"
    locator: "§3.2, Thm 8"
same_as:
  wikidata: Q180967
  proofwiki: "Lagrange's_Theorem_(Group_Theory)"
  nlab: null                     # explicitly checked, none exists
history:
  first_stated: "Lagrange 1771 (special case); Galois; Jordan 1861"
---

## Statement

Let $G$ be a finite [[group|1A2B3C]] and $H$ a [[subgroup|1H8D2C]] of $G$.
Then $|H|$ divides $|G|$.

## Remarks

The converse fails: $A_4$ has order 12 and no subgroup of order 6 ([[counterexample|5ZQ1PM]]).
```

The rules for the body:

- **Math is LaTeX** inside `$…$` and `$$…$$`. Use only the subset that both KaTeX and MathJax render, plus macros declared in `macros/*.sty`. Audit rule `B01` renders every body headlessly and fails on any error. Free LaTeX is not allowed. A body that renders in only one engine cannot be served everywhere.
- **Links are `[[text|TAG]]`.** A link to something that is not a tag fails `B02`. Specifically, links to URLs are not allowed in the statement section. Put sources in `sources:`.
- **Sections are fixed per kind.** A statement has `Statement`, then optionally `Remarks`, `History` and `Explanations`. A proof has `Proof`, then optionally `Idea`. The full list is in the schema. A section that the schema does not know fails `B03`. This rule lets a program parse bodies without a model.
- **The statement section states one thing.** It does not hold a proof, a history or a motivation. The explanation agent (document 12.6) writes into `Explanations`, and into no other section.
- **Explanations are levelled.** Each explanation has a level tag: `L1` for a person who has not met the concept, `L2` for an undergraduate, `L3` for a graduate, `L4` for a researcher. Each level also has a tag that says human-written or agent-written (4.8).

Not every node has text that a person wrote. A node can come from a well-structured source, such as an OEIS sequence, an LMFDB object or a Mathlib declaration with a good docstring. Such an imported node can have a generated body. The body has a mark that says it is generated. An `imported` provenance entry names the source and its version (document 06.5).

## 4.6 Proof nodes

A proof is a separate file with `kind: proof` and a `proves:` field that holds exactly one tag. It can have:

- an informal `## Proof` section in the same constrained Markdown;
- a `formal:` binding to a Lean declaration that proves the formal binding of the statement, or proves it up to a recorded adapter (document 07.4);
- `uses:` edges to every node that the argument depends on.

`uses` on a proof means: "if any of these is false, this argument fails". It does not mean "this is mentioned in passing". For formally bound proofs, `uses` is computed from the Lean dependency graph (document 07.6) and compared with the hand-written list. If `uses` does not include a formal dependency on a math2-bound declaration, that is a finding (`G04`). A hand-written `uses` edge with no formal counterpart is allowed, because informal proofs cite at a coarser grain than Lean.

A proof can name a **method**: induction, diagonal argument, probabilistic method, compactness, the circle method, and so on. Methods are `concept` nodes in the `methods` topic. Because of this, math2 can answer a query like "all proofs in additive combinatorics that use the polynomial method" (document 14).

## 4.7 Status is computed, never asserted

No node file has a field called `status`, `verified` or `proved`. Status is the output of `m2 build` (document 07, 08, 15). The build writes it into the derived store and the release manifest, and never into the content.

This rule exists because the opposite rule fails. A person or an agent who writes `verified: true` makes a claim. Nothing in the file makes the claim true. A year later, Mathlib renames a lemma, a definition below it changes, or a reviewer leaves. The flag stays true. In math2, status is a function of the evidence at a commit. So every build computes it again from the evidence.

Status has two independent axes.

### Truth evidence

The strongest evidence that is present sets the level. The levels have this order:

| Level | Name | Meaning | Who can produce it |
|---|---|---|---|
| E5 | `kernel` | A formal binding with `match: exact` (or `stronger`/`variant` through a checked adapter, document 07.3) and fidelity at least F2 is proved by a Lean declaration that the kernel checks. Its axioms are a subset of `{propext, Classical.choice, Quot.sound}`, plus any named hypotheses, which make the result conditional. It has no `sorry` and no `native_decide`, which caps a node at E4 (07.5). It passes independent replay and the comparator check (07.7) | the build |
| E4 | `certified` | A computation produced a certificate (an LRAT proof, an interval-arithmetic enclosure, a primality certificate, an exact rational witness), and an independent checker accepted it | the build, through the computational engine |
| E3 | `refereed` | At least one proof in the literature is in a refereed venue, and a source record points to it with a locator | curation |
| E2 | `standard` | The result is textbook-standard. At least two independent textbook sources with locators | curation |
| E1 | `claimed` | Someone claims it: a preprint, an agent's informal proof, a MathOverflow answer | anyone |
| E0 | `open` | It is posed and unresolved. Only conjectures and problems may sit here | anyone |
| E⊥ | `refuted` | A counterexample or a proof of the negation exists, at level E2 or above | the build or curation |

This has two important consequences.

**E5 does not mean "true"**, and the workspace never says that it does. E5 means two things. First, this Lean statement is a theorem of Lean's type theory with the standard axioms. Second, three to five people (or one person and a cross-formalisation) agreed that the Lean statement says what the informal statement says. The errors are in the second part. Document 10 is about how to make it reliable.

**An agent's informal proof is E1**, however good it looks. It moves up in only three ways: it becomes formal (E5), it is certified (E4), or it is published and refereed (E3). The agent framework cannot write a status. It can only produce evidence, and the build then grades that evidence (document 12).

### Statement fidelity

Fidelity is how strongly we believe that a formal statement means the same thing as the informal one. It applies only when a formal binding exists.

| Level | Meaning |
|---|---|
| F0 | Bound. Nobody has checked the formal statement against the informal one |
| F1 | Machine-checked. Back-translation, a type-level check and a model-based critic all agree, and checks for degenerate statements pass (document 10.4) |
| F2 | One human review. A reviewer who is not the author has signed off on this exact statement hash |
| F3 | Two independent reviews, or one review plus a second formalisation produced independently and proved equivalent in Lean |

A review is a record in `reviews/<shard>/<tag>.yaml`. It holds the identity of the reviewer, the date, the content hash of the informal statement section, and the content hash of the elaborated Lean type (not its source text). If either hash changes, the review no longer counts. This happens automatically. It is the mechanism that keeps F2 and F3 honest when Mathlib refactors a definition.

### Derived flags

The build also computes flags. Each flag can be queried:

- `ac` / `noac`: the proof uses, or avoids, `Classical.choice`. This is computed for each proof. The statement takes the best value over its proofs.
- `computational`: the proof depends on `decide`, `native_decide` (whose per-use auxiliary axioms the axiom audit sees) or an external certificate.
- `sorry_in_closure`: some node that the proof transitively depends on is not at E5. The blueprint style "proved assuming…" view comes from this flag.
- `stale_review`: a review was invalidated by a hash change and has not been done again.
- `orphan`: the node has no inbound edges and is not a topic root.
- `conditional`: the axiom profile includes hypotheses from `Math2/Hypotheses/`. These hypotheses are listed by name (07.5).
- `external_replay`: E5 rests on the replay results of an external library at its pinned commit, not on math2's own replay (07.7).

## 4.8 Provenance on every field

Every node has a provenance log in `prov/<shard>/<tag>.jsonl`. The log has one line for each change to each field. The line records who changed the field, when, with what, and from which source. The model follows W3C PROV. Document 06.5 gives it in full. In outline:

```json
{"field":"formal[0]","op":"add","agent":"m2-agent/formalize@0.9.1","model":"claude-opus-5-5","run":"r-2026-11-02-8812","human":null,"commit":"c4f1e9a","at":"2026-11-02T09:13:44Z"}
{"field":"formal[0].fidelity_reviews","op":"add","agent":null,"human":"orcid:0000-0002-1825-0097","commit":"e01b7d2","at":"2026-11-04T16:20:11Z"}
```

The rule is that **every byte in `nodes/` can be traced to a person or to an agent run**. Every agent run can be traced to the person or schedule that started it, the model, the prompt version and the tool versions (document 11.6). Audit rule `P01` fails a merge that adds bytes without a provenance line.

This is not bureaucracy. It makes three things possible. First, AI contributions get credit and disclosure in the way that journals and the community now expect (document 16). Second, if a model or prompt version is found to have a systematic flaw, a whole class of agent output can be withdrawn. Third, the statement "this was written by a person" stays true and checkable.

## 4.9 Schema versioning

The node schema has its own version (`node.v1`), separate from releases. Two kinds of change are possible:

- **An additive change** adds a new optional field or a new edge type. It needs only a minor bump and a migration that does nothing.
- **A breaking change** renames a field, changes the kinds or changes the meaning of a field. It needs a new major version, a migration script in `tamnd/math2-agent/migrate/`, and a release that accepts both versions. In that release, rule `S09` flags the old version.

The schema is small on purpose. Put information that has no schema field in `## Remarks`. People can read it there, and the build ignores it. Add a field only when a query or a check needs it.

## 4.10 What a node is not allowed to do

- It cannot state its own status (4.7).
- It cannot have two kinds. A conjecture that is proved becomes a statement: the conjecture node is retired with `reason: split`. The new statement node lists the tag of the conjecture in `history.resolves`, and the conjecture node gets a `resolved_by` edge. A citation of the conjecture still goes to a place that says it is resolved, how and when.
- It cannot contain a proof inside its statement.
- It cannot link outside `m2://` from its statement section.
- It cannot reuse a tag, an alias that is in use, or a `same_as` identifier that another live node already claims. `same_as` is functional in both directions within one external system (rule `G02`). If an external system has two entries for one node, or one entry for two nodes, the edge is `close_to` and the mismatch is recorded (document 05.3).
- It cannot be larger than 64 KB. A longer node is several nodes.
