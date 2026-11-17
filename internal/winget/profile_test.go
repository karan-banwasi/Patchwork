package winget

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

func TestWinget_ListInstalled(t *testing.T) {
	mock := &mockProfileRunner{
		stdout: "Name                 Id                      Version   Available  Source\r\n" +
			"--------------------------------------------------------------------------\r\n" +
			"Git                  Git.Git                 2.45.0    2.45.1     winget\r\n" +
			"OpenJS NodeJS        OpenJS.NodeJS           20.12.0              winget\r\n" +
			"Local App            ARP\\Machine\\X64\\Local   1.0.0\r\n",
	}
	mgr := NewWingetManager()
	mgr.runner = mock

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 valid packages, got %d: %+v", len(pkgs), pkgs)
	}
	if pkgs[0].ID != "Git.Git" || pkgs[0].Version != "2.45.0" || pkgs[0].ManagerName != "winget" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
	if pkgs[1].ID != "OpenJS.NodeJS" || pkgs[1].Version != "20.12.0" {
		t.Errorf("unexpected pkg[1]: %+v", pkgs[1])
	}
}

func TestWinget_ListInstalled_Error(t *testing.T) {
	mgr := NewWingetManager()
	mgr.runner = &mockProfileRunner{stderr: "boom", err: errors.New("exit 1")}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestWinget_InstallPackage(t *testing.T) {
	mock := &mockProfileRunner{}
	mgr := NewWingetManager()
	mgr.runner = mock

	if err := mgr.InstallPackage(context.Background(), "Git.Git", "2.45.0"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	joined := strings.Join(mock.ranArgs, " ")
	for _, want := range []string{"install", "--id Git.Git", "--exact", "--version 2.45.0", "--accept-package-agreements"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in args %v", want, mock.ranArgs)
		}
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "Git.Git", "--silent"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
