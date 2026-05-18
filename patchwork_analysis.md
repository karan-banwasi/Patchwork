# Patchwork — Code Review & Improvement Suggestions

## 🐛 Bugs / Correctness

### 1. `*.exe` in `.gitignore` — compiled binaries are committed
`patchwork.exe` and `pw.exe` are both present in the repo root, but `.gitignore` already contains `*.exe`. This means they were force-added at some point, or the gitignore rule was added after the files were already tracked. Committed binaries inflate repo size, pollute `git diff`, and can confuse people building from source.

**Fix:** Untrack the committed binaries:
```powershell
git rm --cached patchwork.exe pw.exe
git commit -m "chore: untrack committed binaries"
```

---

### 2. `--all` flag is not exclusive of positional `[package-id]`
In `cmd/upgrade.go`, the order of precedence is `upgradeAll` → `len(args) > 0` → interactive. Nothing prevents a user from running `pw upgrade --all Microsoft.VSCode`, which silently ignores the package ID and upgrades everything. That's surprising behavior.

**Fix:** Add a mutual-exclusion guard at the top of the `Run` function:
```go
if upgradeAll && len(args) > 0 {
    fmt.Fprintln(os.Stderr, "Error: --all and a package ID are mutually exclusive.")
    os.Exit(1)
}
```

---

### 3. `wingetPath()` only checks one hard-coded location
`winget.exe` lives at `%LOCALAPPDATA%\Microsoft\WindowsApps\winget.exe` on most systems, but that can differ on ARM64 devices, in LTSC/Server SKUs, or in sandboxed CI environments. If the path check fails, the tool is completely unusable with no useful guidance.

**Fix:** Fall back to `exec.LookPath("winget.exe")` after the trusted-path check fails, or at least produce a clearer error message pointing users to `winget --info` to find the real path.

---

## 🛡️ Robustness

### 4. `GetAvailableUpdates` uses `CombinedOutput` — stderr contaminates the parser
`cmd.CombinedOutput()` merges stdout and stderr into a single byte slice. Winget progress spinners and ANSI codes already make parsing fragile; injecting stderr lines into the same buffer makes it worse. The parser may silently skip or misparse packages when winget emits warnings to stderr.

**Fix:** Use separate `Stdout`/`Stderr` capture:
```go
var stdout, stderr bytes.Buffer
cmd.Stdout = &stdout
cmd.Stderr = &stderr
err = cmd.Run()
outputStr := stdout.String()
```

---

### 5. ANSI escape sequences are not stripped before parsing
The parser handles `\r` overwrite tricks from the progress spinner, but winget also emits **ANSI color/cursor codes** (e.g., `\x1b[?25l`, `\x1b[2K`). These can corrupt column-offset calculations if they land inside the header line.

**Fix:** Add a lightweight ANSI-stripping pass using a simple regex before calling `parseWingetOutput`:
```go
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[mGKHFABCDJsuhl?]`)
clean := ansiEscape.ReplaceAllString(raw, "")
```

---

### 6. Parser is brittle to header-column re-ordering
The parser assumes winget always outputs columns in the order `Name | Id | Version | Available | Source`. Winget's locale or future versions could reorder these. The current code would silently produce garbage data.

**Fix:** After finding column offsets, validate that `idIdx > 0`, `versionIdx > idIdx`, `availableIdx > versionIdx`, and `sourceIdx > availableIdx`. Return an error (not silently an empty slice) if the invariant fails.

---

### 7. Silent partial-failure during bulk interactive upgrade
In `cmd/upgrade.go`, when upgrading multiple selected packages, a per-package error is printed but the loop continues and the exit code is always `0`. The caller has no machine-readable signal that some packages failed.

**Fix:** Track a `hadError bool` flag across the loop and call `os.Exit(1)` after the loop completes if any package failed.

---

## 🎨 UX

### 8. `check` and `upgrade` both call `GetAvailableUpdates` independently
If a user runs `pw upgrade` (interactive mode), it calls winget twice — once to show the check result, once internally for the upgrade list. This doubles the wait time, which is noticeable since `winget upgrade` can take 5–10 seconds.

**Fix:** Cache the result in memory for the lifetime of the command, or simply pass the already-fetched update list through to the upgrade flow.

---

### 9. No `--version` / `-v` flag
There is no way to check what version of `pw` is installed. This is especially important since the CI pipeline auto-bumps tags on every push.

**Fix:** Embed the version at build time using `-ldflags` and expose it:
```go
// root.go
var version = "dev"   // overridden by -ldflags
rootCmd.Version = version
```
```powershell
# in release.yml
go build -ldflags "-X github.com/karan-banwasi/patchwork/cmd.version=$NEW_TAG" -o pw.exe main.go
```

---

### 10. No `--dry-run` flag on `upgrade`
Power users often want to see exactly which winget commands would be run before committing. A `--dry-run` flag that prints the resolved commands without executing them is a low-effort, high-value addition.

---

### 11. Interactive survey — "Cancel" option is a footgun
The current cancel experience appends `❌ Cancel (Do not upgrade any packages)` as the last selectable item in the multi-select list. A user who accidentally checks it alongside real packages will abort all upgrades. 

**Fix:** Check for the cancel option *before* iterating the selected packages, and also consider using Ctrl+C / empty submit as the only cancel path — removing the cancel option entirely since the prompt already explains "submit empty to cancel."

> This is actually already documented in the prompt message, making the explicit cancel option redundant.

---

## 🏗️ Architecture

### 12. `cmd/format.go` is tightly coupled to `internal/winget`
`format.go` in the `cmd` package directly imports `internal/winget` just to reference `winget.PackageUpdate`. As you add other package managers (Scoop, Chocolatey), you'll need a common `PackageUpdate` type that all managers share, or `format.go` will need separate formatting functions per manager.

**Fix:** Define a shared `internal/packages` (or `internal/manager`) package that exposes a common `PackageUpdate` struct. Each manager (`winget`, `scoop`, etc.) returns that type. `cmd/format.go` imports only the shared type.

---

### 13. No interface / abstraction for package managers
The `winget` package is called directly from `cmd/check.go` and `cmd/upgrade.go`. There is no `PackageManager` interface, making it impossible to add Scoop/Chocolatey support without editing the command files themselves.

**Fix:** Define an interface in `internal/manager` (or similar):
```go
type Manager interface {
    Name() string
    GetAvailableUpdates() ([]PackageUpdate, error)
    UpgradePackage(id string) error
    UpgradeAll() error
}
```
Commands iterate over a registered slice of `Manager` implementations, making the system truly extensible without touching `cmd/`.

---

## ⚙️ CI/CD

### 14. Release workflow triggers on every push to `main` — no tests gate it
Every single commit to `main` creates a new GitHub Release. There are no tests (`go test ./...`) or lint checks (`go vet ./...`) guarding the release. A bad commit ships immediately.

**Fix:** Split into two jobs — a `test` job that runs `go vet ./...` and `go test ./...`, and a `release` job that `needs: [test]`.

---

### 15. Auto-incrementing patch version is not tied to commit content
The CI always bumps the patch number regardless of whether the change is a fix, a feature, or a breaking change. Conventional Commits + a tool like `release-please` or `semantic-release` would allow the commit message to drive the version bump (patch/minor/major) automatically.

---

### 16. No `go vet` or linter in CI
There is no static analysis step. A simple `go vet ./...` step catches common mistakes (shadowed variables, `fmt.Errorf` with `%w` on nil, etc.) for free.

---

## 🔧 Maintainability

### 17. No tests
There are no unit tests anywhere in the project. The parser logic in `internal/winget/winget.go` is complex enough (ANSI stripping, carriage-return handling, column-offset math) that it would clearly benefit from table-driven tests with fixture output strings from real winget invocations.

**Priority areas to test:**
- `parseWingetOutput` with a variety of real/mocked winget outputs (no updates, one update, spinner artifacts, ANSI codes)
- `getPackageColumnWidths` edge cases (empty slice, very long names)
- `wingetPath` with a mock `LOCALAPPDATA`

---

### 18. `go.mod` declares `go 1.25.0` — this is a pre-release / future version
As of May 2026, Go 1.25 is not yet stable. The `go` directive in `go.mod` sets the **minimum** Go version. Declaring a future/pre-release version means anyone on a stable Go toolchain cannot build the project.

**Fix:** Pin to the latest stable release (e.g., `go 1.23.0` or `go 1.24.0`) unless you are specifically using 1.25 language features.

---

## Summary Table

| # | Area | Severity | Effort |
|---|------|----------|--------|
| 1 | Committed binaries in git | Medium | Low |
| 2 | `--all` + package-ID not mutually exclusive | Low | Low |
| 3 | Hard-coded winget path with no fallback | Medium | Low |
| 4 | `CombinedOutput` corrupts parser input | Medium | Low |
| 5 | ANSI codes not stripped before parsing | High | Low |
| 6 | Parser doesn't validate column ordering | Medium | Low |
| 7 | Bulk upgrade always exits 0 on partial failure | Medium | Low |
| 8 | Double `winget upgrade` invocation | Low | Low |
| 9 | No `--version` flag | Low | Low |
| 10 | No `--dry-run` flag | Low | Medium |
| 11 | Cancel option footgun in interactive mode | Low | Low |
| 12 | `format.go` imports concrete `winget` type | Medium | Medium |
| 13 | No package manager interface/abstraction | High | High |
| 14 | No tests gate the release | High | Medium |
| 15 | Version bump not semantic | Low | Medium |
| 16 | No `go vet` / linter in CI | Medium | Low |
| 17 | No unit tests | High | High |
| 18 | `go 1.25.0` in go.mod (pre-release) | Medium | Low |
