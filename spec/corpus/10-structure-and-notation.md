# K10. Structure, notation and conventions

A node has no position (K8.2). The corpus still needs structure, for two reasons. A reader must find a node and the nodes around it. A packer must put nodes in an order that a reader can follow (K11). This document says which structures the corpus keeps, and how it keeps notation and conventions the same across a million nodes from hundreds of sources.

## K10.1 Three structures

The corpus has three structures. Each comes from different data, and none of them is a copy of another:

| Structure | Made of | Answers | Section |
|---|---|---|---|
| the logical graph | `uses`, `proves`, `implies`, `specializes` and the other edges of spec 05.2 | what must I know to read this node, and what follows from it? | spec 05 |
| the taxonomy | topic nodes, MSC2020 codes, `about` edges | where does this node belong, and what else is there? | K10.2 |
| the paths | curricula and learning paths: topics with ordered members | in what order do I read these nodes? | K10.3 |

The logical graph is the truth. The taxonomy and the paths are views that people curate. A path that puts a node before one of its prerequisites is a finding (rule N03), because the path is wrong, not the graph.

## K10.2 The taxonomy

- **MSC2020 is the base.** Each node has at least one MSC2020 code (spec 18.2). `topics/msc2020.yaml` maps each code to a topic node, with math2's own short labels (the official descriptions have a licence that does not allow them in math2, spec 20.1).
- **Topic nodes for what MSC splits badly.** Some subjects that people teach as one cross several MSC codes, for example "linear algebra" or "measure-theoretic probability". A topic node holds them, with `about` edges from its members. The skeletons of spec 18 are topic nodes of this kind.
- **Concepts give the fine structure.** Below the level of a topic, the structure is the concept nodes and their `specializes` edges: "metric space" specializes "topological space", and "Banach space" specializes "normed space". A concept with no `specializes` edge in or out is a finding of graph health (spec 05.8).
- **Crosswalks, not imports.** The categories of Wikipedia, the structure of the nLab and the module tree of Mathlib are not the taxonomy of math2. They are crosswalk data: the clustering (K4.3) and the classifier of MSC codes use them, and the workspace can show them.

## K10.3 Curricula and learning paths

A **curriculum** is a topic node in `topics/curricula/` with an ordered list of members. It describes a course, an exam syllabus or a book. Its members are nodes, and each member can have a weight: `core`, `standard` or `optional`.

The corpus imports curricula from public sources as data, not as text (K05). A curriculum lists the items that it names, so it is also an inventory source (K4.2), and its items enter the `curriculum` term of the priority (K12.3). Examples of sources: the lists of undergraduate mathematics in Mathlib, published qualifying exam syllabi, the tables of contents of open textbooks, and national school curricula that are published under an open licence.

A **learning path** is computed, not curated. For a set of target nodes and a reader who knows a set of nodes, the path is an order of the prerequisite closure (spec 05.5) of the targets, minus what the reader knows:

1. Take the closure of the targets over `uses` edges and `defines` edges, with the alternatives of spec 05.5. Where a concept has more than one definition, take only the definition that the path chose (K11.1, `choose`), or by default the one with the smallest closure. Where a statement has more than one proof, choose the proof with the smallest closure that is not known yet.
2. Remove the nodes that the reader knows, and their closures.
3. Order the rest topologically. Among the orders, prefer the one that keeps the nodes of one topic together and follows the order of a curriculum where one exists.

The packer (K11) uses learning paths to order a volume. The workspace uses them for "what do I need to read first?".

The choice of one definition for each concept matters. Metacademy gave each concept one set of prerequisites and named this as its main limit (K01). A concept with two equivalent definitions, such as compactness by open covers and by filters, has two sets of prerequisites. The path uses one of them, and the equivalence is a theorem that the path can add.

The workspace also answers **"what can I read next?"** For the nodes that a reader knows, the answer is the nodes whose prerequisites are all known and which are not known. Knowledge space theory calls this set the outer fringe, and the corpus computes it with one query on the graph.

## K10.4 The notation register

Each source has its own notation, and the same symbol means different things in different areas. A corpus written from hundreds of sources mixes them, unless something stops it. The notation register stops it.

The design takes one idea from sTeX: **a symbol is an identity, and a notation is a way to print it** (K01). Today `macros/m2.sty` and `macros/m2.katex.json` are render macros only. They say how to print `\Nat`, but not what `\Nat` means. The register adds the meaning, and the two macro files become build outputs of the register.

The register is a set of files `notation/<topic>.yaml` in `tamnd/math2`, one for each topic, plus `notation/global.yaml` for the symbols of all areas. Each entry is one macro:

```yaml
- macro: \totient             # the only form that a node body uses
  denotes: 2HQ7XKM4CP         # the tag of the concept or definition
  args: 1
  prec: null                  # for operators: the binding strength
  latex: \varphi(#1)          # the expansion for m2.sty and KaTeX
  typst: phi(#1)              # for the Typst output, when it exists
  intent: null                # a MathML intent concept name, when one exists
  openmath: null              # an OpenMath symbol, for alignment
  reads: "phi of #1"          # how a person reads it, for screen readers
  lean: Nat.totient           # the Lean declaration or notation, when one exists
  scope: [number-theory]      # the topics where the macro is in use
  introduced_by: 2HQ7XKM4CP   # the definition node that introduces it
  convention: null            # one line for the list of notation, when it applies
  variants:                   # other ways to print the same thing
    - {name: phi, latex: \phi(#1), sources: [...]}
  conflicts: [geo.goldenratio] # entries in other scopes that print the same glyph
```

The rules:

- **One macro, one meaning.** Each macro has one `denotes`. A glyph with more than one meaning gets one macro for each meaning: `\pair`, `\ointerval` and `\gcd` for the three uses of $(a,b)$, and `\totient` and `\goldenratio` for the two uses of $\varphi$. A body then says what it means even when the printed symbol is the same. This is also what makes the `intent` of MathML and the screen reader text correct.
- **One default notation for each concept.** A concept can have variants, and a volume or a reader can choose one (K11.1). A body always uses the macro, so the choice changes only the output.
- **A body uses macros in its scope.** The scope of a node is its topics. A body can use a macro of the global scope, a macro whose `scope` has one of the topics of the node, or a macro that the node introduces with its `notation` field (K8.3).
- **Glyphs can repeat across scopes, with a record.** Two entries can print the same glyph only if they are different macros. The `conflicts` field records the pair, and the pack check (PC4) stops a volume that uses both.
- **New notation has a home.** A definition that introduces a symbol is the `introduced_by` node of the entry. The packer uses this to place a notation item where the symbol first appears and to build the list of notation, as PreTeXt does with `<notation>` and `<notation-list>`.
- **Generated files.** The build writes `macros/m2.sty` and `macros/m2.katex.json` from the register. A pull request that edits them by hand fails the build. Rule B01 (declared macros only) stays, and the register adds its meaning.
- **Alignment.** The `openmath` and `intent` fields are alignment data, like a Wikidata id. The export of symbols to Wikidata uses the pattern of the properties P7235 and P9758 (K01).

The write role gets the entries of the scope of its target in its context, as part of the cached prompt (K12.4).

## K10.5 Conventions

Some definitions differ between sources by convention: whether $\mathbb{N}$ contains 0, whether a ring has a unit, whether a compact space is Hausdorff, whether a graph can have loops, which argument of an inner product is linear, where the factor $2\pi$ goes in a Fourier transform. A convention changes what a statement means, so the corpus records it.

- **A register of conventions.** `notation/conventions.yaml` lists each convention: an identifier, the options, the sources that use each option, and the default of math2.
- **The default.** The default follows Mathlib where Mathlib has one (settled decision 16), so that the informal text and the formal binding agree. Where Mathlib has no choice, the default follows ISO 80000-2 where that standard has one, and a curator decides the rest. For $\mathbb{N}$, Mathlib and ISO 80000-2 agree: 0 is a natural number, and ISO writes $\mathbb{N}^*$ for the positive integers.
- **Nodes declare the conventions they depend on.** A concept node lists the conventions that change its meaning (K8.3), in short form: "ring: unital, not necessarily commutative". A definition or statement that depends on a convention links the concept that carries it, so that a change of option finds each node that it affects. A node that uses an option other than the default says so in its main section and in the front matter.
- **Bindings must agree.** The F1 battery (spec 10.4) checks that the convention of the informal text and the convention of the formal binding are the same. A mismatch is the misformalization class "convention" of spec 10.5.
- **Volumes choose.** A volume (K11) can choose another option for a convention. The packer writes a Conventions section from the conventions of its nodes, as each chapter of the Stacks Project has one. The pack check (K11.6) reports each node in the volume whose text depends on the convention, so that an editor can check it.

## K10.6 Names and languages

- **One canonical name for each node**, in English, with the aliases in `registers/aliases.reg`. The canonical name is the one that most sources in the row use (K4.1). A curator decides when the sources disagree.
- **Names of people** follow the form that the MacTutor archive or Wikidata uses, with the diacritics: "Hahn–Banach", "Erdős".
- **Other languages.** Wikidata gives labels in many languages under CC0, and the corpus can import them as aliases with a language code. Translations of bodies follow spec 13.4. A translation is a view of the English text, and it never changes a statement.

## K10.7 Rules

| Rule | H/S | Checks |
|---|---|---|
| N01 | H | each macro in a body has an entry in the notation register whose scope includes a topic of the node, or the global scope, or the node defines it |
| N02 | H | two entries with the same glyph in scopes with a topic in common are the same macro, or each lists the other in `conflicts` |
| N03 | S | a curriculum puts a member before one of its prerequisites |
| N04 | S | a node uses an option of a convention other than the default and does not say so in its front matter |
| N05 | S | a concept with no `specializes` edge and no topic other than its MSC code |
| N06 | H | each macro in the register has one `denotes`, and `macros/m2.sty` and `macros/m2.katex.json` are the output of the register |
| N07 | S | a definition introduces a symbol in its `notation` field, and no register entry has it as `introduced_by` |
