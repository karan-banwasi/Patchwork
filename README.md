# Patchwork
A CLI tool to better manage package updates. Built natively in Go for Windows.

## Features
- **Check for Updates**: Fetch a perfectly aligned, tabular list of all available package updates.
- **Interactive Upgrades**: Select exactly which packages you want to upgrade using an intuitive checkbox interface directly in your terminal.
- **Bulk Upgrades**: Use the `--all` flag to automatically apply all pending updates at once.
- **Targeted Upgrades**: Upgrade a single app instantly by passing its Package ID.
- **Multi-Package Manager Support**: Leverages Windows Package Manager (`winget`), Scoop (`scoop`), and Chocolatey (`choco`) under the hood, concurrently fetching and managing updates across all available package managers.

## Installation

### Quick Install (PowerShell)
You can install `pw` directly using PowerShell (no Go compiler or Git required):

```powershell
irm https://raw.githubusercontent.com/karan-banwasi/Patchwork/main/install.ps1 | iex
```

This automatically downloads the latest release binary (`pw.exe`), installs it to `%LOCALAPPDATA%\Programs\Patchwork\bin`, and adds it to your User `PATH`.

### Download Pre-compiled Binary
Alternatively, download `pw.exe` directly from the [GitHub Releases](https://github.com/karan-banwasi/Patchwork/releases) page and place it in any folder in your system's `PATH`.

### Building from Source (Developers)
If you have Go installed, clone the repository and build:

```powershell
go build -o $env:USERPROFILE\go\bin\pw.exe .
```

*(Or run `go install .` to install as `patchwork.exe`.)*

## Development

### Code Generation (`go-winres`)
This project uses [`go-winres`](https://github.com/tc-hib/go-winres) to embed the Windows application manifest and icon into the binary. The pre-built `.syso` files (`rsrc_windows_amd64.syso`, `rsrc_windows_386.syso`) are committed to the repository, so a normal `go build` works out-of-the-box.

If you modify `winres/winres.json` or replace `winres/icon.png`, you must regenerate them:

```powershell
# Regenerate the .syso resource files
go generate ./...
```

## Usage

**Check for available updates:**
```powershell
pw check
```

**Check updates grouped by package manager:**
```powershell
pw check --by-manager
```

**Check updates for a specific package manager only:**
```powershell
pw check -m winget
# or
pw check --manager scoop
```

**Open the interactive upgrade menu:**
```powershell
pw upgrade
```

**Upgrade packages grouped manager-by-manager:**
```powershell
pw upgrade --by-manager
```

**Open interactive upgrade for a specific package manager only:**
```powershell
pw upgrade -m scoop
# or
pw upgrade --manager winget
```

**Upgrade a specific package by ID:**
```powershell
pw upgrade Microsoft.VisualStudioCode

# Target a specific package manager explicitly:
pw upgrade git -m scoop
```

**Upgrade all available packages:**
```powershell
pw upgrade --all

# Upgrade all packages for a single manager:
pw upgrade --all -m winget
```

**Check the tool version:**
```powershell
pw version
# or
pw --version
```

## Roadmap
- [ ] Publish official `Patchwork` package to Windows Package Manager repository (`microsoft/winget-pkgs`) for native `winget install Patchwork` support.
- [x] Expand package manager support beyond Winget (Scoop, Chocolatey).
- [x] Group pending updates by package manager (`--by-manager`).
- [ ] **Package Pinning / Ignore List**: Add `pw pin <id>`, `pw unpin <id>`, and `pw pinned` to hide or skip excluded packages during `pw check` and `pw upgrade`.
- [ ] **Self-Update Subcommand (`pw self-update`)**: Upgrade `pw.exe` in-place directly from GitHub Releases without re-running `install.ps1`.
- [ ] **Auto-Elevation (UAC) Prompt**: Offer automatic UAC elevation when upgrading Chocolatey or administrator-scoped packages from a non-elevated shell.
- [ ] **Machine-Readable Output (`--json` / `--quiet`)**: Support structured JSON output on `pw check` for status bars (Oh My Posh, Yasb, GlazeWM) and scripts.
- [ ] **Dry-Run (`--dry-run`) & Post-Upgrade Summary Table**: Preview planned upgrades and display a final status table (`Succeeded`, `Failed`, `Skipped`) after bulk runs.
- [ ] **Shell Completions (`pw completion powershell`)**: Dynamic tab completion for subcommands, manager names (`-m`), and available package IDs.
- [ ] **Additional Global Tool Managers**: Extend `pm.Manager` to support PowerShell Modules (`PSResourceGet`), `npm` global packages, `uv` / `pipx` tools, `cargo`, and `dotnet tool`.

