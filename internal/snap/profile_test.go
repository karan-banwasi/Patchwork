package snap

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

func TestSnap_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "Name     Version    Rev   Tracking       Publisher   Notes\ncore22   20240408   1380  latest/stable  canonical   base\nfirefox  126.0-2    4259  latest/stable  mozilla     -\n",
	}
	mgr := NewSnapManager()
	mgr.exePath = "/usr/bin/snap"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[1].ID != "firefox" || pkgs[1].Version != "126.0-2" {
		t.Errorf("unexpected pkg[1]: %+v", pkgs[1])
	}
}

func TestSnap_ListInstalled_Error(t *testing.T) {
	mgr := NewSnapManager()
	mgr.exePath = "/usr/bin/snap"
	mgr.runner = &mockProfileRunner{err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSnap_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewSnapManager()
	mgr.exePath = "/usr/bin/snap"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "firefox", ""); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if !strings.Contains(strings.Join(mock.ranArgs, " "), "install firefox") {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "firefox", "--classic"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
