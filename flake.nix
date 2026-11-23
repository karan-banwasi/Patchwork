{
  description = "Patchwork - Cross-platform CLI tool to manage package updates";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      supportedSystems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems =
        f:
        nixpkgs.lib.genAttrs supportedSystems (
          system:
          let
            pkgs = import nixpkgs { inherit system; };
            # nixpkgs nixos-unstable may briefly lag patch releases (e.g. go 1.27.3 vs go.mod 1.27.4 for GO-2026-6859).
            # Override go_1_27 only when needed, falling back cleanly once nixpkgs catches up.
            go =
              if pkgs.lib.versionOlder pkgs.go_1_27.version "1.27.4" then
                pkgs.go_1_27.overrideAttrs (finalAttrs: _: {
                  version = "1.27.4";
                  src = pkgs.fetchurl {
                    url = "https://go.dev/dl/go${finalAttrs.version}.src.tar.gz";
                    hash = "sha256-c8+4VxrsDAEXdofbhxY9Rqog6XeWcncokaoXZU2aYJk=";
                  };
                })
              else
                pkgs.go_1_27;
            buildGoModule = pkgs.buildGoModule.override { inherit go; };
          in
          f {
            inherit
              pkgs
              go
              buildGoModule
              ;
          }
        );
    in
    {
      packages = forAllSystems (
        { pkgs, buildGoModule, ... }:
        let
          version = "0.3.2";
          pw = buildGoModule {
            pname = "pw";
            inherit version;
            src = ./.;

            vendorHash = "sha256-senbhCHNhX66J00p6JzZjY4Hk6N3neGELRGwpDfhCGI=";

            subPackages = [ "cmd/pw" ];

            env.CGO_ENABLED = 0;

            ldflags = [
              "-s"
              "-w"
              "-X github.com/karan-banwasi/patchwork/cmd.version=v${version}"
            ];

            nativeBuildInputs = [ pkgs.installShellFiles ];

            preCheck = ''
              export HOME="$TMPDIR"
            '';

            # Run internal unit & golden fixture tests inside the hermetic sandbox alongside cmd/pw.
            # Script tests under cmd/ that invoke /usr/bin/env bash run in the Nix devShell CI job.
            postCheck = ''
              go test ./internal/...
            '';

            postInstall = pkgs.lib.optionalString (pkgs.stdenv.buildPlatform.canExecute pkgs.stdenv.hostPlatform) ''
              installShellCompletion --cmd pw \
                --bash <($out/bin/pw completion bash) \
                --zsh <($out/bin/pw completion zsh) \
                --fish <($out/bin/pw completion fish)
            '';

            meta = {
              description = "Cross-platform CLI tool to check and manage package updates across multiple package managers";
              homepage = "https://github.com/karan-banwasi/Patchwork";
              changelog = "https://github.com/karan-banwasi/Patchwork/releases/tag/v${version}";
              license = pkgs.lib.licenses.mit;
              mainProgram = "pw";
            };
          };
        in
        {
          inherit pw;
          default = pw;
        }
      );

      devShells = forAllSystems (
        { pkgs, go, ... }:
        {
          default = pkgs.mkShell {
            packages = [
              go
              pkgs.golangci-lint
              pkgs.gopls
              pkgs.goreleaser
              pkgs.govulncheck
              pkgs.jq
            ];

            shellHook = ''
              echo "=== Patchwork Dev Shell ==="
              echo "Go:            $(go version)"
              echo "golangci-lint: $(golangci-lint --version | head -n 1)"
              echo "Nix:           $(nix --version 2>/dev/null || echo 'Nix available')"
            '';
          };
        }
      );
    };
}
