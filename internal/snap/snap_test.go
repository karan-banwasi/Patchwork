package snap

import (
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

func TestSnapManager_BasicInfo(t *testing.T) {
	mgr := NewSnapManager()
	if mgr.Name() != "snap" {
		t.Errorf("expected name snap, got %s", mgr.Name())
	}
	aliases := mgr.Aliases()
	if len(aliases) != 2 || aliases[0] != "snapcraft" {
		t.Errorf("unexpected aliases: %v", aliases)
	}
	if err := mgr.ValidateID("firefox"); err != nil {
		t.Errorf("expected firefox to be valid ID, got: %v", err)
	}
	if err := mgr.ValidateID("-firefox"); err == nil {
		t.Errorf("expected -firefox to be invalid ID")
	}
}

func TestSnapManager_Upgrade(t *testing.T) {
	var capturedArgs []string
	mgr := NewSnapManager()
	mgr.exePath = "/usr/bin/snap"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			capturedArgs = args
			return nil
		},
	}

	err := mgr.UpgradePackages(context.Background(), []string{"firefox", "vlc"})
	if err != nil {
		t.Fatalf("unexpected upgrade error: %v", err)
	}
	joined := strings.Join(capturedArgs, " ")
	if !strings.Contains(joined, "refresh firefox vlc") {
		t.Errorf("expected refresh args, got %v", capturedArgs)
	}
}

func TestSnapManager_GetAvailableUpdates_Success(t *testing.T) {
	mgr := NewSnapManager()
	mgr.exePath = "/usr/bin/snap"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			if len(args) > 0 && args[0] == "list" {
				stdout.Write([]byte("Name Version Rev Tracking Publisher Notes\nfirefox 119.0 3400 latest/stable canonical -"))
				return nil
			}
			stdout.Write([]byte("Name Version Rev Publisher Notes\nfirefox 120.0 3456 canonical✓ -\n"))
			return nil
		},
	}

	updates, err := mgr.GetAvailableUpdates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(updates) != 1 || updates[0].Id != "firefox" {
		t.Fatalf("expected 1 update, got: %+v", updates)
	}
	if updates[0].Version != "119.0" || updates[0].AvailableVersion != "120.0" {
		t.Errorf("expected version 119.0 -> 120.0, got: %+v", updates[0])
	}
}

func TestSnapManager_GetAvailableUpdates_Error(t *testing.T) {
	mgr := NewSnapManager()
	mgr.exePath = "/usr/bin/snap"
	mgr.runner = &mockRunner{
		runFunc: func(ctx context.Context, name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
			stderr.Write([]byte("snapd not running"))
			return errors.New("exit status 1")
		},
	}

	_, err := mgr.GetAvailableUpdates(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "snapd not running") {
		t.Errorf("expected error to contain stderr, got: %v", err)
	}
}
