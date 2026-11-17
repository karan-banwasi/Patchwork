package winget

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/karan-banwasi/patchwork/internal/pm"
)

type mockRunner struct {
	exe  string
	args []string
	err  error
}

func (m *mockRunner) Run(_ context.Context, name string, args []string, _ io.Reader, _, _ io.Writer) error {
	m.exe = name
	m.args = append([]string(nil), args...)
	return m.err
}

func TestWingetRollbackPackage(t *testing.T) {
	mock := &mockRunner{}
	mgr := &WingetManager{
		AcceptAgreements: true,
		runner:           mock,
	}
	var _ pm.Rollbackable = mgr

	if err := mgr.RollbackPackage(context.Background(), "Git.Git", "2.44.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"install", "--id", "Git.Git", "--version", "2.44.0", "--exact", "--force", "--silent", "--accept-source-agreements", "--accept-package-agreements"}
	if !reflect.DeepEqual(mock.args, want) {
		t.Errorf("args = %v, want %v", mock.args, want)
	}

	// AcceptAgreements=false omits --silent and --accept-* flags
	mgr.AcceptAgreements = false
	if err := mgr.RollbackPackage(context.Background(), "Git.Git", "2.44.0"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantNoAccept := []string{"install", "--id", "Git.Git", "--version", "2.44.0", "--exact", "--force"}
	if !reflect.DeepEqual(mock.args, wantNoAccept) {
		t.Errorf("args = %v, want %v", mock.args, wantNoAccept)
	}

	// Shell package omits --silent
	mgr.AcceptAgreements = true
	if err := mgr.RollbackPackage(context.Background(), "Microsoft.PowerShell", "7.4.1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantPS := []string{"install", "--id", "Microsoft.PowerShell", "--version", "7.4.1", "--exact", "--force", "--accept-source-agreements", "--accept-package-agreements"}
	if !reflect.DeepEqual(mock.args, wantPS) {
		t.Errorf("args = %v, want %v", mock.args, wantPS)
	}

	// Validation
	if err := mgr.RollbackPackage(context.Background(), "-bad", "1.0.0"); err == nil {
		t.Errorf("expected error for invalid id")
	}
	if err := mgr.RollbackPackage(context.Background(), "Git.Git", ""); err == nil {
		t.Errorf("expected error for empty version")
	}
	if err := mgr.RollbackPackage(context.Background(), "Git.Git", "--force"); err == nil {
		t.Errorf("expected error for flag-injected version")
	}
}
