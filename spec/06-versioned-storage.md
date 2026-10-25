# 06. Versioned storage: metadata, artefacts, provenance

This is the bottom-right box of the diagram. Every other box writes into it. It holds three things, and each has its own versioning rules:

- **metadata**: the node files, edges, reviews, manifests and registers. They are small and people edit them. They are in git.
- **artefacts**: compiled Lean `.olean` files, built graph Parquet, search indexes, computation outputs, certificates and rendered sites. They are large and derived. They are in content-addressed storage.
- **provenance**: who and what made each byte of the other two, and from what.

The goal of the layer is one sentence: **given a release name, anyone can get back exactly what math2 said, rebuild it, and check that the rebuild matches**.

## 6.1 Three repositories, one lineage

Storage is not one repository.

| Location | Holds | Versioned by | Licence |
|---|---|---|---|
| `tamnd/math2` (git) | `nodes/`, `reviews/`, `prov/`, `lean/` (the math2 Lean library), `manifests/`, `registers/`, `reports/`, `schema/`, `macros/` | git commits and release tags | prose CC BY-SA 4.0; Lean code Apache-2.0; metadata CC0 (document 16.6) |
| `tamnd/math2-agent` (git) | the `m2` toolchain, agent code, prompts, tool images, benchmarks | git and semver | Apache-2.0 |
| the artefact store | everything derived, keyed by BLAKE3 hash | content address | follows its inputs |

The artefact store is an S3-compatible bucket (R2 or similar) behind a plain HTTPS read path. It also has a second, independent mirror for releases (6.6). Nothing in it is mutable. Nothing in it is the only copy of a decision that a person made. If we lose it, we lose compute, not knowledge.

### Why git for metadata

At the scale in document 18, the git repository is large but still workable. It has a few million small files and a few GB packed. This is about the size of the history of the Linux kernel. Shards by tag prefix (document 04.5) keep directories small. Git gives four things for free that a database would make us build:

- review by diff;
- attribution by commit;
- cheap branches for agents (each agent run works on a branch);
- tools for mirrors and archives that have worked for decades.

If the repository becomes larger than 10 GB packed or 5 million files, the record plane moves to its own repository, `tamnd/math2-records`. The record plane holds most of the files. It keeps the same layout and rules. The trigger is a number and not a feeling. Document 21 tracks it.

## 6.2 Releases

math2 uses calendar versions: `YYYY.MM`, monthly. We cut each release on the first Monday of the month. Patch releases `YYYY.MM.N` are for fixes. Lean and Mathlib also do not use semver. Lean has a monthly release candidate cycle. A monthly math2 cadence matches it closely enough, so each math2 release can pin one Lean stable release.

A release is a git tag on `tamnd/math2` plus a **release manifest**, `manifests/releases/2026.11.yaml`:

```yaml
release: 2026.11
commit: 8b1e4c0f2d...            # tamnd/math2
toolchain:
  m2: v0.9.0                      # tamnd/math2-agent tag
  lean: leanprover/lean4:v4.NN.0 # the stable toolchain pinned for this release
  mathlib: <commit sha>           # one Mathlib revision, compatible with the toolchain
  other_libs:                     # optional formal bindings outside Lean
    afp: 2026-10-30
  images:                         # computational engine containers, by digest
    sage: ghcr.io/tamnd/m2-sage@sha256:...
    pari: ghcr.io/tamnd/m2-pari@sha256:...
    gap:  ghcr.io/tamnd/m2-gap@sha256:...
    smt:  ghcr.io/tamnd/m2-smt@sha256:...
artefacts:                        # the hash of the files and the size in bytes
  graph:  {hash: blake3:af01..., size: 3100000000}
  oleans: {hash: blake3:77c2..., size: 9800000000}
  search: {hash: blake3:1e9b..., size: 6400000000}
  site:   {hash: blake3:c0de..., size: 2200000000}
  certs:  {hash: blake3:51aa..., size: 840000000}
counts:                           # filled by the build. The README table is generated from this
  nodes_live: 0
  by_kind: {}
  by_evidence: {E5: 0, E4: 0, E3: 0, E2: 0, E1: 0, E0: 0}
archived:
  swh: swh:1:rev:...              # Software Heritage revision id
  zenodo_doi: 10.5281/zenodo....  # concept DOI is stable across releases
rebuild_check:
  runner: ubuntu-24.04/x86_64/32c/128G
  graph_hash_reproduced: true
  oleans_hash_reproduced: true
  olean_differences: []           # each file that is different, with a reason, when oleans_hash_reproduced is false
```

The rules:

1. **A release pins exactly one Lean toolchain and one Mathlib revision.** Between releases, `main` follows Mathlib's `master` on a weekly bump branch (document 07.9). Never cut a release from a state where the bump is red.
2. **Every artefact that a release names can be rebuilt from the commit and the toolchain.** For the graph and the search index, the rebuild must match exactly. Lean builds are deterministic, so the `.olean`s must be bit-identical. If they are not, the release must say which files are different and why (rule `R02`).
3. **Releases are archived outside the project.** Software Heritage gets the git tag (a SWHID). Zenodo gets the manifest and the graph Parquet (a DOI). If the project disappears, a 2026.11 citation still resolves.
4. **The release notes are generated, then edited.** `m2 release notes 2026.10..2026.11` makes these items: counts per topic, nodes that moved evidence level, newly resolved conjectures, retired tags with successors, and binding changes that Mathlib renames caused. A person edits the prose. A release cannot change counts if the notes do not say so (rule `R03`).

A release has two more files next to its manifest. `2026.11.levels` gives the evidence level and the fidelity level of each live node, one line for each node, thus the notes of the next release can list the nodes that moved evidence level without a build of an old commit. `2026.11.md` has the notes. Their counts table has one row for each count of the manifest that changed: the count, the number before, the number after, the change and the cause. `m2 release notes` writes the cause from the node files: the new nodes, the retired nodes and the nodes that moved between kinds or levels. When the node files do not give the change, the cause is empty, and a person writes it.

`m2 release cut` cuts a release in two runs. The first run builds the tree and commits the three files. Then a person writes the summary of the notes, and a second machine runs `m2 release verify -record` to fill the rebuild check. The second run refuses when a file outside `manifests/releases/` and `reports/` changed after the commit of the manifest, runs the full audit, and makes the tag only when the audit passes.

### Citing math2

The citation form is `m2://<tag>@<release>`. We also print it as a DOI-backed URL. A paper can cite `m2://7K3QX2@2026.11`. In 2040 a reader can resolve it to the exact text, formal statement, evidence and status as of November 2026. This is true even if the node was retired in 2029.

## 6.3 Content addressing

Each artefact has its BLAKE3 hash as its name. Each node has a **content hash**. We compute it over the canonical serialisation of the node: front matter in key-sorted canonical YAML, and the body normalised to NFC with trailing whitespace removed. Each formal binding has an **elaborated-type hash**. This is a hash of the Lean `Expr` of the type of the bound declaration after elaboration. Universe levels are normalised and auxiliary names are removed.

The elaborated-type hash is the most important hash in the system. Fidelity reviews (document 04.7) are bound to it. Mathlib can change a definition under a statement. Then the elaborated type changes and the hash changes. The review stops counting, and the node drops from F2 to F0 with `stale_review` set. This happens even though no text in math2 changed. Text hashes cannot find this change, because the text of `theorem foo : Bar x` is the same before and after `Bar` changes. Type hashes can find it.

Computation records (document 08) have the hash of `(image digest, entry point, inputs, seed)` as their address. If you run the same computation two times, you must get the same output hash. If you do not, that is a finding: rule `C02`, nondeterminism (document 08.6).

## 6.4 Registers

Some facts must never be lost and must never change. They live in append-only registers in `registers/`:

| Register | One line per | Rule |
|---|---|---|
| `tags.reg` | minted tag: tag, minted-at commit, kind | `T01`: lines are never removed or edited |
| `aliases.reg` | alias assignment: alias, tag, from, until | an alias can move. Its history cannot |
| `retired.reg` | retirement: tag, date, reason, successors | |
| `bindings.reg` | formal binding change: tag, system, old decl, new decl, cause (bind/rename/refactor/rebind), Mathlib commit, author | answers "which Mathlib name did this node have in March 2027" |
| `reviewers.reg` | reviewer identity: ORCID or GitHub handle, first review date, areas, affiliations | needed for F2/F3 independence (document 10.6) |
| `crosswalk.reg` | crosswalk decision: tag, edge, system, id, decision (accepted/rejected/confirmed), person, date, record hash | `A02`, `A03`: a person decided on each `same_as` value, and the record did not change after the decision |

Registers are plain text, sorted, with one record per line. Thus `git log -p` on a register is the full history of that decision space. Each build rebuilds the registers and checks them against the node files (rule `T05`).

## 6.5 Provenance

Each change to each field of each node, review and register has a provenance record (document 04.8). The model follows W3C PROV-O, with these classes:

- **Entity**: a node version (tag plus content hash), a formal declaration (name plus type hash plus Mathlib commit), a computation output (hash), a source record (external id plus revision);
- **Activity**: an agent run, a human edit (a commit), an import job, a build;
- **Agent** (in the PROV sense): a person (ORCID or GitHub id), a math2 agent role at a version, a model id, an external system.

The relations are `wasGeneratedBy`, `used`, `wasAssociatedWith`, `wasDerivedFrom`, `wasRevisionOf`, `wasAttributedTo` and `actedOnBehalfOf`. The last relation links an agent run to the person or schedule that started it.

Provenance is in `prov/<shard>/<tag>.jsonl`, with one JSON object per line. Each release exports it as PROV-JSON-LD. The full record of an agent run is larger: the prompts, each tool call and its output, token counts and cost. It is in the artefact store, keyed by run id. The provenance line of a node cites the run id. Thus the chain is complete, and git does not hold gigabytes of transcripts (document 11.6).

### What provenance has to make possible

`math2-agent/bench/prov/` runs these tests against a release:

1. **Attribution.** For a node, list the people and the agent runs that contributed to its current text, formal binding and reviews.
2. **Withdrawal.** A model id and a prompt version can be found to be systematically wrong. For that pair, list each field that it produced and that a person has not replaced or confirmed since. Then open a pull request that reverts or flags all of them (`m2 prov quarantine --model … --prompt …`).
3. **Licence tracing.** For a node, list each external source that its text came from, with the licence of that source. This is the input to the licence gate (document 09.3).
4. **Credit export.** For a person, make a CRediT-style statement of what they contributed to math2 in a period. Then the work counts as scholarly output (document 16.4).

## 6.6 Mirrors, backups, retention

- **git:** GitHub is primary. Codeberg and a self-hosted bare repository are mirrors. Each release is archived to Software Heritage.
- **Artefact store:** two providers. Keep releases forever. Keep builds of `main` for 90 days. Keep agent run records in full for 2 years, and keep their summaries (cost, outcome, the produced diff) forever.
- **Restore drill:** each quarter, CI rebuilds the latest release on a clean machine. It uses only the Software Heritage copy and the second artefact mirror. Then it checks the manifest hashes. A failed drill blocks the next release (rule `R04`). Each drill writes a record to `reports/restore/YYYY-MM-DD.yaml` with the fields `date`, `release`, `runner`, `passed` and `note`. Rule `R04` reads the last record. It fails when that drill failed, or when the drill is more than one quarter (92 days) older than the month of the last release.

## 6.7 Sizes

These estimates are for the end of the milestones in document 19. Each release manifest records the actual numbers.

| Item | Estimate | Basis |
|---|---|---|
| live nodes (statement plane) | 2–5 M | Mathlib-bound nodes (~0.3–0.5 M), curated literature results, OEIS sequences (~390 k), LMFDB objects as nodes only when cited |
| record plane entries | ~5 M records, ~30 M aligned results | zbMATH Open's ~4.9 M documents and arXiv math. Results are extracted only from licence-permitted full text (document 09) |
| git repository | 3–8 GB packed | about 1 KB average per node file, plus provenance lines |
| `.olean` cache | 10–15 GB | Mathlib plus the math2 library |
| graph Parquet | 3–6 GB | |
| search indexes | 5–20 GB | lexical plus vectors at 1024 dimensions, int8-quantised |

These sizes do not need special technology. The storage layer is simple on purpose, because the interesting part of the project is in a different place.
