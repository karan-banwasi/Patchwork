package vscode

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

func TestVSCode_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "golang.go@0.41.4\nms-python.python@2024.8.1\n",
	}
	mgr := NewVSCodeManager()
	mgr.exePath = "code"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(pkgs))
	}
	if pkgs[0].ID != "golang.go" || pkgs[0].Version != "0.41.4" || pkgs[0].ManagerName != "vscode" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestVSCode_ListInstalled_Error(t *testing.T) {
	mgr := NewVSCodeManager()
	mgr.exePath = "code"
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestVSCode_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewVSCodeManager()
	mgr.exePath = "code"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "golang.go", "0.41.4"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "--install-extension golang.go@0.41.4 --force" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "golang.go", "--force"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
