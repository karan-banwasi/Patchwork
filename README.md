# Patchwork
A CLI tool to better manage package updates. Built natively in Go for Windows, macOS, and Linux.

## Features
- **Check for Updates**: Fetch a perfectly aligned, tabular list of all available package updates across your system.
- **Interactive Upgrades**: Select which packages you want to upgrade using an interactive arrow-key and spacebar checkbox selector directly in your terminal (`↑`/`↓` to move, `Space` to select, `→` to select all, `←` to select none, `Enter` to confirm).
- **Bulk & Targeted Upgrades**: Use `--all` to apply all pending updates at once, `--dry-run` to preview what will happen, or pass one or more Package IDs to upgrade specific apps.
- **Multi-Package Manager Support**: Concurrently queries and manages updates across 24 supported package managers: **Homebrew** (`brew`), **Mac App Store** (`mas`), **Winget** (`winget`), **Scoop** (`scoop`), **Chocolatey** (`choco`), **PowerShell Modules** (`psresource`), **APT** (`apt`), **DNF** (`dnf`), **Pacman** (`pacman`), **Zypper** (`zypper`), **Flatpak** (`flatpak`), **Snap** (`snap`), **npm Global** (`npm -g`), **pnpm Global** (`pnpm -g`), **Bun Global** (`bun -g`), **.NET Global Tools** (`dotnet tool`), **uv Python Tools** (`uv tool`), **pipx Python Apps** (`pipx`), **Rust Cargo Crates** (`cargo`), **Rustup Toolchains** (`rustup`), **RubyGems** (`gem`), **mise** (`mise`), **asdf** (`asdf`), and **VS Code Extensions** (`vscode`). Automatically platform-aware to only query relevant package managers for your operating system.
- **Package Pinning (Ignore List)**: Pin one or more packages (`pw pin <id>...`) so they are automatically skipped during `pw check` and `pw upgrade` (also respects native Chocolatey and Homebrew pins).
- **Safe Terminal & Shell Self-Updating**: Automatically defers `Microsoft.PowerShell` and `Microsoft.WindowsTerminal` upgrades to the end of the queue and launches them in an isolated console window (`conhost.exe`) so closing your terminal mid-upgrade doesn't kill the installer or cause MSI error 1603.
- **Auto-Elevation (UAC)**: Automatically prompts for Administrator privileges via Windows UAC when upgrading Chocolatey packages from a non-elevated shell.
- **Machine-Readable Output**: Export pending updates as structured JSON (`--json`) or tab-separated rows (`--quiet`) for status bars (Oh My Posh, Yasb, GlazeWM) and automation scripts.
- **System Diagnostics & Cache Cleanup**: Inspect detected package managers and versions with `pw doctor` (`pw status`) and clean up old versions/caches with `pw cleanup` (`pw clean`).
- **In-Place Self-Updates**: Run `pw self-update` (with `--check`, `--prerelease`/`--channel`, and `--json` support, plus a cached 24h passive update notice) to update the Patchwork binary directly from GitHub Releases with universal SHA-256 checksum verification and Sigstore build provenance attestation across Windows, macOS, and Linux.
- **Upgrade Policies, Changelog Previews & Parallel Execution**: Filter upgrades with `--exclude` glob patterns and semver ceiling `--level` (`patch`, `minor`, `major`), inspect release/package links with `--changelog`, confirm non-interactively with `--yes`, and upgrade across managers concurrently (`--concurrency` / `-j`).
- **Scheduled Background Checks & Notifications**: Register native OS background checks (`pw schedule enable/disable/status/run`) via Windows Task Scheduler, macOS `launchd`, or Linux `systemd --user`/`crontab` with desktop notifications and `--bar` status summaries.
- **Upgrade History & Rollback**: Automatically log every upgrade run (`pw history`) and roll back individual packages or entire runs (`pw rollback`).
- **Machine Profile Export & Import**: Export installed packages and pins to a portable `Patchfile` or JSON (`pw export`) and provision or restore a machine (`pw import`).

## Supported Package Managers

Patchwork automatically detects which package managers are installed on your system and queries them in parallel:

| Manager | Platform | `-m` / `--manager` Flag | Underlying Check Command | Underlying Upgrade Command |
| :--- | :--- | :--- | :--- | :--- |
| **Homebrew** | macOS / Linux | `brew` (or `homebrew`) | `brew outdated --json=v2` | `brew upgrade <id>` |
| **Mac App Store** | macOS | `mas` (or `macappstore`, `appstore`) | `mas outdated` | `mas upgrade <id...>` |
| **Winget** | Windows | `winget` | `winget upgrade` | `winget upgrade --id <id> --exact` |
| **Scoop** | Windows | `scoop` | `scoop update` + `scoop status` | `scoop update <id>` |
| **Chocolatey** | Windows | `choco` (or `chocolatey`) | `choco outdated --limit-output` | `choco upgrade <id> -y --no-progress` |
| **PowerShell Modules** | Windows | `psresource` (or `powershell`) | `Get-InstalledPSResource` + `Find-PSResource` | `Update-PSResource -Name <id>` |
| **APT** | Linux | `apt` (or `apt-get`, `debian`, `ubuntu`) | `apt list --upgradable` | `apt-get install --only-upgrade -y <id...>` |
| **DNF** | Linux | `dnf` (or `yum`, `fedora`, `rhel`) | `dnf check-update --quiet` | `dnf upgrade -y <id...>` |
| **Pacman** | Linux | `pacman` (or `arch`) | `checkupdates` / `pacman -Qu` | `pacman -S --noconfirm <id...>` |
| **Zypper** | Linux | `zypper` (or `opensuse`, `suse`) | `zypper --terse list-updates` | `zypper --non-interactive update <id...>` |
| **Flatpak** | Linux | `flatpak` (or `flathub`) | `flatpak remote-ls --updates` | `flatpak update -y <id...>` |
| **Snap** | Linux | `snap` (or `snapcraft`, `snappy`) | `snap refresh --list` | `snap refresh <id...>` |
| **npm (Global)** | Cross-Platform | `npm` | `npm outdated -g --json` | `npm install -g <id>@latest` |
| **pnpm (Global)** | Cross-Platform | `pnpm` (or `pnpm-global`) | `pnpm outdated -g --format json` | `pnpm update -g <id...>` |
| **Bun (Global)** | Cross-Platform | `bun` (or `bun-global`) | `bun pm ls -g` + concurrent npm check | `bun add -g <id...@latest>` |
| **.NET Tools (Global)** | Cross-Platform | `dotnet` (or `dotnet-tool`) | `dotnet tool list -g` + concurrent NuGet search | `dotnet tool update -g <id>` |
| **uv Python Tools** | Cross-Platform | `uv` (or `uv-tool`) | `uv tool list` + concurrent PyPI check | `uv tool upgrade <id>` |
| **pipx Python Apps** | Cross-Platform | `pipx` (or `pipx-app`) | `pipx list --json` + concurrent PyPI check | `pipx upgrade <id>` |
| **Rust Cargo Crates** | Cross-Platform | `cargo` (or `rust`) | `cargo install --list` + concurrent crates.io check | `cargo install <id>` |
| **Rustup Toolchains** | Cross-Platform | `rustup` (or `rust-toolchain`) | `rustup check` | `rustup update <id...>` |
| **RubyGems** | Cross-Platform | `gem` (or `rubygems`, `ruby`) | `gem outdated` | `gem update <id...>` |
| **mise** | Cross-Platform | `mise` (or `rtx`) | `mise outdated --json` | `mise upgrade <id...>` |
| **asdf** | Cross-Platform | `asdf` | `asdf current` + `asdf latest` | `asdf install <id> latest && asdf set <id> latest` |
| **VS Code Extensions** | Cross-Platform | `vscode` (or `code`, `vs-code`) | `code --list-extensions --show-versions` + Marketplace check | `code --install-extension <id> --force` |

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

**Skip repository/bucket network refresh or disable all network access (`--no-refresh`, `--offline`):**
```bash
# Check using cached local indexes without refreshing remote repositories
pw check --no-refresh

# Disable network access completely and check only local state
pw check --offline
```

**Filter by exclude glob patterns, semver bump level, or preview changelog links:**
```bash
pw check --exclude "python*,*dev*" --level minor
pw check --changelog
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
> **Interactive Controls:** Use `↑`/`↓` (or `j`/`k`) to move, `Space` to toggle selection, `→` (`a`) to select all, `←` (`n`) to select none, `Enter` to confirm, and `Esc`/`q`/`Ctrl+C` to cancel safely.

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

**Filter policies, preview changelogs, and control execution concurrency:**
```bash
# Exclude packages matching globs or cap updates at minor semver bumps
pw upgrade --all --exclude "python*,*dev*" --level minor

# Show release/package changelog URLs and confirm non-interactively
pw upgrade --all --changelog --yes

# Upgrade across package managers concurrently (default sequential = 1; 0 = all managers)
pw upgrade --all -j 4
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

# Pin a package scoped to a specific package manager with constraints, reason, and expiry:
pw pin npm:typescript --version "<6.0.0" --reason "Await plugin upgrade" --expires 30d
pw pin winget:Git.Git
pw pin scoop:git

# List all currently pinned packages (supports --json)
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

Remove old package versions and prune download caches across installed cleanup-capable managers (`brew`, `apt`, `dnf`, `pacman`, `zypper`, `flatpak`, `scoop`, `npm`, `pnpm`, `bun`, `uv`, `gem`, `mise`), as well as Patchwork's temporary completion cache:

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

### 7. Configuration Management (`pw config`)

Inspect and update default settings, disabled managers, and pins persistently:

```bash
pw config list
pw config get accept_agreements
pw config set accept_agreements false
pw config set disabled_managers scoop,choco
pw config edit
```

---

### 8. Scheduled Background Checks & Notifications (`pw schedule`)

Register native user-level scheduled background checks (Windows Task Scheduler, macOS `launchd`, Linux `systemd --user`/`crontab`) with desktop notifications and status-bar output:

```bash
# Enable periodic background checks (e.g. hourly, daily, weekly, or 6h)
pw schedule enable --interval 6h --notify

# Check status or emit compact status-bar text (--bar) / JSON (--json)
pw schedule status
pw schedule status --bar

# Run a scheduled check on demand or disable background checks
pw schedule run
pw schedule disable
```

---

### 9. Upgrade History & Rollback (`pw history`, `pw rollback`)

Inspect logged upgrade runs and roll back packages on rollback-capable managers:

```bash
# View recent upgrade runs (supports --manager, --limit, --json, --clear)
pw history
pw history --limit 10 --json

# Roll back packages from the last run, a specific run ID, or to an explicit version
pw rollback --last --dry-run
pw rollback --run <run-id>
pw rollback <package-id> -m <manager> --to <version>
```

---

### 10. Machine Profile Export & Import (`pw export`, `pw import`)

Capture installed packages and configured pins to a portable `Patchfile` or JSON profile, and restore them on another machine:

```bash
# Export a portable Patchfile (or JSON with --json)
pw export -o Patchfile
pw export --json --include-versions -o machine-profile.json

# Preview and apply a profile on a target machine
pw import Patchfile --dry-run
pw import Patchfile
```

---

### 11. Version Information & Self-Update Check (`pw version`, `pw self-update --check`)

```bash
pw version
# or
pw --version

# Check whether a newer Patchwork release is available without modifying the binary
pw self-update --check
```

## Development

### Development with Nix
Patchwork supports zero-dependency developer environments and hermetic Linux/macOS builds via [Determinate Nix](https://determinate.systems/) and Nix Flakes (including automatic shell completions).

- **Install Determinate Nix**:
  ```bash
  curl -fsSL https://install.determinate.systems/nix | sh -s -- install
  ```
- **Enter the Developer Shell** (provides Go 1.27.4, `golangci-lint`, `gopls`, `goreleaser`, `govulncheck`, and `jq`; loaded automatically if you use `direnv` with `.envrc`):
  ```bash
  nix develop
  ```
- **Build or Run `pw` Hermetically**:
  ```bash
  nix build
  ./result/bin/pw version
  # or run directly without installing:
  nix run . -- --help
  ```
- **Run Flake Checks**:
  ```bash
  nix flake check
  ```

### Code Generation (`go-winres`)
This project uses [`go-winres`](https://github.com/tc-hib/go-winres) to embed the Windows application manifest (`longPathAware`, `asInvoker`), version metadata, and application icon into the binary. The generated `.syso` resource files are excluded from Git (`.gitignore`) to keep the repository clean.

During automated release builds, CI automatically stamps the exact semantic version and compiles the resources. For local development or whenever you modify `winres/winres.json` or `winres/icon.png`, generate the `.syso` files on-demand with:

```powershell
go generate ./...
```

### Publishing to Windows Package Manager (`winget-pkgs`)
This repository includes a validated Winget v1.12.0 portable package manifest (`KaranBanwasi.Patchwork`) under `winget/` and automated release generation in `.github/workflows/release.yml`.

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