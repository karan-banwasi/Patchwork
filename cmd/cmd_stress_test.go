package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

type cliStressManager struct {
	name        string
	available   bool
	timeout     time.Duration
	updates     []pm.PackageUpdate
	warnings    []error
	fetchErr    error
	upgradeErr  error
	upgradedIDs []string
}

func (m *cliStressManager) Name() string                         { return m.name }
func (m *cliStressManager) Aliases() []string                    { return nil }
func (m *cliStressManager) IsAvailable(ctx context.Context) bool { return m.available }
func (m *cliStressManager) FetchTimeout() time.Duration          { return m.timeout }
func (m *cliStressManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	return m.updates, m.warnings, m.fetchErr
}
func (m *cliStressManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return m.updates, m.fetchErr
}
func (m *cliStressManager) UpgradePackage(ctx context.Context, id string) error {
	m.upgradedIDs = append(m.upgradedIDs, id)
	return m.upgradeErr
}
func (m *cliStressManager) ValidateID(id string) error { return nil }

// 1. STRESS TEST: check CLI Timeout Flag Precedence
func TestStress_CheckCmd_TimeoutPrecedence(t *testing.T) {
	mgr := &cliStressManager{
		name:      "stresspm",
		available: true,
		timeout:   42 * time.Second,
		updates: []pm.PackageUpdate{
			{Id: "test.pkg", Name: "TestPkg", Version: "1.0", AvailableVersion: "2.0", ManagerName: "stresspm"},
		},
	}
	reg := pm.NewRegistry(mgr)
	registryOverride = reg
	t.Cleanup(func() { registryOverride = nil })

	// Case A: No --timeout flag passed -> reg.FetchTimeout remains 0, ManagerTimeout returns 42s
	cmdDefault := NewRootCmd()
	var stdoutA, stderrA bytes.Buffer
	cmdDefault.SetOut(&stdoutA)
	cmdDefault.SetErr(&stderrA)
	cmdDefault.SetArgs([]string{"check", "--no-refresh"})
	if err := cmdDefault.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.FetchTimeout != 0 {
		t.Errorf("expected registry.FetchTimeout to remain 0, got %v", reg.FetchTimeout)
	}
	if d := reg.ManagerTimeout(mgr); d != 42*time.Second {
		t.Errorf("expected manager timeout 42s honored, got %v", d)
	}

	// Case B: Explicit --timeout flag passed -> reg.FetchTimeout set, overrides manager timeout
	cmdOverride := NewRootCmd()
	var stdoutB, stderrB bytes.Buffer
	cmdOverride.SetOut(&stdoutB)
	cmdOverride.SetErr(&stderrB)
	cmdOverride.SetArgs([]string{"check", "--no-refresh", "--timeout", "7s"})
	if err := cmdOverride.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.FetchTimeout != 7*time.Second {
		t.Errorf("expected registry.FetchTimeout to be 7s, got %v", reg.FetchTimeout)
	}
	if d := reg.ManagerTimeout(mgr); d != 7*time.Second {
		t.Errorf("expected manager timeout overridden to 7s, got %v", d)
	}
}

// 2. STRESS TEST: check Output Separation (Stdout vs Stderr) Across Formats
func TestStress_CheckCmd_OutputSeparation(t *testing.T) {
	mgr := &cliStressManager{
		name:      "warnpm",
		available: true,
		updates: []pm.PackageUpdate{
			{Id: "pkg1", Name: "Package 1", Version: "1.0", AvailableVersion: "2.0", ManagerName: "warnpm"},
		},
		warnings: []error{fmt.Errorf("partial registry rate limit")},
	}
	reg := pm.NewRegistry(mgr)
	registryOverride = reg
	t.Cleanup(func() { registryOverride = nil })

	// 1. JSON mode: Stdout must be valid parseable JSON, Stderr must contain warning
	{
		var stdout, stderr bytes.Buffer
		cmd := NewRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"check", "--json", "--no-refresh"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error in json mode: %v", err)
		}

		if !strings.Contains(stderr.String(), "Warning: partial registry rate limit") {
			t.Errorf("expected warning on stderr, got: %q", stderr.String())
		}

		var jsonParsed struct {
			Updates []struct {
				ID string `json:"id"`
			} `json:"updates"`
			Count int `json:"count"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &jsonParsed); err != nil {
			t.Fatalf("stdout is not valid JSON: %v\nOutput: %s", err, stdout.String())
		}
		if jsonParsed.Count != 1 || jsonParsed.Updates[0].ID != "pkg1" {
			t.Errorf("unexpected json contents: %+v", jsonParsed)
		}
	}

	// 2. Quiet mode: Stdout must contain TSV, Stderr must contain warning
	{
		var stdout, stderr bytes.Buffer
		cmd := NewRootCmd()
		cmd.SetOut(&stdout)
		cmd.SetErr(&stderr)
		cmd.SetArgs([]string{"check", "-q", "--no-refresh"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("unexpected error in quiet mode: %v", err)
		}

		if !strings.Contains(stderr.String(), "Warning: partial registry rate limit") {
			t.Errorf("expected warning on stderr in quiet mode, got: %q", stderr.String())
		}
		if !strings.Contains(stdout.String(), "pkg1\t1.0\t2.0\twarnpm") {
			t.Errorf("expected TSV on stdout in quiet mode, got: %q", stdout.String())
		}
	}
}

// 3. STRESS TEST: upgrade CLI Timeout Flag Precedence
func TestStress_UpgradeCmd_TimeoutPrecedence(t *testing.T) {
	mgr := &cliStressManager{
		name:      "upgradepm",
		available: true,
		timeout:   55 * time.Second,
		updates: []pm.PackageUpdate{
			{Id: "test.pkg", Name: "TestPkg", Version: "1.0", AvailableVersion: "2.0", ManagerName: "upgradepm"},
		},
	}
	reg := pm.NewRegistry(mgr)
	registryOverride = reg
	t.Cleanup(func() { registryOverride = nil })

	// Case A: No --timeout flag passed -> reg.FetchTimeout remains 0, manager timeout 55s
	cmdDefault := NewRootCmd()
	var stdoutA, stderrA bytes.Buffer
	cmdDefault.SetOut(&stdoutA)
	cmdDefault.SetErr(&stderrA)
	cmdDefault.SetArgs([]string{"upgrade", "--all", "--dry-run", "--no-refresh"})
	if err := cmdDefault.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.FetchTimeout != 0 {
		t.Errorf("expected registry.FetchTimeout to remain 0, got %v", reg.FetchTimeout)
	}
	if d := reg.ManagerTimeout(mgr); d != 55*time.Second {
		t.Errorf("expected manager timeout 55s honored, got %v", d)
	}

	// Case B: Explicit --timeout flag passed -> reg.FetchTimeout set, overrides manager timeout
	cmdOverride := NewRootCmd()
	var stdoutB, stderrB bytes.Buffer
	cmdOverride.SetOut(&stdoutB)
	cmdOverride.SetErr(&stderrB)
	cmdOverride.SetArgs([]string{"upgrade", "--all", "--dry-run", "--no-refresh", "--timeout", "12s"})
	if err := cmdOverride.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reg.FetchTimeout != 12*time.Second {
		t.Errorf("expected registry.FetchTimeout to be 12s, got %v", reg.FetchTimeout)
	}
	if d := reg.ManagerTimeout(mgr); d != 12*time.Second {
		t.Errorf("expected manager timeout overridden to 12s, got %v", d)
	}
}

// 4. STRESS TEST: upgrade Pipeline Failure Propagation & Exit Error
func TestStress_UpgradeCmd_FailurePropagation(t *testing.T) {
	mgr := &cliStressManager{
		name:       "failpm",
		available:  true,
		upgradeErr: fmt.Errorf("installer exit code 1603"),
		updates: []pm.PackageUpdate{
			{Id: "failing.pkg", Name: "FailPkg", Version: "1.0", AvailableVersion: "2.0", ManagerName: "failpm"},
		},
	}
	reg := pm.NewRegistry(mgr)
	registryOverride = reg
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"upgrade", "--all", "--no-refresh"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error from upgrade command when package fails, got nil")
	}
	if !strings.Contains(err.Error(), "one or more package upgrades failed") {
		t.Errorf("unexpected error message: %v", err)
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "=== Upgrade Summary ===") {
		t.Errorf("expected summary table on stdout, got: %s", outStr)
	}
	if !strings.Contains(outStr, "Failed: 1") {
		t.Errorf("expected Failed: 1 in summary, got: %s", outStr)
	}

	errStr := stderr.String()
	if !strings.Contains(errStr, "installer exit code 1603") {
		t.Errorf("expected manager error details on stderr, got: %s", errStr)
	}
}

// 5. STRESS TEST: upgrade All Pinned Packages
func TestStress_UpgradeCmd_AllPinned(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	t.Setenv("PATCHWORK_CONFIG", cfgPath)

	cfgData := `{"pinned_packages": ["pinpm:pkg.pinned"]}`
	if err := os.WriteFile(cfgPath, []byte(cfgData), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	mgr := &cliStressManager{
		name:      "pinpm",
		available: true,
		updates: []pm.PackageUpdate{
			{Id: "pkg.pinned", Name: "PinnedPkg", Version: "1.0", AvailableVersion: "2.0", ManagerName: "pinpm"},
		},
	}
	reg := pm.NewRegistry(mgr)
	registryOverride = reg
	t.Cleanup(func() { registryOverride = nil })

	// Case A: upgrade --all when only pinned package is present -> 0 exit code, prints skipped notice
	var stdoutA, stderrA bytes.Buffer
	cmdAll := NewRootCmd()
	cmdAll.SetOut(&stdoutA)
	cmdAll.SetErr(&stderrA)
	cmdAll.SetArgs([]string{"upgrade", "--all", "--no-refresh"})

	if err := cmdAll.Execute(); err != nil {
		t.Fatalf("expected upgrade --all to exit cleanly when all pinned, got: %v", err)
	}
	if !strings.Contains(stdoutA.String(), "No packages to upgrade (1 pinned package(s) skipped).") {
		t.Errorf("unexpected output: %s", stdoutA.String())
	}

	// Case B: upgrade <pinned-pkg> explicitly without --force -> returns error, renders summary
	var stdoutB, stderrB bytes.Buffer
	cmdExplicit := NewRootCmd()
	cmdExplicit.SetOut(&stdoutB)
	cmdExplicit.SetErr(&stderrB)
	cmdExplicit.SetArgs([]string{"upgrade", "pkg.pinned", "-m", "pinpm", "--no-refresh"})

	errExplicit := cmdExplicit.Execute()
	if errExplicit == nil {
		t.Fatalf("expected error when targeting pinned package explicitly, got nil")
	}
	if !strings.Contains(errExplicit.Error(), "is pinned (use --force or --include-pinned to upgrade)") {
		t.Errorf("unexpected error message: %v", errExplicit)
	}
	if !strings.Contains(stdoutB.String(), "=== Upgrade Summary ===") {
		t.Errorf("expected summary table for skipped package, got: %s", stdoutB.String())
	}
	if !strings.Contains(stdoutB.String(), "Skipped (pinned)") {
		t.Errorf("expected Skipped (pinned) in summary table, got: %s", stdoutB.String())
	}

	// Case C: upgrade <pinned-pkg> with --force -> executes upgrade!
	var stdoutC, stderrC bytes.Buffer
	cmdForced := NewRootCmd()
	cmdForced.SetOut(&stdoutC)
	cmdForced.SetErr(&stderrC)
	cmdForced.SetArgs([]string{"upgrade", "pkg.pinned", "-m", "pinpm", "--force", "--no-refresh"})

	if err := cmdForced.Execute(); err != nil {
		t.Fatalf("expected forced upgrade to succeed, got: %v", err)
	}
	if len(mgr.upgradedIDs) != 1 || mgr.upgradedIDs[0] != "pkg.pinned" {
		t.Errorf("expected pkg.pinned to be upgraded under --force, got: %v", mgr.upgradedIDs)
	}
}
