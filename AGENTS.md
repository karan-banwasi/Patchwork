# Patchwork: Agent Context & Guidelines

## Project Overview
Patchwork is a Windows CLI utility designed to iterate through packages that have available updates and manage them. 

## Technology Stack
- **Language**: Go (Primary)
- **Target OS**: Windows
- **CLI Framework**: Cobra (`github.com/spf13/cobra`)
- **Interactive UI**: Survey (`github.com/AlecAivazis/survey/v2`)

## Core Features (Initial Phase)
1. **Winget Integration**: Iterate through packages managed by Windows Package Manager (`winget`) that have updates available.
2. **Extensibility**: The architecture should allow for adding other package managers (e.g., Chocolatey, Scoop) in the future.
3. **Interactive Selection**: Provides a visual multi-select checklist to allow the user to easily pick which packages to upgrade without memorizing IDs.

## Architecture
- `main.go`: Entry point for the CLI tool. Points execution to the Cobra command tree.
- `cmd/`: Houses the Cobra CLI commands (`root.go`, `check.go`, `upgrade.go`).
- `internal/`: Contains core logic separated by package managers.
  - `internal/winget/`: Houses logic for interacting with the `winget` CLI to fetch updates and perform upgrades.
    - **Note on Winget Parsing**: `winget upgrade` often outputs ANSI progress spinners and color codes. The parser strips ANSI escape sequences, explicitly slices off characters preceding the "Name" column, and validates column ordering. Exec calls also separate `Stdout` and `Stderr` to ensure warnings do not corrupt the parsed table. It then dynamically calculates padded `fmt.Sprintf` boundaries to display perfectly aligned, native-looking tables.
  - Future modules will be placed in their respective `internal/<manager>` directories.

## Development Guidelines
- **Cross-Platform Readiness**: Even though the target is currently Windows, use Go's standard library cross-platform features where appropriate. If Windows-specific packages or syscalls are needed, isolate them or use build tags (`//go:build windows`).
- **Exec calls**: For invoking package managers, use `os/exec`. Keep parsing logic robust, as CLI outputs can change. Stream standard output/error to the user during long-running tasks.
- **Error Handling**: Gracefully handle errors when a package manager is not installed on the system.
- **UI Consistency**: Maintain tabular alignment and spacing for terminal output to match the native feel of tools like `winget`.

## Future Roadmap
- Expand support beyond Winget (Scoop, Chocolatey).
- Add functionality to group updates by package manager.
