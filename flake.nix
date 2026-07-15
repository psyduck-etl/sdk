{
  description = "psyduck plugin sdk: dev shell with protobuf codegen tooling";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        devShells.default = pkgs.mkShell {
          packages = [
            pkgs.go
            pkgs.protobuf # protoc
            pkgs.protoc-gen-go
            pkgs.protoc-gen-go-grpc
          ];

          # Route git hooks through the checked-in .githooks/ so everyone in
          # the dev shell gets the pre-commit protobuf-codegen guard. Local,
          # idempotent config; contributors outside `nix develop` can opt in
          # with the same command.
          shellHook = ''
            git config core.hooksPath .githooks
          '';
        };
      }
    );
}
