package dotnet

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

func TestDotnet_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "Package Id      Version      Commands\n--------------------------------------\ndotnet-ef       8.0.6        dotnet-ef\n",
	}
	mgr := NewDotnetManager()
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 1 || pkgs[0].ID != "dotnet-ef" || pkgs[0].Version != "8.0.6" || pkgs[0].ManagerName != "dotnet" {
		t.Fatalf("unexpected pkgs: %+v", pkgs)
	}
}

func TestDotnet_ListInstalled_Error(t *testing.T) {
	mgr := NewDotnetManager()
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDotnet_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewDotnetManager()
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "dotnet-ef", "8.0.6"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "tool install -g dotnet-ef --version 8.0.6" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "dotnet-ef", "--prerelease"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
