package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/exitcodes"
	"github.com/karan-banwasi/patchwork/internal/pm"
	"github.com/karan-banwasi/patchwork/internal/upgrade"
)

type mockAdversarialUpgradeManager struct {
	name        string
	updates     []pm.PackageUpdate
	upgradedIDs []string
	failUpgrade bool
	refreshErr  error
	warnings    []error
}

func (m *mockAdversarialUpgradeManager) Name() string                         { return m.name }
func (m *mockAdversarialUpgradeManager) Aliases() []string                    { return nil }
func (m *mockAdversarialUpgradeManager) IsAvailable(ctx context.Context) bool { return true }
func (m *mockAdversarialUpgradeManager) Refresh(ctx context.Context) error    { return m.refreshErr }
func (m *mockAdversarialUpgradeManager) GetAvailableUpdates(ctx context.Context) ([]pm.PackageUpdate, error) {
	return m.updates, nil
}
func (m *mockAdversarialUpgradeManager) GetAvailableUpdatesWithWarnings(ctx context.Context) ([]pm.PackageUpdate, []error, error) {
	return m.updates, m.warnings, nil
}
func (m *mockAdversarialUpgradeManager) UpgradePackage(ctx context.Context, id string) error {
	m.upgradedIDs = append(m.upgradedIDs, id)
	if m.failUpgrade {
		return errors.New("installation failed: checksum mismatch")
	}
	return nil
}
func (m *mockAdversarialUpgradeManager) ValidateID(id string) error { return nil }

// TestAdversarial_UpgradeJson_ExplicitArgs checks whether `pw upgrade <package-id> --json`
// produces clean, valid JSON without banner contamination ("Locating 1 package(s)...").
func TestAdversarial_UpgradeJson_ExplicitArgs(t *testing.T) {
	mockMgr := &mockAdversarialUpgradeManager{
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
	cmd.SetArgs([]string{"upgrade", "pkg.one", "--json", "--dry-run", "--no-refresh"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error executing upgrade dry-run with explicit args: %v", err)
	}

	outStr := stdout.String()
	if strings.Contains(outStr, "Locating") {
		t.Errorf("FAIL: stdout contains human banner 'Locating ...':\n%s", outStr)
	}

	var summary upgrade.JSONSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("FAIL: stdout is not valid JSON (%v):\n%s", err, outStr)
	}
	if summary.SchemaVersion != "1.0.0" {
		t.Errorf("expected schema_version '1.0.0', got %q", summary.SchemaVersion)
	}
	if len(summary.Results) != 1 || summary.Results[0].Package.ID != "pkg.one" {
		t.Errorf("expected pkg.one in results, got: %+v", summary.Results)
	}
}

// TestAdversarial_UpgradeJson_ExplicitArgs_Pinned checks whether `pw upgrade <pinned-pkg> --json`
// emits valid JSON or leaks text table.
func TestAdversarial_UpgradeJson_ExplicitArgs_Pinned(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := tmpDir + "/config.json"
	t.Setenv("PATCHWORK_CONFIG", cfgFile)

	mockMgr := &mockAdversarialUpgradeManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "pkg.pinned", Name: "PkgPinned", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	// Pin the package
	pinCmd := NewRootCmd()
	pinCmd.SetArgs([]string{"pin", "mockpm:pkg.pinned"})
	if err := pinCmd.Execute(); err != nil {
		t.Fatalf("failed to pin package: %v", err)
	}

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"upgrade", "pkg.pinned", "--json", "--no-refresh"})

	err := cmd.Execute()
	outStr := stdout.String()
	t.Logf("Output from upgrade pinned --json:\n%s", outStr)
	if strings.Contains(outStr, "=== Upgrade Summary ===") {
		t.Errorf("FAIL: stdout contains textual Upgrade Summary table when --json was requested:\n%s", outStr)
	}

	trimmed := bytes.TrimSpace(stdout.Bytes())
	if len(trimmed) == 0 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		t.Fatalf("FAIL: stdout does not begin and end with JSON object brackets:\n%s", outStr)
	}

	var summary upgrade.JSONSummary
	if jsonErr := json.Unmarshal(stdout.Bytes(), &summary); jsonErr != nil {
		t.Fatalf("FAIL: stdout is not valid JSON (%v):\n%s", jsonErr, outStr)
	}
	if summary.SchemaVersion != "1.0.0" {
		t.Errorf("expected schema_version '1.0.0', got %q", summary.SchemaVersion)
	}
	if summary.Total != 1 || summary.Skipped != 1 {
		t.Errorf("expected 1 skipped package, got total=%d, skipped=%d", summary.Total, summary.Skipped)
	}
	if len(summary.Results) != 1 || summary.Results[0].Package.ID != "pkg.pinned" {
		t.Errorf("expected pkg.pinned in results, got: %+v", summary.Results)
	}
	_ = err
}

// TestAdversarial_UpgradeJson_FailureExitCodeAndPayload tests that when an upgrade fails in
// --json mode, the error string is properly serialized in JSON (not empty {}) and an exit code 2 error is returned.
func TestAdversarial_UpgradeJson_FailureExitCodeAndPayload(t *testing.T) {
	mockMgr := &mockAdversarialUpgradeManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "pkg.fail", Name: "PkgFail", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
		},
		failUpgrade: true,
	}
	registryOverride = pm.NewRegistry(mockMgr)
	t.Cleanup(func() { registryOverride = nil })

	var stdout, stderr bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"upgrade", "--all", "--json", "--no-refresh"})

	ctx := context.Background()
	err := cmd.ExecuteContext(ctx)
	if err == nil {
		t.Fatalf("expected error on failed upgrade, got nil")
	}

	exitCode := exitcodes.Determine(ctx, err)
	if exitCode != exitcodes.ExitUpgradeFailure {
		t.Errorf("expected exit code %d (ExitUpgradeFailure), got %d", exitcodes.ExitUpgradeFailure, exitCode)
	}

	var summary upgrade.JSONSummary
	if jsonErr := json.Unmarshal(stdout.Bytes(), &summary); jsonErr != nil {
		t.Fatalf("stdout is not valid JSON (%v):\n%s", jsonErr, stdout.String())
	}
	if summary.Failed != 1 {
		t.Errorf("expected 1 failed package, got %d", summary.Failed)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(summary.Results))
	}
	if summary.Results[0].Error == nil || !strings.Contains(*summary.Results[0].Error, "checksum mismatch") {
		t.Errorf("expected error message containing 'checksum mismatch', got: %v", summary.Results[0].Error)
	}
}

// TestAdversarial_ExitCodes_AllScenarios verifies that exitcodes.Determine accurately maps
// errors across Success (0), General Error (1), Upgrade Failure (2), and Cancellation (3).
func TestAdversarial_ExitCodes_AllScenarios(t *testing.T) {
	ctx := context.Background()

	// Scenario 1: Success
	if code := exitcodes.Determine(ctx, nil); code != exitcodes.ExitSuccess {
		t.Errorf("expected exit code 0 for nil error, got %d", code)
	}

	// Scenario 2: General Error (flag error, syntax error, unknown manager)
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"upgrade", "--invalid-flag-1234"})
	err := cmd.ExecuteContext(ctx)
	if err == nil {
		t.Fatalf("expected flag error, got nil")
	}
	if code := exitcodes.Determine(ctx, err); code != exitcodes.ExitGeneralError {
		t.Errorf("expected exit code 1 for invalid flag, got %d", code)
	}

	// Scenario 3: Upgrade Failure
	upgradeErr := exitcodes.New(exitcodes.ExitUpgradeFailure, "upgrade failed")
	if code := exitcodes.Determine(ctx, upgradeErr); code != exitcodes.ExitUpgradeFailure {
		t.Errorf("expected exit code 2 for upgrade failure, got %d", code)
	}

	// Scenario 4: User Cancellation via context
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := exitcodes.Determine(canceledCtx, errors.New("aborted")); code != exitcodes.ExitCanceled {
		t.Errorf("expected exit code 3 for canceled context, got %d", code)
	}
}

// TestAdversarial_InteractiveCancellation_ExitCode tests what exit code is produced
// when the user cancels the interactive selection prompt.
func TestAdversarial_InteractiveCancellation_ExitCode(t *testing.T) {
	mockMgr := &mockAdversarialUpgradeManager{
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

	ctx := context.Background()
	err := cmd.ExecuteContext(ctx)

	exitCode := exitcodes.Determine(ctx, err)
	t.Logf("Interactive cancellation returned err: %v, exitCode: %d", err, exitCode)
	if exitCode != exitcodes.ExitCanceled {
		t.Errorf("CHALLENGE: user cancellation in interactive prompt returned exit code %d, expected %d (ExitCanceled)", exitCode, exitcodes.ExitCanceled)
	}
}

// TestAdversarial_Selector_RangeEdgeCases stress-tests parseSelectionIndices
// with adversarial and unusual inputs.
func TestAdversarial_Selector_RangeEdgeCases(t *testing.T) {
	total := 10

	tests := []struct {
		name      string
		input     string
		expected  []int
		expectErr bool
	}{
		{"leading zeros", "01, 03", []int{0, 2}, false},
		{"overlapping ranges", "1-3, 2-5", []int{0, 1, 2, 3, 4}, false},
		{"whitespace variations", "  1  ,   4 - 6  ", []int{0, 3, 4, 5}, false},
		{"duplicate single items", "2, 2, 2", []int{1}, false},
		{"single element range", "3-3", []int{2}, false},
		{"negative number in range", "-1-3", nil, true},
		{"trailing dash", "1-", nil, true},
		{"leading dash", "-3", nil, true},
		{"double dash", "1--3", nil, true},
		{"trailing comma", "1, 2,", []int{0, 1}, false},
		{"empty comma tokens", "1,,,3", []int{0, 2}, false},
		{"out of bounds zero in range", "0-3", nil, true},
		{"out of bounds end in range", "8-11", nil, true},
		{"overflow number", "999999999999999999999999", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSelectionIndices(tt.input, total)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error for input %q, got nil (result: %v)", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if len(got) != len(tt.expected) {
				t.Fatalf("length mismatch: got %v, want %v", got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("at index %d: got %d, want %d", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

// TestHelperProcessInteractiveCancel is the subprocess target for testing actual process exit code 3.
func TestHelperProcessInteractiveCancel(t *testing.T) {
	sub := os.Getenv("GO_TEST_SUBPROCESS_CANCEL")
	if sub == "" {
		return
	}

	mockMgr := &mockAdversarialUpgradeManager{
		name: "mockpm",
		updates: []pm.PackageUpdate{
			{Id: "pkg.cancel", Name: "PkgCancel", Version: "1.0", AvailableVersion: "2.0", ManagerName: "mockpm"},
		},
	}
	registryOverride = pm.NewRegistry(mockMgr)
	if sub == "err_canceled" {
		SetDefaultSelectorForTest(&MockMultiSelector{Err: ErrSelectionCanceled})
	} else if sub == "empty_selection" {
		SetDefaultSelectorForTest(&MockMultiSelector{SelectedIndices: []int{}})
	}

	// Set args to invoke upgrade interactive mode
	os.Args = []string{"pw", "upgrade", "--no-refresh"}

	// Call Execute() - this should invoke os.Exit(3)
	Execute()
}

// TestAdversarial_ProcessExitCode3_EmpiricalOSProcess verifies the OS process exit code
// is strictly 3 (ExitCanceled) when interactive cancellation occurs.
func TestAdversarial_ProcessExitCode3_EmpiricalOSProcess(t *testing.T) {
	scenarios := []struct {
		name       string
		subEnv     string
		expectCode int
	}{
		{"ErrSelectionCanceled", "err_canceled", exitcodes.ExitCanceled},
		{"EmptySelection", "empty_selection", exitcodes.ExitCanceled},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessInteractiveCancel")
			cmd.Env = append(os.Environ(), "GO_TEST_SUBPROCESS_CANCEL="+sc.subEnv)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			if err == nil {
				t.Fatalf("expected subprocess to exit with non-zero exit code, but it exited 0")
			}

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("expected ExitError, got %T: %v", err, err)
			}

			actualExitCode := exitErr.ExitCode()
			t.Logf("[%s] Subprocess terminated with exit code: %d", sc.name, actualExitCode)
			t.Logf("[%s] Subprocess stdout:\n%s", sc.name, stdout.String())
			t.Logf("[%s] Subprocess stderr:\n%s", sc.name, stderr.String())

			if actualExitCode != sc.expectCode {
				t.Errorf("[%s] FAIL: Subprocess exit code was %d, expected %d", sc.name, actualExitCode, sc.expectCode)
			}
		})
	}
}
