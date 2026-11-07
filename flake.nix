{
  description = "Patchwork - Windows CLI package update manager";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = import nixpkgs { inherit system; };
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "patchwork";
          version = "0.0.28";
          src = ./.;

          vendorHash = "sha256-A6TuugLxq+rzH4zRV7D4/RD7YO4ftLvgcDkGvVJaRao=";

          # Ensure go.mod version directive is compatible with Nixpkgs' Go version during hermetic build
          # both in the goModules fixed-output derivation and the final compilation derivation.
          overrideModAttrs = (_: {
            preBuild = ''
              sed -i 's/^go 1\.27\.[0-9]*/go 1.26/' go.mod
              go mod tidy
            '';
          });

          prePatch = ''
            sed -i 's/^go 1\.27\.[0-9]*/go 1.26/' go.mod
          '';

          # Cross-compilation settings for Windows amd64 target via env
          env = {
            CGO_ENABLED = "0";
            GOOS = "windows";
            GOARCH = "amd64";
          };

          ldflags = [
            "-s"
            "-w"
            "-X github.com/karan-banwasi/patchwork/cmd.version=0.0.28"
          ];

          postInstall = ''
            if [ -f "$out/bin/windows_amd64/patchwork.exe" ]; then
              mv "$out/bin/windows_amd64/patchwork.exe" "$out/bin/patchwork.exe"
              rm -rf "$out/bin/windows_amd64"
            elif [ -f "$out/bin/patchwork" ]; then
              mv "$out/bin/patchwork" "$out/bin/patchwork.exe"
            fi
          '';

          meta = with pkgs.lib; {
            description = "A Windows CLI utility to manage package updates";
            homepage = "https://github.com/karan-banwasi/patchwork";
            license = licenses.mit;
            mainProgram = "patchwork.exe";
          };
        };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            golangci-lint
            gopls
            goreleaser
          ];

          shellHook = ''
            echo "=== Patchwork Dev Shell ==="
            echo "Go:  $(go version)"
            echo "Nix: $(nix --version 2>/dev/null || echo 'Nix available')"
          '';
        };
      }
    );
}
