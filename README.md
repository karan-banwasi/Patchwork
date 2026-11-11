# Patchwork
A CLI tool to better manage package updates. Built natively in Go for Windows, macOS, and Linux.

## Features
- **Check for Updates**: Fetch a perfectly aligned, tabular list of all available package updates across your system.
- **Interactive Upgrades**: Select which packages you want to upgrade using an interactive arrow-key and spacebar checkbox selector directly in your terminal (`↑`/`↓` to move, `Space` to select, `→` to select all, `←` to select none, `Enter` to confirm).
- **Bulk & Targeted Upgrades**: Use `--all` to apply all pending updates at once, `--dry-run` to preview what will happen, or pass one or more Package IDs to upgrade specific apps.
- **Multi-Package Manager Support**: Concurrently queries and manages updates across **Homebrew** (`brew`), **Winget** (`winget`), **Scoop** (`scoop`), **Chocolatey** (`choco`), **npm Global** (`npm -g`), **.NET Global Tools** (`dotnet tool`), **uv Python Tools** (`uv tool`), **Rust Cargo Crates** (`cargo`), and **PowerShell Modules** (`psresource`). Automatically platform-aware to only query relevant package managers for your operating system.
- **Package Pinning (Ignore List)**: Pin one or more packages (`pw pin <id>...`) so they are automatically skipped during `pw check` and `pw upgrade` (also respects native Chocolatey and Homebrew pins).
- **Safe Terminal & Shell Self-Updating**: Automatically defers `Microsoft.PowerShell` and `Microsoft.WindowsTerminal` upgrades to the end of the queue and launches them in an isolated console window (`conhost.exe`) so closing your terminal mid-upgrade doesn't kill the installer or cause MSI error 1603.
- **Auto-Elevation (UAC)**: Automatically prompts for Administrator privileges via Windows UAC when upgrading Chocolatey packages from a non-elevated shell.
- **Machine-Readable Output**: Export pending updates as structured JSON (`--json`) or tab-separated rows (`--quiet`) for status bars (Oh My Posh, Yasb, GlazeWM) and automation scripts.
- **System Diagnostics & Cache Cleanup**: Inspect detected package managers and versions with `pw doctor` (`pw status`) and clean up old versions/caches with `pw cleanup` (`pw clean`).
- **In-Place Self-Updates**: Run `pw self-update` to update the Patchwork binary directly from GitHub Releases with universal SHA-256 checksum verification and Sigstore build provenance attestation across Windows, macOS, and Linux.

## Supported Package Managers

Patchwork automatically detects which package managers are installed on your system and queries them in parallel:

| Manager | Platform | `-m` / `--manager` Flag | Underlying Check Command | Underlying Upgrade Command |
| :--- | :--- | :--- | :--- | :--- |
| **Homebrew** | macOS / Linux | `brew` (or `homebrew`) | `brew outdated --json=v2` | `brew upgrade <id>` |
| **Winget** | Windows | `winget` | `winget upgrade` | `winget upgrade --id <id> --exact` |
| **Scoop** | Windows | `scoop` | `scoop update` + `scoop status` | `scoop update <id>` |
| **Chocolatey** | Windows | `choco` (or `chocolatey`) | `choco outdated --limit-output` | `choco upgrade <id> -y --no-progress` |
| **npm (Global)** | Cross-Platform | `npm` | `npm outdated -g --json` | `npm install -g <id>@latest` |
| **.NET Tools (Global)** | Cross-Platform | `dotnet` (or `dotnet-tool`) | `dotnet tool list -g` + concurrent NuGet search | `dotnet tool update -g <id>` |
| **uv Python Tools** | Cross-Platform | `uv` (or `uv-tool`) | `uv tool list` + concurrent PyPI check | `uv tool upgrade <id>` |
| **Rust Cargo Crates** | Cross-Platform | `cargo` (or `rust`) | `cargo install --list` + concurrent crates.io check | `cargo install <id>` |
| **PowerShell Modules** | Windows | `psresource` (or `powershell`) | `Get-InstalledPSResource` + `Find-PSResource` | `Update-PSResource -Name <id>` |

## Installation

### Windows

#### Quick Install (PowerShell)
You can install `pw` directly using PowerShell (no Go compiler or Git required):

```powershell
irm https://raw.githubusercontent.com/karan-banwasi/Patchwork/main/install.ps1 | iex
```

This automatically detects your system architecture (`amd64` or `arm64`), downloads the latest release binary (`pw.exe` or `pw-arm64.exe`), verifies cryptographic build provenance and SHA-256 checksums, installs it to `%LOCALAPPDATA%\Programs\Patchwork\bin`, and adds it to your User `PATH`.

#### Package Managers (Winget & Scoop)
- **Winget**:
  ```powershell
  winget install KaranBanwasi.Patchwork
  ```
- **Scoop**:
  ```powershell
  scoop bucket add patchwork https://github.com/karan-banwasi/Patchwork
  scoop install pw
  ```

---

### macOS & Linux

#### Quick Install (POSIX Shell)
Install `pw` automatically on macOS or Linux with SHA-256 checksum verification and Sigstore build provenance attestation checks:

```bash
curl -fsSL https://raw.githubusercontent.com/karan-banwasi/Patchwork/main/install.sh | sh
```

Or using `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/karan-banwasi/Patchwork/main/install.sh | sh
```

#### Homebrew (macOS & Linux)
Install `pw` directly via repository tap:

```bash
brew install karan-banwasi/patchwork/pw
```

Patchwork also natively queries and manages Homebrew formulae and casks out of the box (`pw check -m brew`, `pw upgrade -m brew`). When Patchwork is installed via Homebrew prefix, `pw self-update` recognizes the installation and prompts `brew upgrade pw`.

#### Manual Download (Pre-compiled Binary via curl)
Download the pre-compiled standalone binary for your platform and architecture directly to `/usr/local/bin` (or `~/.local/bin`):

```bash
# macOS (Apple Silicon - M1/M2/M3/M4)
sudo curl -fsSL https://github.com/karan-banwasi/Patchwork/releases/latest/download/pw-darwin-arm64 -o /usr/local/bin/pw && sudo chmod +x /usr/local/bin/pw

# macOS (Intel)
sudo curl -fsSL https://github.com/karan-banwasi/Patchwork/releases/latest/download/pw-darwin-amd64 -o /usr/local/bin/pw && sudo chmod +x /usr/local/bin/pw

# Linux (x86_64 / amd64)
sudo curl -fsSL https://github.com/karan-banwasi/Patchwork/releases/latest/download/pw-linux-amd64 -o /usr/local/bin/pw && sudo chmod +x /usr/local/bin/pw

# Linux (ARM64 / aarch64)
sudo curl -fsSL https://github.com/karan-banwasi/Patchwork/releases/latest/download/pw-linux-arm64 -o /usr/local/bin/pw && sudo chmod +x /usr/local/bin/pw
```

---

### Download Pre-compiled Binaries (Direct Release Assets)
Standalone binaries for all supported platforms and architectures are available from [GitHub Releases](https://github.com/karan-banwasi/Patchwork/releases):

| OS | Architecture | Binary Asset Name |
| :--- | :--- | :--- |
| **Windows** | x86_64 / AMD64 | `pw.exe` (or `pw-windows-amd64.exe`) |
| **Windows** | ARM64 | `pw-arm64.exe` (or `pw-windows-arm64.exe`) |
| **macOS** | Apple Silicon (ARM64) | `pw-darwin-arm64` |
| **macOS** | Intel (AMD64) | `pw-darwin-amd64` |
| **Linux** | x86_64 / AMD64 | `pw-linux-amd64` |
| **Linux** | ARM64 | `pw-linux-arm64` |

All release assets are accompanied by universal `SHA256SUMS` manifests and signed GitHub Sigstore cryptographic build provenance attestations.

---

### Building from Source (Go Developers)
If you have Go installed (1.22+), you can install the `pw` CLI binary directly via `go install`:

```bash
go install github.com/karan-banwasi/patchwork/cmd/pw@latest
```

Or clone the repository and build:

```bash
git clone https://github.com/karan-banwasi/Patchwork.git
cd Patchwork

# Linux / macOS:
go build -o pw .

# Windows (PowerShell):
go build -o pw.exe .
```

---

### Updating Patchwork (`pw self-update`)
Once installed, update `pw` to the latest release at any time without re-running installer scripts:

```bash
pw self-update
# or
pw update
```

The self-update engine automatically:
- Resolves your host OS and architecture to download the matching binary.
- Verifies integrity against universal `SHA256SUMS`.
- Verifies Sigstore build provenance attestation.
- Detects if Patchwork was installed via Winget, Scoop, or Homebrew, safely preventing in-place overwrite unless `--force` (`-f`) is specified.


## Usage

### 1. Checking for Updates (`pw check`)

**Check for available updates across all installed managers:**
```bash
pw check
```

**Group updates by package manager:**
```bash
pw check --by-manager
```

**Check updates for a specific package manager only:**
```bash
pw check -m brew
pw check -m winget
pw check --manager scoop
pw check -m npm
pw check -m dotnet
```

**Include pinned (ignored) packages in the check output:**
```bash
pw check --include-pinned
```

**Skip repository/bucket network refresh before checking (offline mode):**
```bash
pw check --no-refresh
```

**Set a custom per-manager query timeout:**
```bash
pw check --timeout 1m
```

**Machine-readable output (for status bars, prompts, and scripts):**
```bash
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
```bash
pw upgrade
```
> **Interactive Controls:** Enter comma-separated index numbers or ranges (e.g. `1, 3-5`), type `all` to select every package, or press `Enter` (or type `none`) to cancel safely.

**Interactive upgrade grouped manager-by-manager:**
```bash
pw upgrade --by-manager
```

**Open interactive upgrade for a single package manager:**
```bash
pw upgrade -m scoop
pw upgrade --manager winget
```

**Upgrade one or more specific packages by ID or Name:**
```bash
pw upgrade Microsoft.VisualStudioCode
pw upgrade Git.Git Microsoft.PowerShell

# Target a specific package manager explicitly:
pw upgrade git -m scoop
pw upgrade typescript -m npm
pw upgrade ruff -m uv
```

**Upgrade all available packages at once:**
```bash
pw upgrade --all
# or
pw upgrade -a

# Upgrade all packages for a single manager:
pw upgrade --all -m winget
```

**Preview upgrades without installing anything (`--dry-run`):**
```bash
pw upgrade --all --dry-run
pw upgrade Microsoft.PowerShell --dry-run
pw upgrade --dry-run
```

**Upgrade pinned packages or customize agreement prompts:**
```bash
# Upgrade packages even if pinned in your configuration:
pw upgrade Git.Git --force
# or
pw upgrade --all --include-pinned

# Disable automatic acceptance of Winget source/package agreements:
pw upgrade --all --accept-agreements=false

# Pass -TrustRepository when upgrading untrusted PowerShell PSResource feeds (Windows):
pw upgrade PSReadLine --trust-repository
```

---

### 3. Pinning / Ignoring Packages (`pw pin`, `pw unpin`, `pw pinned`)

If a package has a broken installer or you want to hold back its version, pin it so Patchwork skips it during `pw check` and `pw upgrade`:

```bash
# Pin one or more packages by ID (supports Tab completion)
pw pin Microsoft.VisualStudioCode
pw pin Git.Git nodejs

# Pin a package scoped to a specific package manager only:
pw pin winget:Git.Git
pw pin scoop:git

# List all currently pinned packages
pw pinned

# Unpin one or more packages so updates are shown again
pw unpin Microsoft.VisualStudioCode
pw unpin Git.Git nodejs
```

Pinned packages are saved in the platform-specific user configuration directory:
- **Windows**: `%APPDATA%\Patchwork\config.json` (e.g. `C:\Users\<user>\AppData\Roaming\Patchwork\config.json`)
- **macOS**: `~/Library/Application Support/Patchwork/config.json`
- **Linux**: `~/.config/Patchwork/config.json` (or `$XDG_CONFIG_HOME/Patchwork/config.json`)

You can override the configuration path on any operating system by setting the `PATCHWORK_CONFIG` environment variable.

---

### 4. Shell Tab-Completions (`pw completion`)

Patchwork supports dynamic Tab completion for subcommands, flags, manager names (`-m`), available package IDs (`pw upgrade <Tab>`, `pw pin <Tab>`), and pinned packages (`pw unpin <Tab>`).

#### Bash
```bash
# Current session:
source <(pw completion bash)

# Persist across sessions:
echo 'source <(pw completion bash)' >> ~/.bashrc
```

#### Zsh
```zsh
# Current session & persist:
echo 'source <(pw completion zsh)' >> ~/.zshrc
```

#### Fish
```fish
pw completion fish > ~/.config/fish/completions/pw.fish
```

#### PowerShell
```powershell
# Current session:
pw completion powershell | Out-String | Invoke-Expression

# Persist across sessions:
Add-Content -Path $PROFILE -Value "`npw completion powershell | Out-String | Invoke-Expression"
```

---

### 5. System Diagnostics (`pw doctor` / `pw status`)

Inspect detected package managers, their versions, resolved executable paths, and Patchwork configuration paths:

```bash
pw doctor
# or
pw status
```

---

### 6. Cache & Old Version Cleanup (`pw cleanup` / `pw clean`)

Remove old package versions and prune download caches across installed managers (`brew`, `scoop`, `npm`, `uv`), as well as Patchwork's temporary completion cache:

```bash
# Clean up all supported managers
pw cleanup
# or
pw clean

# Clean up a specific package manager only
pw cleanup -m brew
pw cleanup -m scoop
pw cleanup -m npm
pw cleanup -m uv
```

The short-lived update check completion cache is saved at:
- **Windows**: `%LOCALAPPDATA%\Patchwork\updates-cache.json`
- **macOS**: `~/Library/Caches/Patchwork/updates-cache.json`
- **Linux**: `~/.cache/Patchwork/updates-cache.json` (or `$XDG_CACHE_HOME/Patchwork/updates-cache.json`)
- Override: `PATCHWORK_CACHE` environment variable


---

### 7. Version Information (`pw version`)

```bash
pw version
# or
pw --version
```

## Development

### Code Generation (`go-winres`)
This project uses [`go-winres`](https://github.com/tc-hib/go-winres) to embed the Windows application manifest (`longPathAware`, `asInvoker`), version metadata, and application icon into the binary. The generated `.syso` resource files are excluded from Git (`.gitignore`) to keep the repository clean.

During automated release builds, CI automatically stamps the exact semantic version and compiles the resources. For local development or whenever you modify `winres/winres.json` or `winres/icon.png`, generate the `.syso` files on-demand with:

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