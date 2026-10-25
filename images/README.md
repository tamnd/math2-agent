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
| `m2-sage` | SageMath |

An image has the programs and their libraries, and no shell unless a program needs one. The sandbox gives `/work` (read-only), `/scratch` and `/tmp`, and runs the program as the user 65534 with no network.

These images of spec 08.1 are not here yet:

- `m2-flint`: FLINT needs a C compiler in the image, or a driver program. Until then, use the FLINT of `m2-sage`.
- `m2-julia`: Oscar.jl comes from the Julia registry, not from nixpkgs, so the build is not pinned by Nix yet.
- `m2-lean`: the Lean toolchain and the Mathlib cache come from `lean-toolchain` and `lake`. The `lean` job of CI pins them now.
- `cake_lpr` for `m2-sat`: it is not in nixpkgs. Until it is, an `lrat` certificate cannot reach E4.
