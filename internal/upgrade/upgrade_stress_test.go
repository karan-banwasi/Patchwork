package upgrade_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
)

type stressUpgradeMgr struct {
	name      string
	available bool
	failIDs   map[string]bool
	bulkFail  bool
	deferIDs  map[string]bool
	upgraded  []string
	bulkCalls [][]string
}

func (m *stressUpgradeMgr) Name() string                         { return m.name }
func (m *stressUpgradeMgr) Aliases() []string                    { return nil }
func (m *stressUpgradeMgr) IsAvailable(ctx context.Context) bool { return m.available }
func (m *stressUpgradeMgr) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return nil, nil
}
func (m *stressUpgradeMgr) UpgradePackage(ctx context.Context, id string) error {
	m.upgraded = append(m.upgraded, id)
	if m.failIDs != nil && m.failIDs[id] {
		return fmt.Errorf("upgrade failed for %s", id)
	}
	return nil
}
func (m *stressUpgradeMgr) ValidateID(id string) error {
	if strings.Contains(id, "bad_syntax") {
		return fmt.Errorf("invalid ID syntax: %s", id)
	}
	return nil
}
func (m *stressUpgradeMgr) ShouldDefer(id string) bool {
	return m.deferIDs != nil && m.deferIDs[id]
}

type stressBulkUpgradeMgr struct {
	stressUpgradeMgr
}

func (m *stressBulkUpgradeMgr) UpgradePackages(ctx context.Context, ids []string) error {
	m.bulkCalls = append(m.bulkCalls, ids)
	if m.bulkFail {
		return fmt.Errorf("bulk upgrade batch failed")
	}
	m.upgraded = append(m.upgraded, ids...)
	return nil
}

// 1. STRESS TEST: Empty Plans
func TestStress_Pipeline_EmptyPlans(t *testing.T) {
	var nilPlan *upgrade.Plan
	if !nilPlan.IsEmpty() {
		t.Errorf("expected nilPlan.IsEmpty() to be true")
	}
	if dry := nilPlan.DryRunResults(); dry != nil {
		t.Errorf("expected nil DryRunResults for nilPlan, got: %v", dry)
	}

	emptyPlan := &upgrade.Plan{}
	if !emptyPlan.IsEmpty() {
		t.Errorf("expected emptyPlan.IsEmpty() to be true")
	}

	reg := pm.NewRegistry()
	var stdout, stderr bytes.Buffer
	executor := upgrade.NewExecutor(reg, &stdout, &stderr)

	// Execute with nil plan
	sumNil := executor.Execute(context.Background(), nilPlan)
	if sumNil.Total != 0 || sumNil.HasFailures() {
		t.Errorf("unexpected summary for nil plan: %+v", sumNil)
	}

	// Execute with empty plan
	sumEmpty := executor.Execute(context.Background(), emptyPlan)
	if sumEmpty.Total != 0 || sumEmpty.HasFailures() {
		t.Errorf("unexpected summary for empty plan: %+v", sumEmpty)
	}

	// Render empty summary - should not panic, should output nothing
	stdout.Reset()
	sumEmpty.Render(&stdout)
	if stdout.Len() != 0 {
		t.Errorf("expected empty output for empty summary render, got: %q", stdout.String())
	}
}

// 2. STRESS TEST: All Targets Skipped/Pinned
func TestStress_Pipeline_AllSkippedPinned(t *testing.T) {
	skipped := []upgrade.Result{
		{
			Package:  pm.PackageUpdate{Id: "pin1", ManagerName: "winget"},
			Status:   upgrade.StatusSkippedPinned,
			Duration: 0,
		},
		{
			Package:  pm.PackageUpdate{Id: "pin2", ManagerName: "choco"},
			Status:   upgrade.StatusSkippedPinned,
			Duration: 0,
		},
	}

	plan := &upgrade.Plan{
		Targets: nil,
		Skipped: skipped,
	}

	reg := pm.NewRegistry()
	var stdout, stderr bytes.Buffer
	executor := upgrade.NewExecutor(reg, &stdout, &stderr)

	summary := executor.Execute(context.Background(), plan)
	if summary.Total != 2 {
		t.Fatalf("expected total 2, got %d", summary.Total)
	}
	if summary.Skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", summary.Skipped)
	}
	if summary.Succeeded != 0 || summary.Failed != 0 {
		t.Errorf("expected 0 succeeded and 0 failed, got s=%d, f=%d", summary.Succeeded, summary.Failed)
	}
	if summary.HasFailures() {
		t.Errorf("expected HasFailures() to be false when all skipped")
	}

	// Render table
	summary.Render(&stdout)
	rendered := stdout.String()
	if !strings.Contains(rendered, "Total: 2 | Succeeded: 0 | Failed: 0 | Skipped: 2") {
		t.Errorf("expected correct summary footer, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Skipped (pinned)") {
		t.Errorf("expected Skipped (pinned) status, got: %s", rendered)
	}
}

// 3. STRESS TEST: Mixed Success / Failure Execution
func TestStress_Pipeline_MixedExecution(t *testing.T) {
	mgrBulkGood := &stressBulkUpgradeMgr{
		stressUpgradeMgr: stressUpgradeMgr{name: "bulk_good", available: true},
	}
	mgrBulkBad := &stressBulkUpgradeMgr{
		stressUpgradeMgr: stressUpgradeMgr{name: "bulk_bad", available: true, bulkFail: true},
	}
	mgrSingle := &stressUpgradeMgr{
		name:      "single_mgr",
		available: true,
		failIDs: map[string]bool{
			"fail_single": true,
		},
	}

	reg := pm.NewRegistry(mgrBulkGood, mgrBulkBad, mgrSingle)
	var stdout, stderr bytes.Buffer
	executor := upgrade.NewExecutor(reg, &stdout, &stderr)

	targets := []pm.PackageUpdate{
		// 2 consecutive batchable for bulk_good -> succeed (2)
		{Id: "good_1", ManagerName: "bulk_good"},
		{Id: "good_2", ManagerName: "bulk_good"},
		// 2 consecutive batchable for bulk_bad -> fail (2)
		{Id: "bad_1", ManagerName: "bulk_bad"},
		{Id: "bad_2", ManagerName: "bulk_bad"},
		// 1 for single_mgr -> succeed (1)
		{Id: "ok_single", ManagerName: "single_mgr"},
		// 1 for single_mgr -> fail (1)
		{Id: "fail_single", ManagerName: "single_mgr"},
		// 1 invalid syntax -> fail (1)
		{Id: "pkg_with_bad_syntax", ManagerName: "single_mgr"},
		// 1 unregistered manager -> fail (1)
		{Id: "ghost_pkg", ManagerName: "ghost_mgr"},
	}
	skipped := []upgrade.Result{
		{Package: pm.PackageUpdate{Id: "skip_1", ManagerName: "bulk_good"}, Status: upgrade.StatusSkippedPinned},
		{Package: pm.PackageUpdate{Id: "skip_2", ManagerName: "single_mgr"}, Status: upgrade.StatusSkippedPinned},
	}

	plan := &upgrade.Plan{
		Targets: targets,
		Skipped: skipped,
	}

	summary := executor.Execute(context.Background(), plan)

	// Expectations:
	// Total: 8 targets + 2 skipped = 10
	// Succeeded: 2 (bulk_good) + 1 (single ok) = 3
	// Failed: 2 (bulk_bad) + 1 (fail_single) + 1 (bad_syntax) + 1 (ghost_mgr) = 5
	// Skipped: 2
	if summary.Total != 10 {
		t.Errorf("expected 10 total, got %d", summary.Total)
	}
	if summary.Succeeded != 3 {
		t.Errorf("expected 3 succeeded, got %d", summary.Succeeded)
	}
	if summary.Failed != 5 {
		t.Errorf("expected 5 failed, got %d", summary.Failed)
	}
	if summary.Skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", summary.Skipped)
	}
	if !summary.HasFailures() {
		t.Errorf("expected HasFailures() == true")
	}

	// Verify error output captured failed packages
	errOut := stderr.String()
	for _, expectedErr := range []string{"bad_1", "bad_2", "fail_single", "pkg_with_bad_syntax", "ghost_mgr"} {
		if !strings.Contains(errOut, expectedErr) {
			t.Errorf("expected stderr to contain error for %q, got:\n%s", expectedErr, errOut)
		}
	}
}

// 4. STRESS TEST: Dry-Run Formatting & Unicode Resilience
func TestStress_Pipeline_DryRunUnicodeAndLongNames(t *testing.T) {
	targets := []pm.PackageUpdate{
		{Id: "普通话.软件", ManagerName: "winget", Version: "1.0", AvailableVersion: "2.0"},
		{Id: "🚀.turbo-pkg", ManagerName: "cargo", Version: "0.1", AvailableVersion: "0.2"},
		{Id: strings.Repeat("A", 120), ManagerName: "npm", Version: "1.0", AvailableVersion: "2.0"},
		{Id: "pkg-zero-dur", ManagerName: "scoop", Version: "3.0", AvailableVersion: "4.0"},
	}
	plan := upgrade.NewPlan(nil, targets, nil)

	summary := plan.DryRunSummary()
	if summary.Total != 4 {
		t.Fatalf("expected 4 total in dry run, got %d", summary.Total)
	}
	if summary.Skipped != 4 {
		t.Errorf("expected 4 in skipped count for dry run, got %d", summary.Skipped)
	}
	if summary.Failed != 0 || summary.Succeeded != 0 {
		t.Errorf("expected 0 failed and 0 succeeded in dry-run, got f=%d, s=%d", summary.Failed, summary.Succeeded)
	}

	var buf bytes.Buffer
	summary.Render(&buf)
	rendered := buf.String()

	// Ensure rendering doesn't crash on long runes or emojis
	if !strings.Contains(rendered, "普通话.软件") {
		t.Errorf("missing unicode package in output")
	}
	if !strings.Contains(rendered, "🚀.turbo-pkg") {
		t.Errorf("missing emoji package in output")
	}
	if !strings.Contains(rendered, strings.Repeat("A", 120)) {
		t.Errorf("missing long package ID in output")
	}
}

// 5. STRESS TEST: OrderTargets Stable Deferral
func TestStress_OrderTargets_Stability(t *testing.T) {
	mgr := &stressUpgradeMgr{
		name:      "mgr",
		available: true,
		deferIDs: map[string]bool{
			"defer_1": true,
			"defer_2": true,
		},
	}
	reg := pm.NewRegistry(mgr)

	pkgs := []pm.PackageUpdate{
		{Id: "norm_1", ManagerName: "mgr"},
		{Id: "defer_1", ManagerName: "mgr"},
		{Id: "norm_2", ManagerName: "mgr"},
		{Id: "defer_2", ManagerName: "mgr"},
		{Id: "norm_3", ManagerName: "mgr"},
	}

	ordered := upgrade.OrderTargets(reg, pkgs)
	expectedIDs := []string{"norm_1", "norm_2", "norm_3", "defer_1", "defer_2"}

	if len(ordered) != len(expectedIDs) {
		t.Fatalf("expected %d packages, got %d", len(expectedIDs), len(ordered))
	}
	for i, exp := range expectedIDs {
		if ordered[i].Id != exp {
			t.Errorf("at index %d: expected %s, got %s", i, exp, ordered[i].Id)
		}
	}

	// Edge case: all deferred
	allDef := []pm.PackageUpdate{
		{Id: "defer_1", ManagerName: "mgr"},
		{Id: "defer_2", ManagerName: "mgr"},
	}
	orderedAllDef := upgrade.OrderTargets(reg, allDef)
	if orderedAllDef[0].Id != "defer_1" || orderedAllDef[1].Id != "defer_2" {
		t.Errorf("expected identical order when all deferred, got: %+v", orderedAllDef)
	}

	// Edge case: none deferred
	noDef := []pm.PackageUpdate{
		{Id: "norm_1", ManagerName: "mgr"},
		{Id: "norm_2", ManagerName: "mgr"},
	}
	orderedNoDef := upgrade.OrderTargets(reg, noDef)
	if orderedNoDef[0].Id != "norm_1" || orderedNoDef[1].Id != "norm_2" {
		t.Errorf("expected identical order when none deferred, got: %+v", orderedNoDef)
	}
}
