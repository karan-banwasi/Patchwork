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
          version = "0.0.27";
          src = ./.;

          vendorHash = null;

          # Cross-compilation settings for Windows amd64 target
          CGO_ENABLED = 0;
          GOOS = "windows";
          GOARCH = "amd64";

          ldflags = [
            "-s"
            "-w"
            "-X github.com/karan-banwasi/patchwork/cmd.version=0.0.27"
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
