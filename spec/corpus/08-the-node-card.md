# K08. The node card: what a good node contains

Spec 04.5 says what a node file must contain to be valid. This document says what it must contain to be good. For each kind, a **card** lists the sections and the edges, the order in which a reader sees them, which ones each writing level needs (K2.4), and the style of the text. The writers follow the cards. These writers are people, the roles of K07, and the importers of K05 and K06. The build checks the cards and computes the writing level from them.

The cards are data, not only prose. They live in `schema/cards.v1.yaml` in `tamnd/math2`, and the build reads them. A change to a card is a pull request that a curator approves.

## K8.1 Principles

1. **One node, one thing.** The main section states one definition, one proposition or one argument. Motivation, history and explanation have their own sections (spec 04.5).
2. **Self-contained statements.** A reader can understand the main section with only the linked nodes and the notation register. Each symbol is defined in the section, by a link, or by the notation register (K10.4). "As above" and "under the usual hypotheses" are not allowed.
3. **Reuse by edges, not by copies.** An example is an `example` node with an `instance_of` edge, not a paragraph inside a definition. A card says which edges it shows. The workspace and the packer (K11) put the linked nodes in place. One example can then serve ten definitions and three courses.
4. **Formal and informal side by side.** Each statement and definition card has a place for the formal binding. Each proof card has a structure that maps to a formal proof (K8.5).
5. **Levels are contracts.** An explanation at level L*n* uses only the prerequisites of that level (spec 12.6). The card gives the length and the purpose of each level.
6. **Every section has a writer.** The provenance log records a person or a run for each section (spec 04.8). The card says which sections an agent may write and which need a person to confirm them.

## K8.2 The house style

The text of a node is for a reader who reads one node and leaves. It must be clear on its own, and it must read the same across a million nodes. The house style is plain technical English in the spirit of ASD-STE100, adapted to mathematics:

- **Short sentences.** At most 25 words in a sentence of the main section, and at most 30 words in other sections. A formula counts as one word.
- **One claim in each sentence** in `Statement` and `Definition`. Hypotheses come first, then the conclusion: "Let ... . If ..., then ...".
- **Active voice** in proofs: "We apply Lemma 2" and not "Lemma 2 is applied". "We" is allowed in proofs and explanations. It is not used in `Statement` or `Definition`.
- **No filler words that hide a step.** The words "clearly", "obviously", "trivially", "it is easy to see" and "the reader can verify" are not allowed. If a step is short, give it. If it is long, make it a lemma.
- **Name the convention.** When conventions differ, the text says which one it uses: "Here $\mathbb{N}$ contains 0", "rings have a unit". The concept node records the conventions (K10.5), and the text links to it.
- **Standard words.** Use the term that the concept node names first. Give other terms as aliases in the front matter, not as alternatives in the text.
- **Notation from the register.** Use the macros and symbols of the notation register (K10.4). A new symbol needs a register entry first.
- **No references to place.** Write "the [[Sylow theorems|...]]" and not "the theorem in Section 3". A node has no position. A book gives it one (K11).
- **English first.** The canonical text is English. Translations follow spec 13.4.

`m2 check -style` runs a style linter over the bodies. The linter is mechanical: sentence length, the banned words, undeclared symbols, links that are not tags. Its findings are warnings for W1 and errors for W3.

## K8.3 The common front matter

Each card uses the front matter of node.v1 (spec 04.5). The corpus adds four optional fields. All four are additive (spec 04.9):

| Field | Holds | Why |
|---|---|---|
| `inventory` | the ids of the inventory rows (K04) that this node accounts for | the open count needs the link from row to node, and the reverse link makes the reports cheap |
| `conventions` | for a concept: the conventions that sources use, each with a short name and the sources that use it | K10.5 |
| `notation` | for a concept or a definition: the symbols that it introduces, each with its macro and its meaning | K10.4 |
| `audience` | the lowest curriculum level at which the node is taught: `school`, `ug1` to `ug4`, `grad`, `research` | the packer (K11) and the explain levels need it. The build compares it with the curricula and reports a mismatch |

## K8.4 The cards

The table lists the sections of each card in reading order. **R** means that the section is required for W1, **W2** for W2, **W3** for W3, and **o** means optional. Sections in italics are new, and they come in a minor version of node.v1 (K8.9). Until that version, the text of a new section goes into `Remarks` with a level-three heading of the same name.

### Concept

| Section or edge | Level | Contents |
|---|---|---|
| `Summary` | R | two to four sentences: what the notion is and why it matters. No formula is needed. This is the "Idea" paragraph of the nLab, and readers read it more than any other part of a page (K01). |
| *`Motivation`* | W3 | the question that the notion answers, or the example that leads to it |
| definitions (`defines` edges in) | W2 | at least one definition. The card shows each one with its source, and marks the one that Mathlib uses |
| equivalences (`equivalent` edges) | W2, when there are two or more definitions | a statement for each pair of definitions that a source proves equivalent |
| examples (`instance_of` edges in) | W2 | at least two examples |
| non-examples (`not_instance_of` edges in, K8.9) | W2 | at least one non-example, with the reason |
| `specializes` edges | W2 | the more general notions and the special cases |
| *`Notation`* | o | the symbols and the conventions, from the front matter |
| `History` | W3 | who introduced the notion and when, with sources |
| `Explanations` | W3 | L1 to L3, and L4 for research concepts |
| `Remarks` | o | other facts that have no section |

### Definition

| Section or edge | Level | Contents |
|---|---|---|
| `Definition` | R | the definition, with each condition as a separate numbered clause when there are more than two. It names what is defined in bold the first time, and the linter checks that the bold term is the name of the node or an alias. |
| formal binding | W2, when a library has the definition | `formal:` to a `def`, `structure` or `class` |
| examples and non-examples | W2 | one of each, by edges. A non-example fails exactly one clause, and its edge names the clause (K8.4a) |
| *`Notation`* | o | |
| `Remarks` | o | why this definition and not another, and what changes under other conventions |
| `Explanations` | W3 | |

### Statement

| Section or edge | Level | Contents |
|---|---|---|
| `Statement` | R | the proposition. Hypotheses first, then the conclusion. For a statement with several parts, a numbered list, one part in each item. |
| `uses` edges | R | each definition that the statement needs to make sense |
| `about` edges | W2 | the concepts that the statement is about |
| proofs (`proofs:`) | W2 | at least one proof node, informal or formal |
| examples (`instance_of` or `illustrates` edges) | W2 | one example that shows the statement at work |
| a counterexample to a weaker form | W3, when one is known | the hypothesis that cannot be removed, and the counterexample that shows it |
| formal binding | W2, when a library has the statement; otherwise a formalize job | spec 07.3 |
| `History` | W3 | who stated it first, who proved it first, with sources |
| `Explanations` | W2: L2; W3: each level that applies | |
| exercises (K8.7) | W3, when the node is in a curriculum | at least one |
| `Remarks` | o | generalizations, the converse, common errors |

### Proof

| Section or edge | Level | Contents |
|---|---|---|
| `Idea` | W2 | one to three sentences: the method and the key step |
| `Proof` | R | the argument in numbered steps (K8.5) |
| `uses` edges | R | each node that the argument needs |
| `method` edges | W2 | the methods, from the `methods` topic |
| formal binding | o | a Lean proof of the bound statement |
| *`Variants`* | o | the changes that give a proof of a nearby statement |

### Example and counterexample

| Section or edge | Level | Contents |
|---|---|---|
| `Example` or `Counterexample` | R | the instance, and the check that it is an instance (or that it refutes the target) |
| `instance_of`, `not_instance_of` or `refutes` edges | R | |
| computation | W3, when the check is computational | a `computation` node that runs the check (spec 08) |
| `Explanations` | o | |

### Conjecture and problem

These follow spec 04.2 and the problem page of spec 13.3. The card adds `bounds` edges and a `History` with the year of the first source, both for W2.

### Object

| Section or edge | Level | Contents |
|---|---|---|
| `Description` | R | what the object is, its standard name and its main invariants |
| `same_as` | R | the identifiers in the databases (LMFDB label, OEIS number, GAP id, ATLAS name) |
| invariants | W2 | a table of the main invariants, each with a source or a computation |
| `instance_of` | W2 | the concepts that it is an instance of |

### K8.4a Clauses, examples and non-examples

The research on how people learn definitions says that the best non-examples are near misses: objects that are almost examples (K01). So the corpus makes near misses easy to find and to check:

- Each clause of a definition has a number (K8.4, `Definition`).
- A `not_instance_of` edge has a field `fails` with the number of the clause that the non-example fails. A non-example that fails more than one clause lists them all, and it counts as a near miss only for a single clause.
- An `instance_of` edge can list the clauses that the example shows at work.
- The build reports **clause coverage** for each definition: the clauses that have a near-miss non-example. At W3, each clause of a definition with three or more clauses has one.

## K8.5 Proofs in steps

An informal proof in the corpus is a list of numbered steps. This form serves three readers at once: a person reads it as a proof, a formalizer reads it as a plan, and the packer can show it at two levels of detail.

- **Each step states one claim and its reason.** The reason is a link to a node, an earlier step, or a computation: "3. $H$ has index 2 in $G$, by step 2 and [[Lagrange's theorem|7K3QX2M9R2]]."
- **Steps can have substeps** (3.1, 3.2) to two levels. A step with more than two levels is a lemma, which is its own node.
- **The last step is the conclusion** of the statement, in the words of the statement.
- **A step can be marked `routine`.** A short form of the proof leaves out the routine steps. The marks come from the writer and a person can change them.
- **The form is Markdown.** Steps are a numbered list, and substeps are a nested list. This is the structured proof of Lamport in plain Markdown, with no new syntax.
- **Map to Lean.** When a proof has a formal version, each step can name the `have` or the lemma that matches it. This is the "Unfold this proof" view of spec 13.4 in the other direction. The prove role (spec 12.4) uses the steps as its plan.

A proof that a model wrote is E1 (spec 04.7). The card does not change that. It makes the proof easy to check: a reviewer checks one step at a time, and a formalizer proves one step at a time.

## K8.6 Explanations at levels

Spec 04.5 gives four levels. The card fixes their purpose and their length:

| Level | For | Stage (Tao) | Purpose | Length |
|---|---|---|---|---|
| L1 | a person who has not met the concept | pre-rigorous | what it is, with one picture or one concrete example, no proof | 80 to 250 words |
| L2 | an undergraduate | rigorous | the definition unpacked, why it is true or why it is defined this way, the standard examples and non-examples, the common errors | 150 to 500 words |
| L3 | a graduate student | post-rigorous | the right way to think about it, how it fits with the rest of the field, the generalizations, where it breaks | 150 to 600 words |
| L4 | a researcher | post-rigorous | the state of the art, the open questions, the literature | 100 to 500 words, with sources |

The stage gives the writer and the reviewer a test that does not depend on the label of the audience. The evidence on worked examples (K01) gives three more rules:

- L1 and L2 use **correct** worked examples, written as numbered steps, so that a renderer can fade them: it shows the first steps and asks the reader for the rest. An example with an error for the reader to find is an exercise, not an explanation.
- L3 and L4 do not include full worked computations. They help novices and can hurt advanced readers (the expertise reversal effect).
- A guide on how to read a proof is one explanation node at the level of a topic, linked from proof pages. It is not repeated on each proof.

A node does not need each level. A research lemma needs L3 or L4, not L1. A school concept needs L1 and L2. The card of each kind says which levels W3 needs. The `audience` field (K8.3) decides it: each level from the audience up to L3, and L4 for audiences `grad` and `research`.

## K8.7 Exercises

Courses need exercises, and the textbook layer has many of them under open licences (K05). The corpus holds an exercise as its own node, so that a course can choose from a pool:

| Section or field | Contents |
|---|---|
| `Exercise` | the task, self-contained as a statement is |
| `Hint` | optional, one or two sentences |
| `Answer` | the final answer, when the task has one |
| `Solution` | a worked solution in the step form of K8.5 |
| `about`, `uses` edges | the nodes that the exercise practises and needs |
| `difficulty` | 1 to 5, set by the writer and corrected by use |
| `math_group` | `A`, `B` or `C`, from the MATH taxonomy: A is facts and routine procedures, B is transfer and application, C is justification, conjecture and evaluation. The groups describe the kind of task, not its difficulty |
| `check` | how a machine checks an answer: `exact` (a value), `numeric` (a value with a tolerance), `choice` (multiple choice), `cas` (equal up to a CAS normal form, spec 08), `lean` (a Lean statement with `sorry` that the answer must fill, graded with the comparator), or `none` (checked by people) |
| `external` | links to interactive versions in other systems, for example a WeBWorK OPL path or a MyOpenMath id. Links only: most of those banks have licences that the prose zone cannot take (K05) |

The build reports the groups of the exercises of each topic. Banks that models write tend to have few exercises in group C, and the report shows that gap.

An exercise with `check: lean` is also a formal statement. Its reference solution must compile, and the version with `sorry` must have the same type. A course can then grade it with the Lean server (spec 11.3), as the Natural Number Game does.

## K8.8 What the build computes from the cards

- **The writing level** of each node (K2.4), from the sections that are present, the edges, and the provenance of each section.
- **The card findings**: a required section that is missing, a section that is too long, a level that uses a node outside its prerequisites, a style finding. These are warnings, not audit failures, except where a rule of spec 15 already applies.
- **The work list** for each node: the sections that are missing for the next writing level. The orchestrator turns the work list into jobs for the roles of K07 (K12).

## K8.9 Proposed schema changes

Spec 04.9 has two kinds of change. Most of the changes below are additive, and they come in a minor version of node.v1. The new kind changes the list of kinds, and spec 04.9 calls that a breaking change. It comes in node.v2, with a migration and a release in which both versions are accepted.

| Change | Type | Version | Reason |
|---|---|---|---|
| edge `not_instance_of` (example → concept or definition), with the field `fails` | new edge | node.v1 minor | non-examples are half of a good definition card, and `refutes` is for statements (K8.4a) |
| edge `illustrates` (example → statement) | new edge | node.v1 minor | an example that shows a statement at work is not an instance of a concept |
| sections `Motivation`, `Notation` (concept and definition), `Variants` (proof) | new allowed sections | node.v1 minor | K8.4 |
| fields `inventory`, `conventions`, `notation`, `audience` | new optional fields | node.v1 minor | K8.3 |
| `schema/cards.v1.yaml` | new file | none | the cards as data |
| kind `exercise` with the sections and the fields `difficulty`, `math_group`, `check` and `external` of K8.7 | new kind | node.v2 | courses need exercises, and an exercise is not an example: it has a task and an answer |

The migration to node.v2 finds each `example` node that has the role `exercise` in `## Remarks` (K2.3). It changes the kind, and it moves the text into the sections of K8.7. The tag does not change. Spec 21 gets one entry for the new kind.
