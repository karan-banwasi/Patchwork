package asdf

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

func TestAsdf_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "nodejs  20.12.0  /home/user/.tool-versions\npython  3.12.3   /home/user/.tool-versions\n",
	}
	mgr := NewAsdfManager()
	mgr.exePath = "asdf"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(pkgs))
	}
	if pkgs[0].ID != "nodejs" || pkgs[0].Version != "20.12.0" || pkgs[0].ManagerName != "asdf" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestAsdf_ListInstalled_Error(t *testing.T) {
	mgr := NewAsdfManager()
	mgr.exePath = "asdf"
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAsdf_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewAsdfManager()
	mgr.exePath = "asdf"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "nodejs", "20.12.0"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install nodejs 20.12.0" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "nodejs", ""); err != nil {
		t.Fatalf("InstallPackage without version failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install nodejs latest" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "nodejs", "--keep-download"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
