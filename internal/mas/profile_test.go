package mas

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

func TestMas_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "497799835  Xcode                     (15.4)\n1295203466 Microsoft Remote Desktop  (10.9.7)\n",
	}
	mgr := NewMasManager()
	mgr.exePath = "/opt/homebrew/bin/mas"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "1295203466" || pkgs[0].Name != "Microsoft Remote Desktop" || pkgs[0].Version != "10.9.7" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestMas_ListInstalled_Error(t *testing.T) {
	mgr := NewMasManager()
	mgr.exePath = "/opt/homebrew/bin/mas"
	mgr.runner = &mockProfileRunner{stderr: "boom", err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestMas_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewMasManager()
	mgr.exePath = "/opt/homebrew/bin/mas"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "497799835", ""); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install 497799835" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "xcode", ""); err == nil {
		t.Error("expected error for non-numeric ID")
	}
	if err := mgr.InstallPackage(context.Background(), "497799835", "--force"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
