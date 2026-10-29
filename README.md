# Patchwork
A CLI tool to better manage package updates. Built natively in Go for Windows.

## Features
- **Check for Updates**: Fetch a perfectly aligned, tabular list of all available package updates across your system.
- **Interactive Upgrades**: Select exactly which packages you want to upgrade using an intuitive checkbox interface directly in your terminal (`Space` to select, `<Right>` to select all, `<Left>` to select none, or type to filter).
- **Bulk & Targeted Upgrades**: Use `--all` to apply all pending updates at once, `--dry-run` to preview what will happen, or pass one or more Package IDs to upgrade specific apps.
- **Multi-Package Manager Support**: Concurrently queries and manages updates across **Winget** (`winget`), **Scoop** (`scoop`), **Chocolatey** (`choco`), **npm Global** (`npm -g`), **.NET Global Tools** (`dotnet tool`), **uv Python Tools** (`uv tool`), **Rust Cargo Crates** (`cargo`), and **PowerShell Modules** (`psresource`).
- **Package Pinning (Ignore List)**: Pin one or more packages (`pw pin <id>...`) so they are automatically skipped during `pw check` and `pw upgrade` (also respects native Chocolatey pins).
- **Safe Terminal & Shell Self-Updating**: Automatically defers `Microsoft.PowerShell` and `Microsoft.WindowsTerminal` upgrades to the end of the queue and launches them in an isolated console window (`conhost.exe`) so closing your terminal mid-upgrade doesn't kill the installer or cause MSI error 1603.
- **Auto-Elevation (UAC)**: Automatically prompts for Administrator privileges via Windows UAC when upgrading Chocolatey packages from a non-elevated shell.
- **Machine-Readable Output**: Export pending updates as structured JSON (`--json`) or tab-separated rows (`--quiet`) for status bars (Oh My Posh, Yasb, GlazeWM) and automation scripts.
- **System Diagnostics & Cache Cleanup**: Inspect detected package managers and versions with `pw doctor` (`pw status`) and clean up old versions/caches with `pw cleanup` (`pw clean`).
- **In-Place Self-Updates**: Run `pw self-update` to update `pw.exe` directly from GitHub Releases with automatic SHA-256 checksum verification.

## Supported Package Managers

Patchwork automatically detects which package managers are installed on your system and queries them in parallel:

| Manager | `-m` / `--manager` Flag | Underlying Check Command | Underlying Upgrade Command |
| :--- | :--- | :--- | :--- |
| **Winget** | `winget` | `winget upgrade` | `winget upgrade --id <id> --exact` |
| **Scoop** | `scoop` | `scoop update` + `scoop status` | `scoop update <id>` |
| **Chocolatey** | `choco` (or `chocolatey`) | `choco outdated --limit-output` | `choco upgrade <id> -y --no-progress` |
| **npm (Global)** | `npm` | `npm outdated -g --json` | `npm install -g <id>@latest` |
| **.NET Tools (Global)** | `dotnet` (or `dotnet-tool`) | `dotnet tool list -g` + concurrent NuGet search | `dotnet tool update -g <id>` |
| **uv Python Tools** | `uv` (or `uv-tool`) | `uv tool list` + concurrent PyPI check | `uv tool upgrade <id>` |
| **Rust Cargo Crates** | `cargo` (or `rust`) | `cargo install --list` + concurrent crates.io check | `cargo install <id>` |
| **PowerShell Modules** | `psresource` (or `powershell`) | `Get-InstalledPSResource` + `Find-PSResource` | `Update-PSResource -Name <id>` |

## Installation

### Quick Install (PowerShell)
You can install `pw` directly using PowerShell (no Go compiler or Git required):

```powershell
irm https://raw.githubusercontent.com/karan-banwasi/Patchwork/main/install.ps1 | iex
```

This automatically detects your system architecture (`amd64` or `arm64`), downloads the latest release binary (`pw.exe`), installs it to `%LOCALAPPDATA%\Programs\Patchwork\bin`, and adds it to your User `PATH`.

### Updating Patchwork
Once installed, you can update `pw` to the latest release at any time without re-running the installer script:

```powershell
pw self-update
# or
pw update
```

### Download Pre-compiled Binary
Alternatively, download `pw.exe` (x64) or `pw-arm64.exe` (ARM64) directly from the [GitHub Releases](https://github.com/karan-banwasi/Patchwork/releases) page and place it in any folder in your system's `PATH`.

### Building from Source (Developers)
If you have Go installed, clone the repository and build:

```powershell
go build -o $env:USERPROFILE\go\bin\pw.exe .
```

*(Or run `go install .` to install as `patchwork.exe`.)*

## Usage

### 1. Checking for Updates (`pw check`)

**Check for available updates across all installed managers:**
```powershell
pw check
```

**Group updates by package manager:**
```powershell
pw check --by-manager
```

**Check updates for a specific package manager only:**
```powershell
pw check -m winget
pw check --manager scoop
pw check -m npm
pw check -m dotnet
```

**Include pinned (ignored) packages in the check output:**
```powershell
pw check --include-pinned
```

**Machine-readable output (for status bars, prompts, and scripts):**
```powershell
# Structured JSON array of pending updates
pw check --json

# Tab-separated output: <Id>\t<Version>\t<AvailableVersion>\t<Manager>
pw check --quiet
# or
pw check -q
```

---

### 2. Upgrading Packages (`pw upgrade`)

Every upgrade run concludes with an **Upgrade Summary Table** displaying each package's final status (`Succeeded`, `Failed`, `Dry-Run`, or `Skipped`) and execution duration.

**Open the interactive upgrade checklist:**
```powershell
pw upgrade
```
> **Interactive Controls:** Use `Up`/`Down` arrows to navigate, `Space` to toggle a package, `Right Arrow` to select all, `Left Arrow` to select none, type characters to filter the list, and press `Enter` to confirm. Submitting an empty selection cancels safely.

**Interactive upgrade grouped manager-by-manager:**
```powershell
pw upgrade --by-manager
```

**Open interactive upgrade for a single package manager:**
```powershell
pw upgrade -m scoop
pw upgrade --manager winget
```

**Upgrade one or more specific packages by ID or Name:**
```powershell
pw upgrade Microsoft.VisualStudioCode
pw upgrade Git.Git Microsoft.PowerShell

# Target a specific package manager explicitly:
pw upgrade git -m scoop
pw upgrade typescript -m npm
pw upgrade ruff -m uv
```

**Upgrade all available packages at once:**
```powershell
pw upgrade --all
# or
pw upgrade -a

# Upgrade all packages for a single manager:
pw upgrade --all -m winget
```

**Preview upgrades without installing anything (`--dry-run`):**
```powershell
pw upgrade --all --dry-run
pw upgrade Microsoft.PowerShell --dry-run
pw upgrade --dry-run
```

---

### 3. Pinning / Ignoring Packages (`pw pin`, `pw unpin`, `pw pinned`)

If a package has a broken installer or you want to hold back its version, pin it so Patchwork skips it during `pw check` and `pw upgrade`:

```powershell
# Pin one or more packages by ID or Name (supports Tab completion)
pw pin Microsoft.VisualStudioCode
pw pin Git.Git nodejs

# List all currently pinned packages
pw pinned

# Unpin one or more packages so updates are shown again
pw unpin Microsoft.VisualStudioCode
pw unpin Git.Git nodejs
```

Pinned packages are saved in `%APPDATA%\Patchwork\config.json` (you can override this path by setting the `PATCHWORK_CONFIG` environment variable).

---

### 4. Shell Tab-Completions (`pw completion`)

Patchwork supports dynamic Tab completion for subcommands, flags, manager names (`-m`), available package IDs (`pw upgrade <Tab>`, `pw pin <Tab>`), and pinned packages (`pw unpin <Tab>`).

**Enable completions in your current PowerShell session:**
```powershell
pw completion powershell | Out-String | Invoke-Expression
```

**Persist completions across all future PowerShell sessions:**
```powershell
Add-Content -Path $PROFILE -Value "`npw completion powershell | Out-String | Invoke-Expression"
```
*(Bash, Zsh, and Fish completions are also supported via `pw completion [bash|zsh|fish]`.)*

---

### 5. System Diagnostics (`pw doctor` / `pw status`)

Inspect detected package managers, their versions, resolved executable paths, and Patchwork configuration paths:

```powershell
pw doctor
# or
pw status
```

---

### 6. Cache & Old Version Cleanup (`pw cleanup` / `pw clean`)

Remove old package versions and prune download caches across installed managers (`scoop`, `npm`, `uv`), as well as Patchwork's temporary completion cache:

```powershell
# Clean up all supported managers
pw cleanup
# or
pw clean

# Clean up a specific package manager only
pw cleanup -m scoop
pw cleanup -m npm
pw cleanup -m uv
```

---

### 7. Version Information (`pw version`)

```powershell
pw version
# or
pw --version
```

## Development

### Code Generation (`go-winres`)
This project uses [`go-winres`](https://github.com/tc-hib/go-winres) to embed the Windows application manifest, version metadata, and icon into the binary. The pre-built `.syso` files (`rsrc_windows_amd64.syso`, `rsrc_windows_arm64.syso`, `rsrc_windows_386.syso`) are committed to the repository, so a normal `go build` works out-of-the-box.

If you modify `winres/winres.json` or replace `winres/icon.png`, regenerate them with:

```powershell
go generate ./...
```

### Publishing to Windows Package Manager (`winget-pkgs`)
This repository includes a validated Winget v1.6.0 portable package manifest (`KaranBanwasi.Patchwork`) under `winget/` and automated release generation in `.github/workflows/release.yml`.

1. **Generate & Validate Local Manifests**:
   ```powershell
   .\winget\generate-manifest.ps1
   ```
   This downloads the latest release binaries (`pw.exe` and `pw-arm64.exe`), computes their `SHA256` hashes, populates `winget/manifests/k/KaranBanwasi/Patchwork/<version>/`, and runs `winget validate`.
2. **Submit Package to `microsoft/winget-pkgs`**:
   ```powershell
   .\winget\submit-pr.ps1
   ```
   *(Uses a fast sparse git checkout + `gh pr create` to avoid GitHub's `429 gitmon` directory listing rate-limit in `wingetcreate`.)*
3. **Automated Updates on Release**:
   Add a classic GitHub Personal Access Token (with `public_repo` scope) as a repository secret named `WINGET_TOKEN` in your GitHub repository settings. Every new release will automatically open a pull request to `microsoft/winget-pkgs`.

## Roadmap
- [x] Prepare and validate official `Patchwork` package manifests (`KaranBanwasi.Patchwork`) and CI release automation for `microsoft/winget-pkgs`.
- [x] Expand package manager support beyond Winget (Scoop, Chocolatey).
- [x] Group pending updates by package manager (`--by-manager`).
- [x] **Package Pinning / Ignore List**: Add `pw pin <id>`, `pw unpin <id>`, and `pw pinned` to hide or skip excluded packages during `pw check` and `pw upgrade`.
- [x] **Self-Update Subcommand (`pw self-update`)**: Upgrade `pw.exe` in-place directly from GitHub Releases without re-running `install.ps1`.
- [x] **Auto-Elevation (UAC) Prompt**: Offer automatic UAC elevation when upgrading Chocolatey or administrator-scoped packages from a non-elevated shell.
- [x] **Machine-Readable Output (`--json` / `--quiet`)**: Support structured JSON output on `pw check` for status bars (Oh My Posh, Yasb, GlazeWM) and scripts.
- [x] **Dry-Run (`--dry-run`) & Post-Upgrade Summary Table**: Preview planned upgrades and display a final status table (`Succeeded`, `Failed`, `Skipped`) after bulk runs.
- [x] **Shell Completions (`pw completion powershell`)**: Dynamic tab completion for subcommands, manager names (`-m`), and available package IDs (with fast short-lived caching).
- [x] **Additional Global Tool Managers**: Extend `pm.Manager` to support `npm`, `dotnet tool`, `uv tool`, `cargo`, and `psresource`.
- [x] **Diagnostics & Cleanup (`pw doctor`, `pw cleanup`)**: Inspect detected managers/paths and prune package caches.

