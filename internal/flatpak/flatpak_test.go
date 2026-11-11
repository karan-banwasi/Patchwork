package flatpak

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type mockRunner struct {
	runFunc func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error
}

func (m *mockRunner) Run(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if m.runFunc != nil {
		return m.runFunc(ctx, name, args, stdin, stdout, stderr)
	}
	return nil
}

func TestFlatpakManager_BasicInfo(t *testing.T) {
	mgr := NewFlatpakManager()
	if mgr.Name() != "flatpak" {
		t.Errorf("expected name flatpak, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 1 || aliases[0] != "flathub" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("org.mozilla.firefox"); err != nil {
		t.Errorf("expected org.mozilla.firefox to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("firefox"); err == nil {
		t.Errorf("expected firefox without domain to be invalid ID")
	}
}

func TestFlatpakManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewFlatpakManager()
	mgr.exePath = "/usr/bin/flatpak"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"org.mozilla.firefox", "org.videolan.VLC"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "update -y org.mozilla.firefox org.videolan.VLC") {
		t.Errorf("expected flatpak update args, got %v", capturedArgs)
	}
}

func TestFlatpakManager_Cleanup(t *testing.T) {
	var capturedArgs []string
	mgr := NewFlatpakManager()
	mgr.exePath = "/usr/bin/flatpak"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	var buf bytes.Buffer
	if err := mgr.Cleanup(context.Background(), &buf, &buf); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "uninstall --unused -y") {
		t.Errorf("expected uninstall --unused -y, got %v", capturedArgs)
	}
}

func TestFlatpakManager_GetAvailableUpdates_Success(t *testing.T) {
	mgr := NewFlatpakManager()
	mgr.exePath = "/usr/bin/flatpak"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 0 && args[0] == "list" {
				stdout.Write([]byte("org.mozilla.firefox\t119.0.0\n"))
				return nil
			}
			stdout.Write([]byte("org.mozilla.firefox\tFirefox\t120.0.1\n"))
			return nil
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "org.mozilla.firefox" {
		t.Fatalf("expected 1 update, got: %+v", updates)
	}
	if updates[0].Version != "119.0.0" || updates[0].AvailableVersion != "120.0.1" {
		t.Errorf("expected version 119.0.0 -> 120.0.1, got: %+v", updates[0])
	}
}

func TestFlatpakManager_GetAvailableUpdates_Error(t *testing.T) {
	mgr := NewFlatpakManager()
	mgr.exePath = "/usr/bin/flatpak"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("no remote configured"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no remote configured") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}
