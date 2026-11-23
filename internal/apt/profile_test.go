package apt

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestApt_ListInstalled(t *testing.T) {
	mgr := NewAptManager()
	mgr.exePath = "/usr/bin/apt-get"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			_, _ = stdout.Write([]byte("Listing... Done\ncurl/noble-updates,now 8.5.0-2ubuntu10.6 amd64 [installed]\ngit/noble,now 1:2.43.0-1ubuntu7 amd64 [installed,automatic]\n"))
			return nil
		},
	}

	pkgs, err := mgr.ListInstalled(context.Background())
	if err != nil {
		t.Fatalf("ListInstalled failed: %v", err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if pkgs[0].ID != "curl" || pkgs[0].Version != "8.5.0-2ubuntu10.6" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
	if pkgs[1].ID != "git" || pkgs[1].Version != "1:2.43.0-1ubuntu7" {
		t.Errorf("unexpected pkg[1]: %+v", pkgs[1])
	}
}

func TestApt_ListInstalled_Error(t *testing.T) {
	mgr := NewAptManager()
	mgr.exePath = "/usr/bin/apt-get"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			return errors.New("exit 1")
		},
	}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestApt_InstallPackage(t *testing.T) {
	var capturedArgs []string
	mgr := NewAptManager()
	mgr.exePath = "/usr/bin/apt-get"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	if err := mgr.InstallPackage(context.Background(), "git", "1:2.43.0-1ubuntu7"); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if !strings.Contains(strings.Join(capturedArgs, " "), "install -y git=1:2.43.0-1ubuntu7") {
		t.Errorf("unexpected args: %v", capturedArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "git", "--reinstall"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
