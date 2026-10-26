# 07. The formal engine: Lean 4, Mathlib, proof checking

The formal engine is the middle box of the top row of the diagram, with the label "Lean 4, Mathlib, Proof Checker". It is the only component that can produce E5. Thus its specification is stricter than that of any other component. The content side is in `tamnd/math2/lean/`, the `Math2` Lake package. The toolchain side is in `tamnd/math2-agent/lean` and in the `lean/M2Extract` package.

The job of the engine is one sentence: **given the content at a commit, produce for every formal binding a checked declaration, its elaborated-type hash, its axiom profile, its formal dependencies, and the results of independent replay, all pinned to one Lean toolchain and one Mathlib version.**

## 7.1 What is pinned

Each release pins three things. Its manifest records them (06.2):

| Item | Pinned as | Source of truth |
|---|---|---|
| Lean toolchain | a stable release, e.g. `leanprover/lean4:v4.35.0` | `lean/lean-toolchain` |
| Mathlib | the matching Mathlib tag `v4.35.0`, plus the commit it names | `lean/lake-manifest.json` |
| Checkers | `nanoda`, `lean4lean`, `con-leche`, `con-ron` and comparator versions, as bundled with the toolchain or pinned separately | `manifests/releases/<r>.yaml` → `checkers:` |

A release never pins a Lean release candidate or Mathlib `master`. Mathlib has no semantic versions. It tags each toolchain bump `v4.X.Y`, and its `stable` branch follows those tags (document 01). math2 pins those tags. This gives monthly releases a natural rhythm: each release takes the newest tag that the bump branch (7.9) has built green.

When a node binds to other formal libraries, we pin them in the same way. These libraries are Tau Ceti, the textbook libraries of Formal Frontier, the formal-conjectures repository, the Imperial FLT project, Carleson, PFR, and the AFP for Isabelle bindings. Each one is an entry in `manifests/sources.yaml` with a commit and a toolchain. Some libraries have a toolchain that is different from the toolchain of the release. We check a binding to such a library in the environment of that library, and the manifest marks it.

## 7.2 The `Math2` package

`lean/` is an ordinary Lake package. Its contents follow the 2026 community consensus on how to grow a formal library (the "How to formalize all of mathematics" statement by the Mathlib maintainers; document 01): **defer to Mathlib, avoid duplication, keep quality high, and link to the literature.**

- **Mathlib first.** If the result of a node is in Mathlib, the node binds to the Mathlib declaration and has no Lean code in math2. `Math2/` holds only what Mathlib does not have. That includes statements of research-level results (so we can bind and review them before anyone proves them), proofs that are not upstream yet, conjecture statements, hypotheses for conditional results, and certificate checkers.
- **Module system only.** Each file in `Math2/` is a Lean `module`, with explicit `public import`s. Mathlib has moved to the module system. It will stop support for downstream projects that have not moved. If we start there, it costs nothing.
- **Mathlib style.** Names, formats and linters follow the Mathlib conventions, and `lake lint --code-quality` runs in CI. This is a condition for upstreaming, and the plan is to upstream all general material (16.5).
- **Hypotheses are the only axioms.** `Math2/Hypotheses/` holds named axioms such as `GRH` or `ABC`. Each one is bound to its conjecture node. Then the axiom profile of a conditional theorem names exactly the hypotheses that it depends on. Nothing else in `Math2/` may contain `axiom`, `sorry`, `admit`, `@[implemented_by]` or `unsafe` (rule `F06`).
- **Conjectures are definitions, not axioms.** `Math2/Conjectures/Collatz.lean` defines `def Collatz : Prop := …`. A proof of the conjecture is a term of type `Collatz`. A refutation is a term of type `¬ Collatz`. If the formal-conjectures repository has conventions for a statement of an open problem, follow them, and bind to its entries with `same_as`.

### The binding attribute

```lean
module
public import Mathlib.GroupTheory.Coset.Card
public import Math2.Meta.Attr

namespace Math2

/-- m2://9TQ4WA: every finite subgroup of the multiplicative group of a field is cyclic. -/
@[m2 "9TQ4WA"]
theorem finite_subgroup_units_cyclic {K : Type*} [Field K] (G : Subgroup Kˣ) [Finite G] :
    IsCyclic G :=
  inferInstance            -- Mathlib's instance; the binding is to a math2 restatement

end Math2
```

`@[m2 "TAG"]` records the binding on the Lean side. Rule `F01` checks that it agrees with the front matter of the node. A node can also bind to a Mathlib declaration that has no attribute, because math2 cannot add attributes to Mathlib. Only the front matter and `registers/bindings.reg` list those bindings.

## 7.3 Binding kinds and adapters

A binding states how the formal declaration relates to the informal statement of the node (04.5). The `match` field takes one of five values:

| `match` | Meaning | Can reach E5 for the node? |
|---|---|---|
| `exact` | the declaration states exactly the node's statement | yes |
| `stronger` | the declaration implies the statement; an adapter in `Math2/Meta/Match.lean` derives the exact form | yes, through the adapter, which is itself a checked declaration |
| `weaker` | the declaration is a special case | no; contributes evidence for the special case only |
| `variant` | same mathematics, different formal packaging (e.g. `Finset` against `Set`, bundled against unbundled) | yes, if the adapter proves the equivalence |
| `partial` | a statement-only binding for an unproved result, or one that is still `sorry`-free only in its statement | never (rule `F07`) |

Mathlib often states a classical theorem in a more general form than a textbook does, or packages it differently. Adapters are how math2 handles this. The adapter is small and checked, and we review it together with the binding. The F2 review covers the composite: the conclusion of the adapter against the informal statement.

## 7.4 Extraction

After a build, `M2Extract` walks the environment. It writes one record for each bound declaration, and one record for each declaration in the closure of each bound declaration:

```yaml
decl: Math2.finite_subgroup_units_cyclic
module: Math2.Algebra.Field.Units
tag: 9TQ4WA
type_hash: blake3:5f1c…            # hash of the elaborated type, after instantiating universe params canonically
value_hash: blake3:a903…
statement_pp: "∀ {K : Type u_1} [inst : Field K] (G : Subgroup Kˣ) [inst_1 : Finite ↥G], IsCyclic ↥G"
axioms: [propext, Classical.choice, Quot.sound]
native_decide_axioms: []
uses_decls: [IsCyclic, Subgroup, Units, Field, Finite, instIsCyclicSubtypeMemSubgroupUnitsOfFinite, …]
uses_constants_in_statement: [IsCyclic, Subgroup, Units, Field, Finite]
deprecated: null                   # or {new_name, since, text} from the @[deprecated] attribute
checks: {}                         # or {vacuous, trivial, refuted}, each the tactic that closed the goal
heartbeats: 1840
```

- **Type hash.** This is the key hash in math2 (06.3). We compute it over the elaborated type in a canonical serialization: universe parameters get names by position, binder names are erased, and instance arguments stay. If two declarations have the same type hash, they state the same thing. Because of this, a review stays valid after a rename (Flow C, 03.4), and align can propose `same_as` without judgement.
- **Dependencies.** `uses_decls` comes from the value of the declaration. It is each constant that the proof term mentions, closed transitively at the boundary of other bound declarations. Formal dependencies between bound declarations become `uses` evidence in the graph. Rule `G04` reports each formal dependency that the `uses` of the node does not show.
- **Statement constants.** `uses_constants_in_statement` drives the review tool. A reviewer must see each non-trivial definition that the statement relies on (16.3).
- **Pretty-printing.** We print the statement normally, and also with full elaboration and with notation off. Reviewers see both forms.
- **Deprecation.** `deprecated` copies the entry of the `@[deprecated]` attribute. Rule `F08` reports each bound declaration that has one, with the new name. The bump agent (07.9) uses the same data to propose the rename.
- **Degenerate statements.** For each theorem, `checks` gives the result of the checks 4, 5 and 6 of 10.4. Automation tries to prove `False` from the hypotheses (`vacuous`), to prove the statement without the theorem itself (`trivial`) and to prove the negation (`refuted`). Each tactic has a limit of 10 million heartbeats, which is 5 percent of the default limit of Lean, and the record keeps the first tactic that closed the goal. Rule `F09` fails on `vacuous` and on `refuted`. A `trivial` result does not fail, because a trivial statement can be the correct one.

Extraction runs in the same pinned environment as the build. Its output is a build artefact, with its hash as its address. The extraction code is part of the trusted base (15.1). Thus it is small, and we test it against golden files in `M2Extract/test/`.

## 7.5 Axioms and `native_decide`

- **The allowed set** for E5 is `{propext, Classical.choice, Quot.sound}`. A node can allow fewer: front matter `axioms: constructive` limits it to `propext` and `Quot.sound`. Then status reports the node as `noac` (04.7).
- **Hypotheses** from `Math2/Hypotheses/` can be in the axiom profile of a conditional result. Status reports them by name. The evidence of the node is then "E5 conditional on GRH", and the workspace shows this in words.
- **`native_decide`.** Since Lean 4.35, each use of `native_decide` adds a new auxiliary axiom, and `Lean.ofReduceBool` is removed. Thus the axiom audit sees `native_decide` without a special case. It is not in the allowed set. A node that depends on it can be at most E4, with the certificate kind `compiled-evaluation`, and rule `F04` lists it. If `decide`, `decide_cbv` or `cbv` can do the same job with kernel-only reduction, we tell the prove role to use them.
- **The checker sees the exported environment**, not the source. Axiom profiles come from the environment as the kernel checked it. A search of the source for `axiom` is not sufficient, because metaprograms can add declarations.

## 7.6 Building

```
m2 build                  # nodes → lean → extract → replay → battery → status → graph → store → search
m2 build -only lean       # lake build of Math2, with the Mathlib cache
```

- **Cache.** Lake's cache tools fetch the Mathlib `.olean` cache, and we check it against the hash in the release manifest. The `.olean` files are a build cache, not evidence. Evidence comes from the replay (7.7), which checks the exported environment again from the start. A compromised cache can make the build lie, but it cannot make the replay lie.
- **Incremental.** PR CI builds only the modules that a change touches, plus their reverse dependencies inside `Math2`. CI on `main` builds everything. The release build runs on a clean runner with no cache from earlier PR runs.
- **Budgets.** We record the heartbeat count of each declaration. If a PR makes the build time of `Math2` more than 5% longer, its description must say why.

## 7.7 Replay and the checker stack

A proof is E5 only after it passes **independent replay** (rule `F05`). The toolchain provides this directly since 2026. This came after the kernel soundness incidents of that summer (document 01):

| Layer | Tool | What it adds |
|---|---|---|
| 1 | `lake build` | elaboration and the official kernel, in the build process |
| 2 | `lake check` | re-checks the exported environment with the official kernel, in a separate sandboxed process |
| 3 | `lake check --paranoid` | adds the independent kernels nanoda (Rust), lean4lean (Lean), con-leche (Lean, proved consistent) and con-ron (Rust, proved to refine con-leche) |
| 4 | comparator | checks that the replayed declaration's statement matches the **pinned statement** (a challenge module built from the binding's recorded type, against Mathlib and `Math2` only), and that its axioms are permitted |

At release time, math2 runs all four layers for each E5 candidate. On each PR, it runs layers 1, 2 and 4. The release manifest records the version of each checker. In 2026, a checker that was one week out of date accepted an exploit that the current checker rejected. The release audit fails if a checker is older than the version bundled with the pinned toolchain.

When this document was written, comparator had known open issues (document 01). Thus math2 also compares type hashes from its own extraction against the pinned binding (7.4). Two independent statement checks are cheap, and each one covers the gaps of the other.

For the largest proofs, a full paranoid replay is expensive. The 13M-line Fermat formalization needed 230 GB of RAM and 15 hours under comparator. math2 does not hold proofs of that size. It binds to the declaration in the external library. It records the replay results of that library with their checker versions, at E5 with the flag `external_replay` (04.7). It runs those replays again at the pinned commit of the library only when a checker changes.

## 7.8 Interfaces for agents and people

The formal engine has three front doors. All three use the pinned toolchain.

- **Lean sessions** for agents (11.3). These are REPL processes with Mathlib and `Math2` loaded from a snapshot. They expose the state at tactic level. Implementations to build on are the community REPL, Pantograph, and lean-beam, which is the agent interface of the Lean FRO and is in beta now (document 01). math2 uses the one that gives the best control at goal level under its sandbox. The protocol in 11.3 belongs to math2, so the implementation can change and the agents do not see a change.
- **An MCP server.** `m2 serve` exposes `lean_check`, `lean_goal` and the session calls as MCP tools. With the public `lean-lsp-mcp`, any MCP agent, Claude Code included, can work on `Math2` from a local checkout.
- **Workbench and editors for people.** A contributor works in VS Code with the Lean extension on a local checkout. Or the contributor works in the zero-install Workbench of the Lean FRO in the browser (beta in 2026). math2 adds nothing to the editor experience other than the binding attribute. This is intentional.

## 7.9 The bump

The `bump` role and `m2 bump` keep math2 current. The process:

1. **Weekly**, the bump branch moves to the newest Mathlib tag. It also tries Mathlib `master` as an early-warning build. We never merge that build.
2. **Breaks are classified.** Each failure is one of four kinds:
   - a rename with a deprecation alias (mechanical: take the new name from the `@[deprecated]` attribute);
   - a rename without one (find the new declaration by type hash);
   - a refactor (the type hash changed);
   - a real break in `Math2` code (needs a fix).
3. **Mechanical renames auto-merge** (12.1). Migrate deprecated names before Mathlib deletes them. Mathlib deletes them in batches, about six months after deprecation.
4. **Refactors** make reviews stale (rule `F02`). Each affected node drops to F0 and goes into the fidelity queue, ranked by `unlock`. It gets a review-assist packet that shows the old and new types side by side.
5. **`Math2` breaks** become prove jobs to repair the proof, at high priority. The release cannot pin the new Mathlib until they are fixed.
6. **Upstream catch-up.** A new Mathlib declaration can have the same type hash as a `Math2` declaration. Then the bump proposes to rebind the node to Mathlib and to deprecate the `Math2` copy. `Math2` gets smaller as Mathlib gets larger. That is the intended outcome.
7. **Toolchain bumps** follow Mathlib, because each Mathlib tag names its toolchain. A toolchain bump also runs the full paranoid replay again, because the checkers change with it.

The bump report goes into `reports/bump/<date>.md`. It counts breaks by kind, time to green, and the nodes whose fidelity reviews became stale. Over months, it measures the cost to stay current. Each downstream Lean project pays this cost, and few measure it.

## 7.10 Other proof assistants

math2 is Lean-first, not Lean-only. Bindings have a `system:` field. Two other systems have support from the start:

- **Isabelle/AFP.** The AFP is the second-largest library, with more than a thousand entries. Some results exist only there. For example, the 2026 Jacobian counterexample was formalised in the AFP first. To check an AFP binding, build the entry at its pinned AFP and Isabelle release. It counts as E5 with `system: isabelle`. The trusted base for such nodes is the Isabelle kernel, and the workspace says so.
- **Rocq/MathComp, Metamath, Mizar, HOL Light** are crosswalk targets (`same_as`, 05.3). In v1 they are not bindings for E5. Metamath's set.mm is the exception. It is a candidate for a v2 binding, because its verifier is very small and there are several independent implementations of it.

Translation between systems (lean2dk, lean2agda and similar tools) was still experimental in 2026, and it is not on the critical path. We record agreement between systems as `same_as` between bindings. It counts towards F3, as an independent formalization of the same statement, only when an equivalence is proved or both formalizations have independent F2 reviews (04.7).
