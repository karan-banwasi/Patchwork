package psresource

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

func TestPSResource_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: `[{"Name":"Pester","Version":"5.5.0"},{"Name":"Az.Accounts","Version":"3.0.0"}]`,
	}
	mgr := NewPSResourceManager()
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "Az.Accounts" || pkgs[0].Version != "3.0.0" || pkgs[0].ManagerName != "psresource" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestPSResource_ListInstalled_SingleAndError(t *testing.T) {
	mgr := NewPSResourceManager()
	mgr.runner = &mockProfileRunner{stdout: `{"Name":"Pester","Version":"5.5.0"}`}
	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil || len(pkgs) != 1 || pkgs[0].ID != "Pester" {
		t.Fatalf("expected 1 package, got %+v, err=%v", pkgs, err)
	}

	mgr.runner = &mockProfileRunner{stderr: "boom", err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPSResource_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewPSResourceManager()
	mgr.TrustRepository = true
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "Pester", "5.5.0"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	joined := strings.Join(mock.ranArgs, " ")
	if !strings.Contains(joined, "Install-PSResource") || !strings.Contains(joined, "-TrustRepository") {
		t.Errorf("unexpected args: %v", mock.ranArgs)
	}
	if mock.ranArgs[len(mock.ranArgs)-2] != "Pester" || mock.ranArgs[len(mock.ranArgs)-1] != "5.5.0" {
		t.Errorf("expected trailing parameterized args Pester 5.5.0, got %v", mock.ranArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "Pester", "-Scope"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
