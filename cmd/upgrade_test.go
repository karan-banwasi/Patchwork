package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/exitcodes"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
)

func TestUpgradeCmd_InteractiveWithMockSelector(t *testing.T) {
	mockMgr := &mockUpgradeManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "pkg.one", Name: "PkgOne", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
			{Id: "pkg.two", Name: "PkgTwo", Version: "2.0", AvailableVersion: "3.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	// Select only the second package (index 1: pkg.two)
	mockSel := &MockMultiSelector{SelectedIndices: []int{1}}
	SetDefaultSelectorForTest(mockSel)
	t.Cleanup(func() { SetDefaultSelectorForTest(nil) })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"upgrade", "--no-refresh"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockMgr.upgradedIDs) != 1 || mockMgr.upgradedIDs[0] != "pkg.two" {
		t.Errorf("expected only pkg.two upgraded, got: %v", mockMgr.upgradedIDs)
	}
}

func TestUpgradeCmd_InteractiveCanceled(t *testing.T) {
	mockMgr := &mockUpgradeManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "pkg.one", Name: "PkgOne", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	mockSel := &MockMultiSelector{Err: ErrSelectionCanceled}
	SetDefaultSelectorForTest(mockSel)
	t.Cleanup(func() { SetDefaultSelectorForTest(nil) })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"upgrade", "--no-refresh"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error on canceled selection, got nil")
	}
	exitCode := exitcodes.Determine(context.Background(), err)
	if exitCode != exitcodes.ExitCanceled {
		t.Errorf("expected exit code %d (ExitCanceled), got: %d", exitcodes.ExitCanceled, exitCode)
	}
	if !strings.Contains(stdout.String(), "Selection canceled.") {
		t.Errorf("expected cancellation message, got: %s", stdout.String())
	}
	if len(mockMgr.upgradedIDs) != 0 {
		t.Errorf("expected no packages upgraded, got: %v", mockMgr.upgradedIDs)
	}
}

func TestUpgradeCmd_JsonRequiresAllOrArgs(t *testing.T) {
	mockMgr := &mockUpgradeManager{name: "mockpm"}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"upgrade", "--json"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when --json used without --all or args, got nil")
	}
	if !strings.Contains(err.Error(), "--json requires either --all or explicit package IDs") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestUpgradeCmd_JsonDryRun(t *testing.T) {
	mockMgr := &mockUpgradeManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "pkg.one", Name: "PkgOne", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"upgrade", "--all", "--dry-run", "--json", "--no-refresh"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var summary upgrade.JSONSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("failed to unmarshal JSON output (%v):\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != "1.0.0" {
		t.Errorf("expected schema_version 1.0.0, got %q", summary.SchemaVersion)
	}
	if !summary.DryRun {
		t.Errorf("expected dry_run to be true in JSON output")
	}
	if len(summary.Results) != 1 || summary.Results[0].Package.ID != "pkg.one" {
		t.Errorf("unexpected results in JSON: %+v", summary.Results)
	}
}

func TestUpgradeCmd_JsonExecution(t *testing.T) {
	mockMgr := &mockUpgradeManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "pkg.one", Name: "PkgOne", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"upgrade", "--all", "--json", "--no-refresh"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var summary upgrade.JSONSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("failed to unmarshal JSON output (%v):\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != "1.0.0" {
		t.Errorf("expected schema_version 1.0.0, got %q", summary.SchemaVersion)
	}
	if summary.DryRun {
		t.Errorf("expected dry_run to be false in execution JSON output")
	}
	if summary.Total != 1 || summary.Succeeded != 1 {
		t.Errorf("unexpected summary counts: total=%d, succeeded=%d", summary.Total, summary.Succeeded)
	}
}

func TestUpgradeCmd_MutuallyExclusiveAllAndArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"upgrade", "--all", "Some.Package"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when combining --all and package argument, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestUpgradeCmd_UnknownManager(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"upgrade", "Some.Package", "-m", "unknown-mgr"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error for unknown manager, got nil")
	}
	if !strings.Contains(err.Error(), "unknown package manager") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestUpgradeCmd_MutuallyExclusiveByManagerAndArg(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"upgrade", "--by-manager", "Some.Package"})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected error when combining --by-manager and package argument, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestOrderSelectedPackages_DefersShellAndTerminal(t *testing.T) {
	input := []pm.PackageUpdate{
		{Id: "Microsoft.PowerShell", Name: "PowerShell", ManagerName: "winget"},
		{Id: "Git.Git", Name: "Git", ManagerName: "winget"},
		{Id: "Microsoft.WindowsTerminal", Name: "Windows Terminal", ManagerName: "winget"},
		{Id: "OpenJS.NodeJS.LTS", Name: "Node.js LTS", ManagerName: "winget"},
	}

	ordered := orderSelectedPackages(input)
	expectedOrder := []string{
		"Git.Git",
		"OpenJS.NodeJS.LTS",
		"Microsoft.PowerShell",
		"Microsoft.WindowsTerminal",
	}

	if len(ordered) != len(expectedOrder) {
		t.Fatalf("expected %d packages, got %d", len(expectedOrder), len(ordered))
	}
	for i, exp := range expectedOrder {
		if ordered[i].Id != exp {
			t.Errorf("at index %d: expected %q, got %q", i, exp, ordered[i].Id)
		}
	}
}

func TestSplitPinnedTargets_SkipsPinnedUnlessForced(t *testing.T) {
	tmpCfg := t.TempDir() + "/config.json"
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"pin", "winget:Git.Git"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("failed to pin: %v", err)
	}

	targets := []pm.PackageUpdate{
		{Id: "Git.Git", Name: "Git", ManagerName: "winget"},
		{Id: "Git.Git", Name: "Git", ManagerName: "scoop"},
	}

	allowed, skipped, err := splitPinnedTargets(targets, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allowed) != 1 || allowed[0].ManagerName != "scoop" {
		t.Errorf("expected only scoop Git.Git to be allowed, got: %+v", allowed)
	}
	if len(skipped) != 1 || skipped[0].status != "Skipped (pinned)" {
		t.Errorf("expected winget Git.Git to be Skipped (pinned), got: %+v", skipped)
	}

	allowedForced, skippedForced, err := splitPinnedTargets(targets, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allowedForced) != 2 || len(skippedForced) != 0 {
		t.Errorf("expected all targets allowed when ignorePins=true, got allowed=%d, skipped=%d", len(allowedForced), len(skippedForced))
	}
}

func TestSplitPinnedTargets_CorruptConfigFailsClosed(t *testing.T) {
	tmpCfg := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PATCHWORK_CONFIG", tmpCfg)

	if err := os.WriteFile(tmpCfg, []byte("{ bad-json: 123"), 0600); err != nil {
		t.Fatalf("failed to write corrupt config: %v", err)
	}

	targets := []pm.PackageUpdate{
		{Id: "Git.Git", Name: "Git", ManagerName: "winget"},
	}

	_, _, err := splitPinnedTargets(targets, false)
	if err == nil {
		t.Fatalf("expected error from splitPinnedTargets on corrupt config, got nil")
	}
	if !strings.Contains(err.Error(), "cannot verify pinned packages") {
		t.Errorf("expected fail-closed error message, got: %v", err)
	}

	// Forced override succeeds
	allowed, _, err := splitPinnedTargets(targets, true)
	if err != nil {
		t.Fatalf("expected forced splitPinnedTargets to succeed, got: %v", err)
	}
	if len(allowed) != 1 {
		t.Errorf("expected 1 allowed target, got %d", len(allowed))
	}
}

func TestUpgradeCmd_AcceptAgreementsNotice(t *testing.T) {
	var buf bytes.Buffer
	pkgs := []pm.PackageUpdate{
		{Id: "Git.Git", Name: "Git", ManagerName: "winget"},
	}

	// When acceptAgreements is true, prints notice
	maybePrintWingetAgreementsNotice(&buf, pkgs, true)
	if !strings.Contains(buf.String(), "Notice: Automatically accepting Winget package and source agreements") {
		t.Errorf("expected agreement notice, got: %s", buf.String())
	}

	// When acceptAgreements is false, does not print notice
	buf.Reset()
	maybePrintWingetAgreementsNotice(&buf, pkgs, false)
	if buf.Len() != 0 {
		t.Errorf("expected no notice when acceptAgreements=false, got: %s", buf.String())
	}

	// When no winget packages are present, does not print notice
	buf.Reset()
	nonWinget := []pm.PackageUpdate{
		{Id: "git", Name: "git", ManagerName: "brew"},
	}
	maybePrintWingetAgreementsNotice(&buf, nonWinget, true)
	if buf.Len() != 0 {
		t.Errorf("expected no notice for non-winget packages, got: %s", buf.String())
	}
}

func TestUpgradeCmd_DefaultTimeoutIsZero(t *testing.T) {
	cmd := newUpgradeCmd()
	f := cmd.Flags().Lookup("timeout")
	if f == nil {
		t.Fatalf("expected --timeout flag to exist")
	}
	if f.DefValue != "0s" && f.DefValue != "0" {
		t.Errorf("expected default timeout to be 0s, got: %s", f.DefValue)
	}
}

type mockUpgradeManager struct {
	name        string
	updates     []pm.PackageUpdate
	upgradedIDs []string
}

func (m *mockUpgradeManager) Name() string                         { return m.name }
func (m *mockUpgradeManager) Aliases() []string                    { return nil }
func (m *mockUpgradeManager) IsAvailable(ctx context.Context) bool { return true }
func (m *mockUpgradeManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return m.updates, nil
}
func (m *mockUpgradeManager) UpgradePackage(ctx context.Context, id string) error {
	m.upgradedIDs = append(m.upgradedIDs, id)
	return nil
}
func (m *mockUpgradeManager) ValidateID(id string) error { return nil }

func TestUpgradeCmd_DryRunAllPlan(t *testing.T) {
	mockMgr := &mockUpgradeManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "pkg.one", Name: "PkgOne", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
			{Id: "pkg.two", Name: "PkgTwo", Version: "2.0", AvailableVersion: "3.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"upgrade", "--all", "--dry-run", "--no-refresh"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected dry-run upgrade to succeed, got: %v", err)
	}

	outStr := stdout.String()
	if !strings.Contains(outStr, "[Dry Run] Would upgrade 2 package(s):") {
		t.Errorf("expected dry run header, got: %s", outStr)
	}
	if !strings.Contains(outStr, "=== Upgrade Summary ===") {
		t.Errorf("expected upgrade summary table, got: %s", outStr)
	}
	if !strings.Contains(outStr, "Dry-Run") {
		t.Errorf("expected Dry-Run status in summary table, got: %s", outStr)
	}

	if len(mockMgr.upgradedIDs) != 0 {
		t.Errorf("expected 0 packages upgraded during dry-run, got: %v", mockMgr.upgradedIDs)
	}
}
