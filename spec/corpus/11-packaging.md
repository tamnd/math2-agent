# K11. Packaging: from nodes to courses, books and articles

The goal says that each node is "ready to go into a course, a book or an article" (K2.1). This document says what that means as a process that a machine can run and check. A **volume** is a selection of nodes with an order, some editorial text and a set of choices. The **packer** compiles a volume into its outputs. The **pack check** says whether the volume is ready.

The packer adds no mathematics. Each definition, statement, proof and example in a volume is a node, and the volume shows it as the node says it. This keeps one truth: an error that is fixed in a node is fixed in each volume at its next build.

## K11.1 The volume file

A volume is a file `volumes/<id>.yaml` in `tamnd/math2`:

| Field | Holds |
|---|---|
| `id`, `title`, `editors` | the identity of the volume and the people who are responsible for it |
| `kind` | `course`, `book`, `notes`, `survey`, `article`, `reference` or `problems` (K11.5) |
| `audience` | the level of the reader, as in K8.3 |
| `targets` | the nodes, topics or curricula that the volume must cover |
| `assumes` | the nodes that the reader knows. The volume lists them and does not explain them |
| `release` | the math2 release that an edition pins |
| `levels` | the lowest writing level and evidence level that a node must have to go in (K11.6) |
| `proofs` | `all`, `main` (the main proof of each statement, with the routine steps folded, K8.5), or `none` |
| `exercises` | how many exercises for each section, and the difficulties |
| `choose` | for each concept with more than one definition in the closure, the definition that the volume uses. The others become theorems of the form "equivalent characterizations", and the packer adds the proofs of the equivalence (K11.2) |
| `conventions`, `notation` | the options of K10.5 and the notation variants of K10.4 that the volume uses |
| `outline` | optional: the chapters and the sections, each with its nodes. When it is absent, the packer makes it (K11.2) |
| `glue` | the editorial text: K11.3 |
| `formats` | the outputs of K11.4 |

## K11.2 Linearization

A volume becomes a sequence in three steps:

1. **The closure.** The packer takes the learning path of K10.3 from `assumes` to `targets`, over the definitions of `choose` only. Each node in the closure that is not in `assumes` goes in the volume. For each concept in `choose`, the other definitions go in as theorems, with their proofs of equivalence.
2. **The outline.** When the volume has no `outline`, the packer groups the nodes by topic into chapters, and by concept into sections. A graph with no cycles has many orders with no forward reference, and the choice among them is about keeping related nodes close. The packer breaks ties in this order: the outline of the topic nodes, then a statement next to its proof and its examples, then the order of a curriculum where one exists, then the MSC code. An editor can then write an `outline`, and the packer checks it.
3. **No forward references in proofs.** This is the rule of the Stacks Project: a proof does not refer to a result that comes later. When an `outline` breaks it, the packer stops with the list of the references, and pack check PC2 fails. The packer never moves a node silently.
4. **Numbering and references.** The packer gives each node a number in the volume: "Theorem 3.4", "Definition 2.1". A link in a body to a node in the volume becomes a reference with the number. A link to a node outside the volume becomes a citation of the node by its URI and its name, in the form that K11.8 gives.

A node can appear in more than one place only as a reference. It has one home in each volume.

The packer also writes two generated sections: **Conventions**, from the conventions of the nodes in the volume (K10.5), and the **list of notation**, from the `introduced_by` entries of the notation register in reading order (K10.4).

**The lock file.** Each build writes `volumes/<id>.lock`, with the `TAG@release` of each node in the volume and the choices that the packer made. The volume file and the lock file together are the method of the volume. A person can read each decision as data, and a second build from the lock gives the same bytes (PC9).

## K11.3 Editorial text

A book needs text that is not a node: a preface, the introduction of a chapter, a sentence that leads from one section to the next. This is the **glue**.

- Glue lives in the volume file, or in files next to it in `volumes/<id>/`. It is prose in the CC BY-SA 4.0 zone, with provenance as for any other text (spec 04.8).
- **Glue does not state mathematics.** It cannot contain a definition, a statement or a proof. If an editor wants to say something mathematical, it is a node. The pack check finds glue that has the form of a statement (pack check PC7).
- Glue that a model wrote is marked as such, and it needs confirmation by an editor of the volume before an edition (rule P04).

## K11.4 The outputs

The packer has one internal form, a typed document tree, and one back end for each output. The tree holds the nodes, the glue, the numbering and the references. Each back end writes it in one format, and no back end changes the content.

| Output | For | Notes |
|---|---|---|
| HTML | the web, in the site of math2 | the same templates as the site, with the status of each node shown, and JSON-LD on each page (K11.4a) |
| PreTeXt XML | books | the PreTeXt tools make web pages, PDF, EPUB, braille and WeBWorK output from one file, so math2 does not write five back ends. Most kinds map to a PreTeXt element. The notation items map to `<notation>`. The PreTeXt tools are GPL, and math2 runs them as separate programs |
| LaTeX and PDF | print, and arXiv | `amsthm` environments and `macros/m2.sty`, one class for each kind of volume. For an `article`, an arXiv source tree: `main.tex`, a copy of `m2.sty` (arXiv has only the packages of TeX Live), a `00README.json` that names the compiler and the TeX Live year, and a `.bib` from the sources of the nodes |
| JATS 1.4 | articles for journals and repositories | each formula in `<alternatives>` with `<tex-math>` (with no `$` delimiters, as JATS4R recommends) and MathML from KaTeX |
| Common Cartridge 1.3 | courses in a learning management system | HTML pages for each node, QTI items from exercises with `check: choice` or `numeric`, and LTI 1.3 links to the workspace for exercises with `check: lean` or `cas` |
| Verso Blueprint | Lean projects | a blueprint document with one directive for each node, the math2 tag as the label, and the `uses` edges of the graph (K11.9) |
| MyST Markdown | Jupyter Book and the notebook tools | computation nodes become code cells |
| Typst | a second typesetting target | only when the `typst` field of the notation register (K10.4) covers the macros of the volume |
| slides | lectures | Beamer, from a volume of kind `course` with one section for each lecture |

The first back ends are HTML, LaTeX and PreTeXt XML (K14.2). EPUB comes from PreTeXt. The others come when a user asks for them. math2 does not write SCORM: Common Cartridge covers the same need, and its makers say that it does not replace SCORM, so a learning management system that needs SCORM can convert. Records of what learners did (xAPI) belong to the workspace (spec 13), never to `tamnd/math2`.

### K11.4a Metadata on each page

The site and each HTML output put JSON-LD of schema.org on each page:

| Property | From |
|---|---|
| `@type` | `DefinedTerm` for a concept, `LearningResource` for an explanation or an exercise |
| `teaches` | the concepts of the node |
| `competencyRequired` | the direct prerequisites: `uses` at depth 1 |
| `assesses` | for an exercise, the nodes that it exercises |
| `educationalLevel` | L1 to L4 (K8.6), as terms of a published SKOS scheme, not free text |
| `learningResourceType` | an LRMI term, for example "worked example" or "exercise" |

This metadata is CC0, like the rest of the metadata zone.

## K11.5 Kinds of volumes

| Kind | Made from | Has |
|---|---|---|
| `course` | a curriculum, with a schedule | sections for each lecture, exercises, problem sheets, solutions in a separate output for teachers |
| `book` | a topic or a set of targets | chapters, full proofs, exercises, an index of terms and of notation |
| `notes` | a short set of targets | proofs `main`, few exercises |
| `survey` | a topic in the research layer | statements without proofs, the history of each result, the citations of the research layer (the `aligned` records), the open problems |
| `article` | a small set of targets with a new result | the format of a journal, with the formal status of each claim |
| `reference` | a topic | definitions and statements only, in the order of the taxonomy, as a handbook |
| `problems` | exercises and problems | an exam pack or a problem book, with solutions |

## K11.6 The pack check

`m2 pack -check VOLUME` says whether a volume is ready. It runs with no model and no network, on a pinned release. Its checks have the prefix PC, so that they are not confused with the audit rules of spec 15.3:

| Check | Requires |
|---|---|
| PC1 levels | each node meets the `levels` of the volume. Each statement that the volume shows as a theorem is at E2 or better. A statement below that is shown with its level, for example as a conjecture or as "claimed" |
| PC2 order | no node comes before one of its prerequisites, and no proof refers to a result that comes after it (K11.2) |
| PC3 closure | each `uses` edge of each node in the volume ends in the volume or in `assumes` |
| PC4 notation | each printed glyph has one meaning in the volume (K10.4). Two macros in the `conflicts` of each other in one volume are a failure, unless the volume chooses a variant for one of them |
| PC5 conventions | each node that depends on a convention uses the option of the volume, or the volume lists it with the difference |
| PC6 references | each reference resolves, in the volume or to a citation |
| PC7 glue | the glue contains no statement, definition or proof, and each part of it has a confirmed writer |
| PC8 licence | the attribution of K11.7 is complete, and rule L02 is clean for each node in the volume |
| PC9 outputs | each output format builds with no error, and the build is reproducible: the same inputs give the same bytes |
| PC10 exercises | each section has the exercises that the volume asks for, and each exercise with an answer check passes it |

The **standard volumes** are the volumes that the corpus itself maintains, one for each area of the undergraduate core (K14.5). The goal of K2.1 counts as met for an area when the pack check passes on its standard volume.

## K11.7 Licence and attribution

- **The prose of a volume is CC BY-SA 4.0**, like the prose of math2 (settled decision 17). A volume cannot have a licence with more restrictions. Anyone can print and sell it, as the licence allows.
- **The Lean companion is Apache-2.0**, like the code of math2. So are the Lean statement and the reference solution of an exercise with `check: lean`: they are Lean code. The text of the exercise is prose.
- **The attribution is generated.** The packer writes an attribution page from the provenance of each node and each part of the glue: the people who wrote and reviewed the content, the share-alike sources that rule L03 requires, and the disclosure of agent contributions that `m2 prov disclose` generates (settled decision 24).
- **Credit to people goes first.** The page lists people by their contributions to the volume. The list of agent runs comes after it, in the form of the disclosure.

## K11.8 Editions, citations and errata

- **An edition pins a release.** The edition of a volume records the math2 release, so that its content is fixed. The volume has its own version, `<id>@<release>`.
- **Citations of nodes.** A volume cites a node outside it by its name and its URI, for example "Sylow theorems, `m2://7K3QX2M9R2`". Printed works can show the tag in the reading form of K3.4.
- **Errata are generated.** When a node of an edition changes in a later release in its main section, the next build of the volume lists the change as an erratum of the edition. A change that only adds sections or explanations is not an erratum.
- **A volume can be a citable work.** Its editors can ask for a DOI for an edition. This is a publication, and it needs the approval of spec 06.2.

## K11.9 The Lean companion

Each volume can have a Lean companion: a Lake project that requires Mathlib and the math2 Lean library at the commits of the release.

- It has one file for each chapter. For each node with a binding, the file has a reference to the declaration with the number of the node in the volume as a comment, and the proof when math2 holds it.
- For each statement with no formal proof, it has the formal statement with `sorry`, marked as open, and a Verso Blueprint of the dependency graph (K11.4). The companion is then a plan for the formalization of the volume.
- The blueprint keeps the origin of each edge: "automatic" when Lean found it, "manual" when the graph of math2 has it and Lean does not. The import of blueprints (spec 09.4) keeps the same distinction in the other direction.
- The companion builds in CI with the release. A companion that does not build fails pack check PC9.
