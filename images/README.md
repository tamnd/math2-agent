# The images of the computational engine

Each computation of math2 runs in one of these images (spec 08.1). `flake.nix` builds each image with `dockerTools.buildLayeredImage` from a pinned commit of nixpkgs. Two builds of the same `flake.lock` give the same bytes, thus the same digest. The digest of an image is the sha256 of its config. It is also the image id that `docker load` gives.

To build an image and load it:

```sh
nix build ./images#m2-pari
docker load < result
```

The `images` workflow builds each image on x86-64 and on arm64. It builds each image a second time with `nix build --rebuild`, and fails when the two builds differ. Then it compares the digests with `manifests/images.yaml` of tamnd/math2. When nixpkgs moves, the digests change: update the commit in `flake.nix`, run the workflow, and copy the digests from its summary to that manifest. A computation that names an old digest stays valid, and rule C03 asks for a new run with the new image.

| Image | Contents |
|---|---|
| `m2-pari` | PARI/GP |
| `m2-gap` | GAP with its standard packages |
| `m2-python` | CPython with SymPy, NumPy, SciPy, mpmath and NetworkX |
| `m2-smt` | Z3, cvc5 and Bitwuzla |
| `m2-sat` | CaDiCaL, Kissat and drat-trim |
| `m2-lp` | SCIP and HiGHS |
| `m2-m2` | Macaulay2 |
| `m2-sage` | SageMath, on x86-64 only |

`m2-sage` has no arm64 image for now. On arm64, the Sage doctests of nixpkgs fail now and then: in one run, `GF((2^29-3)^2)` raised "the order of a finite field must be a prime power", and a GCD after it failed with "GCD over rings not implemented". The same `flake.lock` passed in an earlier run. A program that gives a wrong answer only some of the time is not safe for a computation, thus the workflow does not build it, and `manifests/images.yaml` has no arm64 digest of it. Rule C02 needs a run on each architecture, thus a computation in `m2-sage` cannot reach E4 until the arm64 image is back. Run it in another image when you can.

An image has the programs and their libraries, and no shell unless a program needs one. The sandbox gives `/work` (read-only), `/scratch` and `/tmp`, and runs the program as the user 65534 with no network.

These images of spec 08.1 are not here yet:

- `m2-flint`: FLINT needs a C compiler in the image, or a driver program. Until then, use the FLINT of `m2-sage`.
- `m2-julia`: Oscar.jl comes from the Julia registry, not from nixpkgs, so the build is not pinned by Nix yet.
- `m2-lean`: the Lean toolchain and the Mathlib cache come from `lean-toolchain` and `lake`. The `lean` job of CI pins them now. The Lean sandbox of the agent runs (`m2 agent run -sandbox`) needs this image. It must have `lake`, the toolchain of the tree on the `PATH` and M2Extract at `/usr/local/bin/m2extract`. Each run mounts `lean/` of the work tree at `/out` and the Lake packages of the tree at `/work`, and runs `lake -d /out ...` as the user 65534.
- `cake_lpr` for `m2-sat`: it is not in nixpkgs. Until it is, an `lrat` certificate cannot reach E4.
