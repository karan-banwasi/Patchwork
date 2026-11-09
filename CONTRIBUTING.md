# Contributing to Patchwork

First off, thank you for taking the time to contribute to Patchwork! :tada:

Contributions from the community help make Patchwork a reliable, cross-platform package management companion across Windows, macOS, and Linux.

---

## Code of Conduct

All contributors and participants in the Patchwork project are expected to adhere to our [Code of Conduct](CODE_OF_CONDUCT.md). Please read it to understand our community standards and expectations.

---

## Getting Started

### Prerequisites
- **Go**: Version `1.22+` (or `1.27.3` matching `go.mod`).
- **Git**: Installed and configured.
- **Operating System**: Windows, macOS, or Linux.
- **Optional Tools**:
  - `golangci-lint`: For local linting.
  - `govulncheck`: For vulnerability auditing (`go install golang.org/x/vuln/cmd/govulncheck@latest`).

### Clone and Build
1. Fork and clone the repository:
   ```bash
   git clone https://github.com/<your-username>/Patchwork.git
   cd Patchwork
   ```
2. Build the executable locally:
   ```bash
   # On macOS / Linux:
   go build -o pw .

   # On Windows:
   go build -o pw.exe .
   ```
3. Test your build:
   ```bash
   ./pw --help
   ```

---

## Development Workflow & Branching Conventions

- **Main Branch**: The `main` branch represents production-ready code. Pull requests should target `main`.
- **Branch Naming**:
  - `feat/<short-description>`: New features or capabilities.
  - `fix/<short-description>`: Bug fixes and security patches.
  - `docs/<short-description>`: Documentation additions and corrections.
  - `refactor/<short-description>`: Structural code improvements without behavioral changes.
  - `test/<short-description>`: New test suites, benchmarks, or fixture additions.
- **Commit Messages**: Follow [Conventional Commits](https://www.conventionalcommits.org/):
  - `feat(winget): add support for portable installers`
  - `fix(selfupdate): resolve correct binary name on linux arm64 (#23)`
  - `docs: update cross-platform installation instructions (#40)`
  - Reference relevant issue numbers (e.g., `(#41)`) in commit descriptions.

---

## Coding Standards & Invariants

To keep Patchwork clean, maintainable, and secure, please adhere to these core principles:

### 1. Code Formatting
All Go source files must be formatted with `gofmt -s`. The Ubuntu CI workflow enforces:
```bash
gofmt -s -l .
```
Ensure you format code before committing:
```bash
gofmt -s -w .
```

### 2. Zero Leaks in `internal/` Packages
**Strict Project Rule**: Code inside `internal/` must NEVER call `fmt.Print*`, `os.Stdout`, or `os.Stderr` directly (except inside `*_test.go` files).
All terminal or stream output must be routed through explicit `io.Writer` instances, Cobra command output streams, or structured UI abstractions. CI validates this with:
```bash
grep -rnE 'fmt\.Print|os\.Stdout|os\.Stderr' internal/ | grep -v '_test\.go:'
```
This invariant guarantees that output is mockable, machine-readable (`--json`), and clean.

### 3. Cross-Platform Portability
- Always use standard library cross-platform abstractions:
  - File paths: `filepath.Join()`, `filepath.Clean()`.
  - User configuration: `os.UserConfigDir()`.
  - Platform detection: `runtime.GOOS`.
- Isolate OS-specific features (such as Windows UAC elevation, `conhost.exe`, or registry lookups) behind Go build tags:
  - Windows files: `//go:build windows`
  - Unix / POSIX files: `//go:build !windows` or `//go:build darwin`

### 4. Error Handling
- Never discard returned errors with `_`.
- Wrap errors with rich context: `fmt.Errorf("parsing winget output: %w", err)`.
- If an external package manager tool is not installed on the system, handle it gracefully by returning appropriate status warnings rather than crashing or panicking.

---

## Testing Guidelines

Every bug fix or new feature must be backed by automated tests.

### Running Tests Locally
Run the full test suite with test cache disabled:
```bash
go test -count=1 ./...
```

Run tests with race detection (supported on macOS, Linux, and Windows 64-bit):
```bash
go test -v -race ./...
```

Run static analysis:
```bash
go vet ./...
```

Run vulnerability scanning:
```bash
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
```

### Golden Parser Fixtures
When adding or modifying package manager output parsers (`internal/brew`, `internal/winget`, etc.), include or update golden-file fixtures in `tests/fixtures/` or the corresponding test directory to ensure parsing logic remains resilient to CLI output format shifts.

---

## Pull Request Lifecycle & The CI Invariant

1. **Self-Review**: Verify your changes locally against all tests, vetting, formatting, and leak checks.
2. **Open Pull Request**: Create a PR targeting `main` using our [Pull Request Template](.github/pull_request_template.md).
3. **CI Validation Invariant**:
   - Every pull request triggers GitHub Actions CI workflows running across **Ubuntu**, **macOS**, and **Windows**.
   - **All jobs must complete with status `completed` and conclusion `success`**.
   - If CI fails on any platform, inspect the run logs, reproduce the issue locally if possible, push fixes, and verify that the workflow is green.
4. **Code Review**: Pull requests require review and approval from repository maintainers (`@karan-banwasi`).
