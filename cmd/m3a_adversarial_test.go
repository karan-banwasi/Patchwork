package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/config"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
)

// mockAdversarialManager implements pm.Manager, pm.Refresher, and pm.WarningFetcher
type mockAdversarialManager struct {
	name       string
	aliases    []string
	updates    []pm.PackageUpdate
	refreshErr error
	fetchWarns []error
	fetchErr   error
	upgrades   []string
}

func (m *mockAdversarialManager) Name() string                         { return m.name }
func (m *mockAdversarialManager) Aliases() []string                    { return m.aliases }
func (m *mockAdversarialManager) IsAvailable(ctx context.Context) bool { return true }
func (m *mockAdversarialManager) Refresh(ctx context.Context) error    { return m.refreshErr }
func (m *mockAdversarialManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return m.updates, m.fetchErr
}
func (m *mockAdversarialManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	return m.updates, m.fetchWarns, m.fetchErr
}
func (m *mockAdversarialManager) UpgradePackage(ctx context.Context, id string) error {
	m.upgrades = append(m.upgrades, id)
	return nil
}
func (m *mockAdversarialManager) ValidateID(id string) error { return nil }

type jsonCheckOutput struct {
	Updates []struct {
		Name             string `json:"name"`
		ID               string `json:"id"`
		Version          string `json:"version"`
		AvailableVersion string `json:"available_version"`
		Manager          string `json:"manager"`
	} `json:"updates"`
	Count  int `json:"count"`
	Pinned int `json:"pinned_skipped,omitempty"`
}

// TestAdversarial_CheckJson_WarningsDoNotContaminateStdout verifies that when warnings are
// emitted during check (from refresh, warning fetchers, or partial manager failures),
// stdout contains ONLY valid, parseable JSON and stderr receives the warnings.
func TestAdversarial_CheckJson_WarningsDoNotContaminateStdout(t *testing.T) {
	mgr1 := &mockAdversarialManager{
		name: "mgr1",
		updates: []pm.PackageUpdate{
			{Id: "pkg.alpha", Name: "Alpha", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mgr1"},
			{Id: "pkg.beta", Name: "Beta", Version: "1.1", AvailableVersion: "1.2", ManagerName: "mgr1"},
		},
		refreshErr: errors.New("connection timeout to mirror"),
		fetchWarns: []error{errors.New("checksum warning on beta metadata")},
	}
	mgr2 := &mockAdversarialManager{
		name:     "mgr2",
		fetchErr: errors.New("mgr2 server returned 503 Service Unavailable"),
	}

	reg := pm.NewRegistry(mgr1, mgr2)
	registryOverride = reg
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"check", "--json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected check --json to succeed with partial warnings, got: %v", err)
	}

	outBytes := stdout.Bytes()
	if len(outBytes) == 0 {
		t.Fatalf("expected non-empty JSON stdout")
	}

	// Verify stdout is pure JSON without contamination
	var parsed jsonCheckOutput
	dec := json.NewDecoder(bytes.NewReader(outBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&parsed); err != nil {
		t.Fatalf("stdout failed strict JSON unmarshaling: %v\nRaw stdout:\n%s", err, stdout.String())
	}

	// Verify no remaining trailing data in stdout after JSON
	if dec.More() {
		t.Errorf("unexpected trailing data in stdout after JSON payload")
	}

	outStr := stdout.String()
	if strings.Contains(outStr, "Warning:") {
		t.Errorf("stdout was contaminated with Warning text:\n%s", outStr)
	}
	if strings.Contains(outStr, "Checking for available updates") {
		t.Errorf("stdout was contaminated with status progress text:\n%s", outStr)
	}

	// Verify stderr received the warnings
	errStr := stderr.String()
	if !strings.Contains(errStr, "Warning: [mgr1] refresh failed: connection timeout to mirror") {
		t.Errorf("stderr missing mgr1 refresh warning. Stderr:\n%s", errStr)
	}
	if !strings.Contains(errStr, "Warning: checksum warning on beta metadata") {
		t.Errorf("stderr missing mgr1 fetch warning. Stderr:\n%s", errStr)
	}
	if !strings.Contains(errStr, "Warning: [mgr2] mgr2 server returned 503 Service Unavailable") {
		t.Errorf("stderr missing mgr2 partial failure warning. Stderr:\n%s", errStr)
	}

	// Verify parsed JSON contents
	if parsed.Count != 2 || len(parsed.Updates) != 2 {
		t.Errorf("expected 2 updates in JSON, got count=%d, len=%d", parsed.Count, len(parsed.Updates))
	}
}

// TestAdversarial_CheckJson_ZeroUpdatesWithWarnings verifies pure JSON when no updates exist
// but multiple warnings are emitted.
func TestAdversarial_CheckJson_ZeroUpdatesWithWarnings(t *testing.T) {
	mgr := &mockAdversarialManager{
		name:       "mgr",
		updates:    nil,
		refreshErr: errors.New("refresh failed"),
		fetchWarns: []error{errors.New("fetch warning")},
	}
	registryOverride = pm.NewRegistry(mgr)
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"check", "--json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected check to succeed, got: %v", err)
	}

	var parsed jsonCheckOutput
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nRaw stdout:\n%s", err, stdout.String())
	}
	if parsed.Count != 0 || len(parsed.Updates) != 0 {
		t.Errorf("expected 0 updates, got: %+v", parsed)
	}

	outStr := stdout.String()
	if strings.Contains(outStr, "No updates available") {
		t.Errorf("stdout contained human-readable text 'No updates available': %s", outStr)
	}

	errStr := stderr.String()
	if !strings.Contains(errStr, "Warning:") {
		t.Errorf("expected warnings in stderr, got: %s", errStr)
	}
}

// TestAdversarial_CheckJson_WithFilterAndPinned verifies pure JSON when filtering by manager
// and pinned packages are present.
func TestAdversarial_CheckJson_WithFilterAndPinned(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")
	t.Setenv("PATCHWORK_CONFIG", cfgFile)

	cfg := &config.Config{
		PinnedPackages: []string{"mgr:pkg.pinned"},
	}
	_ = cfg.Save()

	mgr := &mockAdversarialManager{
		name: "mgr",
		updates: []pm.PackageUpdate{
			{Id: "pkg.pinned", Name: "PinnedPkg", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mgr"},
			{Id: "pkg.active", Name: "ActivePkg", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mgr"},
		},
		refreshErr: errors.New("minor refresh issue"),
	}
	registryOverride = pm.NewRegistry(mgr)
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"check", "-m", "mgr", "--json"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("check failed: %v", err)
	}

	var parsed jsonCheckOutput
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\nRaw stdout:\n%s", err, stdout.String())
	}

	if parsed.Count != 1 || len(parsed.Updates) != 1 || parsed.Updates[0].ID != "pkg.active" {
		t.Errorf("unexpected updates in parsed JSON: %+v", parsed)
	}
	if parsed.Pinned != 1 {
		t.Errorf("expected pinned_skipped == 1, got %d", parsed.Pinned)
	}

	outStr := stdout.String()
	if strings.Contains(outStr, "Checking for available updates using mgr") {
		t.Errorf("stdout contaminated with manager check message: %s", outStr)
	}
	if strings.Contains(outStr, "Warning:") {
		t.Errorf("stdout contaminated with Warning: %s", outStr)
	}
}

type mockDeferrerManager struct {
	name      string
	deferIDs  map[string]bool
	upgrades  []string
	available bool
}

func (m *mockDeferrerManager) Name() string                         { return m.name }
func (m *mockDeferrerManager) Aliases() []string                    { return nil }
func (m *mockDeferrerManager) IsAvailable(ctx context.Context) bool { return true }
func (m *mockDeferrerManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return nil, nil
}
func (m *mockDeferrerManager) UpgradePackage(ctx context.Context, id string) error {
	m.upgrades = append(m.upgrades, id)
	return nil
}
func (m *mockDeferrerManager) ValidateID(id string) error { return nil }
func (m *mockDeferrerManager) ShouldDefer(id string) bool {
	return m.deferIDs != nil && m.deferIDs[strings.ToLower(id)]
}

// TestAdversarial_DeferrerOrdering_Exhaustive validates that all variants of shell and
// terminal packages are placed at the end while preserving the relative ordering of
// non-deferred and deferred targets.
func TestAdversarial_DeferrerOrdering_Exhaustive(t *testing.T) {
	deferIDs := map[string]bool{
		"microsoft.powershell":         true,
		"microsoft.powershell.preview": true,
		"microsoft.windowsterminal":    true,
		"pwsh":                         true,
		"windows-terminal":             true,
	}

	mgrWinget := &mockDeferrerManager{name: "winget", deferIDs: deferIDs}
	mgrChoco := &mockDeferrerManager{name: "choco", deferIDs: nil} // choco does not defer
	reg := pm.NewRegistry(mgrWinget, mgrChoco)

	cases := []struct {
		name     string
		input    []pm.PackageUpdate
		expected []string
	}{
		{
			name: "interleaved winget deferred and non-deferred",
			input: []pm.PackageUpdate{
				{Id: "Microsoft.PowerShell", ManagerName: "winget"},
				{Id: "Git.Git", ManagerName: "winget"},
				{Id: "Microsoft.WindowsTerminal", ManagerName: "winget"},
				{Id: "7zip.7zip", ManagerName: "winget"},
				{Id: "pwsh", ManagerName: "winget"},
				{Id: "Node.Node", ManagerName: "winget"},
			},
			expected: []string{
				"Git.Git",
				"7zip.7zip",
				"Node.Node",
				"Microsoft.PowerShell",
				"Microsoft.WindowsTerminal",
				"pwsh",
			},
		},
		{
			name: "cross-manager: choco non-deferring even with powershell id",
			input: []pm.PackageUpdate{
				{Id: "powershell", ManagerName: "choco"},
				{Id: "Microsoft.PowerShell", ManagerName: "winget"},
				{Id: "curl", ManagerName: "choco"},
				{Id: "Microsoft.WindowsTerminal", ManagerName: "winget"},
			},
			expected: []string{
				"powershell", // choco does not implement Deferrer or has no deferIDs
				"curl",
				"Microsoft.PowerShell",
				"Microsoft.WindowsTerminal",
			},
		},
		{
			name: "already ordered input",
			input: []pm.PackageUpdate{
				{Id: "Git.Git", ManagerName: "winget"},
				{Id: "Microsoft.PowerShell", ManagerName: "winget"},
			},
			expected: []string{
				"Git.Git",
				"Microsoft.PowerShell",
			},
		},
		{
			name: "all deferred input preserves stability",
			input: []pm.PackageUpdate{
				{Id: "Microsoft.PowerShell", ManagerName: "winget"},
				{Id: "Microsoft.WindowsTerminal", ManagerName: "winget"},
				{Id: "pwsh", ManagerName: "winget"},
			},
			expected: []string{
				"Microsoft.PowerShell",
				"Microsoft.WindowsTerminal",
				"pwsh",
			},
		},
		{
			name: "no deferred input preserves stability",
			input: []pm.PackageUpdate{
				{Id: "pkg3", ManagerName: "winget"},
				{Id: "pkg1", ManagerName: "winget"},
				{Id: "pkg2", ManagerName: "winget"},
			},
			expected: []string{
				"pkg3",
				"pkg1",
				"pkg2",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := upgrade.NewPlan(reg, tc.input, nil)
			if len(plan.Targets) != len(tc.expected) {
				t.Fatalf("expected %d targets, got %d", len(tc.expected), len(plan.Targets))
			}
			for i, exp := range tc.expected {
				if plan.Targets[i].Id != exp {
					t.Errorf("at index %d: expected %q, got %q", i, exp, plan.Targets[i].Id)
				}
			}

			// Also verify that Executor runs in exactly this deferred sequence
			var stdout, stderr bytes.Buffer
			executor := upgrade.NewExecutor(reg, &stdout, &stderr)
			summary := executor.Execute(context.Background(), plan)
			if summary.HasFailures() {
				t.Fatalf("executor failed: %+v", summary)
			}
			// Verify execution order in winget
			wingetTargets := []string{}
			for _, exp := range tc.expected {
				for _, in := range tc.input {
					if in.Id == exp && in.ManagerName == "winget" {
						wingetTargets = append(wingetTargets, exp)
						break
					}
				}
			}
			if len(mgrWinget.upgrades) != len(wingetTargets) {
				t.Fatalf("expected %d winget upgrades, got %d: %v", len(wingetTargets), len(mgrWinget.upgrades), mgrWinget.upgrades)
			}
			for i, exp := range wingetTargets {
				if mgrWinget.upgrades[i] != exp {
					t.Errorf("executed winget upgrade index %d: expected %s, got %s", i, exp, mgrWinget.upgrades[i])
				}
			}
			mgrWinget.upgrades = nil
		})
	}
}

// TestAdversarial_UpgradeCmd_PlanPreservesDeferralInAllModes tests deferral in
// command-line explicit arguments and dry-run execution.
func TestAdversarial_UpgradeCmd_PlanPreservesDeferralInAllModes(t *testing.T) {
	deferIDs := map[string]bool{
		"microsoft.powershell":      true,
		"microsoft.windowsterminal": true,
	}
	mgr := &mockDeferrerManager{name: "winget", deferIDs: deferIDs}
	reg := pm.NewRegistry(mgr)
	registryOverride = reg
	t.Cleanup(func() { registryOverride = nil })

	// Explicit args with -m winget
	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"upgrade", "Microsoft.PowerShell", "Git.Git", "Microsoft.WindowsTerminal", "-m", "winget", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected dry-run upgrade to succeed, got: %v", err)
	}

	outStr := stdout.String()
	// Check order of "[Dry Run] Would upgrade package ..." in stdout
	idxGit := strings.Index(outStr, "Would upgrade package Git.Git")
	idxPS := strings.Index(outStr, "Would upgrade package Microsoft.PowerShell")
	idxWT := strings.Index(outStr, "Would upgrade package Microsoft.WindowsTerminal")

	if idxGit == -1 || idxPS == -1 || idxWT == -1 {
		t.Fatalf("expected all 3 packages in dry-run output:\n%s", outStr)
	}
	if !(idxGit < idxPS && idxPS < idxWT) {
		t.Errorf("expected Git.Git (%d) before PowerShell (%d) before WindowsTerminal (%d) in dry run output", idxGit, idxPS, idxWT)
	}
}
