# Patchwork Architecture Guide

## 1. Overview & Core Philosophy

Patchwork is a cross-platform CLI tool built in Go that discovers, plans, and executes package updates across multiple package ecosystems on Windows, macOS, and Linux.

The engine is built around several architectural tenets:
- **Decoupled Lifecycle**: Clear separation between discovery (`check`), interactive selection / target resolution, planning (`upgrade.NewPlan`), and execution (`upgrade.Executor`).
- **Safe Defaults**: Non-destructive dry-run previews (`--dry-run`), atomic configuration writes, ecosystem-aware downgrade prevention, and fail-closed security.
- **Platform Awareness**: Compile-time and runtime gating ensuring each host platform queries only native or applicable package managers without spawning unnecessary subprocesses.
- **Pluggable Architecture**: Clear interfaces (`pm.Manager` and extension interfaces) allowing new package managers to be added without modifying the core pipeline.
- **Process Isolation & Mockability**: All external CLI commands execute through the `pm.Runner` interface, allowing full unit and golden-fixture test coverage without requiring external CLI tools to be installed on developer machines or CI runners.

---

## 2. System Architecture Diagram

```
              +---------------------------------------------+
              |           Cobra CLI Entry (cmd/)            |
              +---------------------------------------------+
                                     |
               +---------------------+---------------------+
               |                     |                     |
        pw check / doctor     pw upgrade / pin      pw self-update
               |                     |                     |
               v                     v                     v
       +-----------------+   +------------------+   +---------------+
       |   pm.Registry   |   | internal/upgrade |   | cmd/selfupdate|
       +-----------------+   +------------------+   +---------------+
               |                     |                     |
       +-----------------+           |                     |
       |   pm.Manager    | <---------+                     |
       +-----------------+                                 |
          |           |                                    |
    ExecRunner   Extension Interfaces                      |
          |           |                                    |
          v           v                                    v
    System CLIs   DetailedBulkUpgrader /              GitHub API /
   (winget, brew, Deferrer / Cleaner /                Attestation
    scoop, etc.)  Refresher / TimeoutProvider         Verification
```

---

## 3. Core Domain Interfaces (`internal/pm`)

The core abstraction for package managers is defined in `internal/pm/pm.go`.

### 3.1 `pm.Manager` (Base Interface)
Every package manager driver must implement `pm.Manager`:
```go
type Manager interface {
    Name() string
    Aliases() []string
    IsAvailable(ctx context.Context) bool
    GetAvailableUpdates(ctx context.Context) ([]PackageUpdate, error)
    UpgradePackage(ctx context.Context, id string) error
    ValidateID(id string) error
}
```
- `Name()`: Returns the canonical lowercase identifier (e.g. `"brew"`, `"winget"`, `"npm"`).
- `Aliases()`: Returns user-friendly CLI aliases (e.g. `[]string{"homebrew"}`).
- `IsAvailable(ctx)`: Probes whether the manager CLI is installed and usable on the host.
- `GetAvailableUpdates(ctx)`: Discovers outdated packages, returning a slice of `pm.PackageUpdate`.
- `UpgradePackage(ctx, id)`: Upgrades a single package by its ID.
- `ValidateID(id)`: Sanitizes package IDs to protect against command and argument injection.

### 3.2 Optional Extension Interfaces
Managers can optionally implement capability-specific interfaces to enable advanced engine features:

| Interface | Method Signature | Purpose |
| :--- | :--- | :--- |
| `Locator` | `Path() (string, error)`<br>`Version(ctx) (string, error)` | Exposes binary path and tool version for `pw doctor`. |
| `Cleaner` | `Cleanup(ctx, out, errOut) error` | Prunes package caches and old versions for `pw cleanup`. |
| `Deferrer` | `ShouldDefer(id string) bool` | Flags packages that must be installed last (e.g. running shell or terminal). |
| `Refresher` | `Refresh(ctx) error` | Refreshes remote repository indexes prior to update checking (skipped with `--no-refresh`). |
| `BulkUpgrader` | `UpgradePackages(ctx, ids) error` | Batches multiple package upgrades into a single CLI invocation. |
| `DetailedBulkUpgrader` | `UpgradePackagesDetailed(ctx, ids) ([]BulkPackageResult, error)` | Performs batched upgrades while returning granular per-package outcomes. |
| `OfflineConfigurable` | `SetOffline(offline bool)` | Signals to manager drivers to bypass network checks when `--offline` is active. |
| `NetworkDependent` | `RequiresNetwork() bool` | Identifies managers that cannot perform checks without an active network connection. |
| `IOConfigurable` | `SetIO(stdin io.Reader, stdout, stderr io.Writer)` | Connects standard I/O streams for interactive prompts or progress display. |
| `OptionsConfigurable` | `ConfigureOptions(opts UpgradeOptions)` | Receives runtime flags (e.g., `AcceptAgreements`, `TrustRepository`). |
| `WarningFetcher` | `GetAvailableUpdatesWithWarnings(ctx) ([]PackageUpdate, []error, error)` | Surfaces non-fatal warnings (e.g., individual unparseable lines) alongside updates. |
| `TimeoutProvider` | `FetchTimeout() time.Duration` | Specifies an ecosystem-tailored default timeout for update queries. |

---

## 4. Package Manager Registry (`pm.Registry`)

The `Registry` in `internal/pm/registry.go` acts as the central coordinator for all enabled package managers.

### Key Capabilities:
1. **Concurrent Discovery**: `FetchUpdates(ctx)` queries all available managers simultaneously via goroutines, streaming results and errors over channels.
2. **Timeout Hierarchy**: Per-manager query timeouts are resolved with the following priority:
   $$\text{CLI Flag } \texttt{--timeout} \longrightarrow \text{Manager } \texttt{TimeoutProvider.FetchTimeout()} \longrightarrow \text{Default } (2\text{ minutes})$$
3. **Partial Failure Resilience**: If some managers succeed while others encounter non-fatal errors or timeouts, the registry aggregates the issues as non-fatal warnings and continues processing updates. A fatal error is returned only if all managers fail.
4. **Downgrade Prevention**: Every discovered update candidate is verified against `internal/version.IsUpgrade()`. Candidate versions that would result in a downgrade or lateral version shift are safely discarded.

---

## 5. Upgrade Pipeline (`internal/upgrade`)

Package upgrading is structured as a 4-stage pipeline in `internal/upgrade/upgrade.go`:

```
   [Target Packages]
          │
          ▼
   1. Pin Filtering ────────► Fails closed if config.json corrupted;
          │                   Removes pinned packages unless forced.
          ▼
   2. Plan Construction ────► Orders packages; defers shell/terminal updates to tail.
          │
          ▼
   3. Dry-Run Preview ──────► (If --dry-run) Generates preview summary; exits cleanly.
          │
          ▼
   4. Execution Engine ─────► Validates IDs; uses DetailedBulkUpgrader,
                              BulkUpgrader, or sequential fallback.
```

### Pipeline Stages:
1. **Target Resolution & Pin Filtering (`FilterPins`)**: Compares requested packages against pinned definitions in `config.json`. If `config.json` is malformed or corrupted, the pipeline fails closed to prevent accidental upgrades of held packages.
2. **Plan Construction & Ordering (`NewPlan`, `OrderTargets`)**: Groups packages by manager and inspects `pm.Deferrer`. Shell and terminal applications (such as `Microsoft.PowerShell` and `Microsoft.WindowsTerminal`) are re-ordered to the end of the queue so that restarting a terminal does not abort remaining updates.
3. **Dry-Run Simulation (`plan.DryRunSummary`)**: If `--dry-run` is requested, renders the execution plan and predicted operations without invoking any system upgrade commands.
4. **Execution Engine (`Executor.Execute`)**:
   - Sanitizes each package identifier using `mgr.ValidateID()`.
   - Attempts batched execution via `DetailedBulkUpgrader` if implemented.
   - Falls back to `BulkUpgrader` or sequential single-package execution (`UpgradePackage`).
   - Tracks per-package durations and generates an `upgrade.Summary`.

---

## 6. Execution Abstraction (`pm.Runner`)

To ensure reliability, testability, and security:
- Subprocess invocations are abstracted through `pm.Runner`:
  ```go
  type Runner interface {
      Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error
  }
  ```
- The production implementation (`pm.ExecRunner`) delegates to `os/exec.CommandContext` with parameterized argument slices, preventing shell-injection attacks.
- Mock runners (`MockRunner` / `RecorderRunner`) allow unit tests to simulate command output, exit codes, and network latency without executing real system utilities.
- Output leak enforcement: internal packages strictly direct command output to assigned buffers or writers, guaranteeing that machine-readable outputs (`--json`) are never corrupted by unexpected stdout/stderr noise.

---

## 7. Version Comparison Engine (`internal/version`)

Package ecosystems follow varied versioning standards. The engine in `internal/version/version.go` provides ecosystem-aware semver comparison:
- **Ecosystem Gating**: Normalizes versions according to ecosystem rules (`"uv"` / `"npm"` / `"brew"` / `"winget"` / `"choco"`).
- **PEP 440 Pre-Release Ordering**: Python/uv version strings evaluate pre-releases with appropriate weights:
  $$\texttt{dev} < \texttt{alpha} < \texttt{beta} < \texttt{preview} < \texttt{rc} < \texttt{final} < \texttt{post}$$
- **Epoch & Revision Support**: Handles Debian/Brew style epochs (`1:2.0`) and revisions (`_1`), as well as Python epochs (`1!2.0`).
- **Build Metadata**: Strips build metadata (`+build.123`) before comparing core version segments.
- **Downgrade Guard**: `version.IsUpgrade(ecosystem, current, available)` prevents outdated registry indexes from prompting unintended downgrades.

---

## 8. Configuration Subsystem (`internal/config`)

The configuration subsystem in `internal/config/config.go` manages pinned packages and user preferences:
- **Platform Paths**: Locates configuration at standard OS directories via `os.UserConfigDir()`:
  - Windows: `%APPDATA%\Patchwork\config.json`
  - macOS: `~/Library/Application Support/Patchwork/config.json`
  - Linux: `~/.config/Patchwork/config.json` (or `$XDG_CONFIG_HOME`)
  - Override: `PATCHWORK_CONFIG` environment variable
- **Atomic Writes**: Writes are executed to a temporary file in the same directory, flushed to disk (`Sync()`), set to restrictive permissions (`0600`), and atomically renamed into place. This guarantees that power interruptions or process termination cannot result in a corrupted configuration file.
- **Fail-Closed Principle**: If configuration cannot be parsed due to invalid syntax or permissions, update operations fail closed rather than proceeding without pin constraints.

---

## 9. HTTP Client Infrastructure (`internal/httputil`)

Network communication (self-update checks and release downloads) routes through `internal/httputil`:
- **Exponential Backoff**: Automatic retries with jitter for transient HTTP 5xx errors or network timeouts.
- **Proxy Support**: Honors standard `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment variables.
- **User-Agent Stamping**: Identifies requests cleanly with the application version.
- **Offline Gating**: Explicitly checks `--offline` or `PATCHWORK_OFFLINE` to immediately return `httputil.ErrOffline` without attempting network round-trips.

---

## 10. Adding New Package Managers

For step-by-step instructions on implementing a new package manager driver, see [Developer Guide: Adding a Package Manager](docs/adding-a-package-manager.md).
