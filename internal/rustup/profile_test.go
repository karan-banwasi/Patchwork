package rustup

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

func TestRustup_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "stable-x86_64-unknown-linux-gnu (default)\nnightly-x86_64-unknown-linux-gnu (active)\n",
	}
	mgr := NewRustupManager()
	mgr.exePath = "rustup"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 toolchains, got %d", len(pkgs))
	}
	if pkgs[1].ID != "stable-x86_64-unknown-linux-gnu" || pkgs[1].ManagerName != "rustup" {
		t.Errorf("unexpected pkg[1]: %+v", pkgs[1])
	}
}

func TestRustup_ListInstalled_Error(t *testing.T) {
	mgr := NewRustupManager()
	mgr.exePath = "rustup"
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRustup_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewRustupManager()
	mgr.exePath = "rustup"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "stable", ""); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "toolchain install stable" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "stable", "--force"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
