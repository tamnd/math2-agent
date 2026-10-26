# K04. The inventory

The goal of K02 says "each definition, theorem, proof and example ... that a person can find in a public source". To know when that goal is reached, math2 needs a list of what is out there. The inventory is that list. It is the denominator of the corpus: the open count (K2.1) counts its rows that have no outcome.

This document says what a row is, how rows come from sources, how mentions of one item become one row, which outcomes a row can have, how we measure what the inventory misses, and where it is stored.

## K4.1 Mentions and rows

The inventory has two levels:

- A **mention** is one item as one source presents it. "Theorem 3.14 of Rudin's *Principles of Mathematical Analysis*, 3rd edition", "the Wikipedia article *Heine–Borel theorem*", "the Mathlib declaration `Metric.isCompact_iff_isClosed_bounded`" and "item 47 of the list that the enumerate role made for MSC 54D30" are four mentions.
- A **row** is one distinct item, in the sense of K2.3. It groups the mentions of that item. The four mentions above can be one row, "Heine–Borel theorem for $\mathbb{R}^n$", or two rows if one of them states a more general form.

Mentions are records of the record plane. They use the record identifier of spec 9.5: `m2://rec/<source>/<id>#<n>`, or `ax://paper/<id>#<tag>` for arXiv. Rows have identifiers of the format of K03, in the space that they share with node tags (K3.6).

A mention holds:

| Field | Holds |
|---|---|
| `id` | the record identifier |
| `source` | the source, from `manifests/sources.yaml` |
| `snapshot` | the hash of the source snapshot that the importer read |
| `locator` | where in the source: a URL with an anchor, a theorem number, a page, a declaration name |
| `native_id` | the identifier of the item in the source, when it has one: a Wikidata QID, a ProofWiki page, a Mathlib declaration, an nLab page, an OEIS number |
| `kind` | the kind that the importer or the model reads, one of the 11 kinds of spec 04.2, or `unknown` |
| `name` | the name that the source uses, if it has one |
| `text` | the statement text, only when the licence class allows text in the record plane (spec 9.3); otherwise its hash and its embedding, which stay internal |
| `msc` | the MSC2020 codes, from the source or from the classifier |
| `class` | the licence class of the source or of the item (spec 9.3) |
| `by` | the importer or the run that made the mention |

A row holds:

| Field | Holds |
|---|---|
| `id` | the row identifier |
| `mentions` | the mention identifiers |
| `kind`, `name`, `msc` | the values that the clustering decided |
| `layer` | `textbook`, `formal` or `research` (K2.7), computed from the sources of the mentions |
| `spread` | the number of independent sources among the mentions (K12.3) |
| `outcome` | K4.4, or empty |
| `history` | each merge, split and outcome change, with the commit and the person or run |

## K4.2 Where mentions come from

Each source in scope has an importer. K05 lists the sources of people and K06 the formal libraries. The importers give the mentions of their source, and nothing else: an importer does not decide which mentions are the same item.

Mentions come from four kinds of place:

1. **Reference works and wikis**: Wikidata, Wikipedia, ProofWiki, nLab, PlanetMath, the Encyclopedia of Mathematics, the Stacks project, Kerodon, the Digital Library of Mathematical Functions and others (K05). These give names, statements and a structure of links.
2. **Textbooks and lecture notes**: the theorem environments of open textbooks give text. The indexes and the tables of contents of closed textbooks give records and locators only (K05).
3. **Formal libraries**: Mathlib and the other libraries of K06. Each declaration is a mention. Most of them are not items that a person cites (K4.5).
4. **Models**: the enumerate role (K7.3) asks models to list each item of an area that they know, with a source for each. A model list is a mention source like any other. Its mentions are marked `by: agent`, and K4.3 never makes a row from model mentions alone (rule I03).

The research literature is the record plane of spec 09. Its extracted results are mentions already.

## K4.3 From mentions to rows

The clustering follows the four passes of spec 05.4, in order. Each pass is cheaper and more certain than the next one:

1. **Identifiers.** Two mentions with the same identifier, or with identifiers that a crosswalk links, belong to one row. The crosswalks are the `same_as` edges of the graph, the external-identifier properties of Wikidata, and the links between the sources themselves (an nLab page that links to a Wikipedia article on the same notion, or a Mathlib docstring that names a theorem).
2. **Formal equivalence.** Two mentions with formal statements that a Lean proof shows equivalent belong to one row.
3. **Similarity and judgement.** For each mention that has no row, the clustering finds the nearest rows by name, by embedding and by the links of the mention. The dedup judge classifies each candidate pair as `same`, `generalizes`, `specializes`, `related` or `different` (spec 05.4). A `same` with high confidence and with agreement of the name and the MSC code joins the row. Other `same` verdicts go to the `cluster` queue (K12.2).
4. **People.** Curators decide the close cases in the queue.

The granularity rules of spec 05.4 apply to rows. A family of forms that people cite separately is a row for each form, and a row for the hub. A `generalizes` or `specializes` verdict gives two rows and an edge between them, not one row.

The clustering is measured. A gold set of 2,000 pairs for each layer, with decisions by curators, gives the precision and the recall of each pass. The release manifest records them. An error in clustering has two forms, and both are bad. A false merge hides an item, and the open count does not show it. A false split gives two nodes for one item, and the dedup queue must find it later.

## K4.4 Outcomes

Each row must get an outcome. The open count is the number of rows that have none. The target is zero (K2.1).

| Outcome | Means | Who decides |
|---|---|---|
| `node` | the row is a node, with its tag | the curate role proposes, the merge decides |
| `member` | the row is a member of a hub family, as its own node | the same |
| `merged` | the row is the same item as another row, and it points to that row | the clustering, or a curator |
| `split` | the row was more than one item, and it points to the rows that replace it | a curator |
| `aligned` | the row is a result of the research layer that a node states, and it has an `aligned` edge to the node | the align role, with the rules of spec 09.6 |
| `record` | the row stays a record of the research layer, with no node, until the graph needs it (spec 9.8) | the rule K4.5 |
| `formal_only` | the row is a declaration of a formal library that no person cites, kept in the crosswalk (K4.5) | the rule K4.5 |
| `object_record` | the row is a database object that stays a record (K4.6) | the rule K4.6 |
| `out_of_scope` | the row is not mathematics in the sense of spec 18.2, or it is a notation, a teaching note or a mistake in the source. The reason is required | a curator, or a rule that a curator approved |
| `deferred` | the decision waits for something named, until a date | a curator |

`merged` and `split` keep the row identifier in the register forever, as a tag is kept (spec 04.3). A link to an old row identifier resolves to its successor.

A `deferred` row counts in the open count when its date passes. A `deferred` row with no date is not allowed.

Most outcomes come from rules, not from people. A rule is a decision that a curator approved once and that applies to many rows, for example "each mention from the enumerate role that has no other mention after 90 days gets `out_of_scope`, reason: no source confirmed". The rules live in `schema/outcomes.v1.yaml`, and each outcome that a rule gave names the rule.

## K4.5 Which formal declarations and research results become nodes

The formal libraries have many declarations that no person would cite: API lemmas, simp lemmas, instances and auxiliary results. A node for each one would give a graph that nobody can read. The research layer has 10 to 30 million results, and spec 9.8 already keeps them as records until the graph needs them.

**A formal declaration becomes a node** when at least one of these is true:

- a mention from a human source (K05) is in the same row;
- the library documents it as a main result, in the module documentation, the list of named theorems, or a list such as "100 theorems" or "1000+ theorems";
- a node of the graph uses it in a `uses` edge that a person wrote;
- a person asks for it.

Each other declaration gets the outcome `formal_only`. It stays in the crosswalk, so that the formal binding of a node can point to it, and search can find it. Spec 07 and K06 give the details for each library.

**A research result becomes a node** when spec 9.8 says so: when the graph needs it, or when a project or a campaign asks for its area (spec 18.5). Until then its row has the outcome `record` or `aligned`. The open count of the research layer goes to zero by these two outcomes, not by nodes.

## K4.6 Which objects become nodes

The databases of mathematical objects hold millions of records: the OEIS, the LMFDB, the House of Graphs, the ATLAS of finite groups, the GAP libraries of small groups and others. A node for each would add little that the database does not have. The rule is in line with spec 06.7:

- **An object becomes a node** when a human source names it, when a node cites it, when it is in a curated list (the sporadic groups, the exceptional Lie algebras, the regular polytopes), or when a project asks for it.
- **The OEIS** is a special case. Each sequence has a name and a curated entry, and spec 06.7 counts them as nodes. They become nodes in bulk, at W0, with `same_as` to the A-number.
- **Each other object** gets the outcome `object_record`. It keeps its database identifier as a `same_as` target, so that a node can point to it.

## K4.7 Measuring what the inventory misses

The open count is zero only for the rows that exist. An item that no source in scope holds, and that no model lists, has no row. So the inventory needs a measure of its own recall.

1. **Capture and recapture.** When two independent sources cover the same area, the overlap of their mentions gives an estimate of the total. If source A has *a* items of an area, source B has *b*, and *m* items are in both, the estimate of the total is *a* · *b* / *m* (the Lincoln and Petersen estimator). With three or more sources, log-linear models give a better estimate, and they allow for sources that depend on each other. The estimate is computed for each MSC code at two digits, each quarter.
2. **A held-out source.** When a new source enters scope, the clustering runs on it before the importer adds its mentions. The share of its items that already have rows is an estimate of the recall of the inventory for its area.
3. **Index terms.** The indexes of standard textbooks list the terms that each book defines. Each index term of a book in scope must match a row. The match rate for each book is in the report.
4. **Model lists.** The enumerate role is a recall tool, not a source of truth. An item that a model lists, and that the inventory does not have, is a candidate gap. A gap becomes a row only when K9.5 confirms a source for it.

The report gives the estimated recall for each layer and each MSC code at two digits, with the method and the interval. Where the estimate of the total is much larger than the number of rows, the area goes to the `enumerate` queue and to the importer plan of K05.

## K4.8 Storage

- **Mentions** are files of the record plane: `records/mentions/<source>/<c1c2c3>.jsonl`, sorted by identifier, one mention per line. The shard key is the first three characters of a hash of the mention identifier.
- **Rows** are in `records/inventory/<c1c2c3>.jsonl`, sharded by the first three characters of the row identifier (K3.5), one row per line.
- **The identifier register** is `registers/inventory/<c1c2c3>.reg`, sharded in the same way, because at the scale of the research layer one file would be larger than a gigabyte. Its rules are those of `registers/tags.reg`: lines are never removed or edited. K3.6 and rule T08 apply to it.
- **The licence zone** of mentions and rows is metadata, CC0 (settled decision 17). A mention with text holds only text that its class allows in the record plane (spec 9.3), under the licence of its source.
- When the record plane moves to `tamnd/math2-records` (spec 06.1), mentions, rows and the inventory register move with it.

The build reads the inventory and writes the open count, the coverage of each layer and the queues of K12.2 into the derived store.

## K4.9 Rules

| Rule | H/S | Checks |
|---|---|---|
| I01 | H | each `inventory` value in a node points to a row whose outcome is `node` or `member` with the tag of that node |
| I02 | H | each row with the outcome `node` or `member` points to a live node, or to a retired node with a successor |
| I03 | H | no row has only mentions from models. Such mentions wait in the `enumerate` results until K9.5 confirms a source |
| I04 | H | each outcome that a rule gave names a rule in `schema/outcomes.v1.yaml` |
| I05 | S | a `deferred` row whose date has passed |
| I06 | S | an MSC code at two digits whose estimated recall (K4.7) is below 90% |
