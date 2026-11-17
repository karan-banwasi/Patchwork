package brew

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

func TestBrew_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: `{
			"formulae": [{"name": "git", "installed": [{"version": "2.45.0"}], "linked_keg": "2.45.0"}],
			"casks": [{"token": "firefox", "name": ["Mozilla Firefox"], "installed": "126.0"}]
		}`,
	}
	mgr := NewBrewManager()
	mgr.exePath = "/opt/homebrew/bin/brew"
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "firefox" || pkgs[0].Name != "Mozilla Firefox" || pkgs[0].Version != "126.0" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
	if pkgs[1].ID != "git" || pkgs[1].Version != "2.45.0" || pkgs[1].ManagerName != "brew" {
		t.Errorf("unexpected pkg[1]: %+v", pkgs[1])
	}
}

func TestBrew_ListInstalled_Error(t *testing.T) {
	mgr := NewBrewManager()
	mgr.exePath = "/opt/homebrew/bin/brew"
	mgr.runner = &mockProfileRunner{stderr: "boom", err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBrew_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewBrewManager()
	mgr.exePath = "/opt/homebrew/bin/brew"
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "git", "2.45.0"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if strings.Join(mock.ranArgs, " ") != "install git" {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "git", "--HEAD"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
