package scoop

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

func TestScoop_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: `{"apps":[{"Name":"ripgrep","Version":"14.1.0","Source":"main"},{"Name":"git","Version":"2.45.0","Source":"main"}]}`,
	}
	mgr := NewScoopManager()
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "git" || pkgs[0].Version != "2.45.0" || pkgs[0].ManagerName != "scoop" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestScoop_ListInstalled_Error(t *testing.T) {
	mgr := NewScoopManager()
	mgr.runner = &mockProfileRunner{stderr: "failed", err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestScoop_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewScoopManager()
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "git", "2.45.0"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install git@2.45.0" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "git", ""); err != nil {
		t.Fatalf("InstallPackage without version failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install git" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "git", "--global"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
