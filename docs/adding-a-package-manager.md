# Developer Guide: Adding a New Package Manager to Patchwork

This guide walks you through the process of implementing a new package manager driver in Patchwork.

Patchwork uses a modular architecture where each package manager is encapsulated in its own package under `internal/<manager>/` and registered into the CLI command tree.

---

## Step 1: Create the Manager Package Directory

Create a new directory under `internal/`:
```bash
mkdir -p internal/<manager>/testdata
```
For example, for Alpine's `apk` package manager: `internal/apk/testdata/`.

---

## Step 2: Implement the `pm.Manager` Base Interface

Create `internal/<manager>/<manager>.go`. Implement the mandatory `pm.Manager` interface:

```go
package <manager>

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

var (
	_ pm.Manager = (*Manager)(nil)
)

type Manager struct {
	runner pm.Runner
}

func NewManager() *Manager {
	return &Manager{runner: pm.ExecRunner{}}
}

// WithRunner allows injecting a mock runner for unit testing.
func WithRunner(r pm.Runner) *Manager {
	return &Manager{runner: r}
}

func (m *Manager) Name() string {
	return "<manager>"
}

func (m *Manager) Aliases() []string {
	return []string{"<alias1>", "<alias2>"}
}

func (m *Manager) IsAvailable(ctx context.Context) bool {
	_, err := exec.LookPath("<manager>")
	return err == nil
}

func (m *Manager) ValidateID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("package identifier cannot be empty")
	}
	// Sanitize against command injection characters
	if strings.ContainsAny(id, "\r\n;&|`$<>") {
		return fmt.Errorf("invalid characters in package identifier %q", id)
	}
	return nil
}

func (m *Manager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	// Execute the CLI command to fetch outdated packages
	// Use pure parsing functions so logic can be independently tested
	return nil, nil
}

func (m *Manager) UpgradePackage(ctx context.Context, id string) error {
	if err := m.ValidateID(id); err != nil {
		return err
	}
	// Execute the upgrade command
	return nil
}
```

---

## Step 3: Implement Relevant Extension Interfaces

Consider whether your package manager supports additional capabilities:

### `pm.Locator`
If the manager can report its executable path and version (used by `pw doctor`):
```go
func (m *Manager) Path() (string, error) {
	return exec.LookPath("<manager>")
}

func (m *Manager) Version(ctx context.Context) (string, error) {
	// Run `<manager> --version` via m.runner and parse output
}
```

### `pm.Refresher`
If the manager needs to synchronize remote repository indexes before checking for updates (e.g., `apt update`, `brew update`):
```go
func (m *Manager) Refresh(ctx context.Context) error {
	// Run refresh command via m.runner
}
```

### `pm.Cleaner`
If the manager supports pruning cached downloads or orphaned packages (used by `pw cleanup`):
```go
func (m *Manager) Cleanup(ctx context.Context, out io.Writer, errOut io.Writer) error {
	// Run cleanup command via m.runner directing output to out/errOut
}
```

### `pm.BulkUpgrader` / `pm.DetailedBulkUpgrader`
If the manager can upgrade multiple packages in a single command invocation (improves performance significantly):
```go
func (m *Manager) UpgradePackages(ctx context.Context, ids []string) error {
	// Execute batched upgrade
}
```

### `pm.Deferrer`
If certain packages (such as shells or terminal emulators) should only be upgraded at the very end of the upgrade plan:
```go
func (m *Manager) ShouldDefer(id string) bool {
	return id == "my-shell"
}
```

### `pm.TimeoutProvider`
If querying this manager's updates typically requires a longer or shorter timeout than the default 2 minutes:
```go
func (m *Manager) FetchTimeout() time.Duration {
	return 3 * time.Minute
}
```

---

## Step 4: Separate Pure Output Parsing

Never mix subprocess execution with output parsing. Isolate raw output parsing into pure functions:

```go
func parseOutdatedOutput(raw string) ([]pm.PackageUpdate, error) {
	// Parse stdout lines or JSON payload into []pm.PackageUpdate
}
```

This ensures that edge cases (empty stdout, extra warning lines, non-standard versions) can be thoroughly tested using golden fixtures without executing system commands.

---

## Step 5: Add Golden Fixture Tests

Create `internal/<manager>/golden_test.go` and add sample command outputs to `internal/<manager>/testdata/`.

### Pattern for Golden Tests:
```go
package <manager>

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update golden test files")

func TestGoldenFixtures(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "*.input"))
	if err != nil {
		t.Fatalf("failed to glob fixture input files: %v", err)
	}

	for _, inputFile := range files {
		testName := strings.TrimSuffix(filepath.Base(inputFile), ".input")
		t.Run(testName, func(t *testing.T) {
			data, err := os.ReadFile(inputFile)
			if err != nil {
				t.Fatalf("failed to read %s: %v", inputFile, err)
			}

			updates, err := parseOutdatedOutput(string(data))
			if err != nil {
				t.Fatalf("parsing error: %v", err)
			}

			goldenFile := filepath.Join("testdata", testName+".golden.json")
			actualJSON, err := json.MarshalIndent(updates, "", "  ")
			if err != nil {
				t.Fatalf("marshal error: %v", err)
			}

			if *updateGolden {
				if err := os.WriteFile(goldenFile, append(actualJSON, '\n'), 0644); err != nil {
					t.Fatalf("failed to update golden file: %v", err)
				}
				return
			}

			expectedJSON, err := os.ReadFile(goldenFile)
			if err != nil {
				t.Fatalf("missing golden file %s (run with -update to generate): %v", goldenFile, err)
			}

			if strings.TrimSpace(string(actualJSON)) != strings.TrimSpace(string(expectedJSON)) {
				t.Errorf("output does not match golden file %s\nDiff:\nActual:\n%s\nExpected:\n%s",
					goldenFile, actualJSON, expectedJSON)
			}
		})
	}
}
```

To initialize or regenerate golden fixture files:
```bash
go test ./internal/<manager> -update
```

---

## Step 6: Platform Gating and Registration

Register your new manager in the CLI registry:
- **Windows-only managers**: Register in `cmd/registry_windows.go` inside `defaultManagers()`.
- **Unix / Cross-Platform managers**: Register in `cmd/registry_unix.go` inside `defaultManagers()`.
- **Platform-Specific (e.g., Linux-only)**: Use `//go:build linux` files or conditionals based on `runtime.GOOS`.

---

## Step 7: Version Comparator Support

If the manager's ecosystem uses specialized version rules (e.g., Debian epoch/revision `1:2.3-4` or Alpine package revisions `-r1`), ensure compatibility in `internal/version/version.go` so that `version.IsUpgrade(ecosystem, current, available)` evaluates correctly.

---

## Step 8: Pre-Submission Checklist

- [ ] `gofmt -s -w .` passes.
- [ ] No direct `fmt.Print*` or `os.Stdout`/`os.Stderr` calls in `internal/<manager>/` (except in tests).
- [ ] Golden fixture tests pass with `go test -count=1 ./internal/<manager>`.
- [ ] Full project test suite passes with `go test -count=1 ./...`.
- [ ] Cross-platform compilation succeeds for Linux, macOS, and Windows.
- [ ] Documentation updated (`README.md`, `ARCHITECTURE.md`).
