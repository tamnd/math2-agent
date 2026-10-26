# K03. Identifiers: tags of ten characters

The corpus programme makes millions of nodes and tens of millions of inventory rows. A tag of six characters (spec 04.3) gives about 1.07 billion values. That is enough for the nodes, but it is too tight for random minting at the scale of the inventory, and it has no protection against a mistyped tag. This document replaces the tag format of spec 04.3 with a tag of ten characters. It also says what happens to the tags that already exist, how files are sharded, and what must change in the main spec and in the code.

## K3.1 The decision

**A new tag has ten characters: nine random characters and one check character, all from Crockford base32.** The tags of six characters that exist on the day of the change stay valid forever, as a frozen legacy set. No new tag of six characters is minted after that day.

| Property | Tag of 6 (legacy) | Tag of 10 (new) |
|---|---|---|
| alphabet | Crockford base32: `0-9 A-Z` without `I L O U` | the same |
| random characters | 6 | 9 |
| number of values | 32^6, about 1.07 × 10^9 | 32^9, about 3.5 × 10^13 |
| check character | none | 1, by the Luhn mod 32 algorithm (K3.3) |
| example | `3C53TD` | `7K3QX2M9R2` |
| minted after the change | no | yes |

The three rules of spec 04.3 do not change. A tag is assigned once, it is never edited, and it is never reused. The legacy tags are not converted to long tags, because a conversion would edit a tag. A node with a legacy tag keeps it. A node never has two tags.

## K3.2 Why ten characters

- **Room for the inventory.** The inventory (K04) has one row for each candidate item, and the research layer has 10 to 30 million rows (K2.7). Rows and nodes share one space of identifiers (K3.6), so that a bare identifier resolves to one thing. With 6 characters and 30 million identifiers in use, about 3% of random draws collide. With 9 random characters, the chance is below 1 in a million for each draw.
- **Random minting without coordination.** Spec 04.3 mints tags at random, so that two branches can mint without a central counter. A sparse space keeps collisions at merge rare. With 3.5 × 10^13 values and 100 million in use, the chance that one draw collides is about 3 in 10^6.
- **A check character.** People copy tags by hand into papers, emails and search boxes. One mistyped character in a tag of six characters gives another valid tag, which can be the tag of another node. The check character detects each error in one character and almost each swap of two adjacent characters. A tool can then say "this tag has a typo" and not show the wrong node.
- **Still short.** Ten characters fit in a citation, a URL and a Lean attribute. A Stacks tag has four characters, a Wikidata id has up to 10 characters, and a DOI is much longer.

Eleven or twelve characters would give more room, but we have no use for it. Ten characters give room for the research layer at a thousand times its estimated size.

## K3.3 The format

A tag of ten characters is `P1 P2 ... P9 C`:

- `P1` to `P9` are random characters of the alphabet `0123456789ABCDEFGHJKMNPQRSTVWXYZ`. The value of a character is its position in this alphabet, from 0 to 31.
- `C` is the check character. It is computed from `P1` to `P9` by the Luhn mod N algorithm with N = 32:
  1. Start at `P9` and go to the left. The factor of `P9` is 2, the factor of `P8` is 1, then 2, then 1, and so on.
  2. For each character, multiply its value by its factor. Write the product in base 32 and add its digits: `product / 32 + product mod 32`.
  3. Add the results. The check value is `(32 - (sum mod 32)) mod 32`. `C` is the character with that value.
- To validate a tag, do the same over all ten characters with the factor 1 for `C`, then 2 for `P9`, and so on. The tag is valid when the sum is a multiple of 32.

Example: for `7K3QX2M9R` the check character is `2`, thus the tag is `7K3QX2M9R2`. If a person types `7K3XQ2M9R2` (two characters swapped) or `7KZQX2M9R2` (one character changed), the validation fails.

These rules of spec 04.3 apply to long tags too:

- **The seed prefix.** A tag that starts with `0` belongs to the bootstrap seed. `Mint` never gives one. `MintSeed` gives one.
- **No tag that reads as another kind of identifier.** `Mint` does not give a tag that YAML reads as a number, or that starts with an MSC code (for example `20D60...`). For long tags it also refuses a tag with fewer than two letters. That rule removes each tag that could be an ISBN-10 (nine digits and a digit or `X`) and each tag that looks like a phone number or a numeric id.

## K3.4 Input and display

- **The canonical form** is ten capital characters with no separator: `7K3QX2M9R2`. Files, registers, URIs and the API use only the canonical form.
- **For reading**, the workspace and printed works can show a tag in two groups of five: `7K3QX-2M9R2`. The hyphen is not part of the tag.
- **Normalization on input.** The resolver accepts lower case, hyphens and spaces, and the Crockford substitutions `I` and `L` for `1` and `O` for `0`. It normalizes, then it validates the check character. A long tag that fails the check gives the error "not a valid tag: the check character does not match". It never resolves to the nearest valid tag, because that can be another node.
- **Legacy tags** have no check character. The resolver accepts a six-character string only when it is in the legacy set of `registers/tags.reg`.

## K3.5 Sharding

Spec 04.5 shards node files by the first two characters of the tag, which gives 1,024 directories. At a million nodes that is about 1,000 files in each directory. The corpus target for nodes is 1 to 5 million (K2.7), which would give up to 5,000 files in each directory. That makes git trees large and slow.

**The new shard key is the first three characters of the tag**, for long and legacy tags. That gives 32,768 directories:

| Nodes | Files in each directory, on average |
|---|---|
| 1 million | about 31 |
| 5 million | about 153 |
| 30 million | about 916 |

The path of a node is `nodes/<c1 c2 c3>/<tag>.md`, for example `nodes/7K3/7K3QX2M9R2.md` and `nodes/3C5/3C53TD.md`. The same key shards `prov/`, `reviews/` and `compute/`. A path is not an identity, so moving the three legacy node files to the new shards changes no tag. The migration (K3.8) moves them.

## K3.6 One space for nodes and inventory rows

The inventory (K04) has one row for each candidate item. A row gets an identifier when the inventory first records it. The identifier has the format of a long tag, and it is minted from the same generator. It is recorded in the inventory register `registers/inventory/`, which is sharded like the nodes (K4.8), not in `registers/tags.reg`.

- A row identifier is never a node tag, and a node tag is never a row identifier. Rule `T08` (K3.8) checks that the two registers have no value in common.
- With this rule, each bare identifier resolves to one thing: a node, or an inventory row. The URI of a row is `m2://inv/<id>`. The URI of a node stays `m2://<tag>`.
- When a row becomes a node, the node gets its own tag, and the row records it in its outcome. The row identifier is not reused as the tag, because a tag promises a node, and most rows of the research layer never become nodes.

The other identifiers of math2 do not change: provisional ids (`~` and 8 hex characters), run ids, review ids, project ids and record ids of the record plane.

## K3.7 What this changes in the main spec

| Place | Change |
|---|---|
| 00, Conventions | "Tags are 6 characters" becomes "tags are 10 characters (9 random and 1 check character); the legacy tags of 6 characters stay valid". The example becomes `7K3QX2M9R2`. |
| 04.3, Format | replaced by K3.3 and K3.4. The text about 32^6 tags is replaced by K3.2. |
| 04.5 | the shard key is the first three characters (K3.5) |
| 04.4 | a URI accepts both tag lengths, and `m2://inv/<id>` names an inventory row |
| 05.1, 06.4 | the registers include the sharded inventory register `registers/inventory/` (K4.8) |
| 06.7 | the sizes use the new shard counts |
| 15.3, family T | `T08`: the tag register and the inventory register have no value in common; `T09`: each long tag has a valid check character; `T10`: no tag of six characters is minted after the cut-over commit |
| 20.1 | the trees show `nodes/<3>/` |

The change is additive for readers of math2: each URI that worked before still works.

## K3.8 What this changes in the code

The toolchain in `tamnd/math2-agent` holds the tag format in a few places. The change is one pull request for each repository, in this order:

1. **`tag`**: `Len` becomes 10 for new tags, with `LegacyLen` 6. `IsTag` accepts a long tag with a valid check character, or a legacy tag. `IsLegacy` reports the second case. `Mint` and `MintSeed` draw nine characters (45 bits, from 6 random bytes), refuse excluded values (K3.3), and append the check character. A new function `Check(payload string) byte` computes the check character, and `Normalize(s string) (string, error)` implements K3.4. Each exported function keeps a testable example.
2. **`schema/node.v1.json`** in both repositories: the tag pattern becomes `^([0-9A-HJKMNP-TV-Z]{6}|[0-9A-HJKMNP-TV-Z]{10})$`. The check character cannot be expressed in JSON Schema, so rule `T09` checks it. The review id pattern `r-TAG-NN` accepts both lengths. This is an additive change of node.v1 (spec 04.9), because each file that was valid stays valid.
3. **The shard function** (in `node` and its users: `build`, `audit`, `site`, `prov`, `review`, `compute`): three characters. `m2 migrate shard3` moves the files of an existing tree, and it is safe to run again.
4. **`audit`**: `T06` accepts both lengths. New rules `T08`, `T09` and `T10` as in K3.7. `T10` reads the cut-over commit from `registers/README.md`.
5. **The Lean attribute** `Math2.Meta.Attr`, which checks the tag argument of `@[math2 TAG]`, accepts both lengths and the check character.
6. **The site and the API**: the resolver normalizes input (K3.4). Pages and JSON use the canonical form. The display form with a hyphen is used only in printed works (K11).
7. **`tamnd/math2`**: run `m2 migrate shard3`, record the cut-over commit, and freeze the legacy set. On the day of this spec the legacy set is three tags: `3C53TD`, `AAYEQE` and `G5S1M9`.

The test fixtures of the toolchain use legacy tags in many places. They stay as they are, because legacy tags stay valid. New fixtures use long tags.

## K3.9 Rejected options

- **Convert each legacy tag to a long tag.** This would edit a tag, which spec 04.3 forbids. It would also break each link to the published pages of the three legacy nodes. The gain is only that all tags have one length.
- **Ten random characters with no check character.** This gives 32 times more values, which we do not need, and it loses typo detection, which we do need for tags that people type.
- **The check symbol of Crockford's own scheme.** It uses modulo 37 and needs the five extra symbols `* ~ $ = U`. These symbols are not safe in URLs, file names and Lean identifiers.
- **A sequential counter.** It needs coordination between branches, and it leaks the order of creation, which some contributors do not want to show.
