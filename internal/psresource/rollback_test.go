package psresource

import (
	"context"
	"io"
	"strings"
	"testing"
)

type mockRollbackRunner struct {
	exe  string
	args []string
	err  error
}

func (m *mockRollbackRunner) Run(_ context.Context, name string, args []string, _ io.Reader, _, _ io.Writer) error {
	m.exe = name
	m.args = append([]string(nil), args...)
	return m.err
}

func TestPSResourceRollbackPackage(t *testing.T) {
	mock := &mockRollbackRunner{}
	mgr := &PSResourceManager{runner: mock}

	if err := mgr.RollbackPackage(context.Background(), "PSReadLine", "2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.args) != 6 || mock.args[4] != "PSReadLine" || mock.args[5] != "2.3.4" {
		t.Fatalf("unexpected args: %v", mock.args)
	}
	if !strings.Contains(mock.args[3], "Install-PSResource") || strings.Contains(mock.args[3], "-TrustRepository") {
		t.Errorf("unexpected script block when TrustRepository=false: %q", mock.args[3])
	}

	mgr.TrustRepository = true
	if err := mgr.RollbackPackage(context.Background(), "PSReadLine", "2.3.4"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(mock.args[3], "-TrustRepository") {
		t.Errorf("expected -TrustRepository in script block when TrustRepository=true: %q", mock.args[3])
	}

	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "PSReadLine", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "PSReadLine", "--force"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
