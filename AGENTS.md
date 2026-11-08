# Patchwork: Agent Context & Guidelines

## Project Overview
Patchwork is a cross-platform CLI utility designed to iterate through packages that have available updates and manage them seamlessly across Windows, macOS, and Linux.

## Technology Stack
- **Language**: Go (Primary)
- **Target OS**: Windows, macOS, Linux (Cross-Platform)
- **CLI Framework**: Cobra (`github.com/spf13/cobra`)
- **Interactive UI**: Survey (`github.com/AlecAivazis/survey/v2`)

## Core Features
1. **Multi-Manager Support**:
   - **macOS / Linux**: Homebrew (`brew`) for formulae and casks.
   - **Windows**: Winget (`winget`), Scoop (`scoop`), Chocolatey (`choco`), and PowerShell PSResource (`psresource`).
   - **Cross-Platform**: npm Global (`npm`), .NET Global Tools (`dotnet`), uv Python Tools (`uv`), Rust Cargo Crates (`cargo`).
2. **Platform Awareness**: The CLI automatically filters and detects package managers appropriate for the host OS.
3. **Interactive Selection**: Provides a visual multi-select checklist to allow the user to easily pick which packages to upgrade without memorizing IDs.
4. **Package Pinning**: Skip updates for selected packages globally or per-manager.

## Architecture
- `main.go`: Entry point for the CLI tool. Points execution to the Cobra command tree.
- `cmd/`: Houses the Cobra CLI commands (`root.go`, `check.go`, `upgrade.go`, `doctor.go`, `cleanup.go`, etc.).
- `internal/`: Contains core logic separated by package managers.
  - `internal/brew/`: Handles Homebrew CLI interactions (`brew outdated --json=v2`, `brew upgrade`, `brew cleanup`, `brew update`).
  - `internal/winget/`: Houses logic for interacting with the `winget` CLI to fetch updates and perform upgrades.
  - Future modules will be placed in their respective `internal/<manager>` directories.

## Development Guidelines
- **Cross-Platform Support**: Use Go's standard library cross-platform features (`os.UserConfigDir()`, `runtime.GOOS`). Isolate platform-specific features (e.g., Windows UAC elevation, `conhost.exe` launching) behind build tags (`//go:build windows` and `//go:build !windows`).
- **Exec calls**: For invoking package managers, use `pm.Runner` / `os/exec`. Keep parsing logic robust and prefer machine-readable formats (`--json`, `--limit-output`) where available.
- **Error Handling**: Gracefully handle errors when a package manager is not installed on the system.
- **UI Consistency**: Maintain tabular alignment and spacing for terminal output to match native system feel.
- **CI/CD Validation Invariant**: Whenever changes, commits, or tags are pushed to the remote repository (`origin`), the agent must actively inspect the triggered GitHub Actions workflow runs (`https://api.github.com/repos/karan-banwasi/Patchwork/actions/runs`) and verify that all jobs (e.g., multi-OS tests across Windows, macOS, and Linux, as well as release builds) finish with status `completed` and conclusion `success` before marking any user task as completed. If CI fails, the agent must inspect job logs, resolve the root cause, and re-validate CI until green.
