package choco

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

type mockTrackingRunner struct {
	calledExe  string
	calledArgs []string
	callCount  int
}

func (m *mockTrackingRunner) Run(ctx context.Context, exe string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	m.calledExe = exe
	m.calledArgs = args
	m.callCount++
	if stdout != nil {
		_, _ = stdout.Write([]byte("Chocolatey v2.4.0\nThe upgrade of testpkg was successful.\nChocolatey upgraded 1/1 packages."))
	}
	return nil
}

type mockFailingRunner struct {
	ran bool
}

func (m *mockFailingRunner) Run(ctx context.Context, exe string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	m.ran = true
	if stdout != nil {
		_, _ = stdout.Write([]byte("Failures\n - testpkg (exited 1) - error\n"))
	}
	return nil
}

func TestChallenger_RunnerDifferentiation_Exhaustive(t *testing.T) {
	// 1. Production runner varieties must all yield IsProductionRunner == true
	prodMgr := NewChocoManager()
	if !IsProductionRunner(prodMgr.runner) {
		t.Fatalf("expected NewChocoManager().runner to be identified as production runner")
	}

	if !IsProductionRunner(nil) {
		t.Fatalf("expected nil runner to be identified as production runner")
	}

	if !IsProductionRunner(pm.ExecRunner{}) {
		t.Fatalf("expected pm.ExecRunner{} to be identified as production runner")
	}

	if !IsProductionRunner(&pm.ExecRunner{}) {
		t.Fatalf("expected &pm.ExecRunner{} to be identified as production runner")
	}

	// 2. Custom mock runners must yield IsProductionRunner == false
	mock1 := &mockTrackingRunner{}
	if IsProductionRunner(mock1) {
		t.Fatalf("expected mockTrackingRunner to NOT be identified as production runner")
	}

	mock2 := &mockChocoRunner{}
	if IsProductionRunner(mock2) {
		t.Fatalf("expected mockChocoRunner to NOT be identified as production runner")
	}

	// 3. Verify mock runner bypasses elevation unconditionally
	trackingRunner := &mockTrackingRunner{}
	mgr := &ChocoManager{
		runner: trackingRunner,
	}

	results, err := mgr.UpgradePackagesDetailed(context.Background(), []string{"testpkg"})
	if err != nil {
		t.Fatalf("unexpected error with mock runner: %v", err)
	}
	if len(results) != 1 || !results[0].Succeeded {
		t.Fatalf("expected 1 successful result with mock runner, got %+v", results)
	}
	if trackingRunner.callCount != 1 {
		t.Fatalf("expected tracking runner to be invoked exactly once, got %d", trackingRunner.callCount)
	}
	if !strings.HasSuffix(trackingRunner.calledExe, "choco") && !strings.HasSuffix(trackingRunner.calledExe, "choco.exe") {
		t.Errorf("expected calledExe to be choco, got %q", trackingRunner.calledExe)
	}

	// 4. Verify UpgradePackages also works through mock runner without elevation
	trackingRunner2 := &mockTrackingRunner{}
	mgr2 := &ChocoManager{runner: trackingRunner2}
	if err := mgr2.UpgradePackages(context.Background(), []string{"testpkg"}); err != nil {
		t.Fatalf("unexpected error in UpgradePackages with mock runner: %v", err)
	}
	if trackingRunner2.callCount != 1 {
		t.Fatalf("expected UpgradePackages to invoke mock runner once, got %d", trackingRunner2.callCount)
	}

	// 5. Verify failure handling in mock runner without elevation
	failRunner := &mockFailingRunner{}
	mgrFail := &ChocoManager{runner: failRunner}
	failResults, err := mgrFail.UpgradePackagesDetailed(context.Background(), []string{"testpkg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(failResults) != 1 || failResults[0].Succeeded {
		t.Fatalf("expected package failure, got %+v", failResults)
	}
	if !failRunner.ran {
		t.Fatalf("expected failRunner to have run")
	}
}
