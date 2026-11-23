package bun

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

func TestBun_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "/home/user/.bun/install/global node_modules (2)\n├── typescript@5.4.5\n└── @biomejs/biome@1.8.0\n",
	}
	mgr := NewBunManager()
	mgr.exePath = "bun"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "@biomejs/biome" || pkgs[0].Version != "1.8.0" || pkgs[0].ManagerName != "bun" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestBun_ListInstalled_Error(t *testing.T) {
	mgr := NewBunManager()
	mgr.exePath = "bun"
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBun_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewBunManager()
	mgr.exePath = "bun"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "typescript", "5.4.5"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "add -g typescript@5.4.5" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "typescript", "--force"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
