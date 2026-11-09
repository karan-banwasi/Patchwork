package upgrade

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

type mockUpgradeManager struct {
	name      string
	available bool
	upgraded  []string
	bulk      []string
	err       error
	isBulk    bool
	deferIDs  map[string]bool
}

func (m *mockUpgradeManager) Name() string                         { return m.name }
func (m *mockUpgradeManager) Aliases() []string                    { return nil }
func (m *mockUpgradeManager) IsAvailable(ctx context.Context) bool { return m.available }
func (m *mockUpgradeManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return nil, nil
}
func (m *mockUpgradeManager) UpgradePackage(ctx context.Context, id string) error {
	m.upgraded = append(m.upgraded, id)
	return m.err
}
func (m *mockUpgradeManager) ValidateID(id string) error {
	if strings.Contains(id, "invalid") {
		return errors.New("invalid package id")
	}
	return nil
}
func (m *mockUpgradeManager) ShouldDefer(id string) bool {
	return m.deferIDs != nil && m.deferIDs[id]
}

type mockBulkUpgradeManager struct {
	mockUpgradeManager
}

func (m *mockBulkUpgradeManager) UpgradePackages(ctx context.Context, ids []string) error {
	m.bulk = append(m.bulk, ids...)
	return m.err
}

func TestOrderTargets(t *testing.T) {
	m := &mockUpgradeManager{
		name: "winget",
		deferIDs: map[string]bool{
			"Microsoft.PowerShell":      true,
			"Microsoft.WindowsTerminal": true,
		},
	}
	reg := pm.NewRegistry(m)

	input := []pm.PackageUpdate{
		{Id: "Microsoft.PowerShell", ManagerName: "winget"},
		{Id: "Git.Git", ManagerName: "winget"},
		{Id: "Microsoft.WindowsTerminal", ManagerName: "winget"},
		{Id: "Node.Node", ManagerName: "winget"},
	}

	ordered := OrderTargets(reg, input)
	expected := []string{
		"Git.Git",
		"Node.Node",
		"Microsoft.PowerShell",
		"Microsoft.WindowsTerminal",
	}

	if len(ordered) != len(expected) {
		t.Fatalf("expected %d packages, got %d", len(expected), len(ordered))
	}
	for i, id := range expected {
		if ordered[i].Id != id {
			t.Errorf("at index %d: expected %s, got %s", i, id, ordered[i].Id)
		}
	}

	// Nil registry test
	same := OrderTargets(nil, input)
	if len(same) != len(input) {
		t.Errorf("expected same length with nil registry")
	}
}

func TestFilterPins(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")
	t.Setenv("PATCHWORK_CONFIG", cfgFile)

	cfgContent := `{
		"pinned_packages": [
			"winget:Git.Git"
		]
	}`
	if err := os.WriteFile(cfgFile, []byte(cfgContent), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	pkgs := []pm.PackageUpdate{
		{Id: "Git.Git", ManagerName: "winget"},
		{Id: "Git.Git", ManagerName: "scoop"},
		{Id: "Node.Node", ManagerName: "winget"},
	}

	// Standard filtering
	allowed, skipped, err := FilterPins(pkgs, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allowed) != 2 {
		t.Errorf("expected 2 allowed packages, got %d", len(allowed))
	}
	if len(skipped) != 1 || skipped[0].Package.Id != "Git.Git" || skipped[0].Status != StatusSkippedPinned {
		t.Errorf("expected 1 skipped pinned package, got %+v", skipped)
	}

	// Ignore pins
	allowedForced, skippedForced, err := FilterPins(pkgs, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allowedForced) != 3 || len(skippedForced) != 0 {
		t.Errorf("expected all 3 allowed when ignorePins=true, got %d", len(allowedForced))
	}

	// Corrupt config fails closed
	_ = os.WriteFile(cfgFile, []byte("corrupt-json"), 0600)
	_, _, err = FilterPins(pkgs, false)
	if err == nil || !strings.Contains(err.Error(), "cannot verify pinned packages") {
		t.Errorf("expected corrupt config error, got: %v", err)
	}
}

func TestPlan_DryRunSummaryAndResults(t *testing.T) {
	m := &mockUpgradeManager{
		name: "winget",
		deferIDs: map[string]bool{
			"terminal": true,
		},
	}
	reg := pm.NewRegistry(m)

	targets := []pm.PackageUpdate{
		{Id: "terminal", ManagerName: "winget"},
		{Id: "git", ManagerName: "winget"},
	}
	skipped := []Result{
		{Package: pm.PackageUpdate{Id: "pinned-pkg", ManagerName: "winget"}, Status: StatusSkippedPinned},
	}

	plan := NewPlan(reg, targets, skipped)
	if plan.IsEmpty() {
		t.Errorf("expected plan not to be empty")
	}

	// Targets should be ordered
	if plan.Targets[0].Id != "git" || plan.Targets[1].Id != "terminal" {
		t.Errorf("expected targets to be ordered: %+v", plan.Targets)
	}

	dryRunRes := plan.DryRunResults()
	if len(dryRunRes) != 3 {
		t.Fatalf("expected 3 dry run results, got %d", len(dryRunRes))
	}
	if dryRunRes[0].Status != StatusSkippedPinned || dryRunRes[1].Status != StatusDryRun || dryRunRes[2].Status != StatusDryRun {
		t.Errorf("unexpected dry run statuses: %+v", dryRunRes)
	}

	summary := plan.DryRunSummary()
	if summary.Total != 3 || summary.Skipped != 3 || summary.Failed != 0 {
		t.Errorf("unexpected dry run summary: %+v", summary)
	}
	if summary.HasFailures() {
		t.Errorf("expected no failures in dry run summary")
	}

	var buf bytes.Buffer
	summary.Render(&buf)
	out := buf.String()
	if !strings.Contains(out, "=== Upgrade Summary ===") || !strings.Contains(out, "Dry-Run") || !strings.Contains(out, "Skipped (pinned)") {
		t.Errorf("unexpected rendered summary: %s", out)
	}
}

func TestExecutor_Execute(t *testing.T) {
	mChoco := &mockBulkUpgradeManager{
		mockUpgradeManager: mockUpgradeManager{name: "choco", available: true},
	}
	mWinget := &mockUpgradeManager{
		name:      "winget",
		available: true,
	}
	reg := pm.NewRegistry(mChoco, mWinget)

	var stdout, stderr bytes.Buffer
	executor := NewExecutor(reg, &stdout, &stderr)

	targets := []pm.PackageUpdate{
		{Id: "pkg1", ManagerName: "choco"},
		{Id: "pkg2", ManagerName: "choco"},        // batchable with pkg1
		{Id: "w1", ManagerName: "winget"},         // single upgrade
		{Id: "invalid-id", ManagerName: "winget"}, // validation error
		{Id: "missing", ManagerName: "unknown"},   // unknown manager
	}
	skipped := []Result{
		{Package: pm.PackageUpdate{Id: "skip1", ManagerName: "choco"}, Status: StatusSkippedPinned},
	}

	plan := &Plan{
		Targets: targets,
		Skipped: skipped,
	}

	summary := executor.Execute(context.Background(), plan)
	if summary.Total != 6 {
		t.Fatalf("expected 6 total results, got %d", summary.Total)
	}
	if summary.Succeeded != 3 { // pkg1, pkg2 (bulk), w1 (single)
		t.Errorf("expected 3 succeeded, got %d", summary.Succeeded)
	}
	if summary.Failed != 2 { // invalid-id, missing
		t.Errorf("expected 2 failed, got %d", summary.Failed)
	}
	if summary.Skipped != 1 { // skip1
		t.Errorf("expected 1 skipped, got %d", summary.Skipped)
	}
	if !summary.HasFailures() {
		t.Errorf("expected summary to have failures")
	}

	if len(mChoco.bulk) != 2 || mChoco.bulk[0] != "pkg1" || mChoco.bulk[1] != "pkg2" {
		t.Errorf("expected choco bulk upgrade with pkg1 and pkg2, got: %v", mChoco.bulk)
	}
	if len(mWinget.upgraded) != 1 || mWinget.upgraded[0] != "w1" {
		t.Errorf("expected winget upgrade with w1, got: %v", mWinget.upgraded)
	}
}

func TestExecutor_Execute_BulkFailure(t *testing.T) {
	mChoco := &mockBulkUpgradeManager{
		mockUpgradeManager: mockUpgradeManager{
			name:      "choco",
			available: true,
			err:       errors.New("bulk failure"),
		},
	}
	reg := pm.NewRegistry(mChoco)

	executor := NewExecutor(reg, nil, nil)
	plan := &Plan{
		Targets: []pm.PackageUpdate{
			{Id: "p1", ManagerName: "choco"},
			{Id: "p2", ManagerName: "choco"},
		},
	}

	summary := executor.Execute(context.Background(), plan)
	if summary.Failed != 2 || summary.Succeeded != 0 {
		t.Errorf("expected 2 failures on bulk error, got: %+v", summary)
	}
}

type mockDetailedBulkUpgradeManager struct {
	mockUpgradeManager
	detailedResults []pm.BulkPackageResult
	detailedErr     error
}

func (m *mockDetailedBulkUpgradeManager) UpgradePackagesDetailed(ctx context.Context, ids []string) ([]pm.BulkPackageResult, error) {
	m.bulk = append(m.bulk, ids...)
	return m.detailedResults, m.detailedErr
}

func TestExecutor_Execute_DetailedBulk(t *testing.T) {
	pkgErr := errors.New("install error 1603")
	mChoco := &mockDetailedBulkUpgradeManager{
		mockUpgradeManager: mockUpgradeManager{
			name:      "choco",
			available: true,
		},
		detailedResults: []pm.BulkPackageResult{
			{ID: "p1", Succeeded: true},
			{ID: "p2", Succeeded: false, Error: pkgErr},
		},
	}
	reg := pm.NewRegistry(mChoco)

	executor := NewExecutor(reg, nil, nil)
	plan := &Plan{
		Targets: []pm.PackageUpdate{
			{Id: "p1", ManagerName: "choco"},
			{Id: "p2", ManagerName: "choco"},
		},
	}

	summary := executor.Execute(context.Background(), plan)
	if summary.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", summary.Succeeded)
	}
	if summary.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", summary.Failed)
	}
	if len(summary.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(summary.Results))
	}
	if summary.Results[0].Status != StatusSucceeded {
		t.Errorf("expected p1 to succeed, got %s", summary.Results[0].Status)
	}
	if summary.Results[1].Status != StatusFailed || !errors.Is(summary.Results[1].Error, pkgErr) {
		t.Errorf("expected p2 to fail with %v, got %v (%s)", pkgErr, summary.Results[1].Error, summary.Results[1].Status)
	}
}
