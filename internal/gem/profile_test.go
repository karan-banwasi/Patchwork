package gem

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

func TestGem_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "\n*** LOCAL GEMS ***\n\nbundler (2.5.10, default: 2.5.9)\nrake (13.2.1)\n",
	}
	mgr := NewGemManager()
	mgr.exePath = "gem"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 gems, got %d", len(pkgs))
	}
	if pkgs[0].ID != "bundler" || pkgs[0].Version != "2.5.10" || pkgs[0].ManagerName != "gem" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestGem_ListInstalled_Error(t *testing.T) {
	mgr := NewGemManager()
	mgr.exePath = "gem"
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGem_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewGemManager()
	mgr.exePath = "gem"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "bundler", "2.5.10"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install bundler -v 2.5.10" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "bundler", "--pre"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
