# 08. The computational engine: CAS, SMT, numerical experiments

The computational engine is the box that makes math2 executable in the second sense. In the first sense, formal proofs run in the Lean kernel (document 07). In the second sense, computations are part of the knowledge. These are the examples, counterexamples, tables, searches and numerical experiments that mathematicians use to find conjectures and to check them. The Research Agents call the engine directly. Its outputs go into the knowledge graph (as `computation`, `example` and `counterexample` nodes, and as evidence) and into versioned storage (as artefacts with certificates).

The engine has one design principle: **a computation is a claim, and the strength of the claim depends on what can be checked about it afterwards.**

## 8.1 What runs

The engine runs pinned container images, one for each system family. The image digest is part of the identity of each computation (document 06.3).

| Image | Contents | Used for |
|---|---|---|
| `m2-sage` | SageMath (current stable), with its bundled PARI/GP, GAP, FLINT, Singular, Maxima, NTL, ECL | general CAS, algebra, number theory, combinatorics |
| `m2-pari` | PARI/GP standalone, current stable | fast number theory, L-functions, elliptic curves |
| `m2-gap` | GAP with the standard packages (SmallGrp, AtlasRep, …) | group theory, representation theory |
| `m2-m2` | Macaulay2 | commutative algebra, algebraic geometry |
| `m2-flint` | FLINT 3, including Arb (merged into FLINT) | ball arithmetic and rigorous numerics |
| `m2-julia` | Julia with Oscar.jl, IntervalArithmetic.jl | computer algebra (Oscar), validated numerics |
| `m2-python` | CPython, SymPy, NumPy/SciPy, mpmath, NetworkX, python-flint, OR-tools | scripting, quick experiments, graph search |
| `m2-smt` | Z3, cvc5, Bitwuzla | SMT queries with models and proofs |
| `m2-sat` | CaDiCaL, Kissat, with DRAT/LRAT proof output; `cake_lpr` and `dpr-trim` as checkers | SAT encodings of combinatorial problems (Boolean Pythagorean triples, Keller, Schur number 5, empty hexagon) |
| `m2-lp` | SCIP, HiGHS, with exact rational verification (VIPR certificates where available) | integer and linear programming bounds |
| `m2-lean` | the pinned Lean toolchain and the Mathlib cache | `#eval`, `decide`, and checking certificates inside Lean (document 07.8) |

Commercial systems (Magma, Mathematica, Maple) are permitted as **E1 evidence only**. An agent or a person can run them, and we can record the output. But the output cannot reach E4, because a reader cannot reproduce the run. They are adapters in `math2-agent`, and they are off by default. The same rule applies to Wolfram|Alpha.

We build each image reproducibly from a Nix flake in `tamnd/math2-agent/images/`. We publish it by digest and refresh it monthly with the release. If an image changes under a recorded computation, the old record stays valid, because the record names the old digest. But the change causes a new run in the next release, to check that the result still holds (rule `C03`).

## 8.2 The computation record

A computation is a node of kind `computation`. Its body is a short description. Its front matter is the record:

```yaml
tag: 3MZ8QD
kind: computation
name: No counterexample to Goldbach below 10^7 (demonstration)
about: [8GQ2N1]                  # the Goldbach conjecture node
claim: >
  Every even n with 4 <= n <= 10^7 is the sum of two primes.
engine:
  image: m2-pari@sha256:...       # by digest
  entry: compute/3MZ8QD/main.gp
  inputs: {N: 10000000}
  seed: null                      # deterministic. Randomised computations must give a seed
  resources: {cpu: 4, mem: 4G, wall: 10m}
output:
  hash: blake3:...                # canonical serialisation of the result
  summary: "checked 4999999 even numbers; 0 failures"
certificate:
  kind: witness-table             # see 8.3
  artefact: blake3:...
  checker: m2-lean@sha256:...    # independent checker image
  checker_entry: compute/3MZ8QD/Check.lean
```

The code is next to the node, in `compute/<tag>/`, under Apache-2.0 like the Lean code. Read a computation in the same way as a proof: what does this show, and how can I check it?

## 8.3 Certificates: how computations reach E4

A computation reaches E4 (`certified`, document 04.7) only if it produces a **certificate** that a **different, smaller, more trusted** checker accepts. The engine knows a fixed list of certificate kinds:

| Certificate | Produced by | Checked by | Typical claim |
|---|---|---|---|
| `witness` | anything | Lean `decide`/`norm_num`, or a 50-line checker | "there exists an X with property P" (show the X) |
| `witness-table` | anything | a Lean or small Python checker that runs over the table | finite verifications where each case has a short witness (Goldbach pairs, factorisations) |
| `lrat` | CaDiCaL/Kissat with proof output | `cake_lpr` (a verified checker, CakeML) | "no X exists" via SAT |
| `smt-proof` | cvc5 (Alethe/CPC proofs), Z3 (proof objects) | Lean's SMT reconstruction, or Carcara for Alethe | decidable arithmetic and bit-vector facts |
| `primality` | PARI/GP or Primo | Lean (Pratt or ECPP certificate checker) | "N is prime" |
| `interval` | FLINT/Arb, IntervalArithmetic.jl | re-evaluation in a second ball arithmetic, or in Lean with exact rationals | rigorous numerical inequalities, zeros of functions |
| `lp-dual` | SCIP/HiGHS | exact rational arithmetic check of the dual certificate (VIPR) | bounds from linear and integer programming |
| `sos` | SDP solver plus rational rounding | exact polynomial identity check | positivity via sums of squares |
| `group-id` | GAP | a recomputation in a second system (Magma excluded), or a Lean check for small orders | "this group is SmallGroup(n,k)" |

A computation without a certificate is E1. That is still useful. Most experiments in math are E1, and the system records them accurately. But the system never shows it as stronger than E1.

### Into the kernel

If Lean can check a certificate in reasonable time, the computation can go all the way to E5. We import the certificate as data. We prove a Lean checker sound one time. Then `decide` or kernel reduction runs the checker on the data. The other route is `native_decide`, which is flagged and is permitted only per node (document 07.7). The Lean ecosystem of 2025 to 2026 made this practical for SAT (LRAT checks inside Lean, as `bv_decide` does) and for many arithmetic certificates. If the certificate kind of a computation has a Lean checker in `lean/Math2/Cert/`, the prove agent tries this route.

## 8.4 Numerical experiments

Experiments are how people find conjectures. FunSearch, AlphaEvolve and PatternBoost-style systems (document 01) are, at the base, experiment loops in which a model proposes programs or constructions. math2 supports them as first-class but untrusted:

- **An experiment is a computation with `role: experiment`.** Its claim is an observation ("for all n ≤ 10^5 tested, a(n) is squarefree"), not a theorem.
- **Search runs record their search space.** An agent can search for a construction (a cap set, a packing, a graph with given properties). Then the record holds the scoring function, the best object found and its score. A best object is a `witness`. A lower bound from it can be E4 if the check of the object is cheap. The claim "this is optimal" is E1 unless an exhaustive or dual certificate supports it.
- **Patterns become conjectures through a gate.** If an agent sees a pattern, it files a `conjecture` node with `source: computation <tag>`. The explore agent (document 12.5) must first run a falsification pass: extend the range, randomise, test against OEIS and LMFDB, and try the obvious generalisations. The agent files the conjecture together with the falsification attempts that failed.

## 8.5 SMT and SAT for the prove loop

SMT and SAT are also tools inside the prove loop (document 12.4). They do more than produce standalone computations:

- **Lean already calls out.** We use `bv_decide` (bit-blasting to CaDiCaL with LRAT replay), `omega`, `grind`, `polyrith` (which calls a CAS and then checks the certificate) and similar tactics where they apply. The Lean axioms audit (document 07.7) handles their trust status.
- **Agents use SMT as a counterexample finder.** Before the prove agent spends prover budget on a goal, it encodes small finite instances where it can. Then it asks Z3 or cvc5 for a model. A model is a counterexample to the formal statement. That is very often a *misformalisation* finding (document 10.5), and the engine reports it to the formalize agent as one.
- **Large SAT proofs are artefacts.** We do not store proofs of the Boolean Pythagorean triples size (about 200 TB uncompressed). We store the encoding, the solver version and the cube split. We also store a checked hash chain if the original authors gave one. That is sufficient to say exactly what was checked and by whom. A claim in this class is E4 only if a certificate was checked, possibly by a third party, and the record names who did it.

## 8.6 Sandboxing, budgets, determinism

Each computation runs in a sandbox. The rules are the same for agents and for the CI runs of people:

- **Isolation:** gVisor or Firecracker microVMs on the runners; no network by default; a read-only image; a writable scratch area of bounded size.
- **Budgets:** CPU, memory, wall clock and output size, all declared in the record. The request of an agent is charged against the budget of the run (document 11.5).
- **Determinism:** computations must be deterministic, or must declare a seed. The engine runs each new E4-candidate computation two times, on two different runner types (x86-64 and arm64), and compares output hashes (rule `C02`). A difference blocks E4, and the engine writes it to the report. Floating-point code that is not interval arithmetic never reaches E4, so it never blocks anything. It stays E1.
- **Data access:** a computation can read OEIS, LMFDB or zbMATH only through the cached snapshots of the ingestion layer (document 09). Those snapshots are pinned by hash and mounted read-only. Thus a computation never depends on a live API.

## 8.7 Executable examples

Each `example` and `counterexample` node should be executable where possible. The executable check is a Lean `example` (document 07), or a computation of 10 seconds or less with a `witness` certificate. The "run this" button of the workspace (document 13) runs exactly that record in a sandbox on the browser side or the server side. It shows the output next to the recorded hash. The build reports the share of executable examples per topic, and the goal in document 02 gives a number for it.

## 8.8 What the engine will not do

- It will not run code that is not in a pinned image.
- It will not let a computation call the network, call a model, or read anything outside its declared inputs.
- It will not accept a certificate checker written by the same agent run that produced the certificate, unless that checker has itself been proved sound in Lean or reviewed by a person. The checker for each certificate kind is fixed in `math2-agent`, and a change to it needs the review of a person.
- It will not show a numerical observation as anything stronger than an observation.
