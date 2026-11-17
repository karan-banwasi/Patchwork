package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karan-banwasi/patchwork/internal/exitcodes"
	"github.com/karan-banwasi/patchwork/internal/history"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
)

type rollbackCall struct {
	id      string
	version string
}

type mockRollbackManager struct {
	mockUpgradeManager
	aliases       []string
	rollbackCalls []rollbackCall
	rollbackErrs  map[string]error
	unavailable   bool
}

func (m *mockRollbackManager) Aliases() []string { return m.aliases }

func (m *mockRollbackManager) IsAvailable(_ context.Context) bool {
	return !m.unavailable
}

func (m *mockRollbackManager) RollbackPackage(_ context.Context, id, targetVersion string) error {
	m.rollbackCalls = append(m.rollbackCalls, rollbackCall{id: id, version: targetVersion})
	if err, ok := m.rollbackErrs[id]; ok {
		return err
	}
	return nil
}

func TestUpgradeCmd_RecordsHistoryNonDryRunOnly(t *testing.T) {
	histFile := filepath.Join(t.TempDir(), "history.jsonl")
	t.Setenv("PATCHWORK_HISTORY", histFile)
	t.Setenv("PATCHWORK_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	mockMgr := &mockRollbackManager{
		mockUpgradeManager: mockUpgradeManager{
			name: "mockpm",
			updates: []pm.PackageUpdate{
				{Id: "pkg.one", Name: "PkgOne", Version: "1.0.0", AvailableVersion: "2.0.0", ManagerName: "mockpm"},
			},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	// 1. Dry-run must NOT record history
	var stdout, stderr bytes.Buffer
	dryCmd := NewRootCmd()
	dryCmd.SetOut(&stdout)
	dryCmd.SetErr(&stderr)
	dryCmd.SetArgs([]string{"upgrade", "--all", "--dry-run", "--no-refresh"})
	if err := dryCmd.Execute(); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	entries, err := history.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 history entries after dry-run, got %d", len(entries))
	}

	// 2. Real upgrade MUST record history
	stdout.Reset()
	stderr.Reset()
	upCmd := NewRootCmd()
	upCmd.SetOut(&stdout)
	upCmd.SetErr(&stderr)
	upCmd.SetArgs([]string{"upgrade", "--all", "-m", "mockpm", "--no-refresh"})
	if err := upCmd.Execute(); err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}

	entries, err = history.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(entries))
	}
	e := entries[0]
	if e.SchemaVersion != history.SchemaVersion || e.RunID == "" || e.ManagerFilter != "mockpm" {
		t.Errorf("unexpected entry metadata: %+v", e)
	}
	if e.Succeeded != 1 || len(e.Items) != 1 {
		t.Fatalf("unexpected entry items/counts: %+v", e)
	}
	item := e.Items[0]
	if item.ID != "pkg.one" || item.FromVersion != "1.0.0" || item.ToVersion != "2.0.0" || item.Status != string(upgrade.StatusSucceeded) {
		t.Errorf("unexpected recorded item: %+v", item)
	}
}

func TestHistoryCmd_ListFilterJsonAndClear(t *testing.T) {
	histFile := filepath.Join(t.TempDir(), "history.jsonl")
	t.Setenv("PATCHWORK_HISTORY", histFile)

	npmMgr := &mockRollbackManager{mockUpgradeManager: mockUpgradeManager{name: "npm"}}
	chocoMgr := &mockRollbackManager{mockUpgradeManager: mockUpgradeManager{name: "choco"}, aliases: []string{"chocolatey"}}
	registryOverride = pm.NewRegistry(npmMgr, chocoMgr)
	t.Cleanup(func() { registryOverride = nil })

	// Empty history
	var stdout bytes.Buffer
	emptyCmd := NewRootCmd()
	emptyCmd.SetOut(&stdout)
	emptyCmd.SetArgs([]string{"history"})
	if err := emptyCmd.Execute(); err != nil {
		t.Fatalf("empty history failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "No upgrade history found.") {
		t.Errorf("expected empty message, got: %s", stdout.String())
	}

	// Seed 2 entries
	_ = history.Append(history.Entry{
		RunID:     "run-1",
		Timestamp: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC),
		Items: []history.Item{
			{Name: "ts", ID: "typescript", Manager: "npm", FromVersion: "5.4.0", ToVersion: "5.5.0", Status: "Succeeded", DurationMs: 1100},
		},
		Succeeded: 1,
	})
	_ = history.Append(history.Entry{
		RunID:     "run-2",
		Timestamp: time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC),
		Items: []history.Item{
			{Name: "git", ID: "git.install", Manager: "choco", FromVersion: "2.44.0", ToVersion: "2.45.0", Status: "Succeeded", DurationMs: 2200},
			{Name: "eslint", ID: "eslint", Manager: "npm", FromVersion: "8.0.0", ToVersion: "9.0.0", Status: "Failed", Error: "boom"},
		},
		Succeeded: 1,
		Failed:    1,
	})

	// Table output with --limit 1 (newest first -> run-2)
	stdout.Reset()
	listCmd := NewRootCmd()
	listCmd.SetOut(&stdout)
	listCmd.SetArgs([]string{"history", "-n", "1"})
	if err := listCmd.Execute(); err != nil {
		t.Fatalf("history -n 1 failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Run run-2") || strings.Contains(stdout.String(), "Run run-1") {
		t.Errorf("expected only run-2 with -n 1, got:\n%s", stdout.String())
	}
	if !strings.Contains(stdout.String(), "2.44.0 -> 2.45.0") {
		t.Errorf("expected version transition in output, got:\n%s", stdout.String())
	}

	// JSON output filtered by manager alias (-m chocolatey)
	stdout.Reset()
	jsonCmd := NewRootCmd()
	jsonCmd.SetOut(&stdout)
	jsonCmd.SetArgs([]string{"history", "-m", "chocolatey", "--json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("history --json failed: %v", err)
	}
	var parsed historyJSONOutput
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("unmarshal history JSON failed: %v\n%s", err, stdout.String())
	}
	if parsed.SchemaVersion != "1.0.0" || parsed.Count != 1 || len(parsed.Entries) != 1 {
		t.Fatalf("unexpected JSON output: %+v", parsed)
	}
	if parsed.Entries[0].RunID != "run-2" || len(parsed.Entries[0].Items) != 1 || parsed.Entries[0].Items[0].ID != "git.install" {
		t.Errorf("unexpected filtered entry: %+v", parsed.Entries[0])
	}

	// Clear history
	stdout.Reset()
	clearCmd := NewRootCmd()
	clearCmd.SetOut(&stdout)
	clearCmd.SetArgs([]string{"history", "--clear"})
	if err := clearCmd.Execute(); err != nil {
		t.Fatalf("history --clear failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Upgrade history cleared.") {
		t.Errorf("unexpected clear output: %s", stdout.String())
	}
	afterClear, _ := history.LoadAll()
	if len(afterClear) != 0 {
		t.Errorf("expected 0 entries after --clear, got %d", len(afterClear))
	}
}

func TestRollbackCmd_ByPackageAndToOverride(t *testing.T) {
	histFile := filepath.Join(t.TempDir(), "history.jsonl")
	t.Setenv("PATCHWORK_HISTORY", histFile)

	npmMgr := &mockRollbackManager{mockUpgradeManager: mockUpgradeManager{name: "npm"}}
	registryOverride = pm.NewRegistry(npmMgr)
	t.Cleanup(func() { registryOverride = nil })

	_ = history.Append(history.Entry{
		RunID:     "run-npm",
		Timestamp: time.Now().UTC(),
		Items: []history.Item{
			{Name: "typescript", ID: "typescript", Manager: "npm", FromVersion: "5.4.5", ToVersion: "5.5.0", Status: "Succeeded"},
		},
		Succeeded: 1,
	})

	// Rollback from history FromVersion
	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"rollback", "typescript"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	if len(npmMgr.rollbackCalls) != 1 || npmMgr.rollbackCalls[0] != (rollbackCall{id: "typescript", version: "5.4.5"}) {
		t.Errorf("unexpected rollback calls: %+v", npmMgr.rollbackCalls)
	}

	// Rollback with explicit --to version and -m (even without history)
	npmMgr.rollbackCalls = nil
	stdout.Reset()
	cmd2 := NewRootCmd()
	cmd2.SetOut(&stdout)
	cmd2.SetErr(&stderr)
	cmd2.SetArgs([]string{"rollback", "eslint", "-m", "npm", "--to", "8.57.0", "--json"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("rollback --to failed: %v", err)
	}
	if len(npmMgr.rollbackCalls) != 1 || npmMgr.rollbackCalls[0] != (rollbackCall{id: "eslint", version: "8.57.0"}) {
		t.Errorf("unexpected rollback calls for --to: %+v", npmMgr.rollbackCalls)
	}
	var summary upgrade.JSONSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("unmarshal rollback JSON failed: %v\n%s", err, stdout.String())
	}
	if summary.SchemaVersion != "1.0.0" || summary.Succeeded != 1 || summary.DryRun {
		t.Errorf("unexpected rollback JSON summary: %+v", summary)
	}
}

func TestRollbackCmd_LastRunAndDryRun(t *testing.T) {
	histFile := filepath.Join(t.TempDir(), "history.jsonl")
	t.Setenv("PATCHWORK_HISTORY", histFile)

	npmMgr := &mockRollbackManager{mockUpgradeManager: mockUpgradeManager{name: "npm"}}
	cargoMgr := &mockRollbackManager{mockUpgradeManager: mockUpgradeManager{name: "cargo"}}
	registryOverride = pm.NewRegistry(npmMgr, cargoMgr)
	t.Cleanup(func() { registryOverride = nil })

	_ = history.Append(history.Entry{
		RunID:     "run-first",
		Timestamp: time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC),
		Items: []history.Item{
			{Name: "ripgrep", ID: "ripgrep", Manager: "cargo", FromVersion: "14.0.0", ToVersion: "14.1.0", Status: "Succeeded"},
		},
		Succeeded: 1,
	})
	_ = history.Append(history.Entry{
		RunID:     "run-latest",
		Timestamp: time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC),
		Items: []history.Item{
			{Name: "typescript", ID: "typescript", Manager: "npm", FromVersion: "5.4.0", ToVersion: "5.5.0", Status: "Succeeded"},
			{Name: "eslint", ID: "eslint", Manager: "npm", FromVersion: "8.0.0", ToVersion: "9.0.0", Status: "Failed"},
			{Name: "bat", ID: "bat", Manager: "cargo", FromVersion: "0.23.0", ToVersion: "0.24.0", Status: "Succeeded"},
		},
		Succeeded: 2,
		Failed:    1,
	})

	// Dry run --last
	var stdout bytes.Buffer
	dryCmd := NewRootCmd()
	dryCmd.SetOut(&stdout)
	dryCmd.SetArgs([]string{"rollback", "--last", "--dry-run", "--json"})
	if err := dryCmd.Execute(); err != nil {
		t.Fatalf("rollback --last --dry-run failed: %v", err)
	}
	if len(npmMgr.rollbackCalls) != 0 || len(cargoMgr.rollbackCalls) != 0 {
		t.Errorf("expected no rollback calls during dry-run")
	}
	var drySummary upgrade.JSONSummary
	if err := json.Unmarshal(stdout.Bytes(), &drySummary); err != nil {
		t.Fatalf("unmarshal dry-run JSON failed: %v", err)
	}
	if !drySummary.DryRun || drySummary.Total != 2 {
		t.Errorf("expected dry_run=true and total=2, got %+v", drySummary)
	}

	// Real --last rolls back both succeeded items (not the failed eslint)
	stdout.Reset()
	lastCmd := NewRootCmd()
	lastCmd.SetOut(&stdout)
	lastCmd.SetArgs([]string{"rollback", "--last"})
	if err := lastCmd.Execute(); err != nil {
		t.Fatalf("rollback --last failed: %v", err)
	}
	if len(npmMgr.rollbackCalls) != 1 || npmMgr.rollbackCalls[0].id != "typescript" {
		t.Errorf("unexpected npm rollback calls: %+v", npmMgr.rollbackCalls)
	}
	if len(cargoMgr.rollbackCalls) != 1 || cargoMgr.rollbackCalls[0].id != "bat" {
		t.Errorf("unexpected cargo rollback calls: %+v", cargoMgr.rollbackCalls)
	}

	// --run run-first
	cargoMgr.rollbackCalls = nil
	stdout.Reset()
	runCmd := NewRootCmd()
	runCmd.SetOut(&stdout)
	runCmd.SetArgs([]string{"rollback", "--run", "run-first"})
	if err := runCmd.Execute(); err != nil {
		t.Fatalf("rollback --run failed: %v", err)
	}
	if len(cargoMgr.rollbackCalls) != 1 || cargoMgr.rollbackCalls[0] != (rollbackCall{id: "ripgrep", version: "14.0.0"}) {
		t.Errorf("unexpected cargo rollback calls for --run: %+v", cargoMgr.rollbackCalls)
	}
}

func TestRollbackCmd_ErrorCases(t *testing.T) {
	histFile := filepath.Join(t.TempDir(), "history.jsonl")
	t.Setenv("PATCHWORK_HISTORY", histFile)

	// mockUpgradeManager does NOT implement pm.Rollbackable
	unsupportedMgr := &mockUpgradeManager{name: "brew"}
	failMgr := &mockRollbackManager{
		mockUpgradeManager: mockUpgradeManager{name: "npm"},
		rollbackErrs:       map[string]error{"badpkg": errors.New("install failed")},
	}
	registryOverride = pm.NewRegistry(unsupportedMgr, failMgr)
	t.Cleanup(func() { registryOverride = nil })

	_ = history.Append(history.Entry{
		RunID:     "run-err",
		Timestamp: time.Now().UTC(),
		Items: []history.Item{
			{Name: "wget", ID: "wget", Manager: "brew", FromVersion: "1.21", ToVersion: "1.24", Status: "Succeeded"},
			{Name: "mystery", ID: "mystery", Manager: "npm", FromVersion: "", ToVersion: "2.0.0", Status: "Succeeded"},
			{Name: "badpkg", ID: "badpkg", Manager: "npm", FromVersion: "1.0.0", ToVersion: "2.0.0", Status: "Succeeded"},
		},
		Succeeded: 3,
	})

	// 1. Unsupported manager
	cmd1 := NewRootCmd()
	cmd1.SetArgs([]string{"rollback", "wget"})
	err := cmd1.Execute()
	if err == nil || !strings.Contains(err.Error(), "does not support rollback") {
		t.Errorf("expected unsupported rollback error, got: %v", err)
	}

	// 2. Unknown FromVersion without --to
	cmd2 := NewRootCmd()
	cmd2.SetArgs([]string{"rollback", "mystery"})
	err = cmd2.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown in history") {
		t.Errorf("expected unknown FromVersion error, got: %v", err)
	}

	// 3. Failed rollback execution returns ExitUpgradeFailure (2)
	var stdout, stderr bytes.Buffer
	cmd3 := NewRootCmd()
	cmd3.SetOut(&stdout)
	cmd3.SetErr(&stderr)
	cmd3.SetArgs([]string{"rollback", "badpkg"})
	err = cmd3.Execute()
	if err == nil {
		t.Fatalf("expected failure error, got nil")
	}
	if code := exitcodes.Determine(context.Background(), err); code != exitcodes.ExitUpgradeFailure {
		t.Errorf("expected exit code %d, got %d", exitcodes.ExitUpgradeFailure, code)
	}

	// 4. Flag validation errors
	flagErrCases := [][]string{
		{"rollback"},
		{"rollback", "--last", "--run", "run-err"},
		{"rollback", "badpkg", "--last"},
		{"rollback", "--last", "--to", "1.0.0"},
		{"rollback", "a", "b", "--to", "1.0.0"},
		{"rollback", "badpkg", "--to", "--flag-injection"},
	}
	for _, args := range flagErrCases {
		c := NewRootCmd()
		c.SetArgs(args)
		if err := c.Execute(); err == nil {
			t.Errorf("expected validation error for args %v, got nil", args)
		}
	}
}
