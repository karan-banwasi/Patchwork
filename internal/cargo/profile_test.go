package cargo

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type mockProfileRunner struct {
	ranArgs []string
	stdout  string
	stderr  string
	err     error
}

func (m *mockProfileRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	m.ranArgs = append([]string(nil), args...)
	if stdout != nil && m.stdout != "" {
		_, _ = stdout.Write([]byte(m.stdout))
	}
	if stderr != nil && m.stderr != "" {
		_, _ = stderr.Write([]byte(m.stderr))
	}
	return m.err
}

func TestCargo_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "ripgrep v14.1.0:\n    rg\nbat v0.24.0:\n    bat\n",
	}
	mgr := NewCargoManager()
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "bat" || pkgs[0].Version != "0.24.0" || pkgs[0].ManagerName != "cargo" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestCargo_ListInstalled_Error(t *testing.T) {
	mgr := NewCargoManager()
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCargo_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewCargoManager()
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "ripgrep", "14.1.0"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install ripgrep --version 14.1.0" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "ripgrep", "--locked"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
