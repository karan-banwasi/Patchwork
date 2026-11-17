package pipx

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

func TestPipx_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: `{"pipx_spec_version":"0.1","venvs":{"black":{"metadata":{"main_package":{"package":"black","package_version":"24.4.2"}}}}}`,
	}
	mgr := NewPipxManager()
	mgr.exePath = "pipx"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].ID != "black" || pkgs[0].Version != "24.4.2" || pkgs[0].ManagerName != "pipx" {
		t.Fatalf("unexpected pkgs: %+v", pkgs)
	}
}

func TestPipx_ListInstalled_Error(t *testing.T) {
	mgr := NewPipxManager()
	mgr.exePath = "pipx"
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPipx_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewPipxManager()
	mgr.exePath = "pipx"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "black", "24.4.2"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install black==24.4.2" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "black", "--force"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
