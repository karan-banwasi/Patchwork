# Patchwork: Agent Context & Guidelines

## Project Overview
Patchwork is a cross-platform CLI utility designed to iterate through packages that have available updates and manage them seamlessly across Windows, macOS, and Linux.

## Technology Stack
- **Language**: Go 1.27.3 (Primary)
- **Target OS**: Windows, macOS, Linux (Cross-Platform)
- **CLI Framework**: Cobra (`github.com/spf13/cobra`)
- **Interactive UI**: Built-in index- and range-based selector (`cmd/selector.go`)
- **Execution & Networking**: Decoupled `pm.Runner` command execution abstraction; resilient `internal/httputil` client with exponential backoff and offline gating

## Core Features
1. **Multi-Manager Support**:
   - **macOS / Linux**: Homebrew (`brew`) for formulae and casks.
   - **Windows**: Winget (`winget`), Scoop (`scoop`), Chocolatey (`choco`), and PowerShell PSResource (`psresource`).
   - **Cross-Platform**: npm Global (`npm`), .NET Global Tools (`dotnet`), uv Python Tools (`uv`), Rust Cargo Crates (`cargo`).
2. **Platform Awareness**: The CLI automatically filters and detects package managers appropriate for the host OS via build tags and runtime availability probes.
3. **Interactive Selection & Batching**: Provides a range/index selection prompt (e.g. `1, 3-5`, `all`). Supports detailed bulk upgrades for managers that support it (e.g. Chocolatey).
4. **Command Pipeline**: Structured `plan` / `execute` pipeline (`internal/upgrade`) featuring dry-run simulation, deferral of self-updating shells/terminals, and pin enforcement.
5. **Ecosystem-Aware Version Comparison**: Semver and epoch-aware comparison (`internal/version`) preventing false upgrades and downgrades.
6. **Package Pinning**: Skip updates for selected packages globally or per-manager (`pw pin`, `pw unpin`, `pw pinned`), backed by atomic config writes.
7. **Documented Exit Codes & Machine Outputs**: Stable exit codes (0, 1, 2, 3) and JSON schemas (`schema_version: "1.0.0"`).

## Architecture & Code Layout
- `main.go`: Entry point for the CLI tool. Points execution to the Cobra command tree.
- `cmd/`: Houses Cobra CLI commands (`root.go`, `check.go`, `upgrade.go`, `doctor.go`, `cleanup.go`, `pin.go`, `selfupdate.go`, etc.).
  - `registry_windows.go` / `registry_unix.go`: Platform-gated registration of default package managers.
- `internal/`: Core business logic decoupled by subsystem and package manager:
  - `internal/pm/`: Core `pm.Manager` interface, extension interfaces, `Registry`, and `Runner` abstraction.
  - `internal/upgrade/`: Command pipeline (`Plan`, `Executor`, `Summary`, JSON serialization).
  - `internal/version/`: Ecosystem-aware version comparison (`Compare`, `IsUpgrade`).
  - `internal/config/`: Configuration and completion cache with atomic file I/O.
  - `internal/exitcodes/`: Standardized process exit codes (`ExitSuccess`, `ExitGeneralError`, `ExitUpgradeFailure`, `ExitCanceled`).
  - `internal/httputil/`: Resilient HTTP client with retry backoff and offline support.
  - `internal/<manager>/`: Concrete drivers (`brew`, `winget`, `scoop`, `choco`, `psresource`, `npm`, `dotnet`, `uv`, `cargo`).
- `docs/`: Reference documentation (`env-vars.md`, `exit-codes.md`, `json-schema.md`) and CLI reference (`docs/cli/`).

## Development & Testing Guidelines
- **Cross-Platform Support**: Use Go's standard library cross-platform features (`os.UserConfigDir()`, `runtime.GOOS`). Isolate platform-specific features (e.g., Windows UAC elevation, `conhost.exe` launching) behind build tags (`//go:build windows` and `//go:build !windows`).
- **Exec calls**: Always use `pm.Runner` / `os/exec`. Keep parsing logic robust, isolated in pure functions, and prefer machine-readable formats (`--json`, `--limit-output`) where available.
- **Golden Fixture Tests**: All package manager parsers MUST have golden fixture tests in `internal/<manager>/golden_test.go` backed by `testdata/*.input` and `testdata/*.golden.json`. Use `-update` flag when updating parser baselines.
- **Exit Codes**: Commands must return standard exit codes via `exitcodes.Determine(ctx, err)`:
  - 0: Success
  - 1: General / Operational / Configuration error
  - 2: Upgrade failure (one or more packages failed)
  - 3: Canceled / Interrupted
- **Error Handling**: Gracefully handle errors when a package manager is not installed on the system; surface partial lookup failures as warnings rather than failing the entire run.
- **UI Consistency**: Maintain tabular alignment and spacing for terminal output to match native system feel.
- **CI/CD Validation Invariant**: Whenever changes, commits, or tags are pushed to the remote repository (`origin`), the agent must actively inspect the triggered GitHub Actions workflow runs (`https://api.github.com/repos/karan-banwasi/Patchwork/actions/runs`) and verify that all jobs (e.g., multi-OS tests across Windows, macOS, and Linux, as well as release builds) finish with status `completed` and conclusion `success` before marking any user task as completed. If CI fails, the agent must inspect job logs, resolve the root cause, and re-validate CI until green.
