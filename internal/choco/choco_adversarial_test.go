package choco

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
)

type mockDetailedBulkManager struct {
	name         string
	results      []pm.BulkPackageResult
	err          error
	upgradeCalls int
}

func (m *mockDetailedBulkManager) Name() string                         { return m.name }
func (m *mockDetailedBulkManager) Aliases() []string                    { return nil }
func (m *mockDetailedBulkManager) IsAvailable(ctx context.Context) bool { return true }
func (m *mockDetailedBulkManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return nil, nil
}
func (m *mockDetailedBulkManager) UpgradePackage(ctx context.Context, id string) error {
	m.upgradeCalls++
	return m.err
}
func (m *mockDetailedBulkManager) ValidateID(id string) error { return nil }
func (m *mockDetailedBulkManager) UpgradePackagesDetailed(ctx context.Context, ids []string) ([]pm.BulkPackageResult, error) {
	m.upgradeCalls++
	if m.err != nil && len(m.results) == 0 {
		return nil, m.err
	}
	return m.results, m.err
}

func TestAdversarialChocoBulkScenarios(t *testing.T) {
	t.Run("mixed success failure negative exit code and case insensitivity", func(t *testing.T) {
		output := `Chocolatey v2.4.0
Upgrading the following packages:
curl;Git.Install;7Zip.Install;vscode

The upgrade of CURL was successful.
git.install v2.43.0 was installed successfully.
The upgrade of 7zip.install was NOT successful.
7zip.install not installed. An error occurred during installation.

Failures
 - 7Zip.Install (exited 1603) - Error 1603 during installation
 - VSCODE (exited -1073741819) - Access violation crash
 - OtherPkg - Unspecified error without exit code

Chocolatey upgraded 2/4 packages. 2 packages failed.`

		ids := []string{"curl", "git.install", "7zip.install", "vscode", "mysterypkg"}
		results, ok := ParseChocoUpgradeOutput(output, ids)
		if !ok {
			t.Fatalf("expected output to be parsed")
		}
		if len(results) != len(ids) {
			t.Fatalf("expected %d results, got %d", len(ids), len(results))
		}

		resMap := make(map[string]pm.BulkPackageResult)
		for _, r := range results {
			resMap[strings.ToLower(r.ID)] = r
		}

		// curl succeeded
		if !resMap["curl"].Succeeded {
			t.Errorf("expected curl succeeded")
		}
		// git.install succeeded (case insensitive match)
		if !resMap["git.install"].Succeeded {
			t.Errorf("expected git.install succeeded")
		}
		// 7zip.install failed with exited 1603
		r7zip := resMap["7zip.install"]
		if r7zip.Succeeded || r7zip.Error == nil || !strings.Contains(r7zip.Error.Error(), "1603") {
			t.Errorf("expected 7zip.install failed with exit code 1603, got: %+v", r7zip)
		}
		// vscode failed with negative exit code
		rvscode := resMap["vscode"]
		if rvscode.Succeeded || rvscode.Error == nil || !strings.Contains(rvscode.Error.Error(), "-1073741819") {
			t.Errorf("expected vscode failed with negative exit code, got: %+v", rvscode)
		}
		// mysterypkg (unmentioned while failures exist) marked inconclusive
		rmystery := resMap["mysterypkg"]
		if rmystery.Succeeded || rmystery.Error == nil || !strings.Contains(rmystery.Error.Error(), "inconclusive") {
			t.Errorf("expected mysterypkg inconclusive, got: %+v", rmystery)
		}
	})

	t.Run("detailed bulk upgrader via executor handles mixed results", func(t *testing.T) {
		output := `Chocolatey v2.4.0
The upgrade of curl was successful.
Failures
 - git (exited 1) - Git upgrade failed
`
		parsedResults, ok := ParseChocoUpgradeOutput(output, []string{"curl", "git"})
		if !ok || len(parsedResults) != 2 {
			t.Fatalf("expected ParseChocoUpgradeOutput to return 2 results, got ok=%v, len=%d", ok, len(parsedResults))
		}

		// Test execution through upgrade.Executor using a DetailedBulkUpgrader
		mgr := &mockDetailedBulkManager{
			name:    "choco",
			results: parsedResults,
		}
		reg := pm.NewRegistry(mgr)
		var outBuf, errBuf bytes.Buffer
		executor := upgrade.NewExecutor(reg, &outBuf, &errBuf)

		pkgs := []pm.PackageUpdate{
			{Id: "curl", Name: "curl", ManagerName: "choco"},
			{Id: "git", Name: "git", ManagerName: "choco"},
		}

		plan := upgrade.NewPlan(reg, pkgs, nil)
		summary := executor.Execute(context.Background(), plan)

		if summary.Total != 2 {
			t.Errorf("expected total 2, got %d", summary.Total)
		}
		if summary.Succeeded != 1 {
			t.Errorf("expected succeeded 1, got %d", summary.Succeeded)
		}
		if summary.Failed != 1 {
			t.Errorf("expected failed 1, got %d", summary.Failed)
		}

		var curlResult, gitResult upgrade.Result
		for _, r := range summary.Results {
			if r.Package.Id == "curl" {
				curlResult = r
			} else if r.Package.Id == "git" {
				gitResult = r
			}
		}

		if curlResult.Status != upgrade.StatusSucceeded {
			t.Errorf("expected curl status succeeded, got %s", curlResult.Status)
		}
		if gitResult.Status != upgrade.StatusFailed || gitResult.Error == nil {
			t.Errorf("expected git status failed, got %s, err: %v", gitResult.Status, gitResult.Error)
		}
	})

	t.Run("executor handles bulk failure when upgrader returns error", func(t *testing.T) {
		mgr := &mockDetailedBulkManager{
			name: "choco",
			err:  errors.New("choco upgrade failed: exit status 255"),
		}
		reg := pm.NewRegistry(mgr)
		var outBuf, errBuf bytes.Buffer
		executor := upgrade.NewExecutor(reg, &outBuf, &errBuf)

		pkgs := []pm.PackageUpdate{
			{Id: "curl", Name: "curl", ManagerName: "choco"},
			{Id: "git", Name: "git", ManagerName: "choco"},
		}

		plan := upgrade.NewPlan(reg, pkgs, nil)
		summary := executor.Execute(context.Background(), plan)

		if summary.Total != 2 {
			t.Errorf("expected total 2, got %d", summary.Total)
		}
		if summary.Failed != 2 {
			t.Errorf("expected both packages failed, got %d failed", summary.Failed)
		}
	})
}
