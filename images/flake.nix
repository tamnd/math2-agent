# The images of the computational engine (spec 08.1). Each image is a Nix derivation, thus two builds of the same flake.lock give the same bytes and the same digest.
#
#   nix build ./images#m2-pari
#   docker load < result
#
# The digest of an image is the sha256 of its config, which is also the image id that docker load gives. manifests/images.yaml of tamnd/math2 records the digest of each image.
{
  description = "The pinned images of the math2 computational engine";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/1f607aa79b6356ed45d3cd3c8079c196e79efa2c"; # nixos-26.05 on 2026-10-25

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAll (pkgs:
        let
          # image makes an image with the programs in contents. The image has no shell unless contents has one. The sandbox gives /scratch, /tmp and /work, and runs the program as the user 65534.
          image = name: contents: pkgs.dockerTools.buildLayeredImage {
            inherit name;
            tag = "nix";
            contents = contents;
            created = "1970-01-01T00:00:01Z";
            maxLayers = 32;
            config = {
              Env = [ "HOME=/scratch" "LANG=C.UTF-8" "PATH=/bin" ];
              WorkingDir = "/scratch";
              User = "65534:65534";
              Labels = {
                "org.opencontainers.image.source" = "https://github.com/tamnd/math2-agent";
                "org.opencontainers.image.title" = name;
              };
            };
          };
          python = pkgs.python3.withPackages (ps: with ps; [ sympy numpy scipy mpmath networkx ]);
        in
        {
          m2-pari = image "m2-pari" [ pkgs.pari ];
          m2-gap = image "m2-gap" [ pkgs.gap ];
          m2-python = image "m2-python" [ python ];
          m2-smt = image "m2-smt" [ pkgs.z3 pkgs.cvc5 pkgs.bitwuzla ];
          m2-sat = image "m2-sat" [ pkgs.cadical pkgs.kissat pkgs.drat-trim ];
          m2-lp = image "m2-lp" [ pkgs.scip pkgs.highs ];
          m2-m2 = image "m2-m2" [ pkgs.macaulay2 ];
          m2-sage = image "m2-sage" [ pkgs.sage ];
        });
    };
}
