package dnf

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

func TestDnf_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "Installed Packages\ncurl.x86_64   8.6.0-1.fc40   @updates\ngit.x86_64    2.45.0-1.fc40  @updates\n",
	}
	mgr := NewDnfManager()
	mgr.exePath = "/usr/bin/dnf"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "curl" || pkgs[0].Version != "8.6.0-1.fc40" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestDnf_ListInstalled_Error(t *testing.T) {
	mgr := NewDnfManager()
	mgr.exePath = "/usr/bin/dnf"
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestDnf_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewDnfManager()
	mgr.exePath = "/usr/bin/dnf"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "git", "2.45.0-1.fc40"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if !strings.Contains(strings.Join(mock.ranArgs, " "), "install -y git-2.45.0-1.fc40") {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "git", "--allowerasing"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
