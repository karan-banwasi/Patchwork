# Project: Patchwork — GitHub Issues Resolution & Release Readiness

## Architecture
- **Language**: Go 1.27.3
- **Entry Points**: `main.go` / `cmd/` (Cobra CLI tree)
- **Core Domain**: `internal/pm` defining `Manager` interface, optional extension interfaces (`Locator`, `Cleaner`, `Deferrer`, `Refresher`, `BulkUpgrader`, `IOConfigurable`, `TimeoutProvider`), and `Runner` abstraction.
- **Package Managers**: `internal/brew`, `internal/winget`, `internal/scoop`, `internal/choco`, `internal/npm`, `internal/dotnet`, `internal/uv`, `internal/cargo`, `internal/psresource`.
- **Configuration & Utilities**: `internal/config`, `cmd/selfupdate.go`.
- **CI / Distribution**: `.github/workflows/release.yml`, `install.ps1`.

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Issue #23 [SEC-1] (P0) | Self-update downloads Windows binary on macOS/Linux; publish cross-platform SHA256SUMS | M1 | GitHub Issue #23 |
| 2 | Issue #24 [SEC-2] (P1) | Cryptographically verify attestation bundle; pin signer workflow; handle `--require-attestation` | M2 | GitHub Issue #24 |
| 3 | Issue #25 [SEC-3] (P1) | Harden release workflow: scope WINGET_TOKEN, draft->attest->publish, fix winget PR version, restrict dispatch to main | M2 | GitHub Issue #25 |
| 4 | Issue #26 [SEC-4] (P1) | Pins fail open on corrupt config; make config writes atomic | M2 | GitHub Issue #26 |
| 5 | Issue #27 [SEC-5] (P1) | Use ecosystem-aware version comparison to avoid suggesting downgrades | M3 | GitHub Issue #27 |
| 6 | Issue #28 [SEC-6] (P2) | install.ps1: fail on attestation verification failure | M2 | GitHub Issue #28 |
| 7 | Issue #29 [SEC-7] (P2) | Reconsider `--accept-agreements` defaulting to true | M2 | GitHub Issue #29 |
| 8 | Issue #30 [SEC-8] (P2) | CI supply-chain hygiene: pin govulncheck, restore setup-go bump, add CodeQL/Scorecard/golangci-lint/-race/SBOM | M2 | GitHub Issue #30 |
| 9 | Issue #31 [ARCH-1] (P1) | Refactor `upgrade` into a plan/execute pipeline; unify single-manager fetching in Registry | M3 | GitHub Issue #31 |
| 10 | Issue #32 [ARCH-2] (P1) | Per-manager `TimeoutProvider` is never honored | M3 | GitHub Issue #32 |
| 11 | Issue #33 [ARCH-3] (P2) | Remove concrete-type switches, Windows IDs in `pm`, `Registry.Warnings`, and dead code | M3 | GitHub Issue #33 |
| 12 | Issue #34 [ARCH-4] (P1) | Surface refresh and registry lookup failures as warnings instead of swallowing them | M3 | GitHub Issue #34 |
| 13 | Issue #35 [ARCH-5] (P2) | Report per-package results for bulk (choco) upgrades | M3 | GitHub Issue #35 |
| 14 | Issue #36 [ARCH-6] (P2) | Replace archived `survey` library; key selections by index | M3 | GitHub Issue #36 |
| 15 | Issue #37 [ARCH-7] (P2) | Shared HTTP client with retries + true `--offline` mode | M3 | GitHub Issue #37 |
| 16 | Issue #38 [ARCH-8] (P2) | Golden-file parser fixtures; evaluate structured winget source | M3 | GitHub Issue #38 |
| 17 | Issue #39 [ARCH-9] (P2) | Documented exit codes, `upgrade --json`, JSON `schema_version` | M3 | GitHub Issue #39 |
| 18 | Issue #40 [DOC-1] (P1) | Make README cross-platform (install, config paths, self-update) | M4 | GitHub Issue #40 |
| 19 | Issue #41 [DOC-2] (P1) | Add SECURITY.md, CONTRIBUTING.md, CoC, issue/PR templates, CODEOWNERS | M4 | GitHub Issue #41 |
| 20 | Issue #42 [DOC-3] (P2) | Add ARCHITECTURE.md and "Adding a package manager" guide; refresh AGENTS.md | M4 | GitHub Issue #42 |
| 21 | Issue #43 [DOC-4] (P2) | Reference docs for env vars, JSON schema, exit codes; generate CLI docs | M4 | GitHub Issue #43 |
| 22 | Issue #44 [DOC-5] (P3) | Remove stale committed winget 0.0.22 manifests | M4 | GitHub Issue #44 |
| 23 | Issue #45 [FEAT-1] (P1) | Distribution: `cmd/pw`, GoReleaser, Homebrew tap, Scoop bucket, install.sh, code signing | M5 | GitHub Issue #45 |
| 24 | Issue #46 [FEAT-2] (P1) | Richer config: disabled managers, default flags, version-constrained pins with reason/expiry | M5 | GitHub Issue #46 |
| 25 | Issue #47 [FEAT-3] (P2) | Add Linux-native and additional package managers | M5 | GitHub Issue #47 |
| 26 | Issue #48 [FEAT-4] (P2) | Upgrade policies: --exclude, --level, --yes, cross-manager parallelism, changelog preview | M5 | GitHub Issue #48 |
| 27 | Issue #49 [FEAT-5] (P2) | Scheduled background checks with desktop notifications | M5 | GitHub Issue #49 |
| 28 | Issue #50 [FEAT-6] (P3) | Upgrade history and rollback | M5 | GitHub Issue #50 |
| 29 | Issue #51 [FEAT-7] (P3) | Export/import machine package profile | M5 | GitHub Issue #51 |
| 30 | Issue #52 [FEAT-8] (P3) | Passive self-update notice and `self-update --check` | M5 | GitHub Issue #52 |
| 31 | Verification & CI Invariant | Multi-OS local tests passing, remote GitHub Actions CI completed with success, clean working tree | M6 | ORIGINAL_REQUEST R3/R4 |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | P0 Critical: Cross-Platform Self-Update & SHA256SUMS | Issue #23: Cross-platform asset mapping for `(GOOS, GOARCH)`, universal `SHA256SUMS` generation in release workflow and consumption in `selfupdate.go` and `install.ps1`, update file cleanup, test matrix | none | DONE (resolved in cmd/selfupdate.go, release.yml, install.ps1; 100% test pass) |
| M2 | Security & Release Pipeline Hardening | Issues #24, #25, #26, #28, #29, #30: Attestation verification, workflow secrets scoping, atomic config saves, install.ps1 error handling, CI hygiene | M1 | DONE |
| M3 | Core Architecture & Engine Refactoring | Issues #27, #31, #32, #33, #34, #35, #36, #37, #38, #39: Upgrade pipeline refactor, timeout honoring, downgrade prevention, structured error reporting, survey migration, exit codes | M1 | PLANNED |
| M4 | Governance & Documentation Alignment | Issues #40, #41, #42, #43, #44: Cross-platform README, SECURITY.md, CONTRIBUTING.md, templates, ARCHITECTURE.md, purge stale manifests | none | PLANNED |
| M5 | Distribution & Extended Features | Issues #45, #46, #47, #48, #49, #50, #51, #52: Rich config, advanced upgrade policies, notifications, history, additional managers | M2, M3 | PLANNED |
| M6 | Final Verification, CI Validation & Release Prep | Verification across all platforms, remote GitHub Actions CI green, clean working tree, ready for release | M1-M5 | PLANNED |

## Interface Contracts
### Self-Update Asset & Checksum Resolver (`cmd/selfupdate.go`)
- `resolveBinaryAsset(goos, goarch string) (assetName string, err error)`
  - Maps `(windows, amd64) -> pw.exe` (or `pw-windows-amd64.exe`), `(windows, arm64) -> pw-arm64.exe` (or `pw-windows-arm64.exe`), `(darwin, arm64) -> pw-darwin-arm64`, `(darwin, amd64) -> pw-darwin-amd64`, `(linux, amd64) -> pw-linux-amd64`, `(linux, arm64) -> pw-linux-arm64`.
- `fetchChecksumManifest(ctx context.Context, client *http.Client, release GitHubRelease) (map[string]string, error)`
  - Fetches and parses `SHA256SUMS` from the release assets (format: `<sha256>  <filename>`). Falls back to winget manifest only if on Windows and `SHA256SUMS` is unavailable.

## Code Layout
- `cmd/`: CLI commands (`root.go`, `selfupdate.go`, `upgrade.go`, `check.go`, `pin.go`, `doctor.go`, `cleanup.go`).
- `internal/pm/`: Package manager core interface and runner abstractions.
- `internal/<manager>/`: Package manager concrete drivers.
- `internal/config/`: Configuration handling (`config.go`).
- `.github/workflows/`: CI and Release workflows (`release.yml`).
