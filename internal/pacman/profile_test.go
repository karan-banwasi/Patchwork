package pacman

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPacman_ListInstalled(t *testing.T) {
	mgr := NewPacmanManager()
	mgr.exePath = "/usr/bin/pacman"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			_, _ = stdout.Write([]byte("curl 8.8.0-1\ngit 2.45.2-1\n"))
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
	if pkgs[0].ID != "curl" || pkgs[0].Version != "8.8.0-1" {
		t.Errorf("unexpected pkg[0]: %+v", pkgs[0])
	}
}

func TestPacman_ListInstalled_Error(t *testing.T) {
	mgr := NewPacmanManager()
	mgr.exePath = "/usr/bin/pacman"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			return errors.New("exit 1")
		},
	}
	if _, err := mgr.ListInstalled(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestPacman_InstallPackage(t *testing.T) {
	var capturedArgs []string
	mgr := NewPacmanManager()
	mgr.exePath = "/usr/bin/pacman"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	if err := mgr.InstallPackage(context.Background(), "git", ""); err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}
	if !strings.Contains(strings.Join(capturedArgs, " "), "-S --noconfirm git") {
		t.Errorf("unexpected args: %v", capturedArgs)
	}

	if err := mgr.InstallPackage(context.Background(), "-bad", ""); err == nil {
		t.Error("expected error for invalid ID")
	}
	if err := mgr.InstallPackage(context.Background(), "git", "--needed"); err == nil {
		t.Error("expected error for flag injection in version")
	}
}
