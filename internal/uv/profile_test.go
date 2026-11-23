package uv

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

func TestUv_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "ruff v0.4.8\n- ruff\nhttpie v3.2.2\n- http\n",
	}
	mgr := NewUvManager()
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "httpie" || pkgs[0].Version != "3.2.2" || pkgs[0].ManagerName != "uv" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestUv_ListInstalled_Error(t *testing.T) {
	mgr := NewUvManager()
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUv_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewUvManager()
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "ruff", "0.4.8"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "tool install ruff==0.4.8" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "ruff", "--force"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
