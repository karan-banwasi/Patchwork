package choco

import (
	"context"
	"io"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

type mockChocoRunner struct {
	ranExe  string
	ranArgs []string
	output  string
	err     error
}

func (m *mockChocoRunner) Run(ctx context.Context, exe string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	m.ranExe = exe
	m.ranArgs = args
	if stdout != nil && m.output != "" {
		_, _ = stdout.Write([]byte(m.output))
	}
	return m.err
}

func TestChocoManager_UpgradePackagesDetailed_MockRunnerNoElevation(t *testing.T) {
	mock := &mockChocoRunner{
		output: `Chocolatey v2.4.0
The upgrade of git.install was successful.
Chocolatey upgraded 1/1 packages.`,
	}

	mgr := &ChocoManager{
		runner: mock,
	}

	results, err := mgr.UpgradePackagesDetailed(context.Background(), []string{"git.install"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Succeeded {
		t.Errorf("expected git.install to succeed, got %+v", results[0])
	}
	if mock.ranExe == "" {
		t.Errorf("expected mock runner to execute without elevation")
	}
}

func TestChocoManager_IsProductionRunner(t *testing.T) {
	prodMgr := NewChocoManager()
	if !isProductionRunner(prodMgr.runner) {
		t.Errorf("expected NewChocoManager() to have isProductionRunner == true, got false")
	}

	var nilRunner pm.Runner
	if !isProductionRunner(nilRunner) {
		t.Errorf("expected nil runner to have isProductionRunner == true, got false")
	}

	execRunner := pm.ExecRunner{}
	if !isProductionRunner(execRunner) {
		t.Errorf("expected pm.ExecRunner{} to have isProductionRunner == true, got false")
	}

	execRunnerPtr := &pm.ExecRunner{}
	if !isProductionRunner(execRunnerPtr) {
		t.Errorf("expected &pm.ExecRunner{} to have isProductionRunner == true, got false")
	}

	mock := &mockChocoRunner{}
	if isProductionRunner(mock) {
		t.Errorf("expected mock runner to have isProductionRunner == false, got true")
	}
}
